package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// remoteRules: a phone paired with the code that mDNS never finds (the PC
// is on Tailscale): `adb devices` lists it only by its address, and serialno
// is what the phone at that address reports.
func remoteRules(serialno string) []testutil.FakeRule {
	return append(append(addrRules(serialno),
		testutil.FakeRule{Match: "^mdns services$", Stdout: "List of discovered mdns services\n"},
		testutil.FakeRule{Match: "^disconnect "},
	), phoneRules()...)
}

// shortWaits makes the mDNS waits of setup and serve short.
func shortWaits(t *testing.T) {
	setVar(t, &codeConnectWait, 50*time.Millisecond)
	setVar(t, &codeAutoConnectWait, 0)
	setVar(t, &connectWait, 50*time.Millisecond)
	setVar(t, &autoConnectWait, 0)
	setVar(t, &knownPhoneWait, 50*time.Millisecond)
	setVar(t, &deviceWait, 50*time.Millisecond)
	setVar(t, &pairingWait, 200*time.Millisecond)
}

func TestSetupCodeWithConnect(t *testing.T) {
	shortWaits(t)
	e := newSetupEnv(t, remoteRules("AAAA1111"))
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--connect", tsAddr, "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	var env struct{ Data setupResult }
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Data.Serial != phoneSerial || env.Data.Active != tsAddr || env.Data.Address != tsAddr {
		t.Fatalf("stdout %s (%v)", out, err)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != phoneSerial || cfg.Address != tsAddr {
		t.Fatalf("config %+v", cfg)
	}
	if !phoneCallsUse(t, e.log, tsAddr) {
		t.Fatalf("calls to the phone not sent to %s: %q", tsAddr, loggedCalls(t, e.log))
	}
	calls := strings.Join(loggedCalls(t, e.log), "|")
	if !strings.Contains(calls, "shell getprop ro.serialno") || !strings.Contains(calls, "shell getprop service.adb.tcp.port") {
		t.Fatalf("identity or invariant not checked: %q", calls)
	}
}

// At a terminal, setup asks for the address when mDNS does not find the phone.
func TestSetupCodeAsksForTheAddress(t *testing.T) {
	shortWaits(t)
	setVar(t, &isTerminal, func(any) bool { return true })
	e := newSetupEnv(t, remoteRules("AAAA1111"))
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n"+tsAddr+"\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 || !strings.Contains(errOut, "Connection address (IP:port on the Wireless debugging screen)") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != phoneSerial || cfg.Address != tsAddr {
		t.Fatalf("config %+v", cfg)
	}
}

// Another phone at the address given: WRONG_PHONE, and nothing is saved.
func TestSetupConnectToAnotherPhone(t *testing.T) {
	shortWaits(t)
	e := newSetupEnv(t, remoteRules("ZZZZ9999"))
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--connect", tsAddr, "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitFailure || !strings.Contains(errOut, "WRONG_PHONE") || !strings.Contains(errOut, i18n.EN.T("err.connect.wrong_phone.fix")) {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if exists(filepath.Join(e.cfgDir, "config.toml")) || exists(e.ws) {
		t.Fatal("saved after WRONG_PHONE")
	}
}

// Without --connect nor a terminal, the phone not found is DEVICE_NOT_FOUND,
// and the fix tells about --connect.
func TestSetupCodeWithoutAnAddress(t *testing.T) {
	shortWaits(t)
	e := newSetupEnv(t, remoteRules("AAAA1111"))
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitTransient || !strings.Contains(errOut, "DEVICE_NOT_FOUND") || !strings.Contains(errOut, "--connect") || strings.Contains(errOut, "Connection address") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	for _, c := range loggedCalls(t, e.log) {
		if strings.HasPrefix(c, "connect ") {
			t.Fatalf("connected without an address: %q", c)
		}
	}
}

func TestSetupConnectFlagMustBeIPAndPort(t *testing.T) {
	e := newSetupEnv(t, remoteRules("AAAA1111"))
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--connect", "phone.local:41234", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitUsage || !strings.Contains(errOut, "--connect") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if calls := loggedCalls(t, e.log); len(calls) > 0 {
		t.Fatalf("adb called %q", calls)
	}
}

// saveKnownPhone writes config.toml for a phone set up before, reached last
// time at tsAddr.
func saveKnownPhone(t *testing.T, cfgDir string) {
	t.Helper()
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	if err := config.Save(filepath.Join(cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
}

// A known phone that mDNS does not find is reached at its saved address.
func TestSetupKnownPhoneAtItsAddress(t *testing.T) {
	shortWaits(t)
	e := newSetupEnv(t, remoteRules("AAAA1111"))
	saveKnownPhone(t, e.cfgDir)
	_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 || !strings.Contains(errOut, "already paired") || strings.Contains(errOut, "Prepare your phone") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != phoneSerial || cfg.Address != tsAddr {
		t.Fatalf("config %+v", cfg)
	}
	if !phoneCallsUse(t, e.log, tsAddr) {
		t.Fatalf("calls to the phone not sent to %s: %q", tsAddr, loggedCalls(t, e.log))
	}
}

// serveOnce runs serve until it is serving (then stops it) or fails, and
// returns its exit code, stderr, what it answered to a ping and the fake adb's log.
func serveOnce(t *testing.T, rules []testutil.FakeRule, cfg config.Config) (int, string, ipc.Ping, string) {
	return serveOnceMarker(t, rules, cfg, "ADBuddy is serving")
}

// serveOnceMarker is serveOnce for a language other than English: marker is
// what serve prints (in that language) when it is serving.
func serveOnceMarker(t *testing.T, rules []testutil.FakeRule, cfg config.Config, marker string) (int, string, ipc.Ping, string) {
	t.Helper()
	data := isolate(t)
	if err := config.Save(filepath.Join(os.Getenv("ADBUDDY_CONFIG_DIR"), "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	_, log := invariantRunner(t, rules...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errb syncBuffer
	var ping ipc.Ping
	pinged := make(chan struct{})
	serving := make(chan struct{})
	cancelWhen(t, &errb, marker, func() { close(serving) })
	go func() {
		defer close(pinged)
		defer cancel()
		select {
		case <-serving:
			ping, _ = ipc.PingDaemon(context.Background(), data)
		case <-ctx.Done():
		}
	}()
	done := make(chan int, 1)
	go func() {
		done <- mainContext(ctx, []string{"serve", "--adb", fakeADB(t), "--workspace", filepath.Join(t.TempDir(), "ws"), "--confirm", "deny"}, strings.NewReader(""), &out, &errb)
	}()
	var code int
	select {
	case code = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop")
	}
	cancel()
	<-pinged
	return code, errb.String(), ping, log
}

// serve reaches the phone at its saved address when mDNS does not find it;
// the journal keeps knowing it by its stable name.
func TestServeUsesTheSavedAddress(t *testing.T) {
	shortWaits(t)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	code, errOut, ping, log := serveOnce(t, remoteRules("AAAA1111"), cfg)
	if code != 0 || !strings.Contains(errOut, "ADBuddy is serving "+tsAddr+" ") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if ping.Serial != tsAddr || ping.Identity != phoneSerial {
		t.Fatalf("ping %+v: want serial %s and identity %s", ping, tsAddr, phoneSerial)
	}
	// Already connected on ADBuddy's adb server: used at once, without
	// waiting for mDNS nor connecting again.
	for _, c := range loggedCalls(t, log) {
		if c == "mdns services" || strings.HasPrefix(c, "connect ") {
			t.Fatalf("unexpected %q: %q", c, loggedCalls(t, log))
		}
	}
}

// A saved address that no longer answers (the port changes when wireless
// debugging is turned on again): the error says to run `adbuddy connect`.
func TestServeSavedAddressGone(t *testing.T) {
	shortWaits(t)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	rules := []testutil.FakeRule{
		{Match: "^connect ", Stdout: "failed to connect to '" + tsAddr + "': Connection refused\n"},
		{Match: "^devices$", Stdout: phoneGone},
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\n"},
	}
	code, errOut, _, _ := serveOnce(t, rules, cfg)
	if code != ExitTransient || !strings.Contains(errOut, "DEVICE_NOT_FOUND") || !strings.Contains(errOut, "adbuddy connect <new ip:port>") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	// The address of another phone is not used either.
	code, errOut, _, _ = serveOnce(t, remoteRules("ZZZZ9999"), cfg)
	if code != ExitTransient || !strings.Contains(errOut, wrongPhoneWarning) || !strings.Contains(errOut, "DEVICE_NOT_FOUND") || !strings.Contains(errOut, "adbuddy connect <new ip:port>") {
		t.Fatalf("other phone: exit %d stderr %q", code, errOut)
	}
}

// V38: with several saved addresses, the error says that none worked, which
// ones were tried, and that the error shown is the last one (not the error
// of every address). For a person it is in their language (M6c catalog).
func TestServeSavedAddressesGoneSaysWhichWereTried(t *testing.T) {
	shortWaits(t)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, lanAddr
	cfg.TailscaleIP, cfg.TailscalePort = "100.64.0.5", 41234
	rules := []testutil.FakeRule{
		{Match: "^connect ", Stdout: "failed to connect: Connection refused\n"},
		{Match: "^devices$", Stdout: phoneGone},
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\n"},
	}
	code, errOut, _, _ := serveOnce(t, rules, cfg)
	want := "none of its saved addresses worked (tried " + lanAddr + ", " + tsAddr + "; last error seen: "
	if code != ExitTransient || !strings.Contains(errOut, "DEVICE_NOT_FOUND") || !strings.Contains(errOut, want) {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	i18ntest.Lang(t, i18n.ES)
	code, errOut, _, _ = serveOnce(t, rules, cfg)
	wantES := "no funcionó ninguna de sus direcciones guardadas (probadas: " + lanAddr + ", " + tsAddr + "; último error visto: "
	if code != ExitTransient || !strings.Contains(errOut, "DEVICE_NOT_FOUND") || !strings.Contains(errOut, wantES) {
		t.Fatalf("Spanish: exit %d stderr %q", code, errOut)
	}
}

// In QR mode, a network where no phone announces wireless debugging gets one
// hint about --code (Tailscale, or a Wi-Fi that blocks mDNS).
func TestSetupQRHintWhenNoPhoneIsVisible(t *testing.T) {
	setVar(t, &qrHintDelay, 50*time.Millisecond)
	setVar(t, &pairingWait, 600*time.Millisecond)
	const hint = "No phone with wireless debugging is visible on this network. If the phone is on another network (for example Tailscale) or this Wi-Fi blocks mDNS, press Ctrl+C and run `adbuddy setup --code`."
	for name, c := range map[string]struct {
		mdns string
		want int
	}{
		"nothing":     {"List of discovered mdns services\n", 1},
		"other phone": {"List of discovered mdns services\nadb-OTHER-ccc\t_adb-tls-connect._tcp\t192.0.2.20:41000\n", 0},
	} {
		t.Run(name, func(t *testing.T) {
			e := newSetupEnv(t, []testutil.FakeRule{{Match: "^start-server$"}, {Match: "^mdns services$", Stdout: c.mdns}})
			_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
			if code != ExitTransient || !strings.Contains(errOut, "PAIRING_TIMEOUT") {
				t.Fatalf("exit %d stderr %q", code, errOut)
			}
			if n := strings.Count(errOut, hint); n != c.want {
				t.Fatalf("hint %d times, want %d: %q", n, c.want, errOut)
			}
		})
	}
}

// Ctrl+C in the new waits (the address prompt, adb connect, serve's try of
// the saved address) exits with 130.
func TestInterruptedWhileConnectingByAddress(t *testing.T) {
	shortWaits(t)
	hang := []testutil.FakeRule{{Match: "^connect ", SleepMs: 20000}}
	cases := map[string]struct {
		rules    []testutil.FakeRule
		args     []string
		terminal bool
		known    bool
		cancelOn string
	}{
		"prompt":      {rules: remoteRules("AAAA1111"), args: []string{"setup", "--code"}, terminal: true, cancelOn: "Connection address"},
		"connect":     {rules: append(hang, remoteRules("AAAA1111")...), args: []string{"setup", "--code", "--connect", tsAddr}, cancelOn: "Connecting to " + tsAddr},
		"serve":       {rules: append(append(hang, testutil.FakeRule{Match: "^devices$", Stdout: phoneGone}), remoteRules("AAAA1111")...), args: []string{"serve", "--confirm", "deny"}, known: true, cancelOn: "Connecting to " + tsAddr},
		"serve-check": {rules: append([]testutil.FakeRule{{Match: `^shell getprop ro.serialno$`, SleepMs: 20000}}, remoteRules("AAAA1111")...), args: []string{"serve", "--confirm", "deny"}, known: true, cancelOn: "Checking the phone at " + tsAddr},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if c.terminal {
				setVar(t, &isTerminal, func(any) bool { return true })
			}
			e := newSetupEnv(t, c.rules)
			t.Setenv("ADBUDDY_DATA_DIR", shortDir(t))
			if c.known {
				saveKnownPhone(t, e.cfgDir)
			}
			r, w := io.Pipe()
			t.Cleanup(func() { w.Close() })
			go io.WriteString(w, "192.0.2.10:37000\n123456\n")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var out, errb syncBuffer
			cancelWhen(t, &errb, c.cancelOn, cancel)
			args := append(append([]string{}, c.args...), "--adb", fakeADB(t), "--workspace", e.ws)
			done := make(chan int, 1)
			go func() { done <- mainContext(ctx, args, r, &out, &errb) }()
			select {
			case code := <-done:
				if code != ExitInterrupt || !strings.Contains(errb.String(), "INTERRUPTED") {
					t.Fatalf("exit %d, stderr %q", code, errb.String())
				}
			case <-time.After(15 * time.Second):
				t.Fatal("did not stop")
			}
		})
	}
}

// A saved address that failed is not kept when setup pairs the phone again
// and finds it by mDNS.
func TestSetupDropsASavedAddressThatFailed(t *testing.T) {
	shortWaits(t)
	setVar(t, &connectWait, 3*time.Second)
	rules := append([]testutil.FakeRule{
		{Match: "^start-server$"},
		{Match: "^connect ", Stdout: "failed to connect to '" + tsAddr + "': Connection refused\n"},
		{Match: "^devices$", Stdout: phoneGone, Times: 2},
		{Match: "^devices$", Stdout: phoneOnline},
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	saveKnownPhone(t, e.cfgDir)
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 || !strings.Contains(errOut, "Paired") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != phoneSerial || cfg.Address != "" {
		t.Fatalf("config %+v", cfg)
	}
}

// wrongPhoneWarning is said when the saved address reaches another phone.
const wrongPhoneWarning = "is not the one you set up; not using that address"

// A saved address that went stale (the phone came back by mDNS with a new
// port): it costs at most knownAddrWait, then the usual wait for mDNS goes on.
func TestServeStaleAddressFallsBackToMDNS(t *testing.T) {
	shortWaits(t)
	setVar(t, &deviceWait, 10*time.Second)
	setVar(t, &knownAddrWait, 300*time.Millisecond)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	rules := append([]testutil.FakeRule{
		{Match: "^connect ", SleepMs: 20000}, // an address nobody answers on
		{Match: "^devices$", Stdout: phoneGone, Times: 1},
		{Match: "^devices$", Stdout: phoneOnline},
		{Match: tcpPortRule, Stdout: "\n"},
	}, remoteRules("AAAA1111")...)
	start := time.Now()
	code, errOut, ping, _ := serveOnce(t, rules, cfg)
	if code != 0 || !strings.Contains(errOut, "Connecting to "+tsAddr) || ping.Serial != phoneSerial {
		t.Fatalf("exit %d ping %+v stderr %q", code, ping, errOut)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %s", d)
	}
}

// Another phone already connected at the saved address: not used, said so,
// disconnected, and the phone is found by mDNS.
func TestServeIgnoresAnotherPhoneAtTheSavedAddress(t *testing.T) {
	shortWaits(t)
	setVar(t, &deviceWait, 10*time.Second)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	rules := append([]testutil.FakeRule{
		{Match: "^devices$", Stdout: "List of devices attached\n" + tsAddr + "\tdevice\n", Times: 1},
		{Match: `^shell getprop ro\.serialno$`, Stdout: "ZZZZ9999\n", Times: 1}, // at the address
		serialnoRule, // the phone found by mDNS
		{Match: "^devices$", Stdout: phoneOnline},
	}, remoteRules("ZZZZ9999")...)
	code, errOut, ping, log := serveOnce(t, rules, cfg)
	if code != 0 || !strings.Contains(errOut, tsAddr+" "+wrongPhoneWarning) || ping.Serial != phoneSerial {
		t.Fatalf("exit %d ping %+v stderr %q", code, ping, errOut)
	}
	calls := loggedCalls(t, log)
	if !strings.Contains(strings.Join(calls, "|"), "disconnect "+tsAddr) {
		t.Fatalf("not disconnected: %q", calls)
	}
	for _, c := range calls {
		if c == "connect "+tsAddr {
			t.Fatalf("connected again to the other phone: %q", calls)
		}
	}
}

// Final review of M1c, M8: with --code, the short mDNS waits are used only
// when there is an address to go on with (--connect or a terminal to ask);
// without one, setup waits the full time.
func TestSetupCodeWaitsLessOnlyWithAnAddress(t *testing.T) {
	shortWaits(t)
	setVar(t, &connectWait, 3*time.Second)
	refused := append([]testutil.FakeRule{
		{Match: "^connect ", Stdout: "failed to connect to '" + tsAddr + "': Connection refused\n"},
	}, remoteRules("AAAA1111")...)
	cases := map[string]struct {
		args     []string
		input    string
		terminal bool
		full     bool
	}{
		"no address": {full: true},
		"--connect":  {args: []string{"--connect", tsAddr}},
		"terminal":   {input: tsAddr + "\n", terminal: true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if c.terminal {
				setVar(t, &isTerminal, func(any) bool { return true })
			}
			e := newSetupEnv(t, refused)
			args := append([]string{"setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws}, c.args...)
			start := time.Now()
			_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n"+c.input, args...)
			d := time.Since(start)
			if code != ExitTransient {
				t.Fatalf("exit %d stderr %q", code, errOut)
			}
			if c.full && d < 3*time.Second || !c.full && d > 2*time.Second {
				t.Fatalf("took %s (full wait %v)", d, c.full)
			}
		})
	}
}
