package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/platformtools"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// phoneRules answers the read-only commands setup sends to the phone.
func phoneRules() []testutil.FakeRule {
	return []testutil.FakeRule{
		{Match: "^start-server$"},
		{Match: "^devices$", Stdout: "List of devices attached\nadb-AAAA1111-bbb._adb-tls-connect._tcp\tdevice\n"},
		{Match: "^pair 192\\.0\\.2\\.10:37000$", Stdout: "Enter pairing code: Successfully paired to 192.0.2.10:37000 [guid=adb-AAAA1111-bbb]\n", EchoStdin: true},
		{Match: "^shell getprop service\\.adb\\.tcp\\.port$", Stdout: "\n"},
		{Match: "^shell getprop ro\\.serialno$", Stdout: "AAAA1111\n"}, // the guid's serial number
		{Match: "^shell getprop ro\\.product\\.model$", Stdout: "Pixel 9 Pro XL\n"},
		{Match: "^shell getprop ro\\.product\\.manufacturer$", Stdout: "Google\n"},
		{Match: "^shell getprop ro\\.product\\.brand$", Stdout: "google\n"},
		{Match: "^shell getprop ro\\.build\\.version\\.release$", Stdout: "17\n"},
		{Match: "^shell getprop ro\\.build\\.version\\.sdk$", Stdout: "37\n"},
		{Match: "^shell getprop persist\\.sys\\.locale$", Stdout: "es-ES\n"},
		{Match: "^shell wm size$", Stdout: "Physical size: 1344x2992\n"},
		{Match: "^shell wm density$", Stdout: "Physical density: 480\n"},
		{Match: "^shell cmd package resolve-activity", Stdout: "com.google.android.apps.nexuslauncher/.NexusLauncherActivity\n"},
	}
}

type setupEnv struct {
	cfgDir, ws, log string
}

func newSetupEnv(t *testing.T, rules []testutil.FakeRule) setupEnv {
	t.Helper()
	fakeADB(t) // built with the go in PATH, so before PATH changes below
	dir := t.TempDir()
	script, err := testutil.WriteScript(dir, rules...)
	if err != nil {
		t.Fatal(err)
	}
	e := setupEnv{cfgDir: filepath.Join(dir, "cfg"), ws: filepath.Join(dir, "ws"), log: filepath.Join(dir, "adb.log")}
	t.Setenv("FAKEADB_SCRIPT", script)
	t.Setenv("FAKEADB_LOG", e.log)
	t.Setenv("ADBUDDY_CONFIG_DIR", e.cfgDir)
	t.Setenv("ADBUDDY_DATA_DIR", filepath.Join(dir, "data"))
	t.Setenv("ADBUDDY_SOURCE", "")
	t.Setenv("ADBUDDY_RELEASE_URL", "http://127.0.0.1:1/") // no app release reachable: setup skips the app
	// No test reaches Google: a download fails at once unless a test serves one.
	t.Setenv("ADBUDDY_REPOSITORY_URL", "http://127.0.0.1:1/")
	// A fake "claude" in PATH, and nothing else.
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o700)
	os.WriteFile(filepath.Join(bin, testutil.Exe("claude")), []byte("x"), 0o700)
	t.Setenv("PATH", bin)
	return e
}

func runWithInput(t *testing.T, input string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb strings.Builder
	code = Main(args, strings.NewReader(input), &out, &errb)
	return out.String(), errb.String(), code
}

func TestSetupPairsWithCodeAndPreparesEverything(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
	}
	var env struct {
		OK   bool
		Data setupResult
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK || env.Data.Serial != "adb-AAAA1111-bbb._adb-tls-connect._tcp" || env.Data.Agent != "claude" || env.Data.Device.Model != "Pixel 9 Pro XL" {
		t.Fatalf("stdout %q (%v)", out, err)
	}
	for _, want := range []string{"Wireless debugging", HelpURL, "pairing code"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr lacks %q", want)
		}
	}
	// The code travelled on stdin, never in argv.
	calls, _ := testutil.ReadLog(e.log)
	paired := false
	for _, c := range calls {
		if strings.Contains(strings.Join(c.Args, " "), "123456") {
			t.Fatalf("pairing code in argv: %q", c.Args)
		}
		if c.Joined() == "pair 192.0.2.10:37000" {
			paired = c.Stdin == "123456\n"
		}
		if c.Port != "5038" {
			t.Fatalf("adb without ADBuddy's server port: %+v", c)
		}
	}
	if !paired {
		t.Fatalf("pair not called with the code on stdin: %+v", calls)
	}
	cfg, err := config.Load(filepath.Join(e.cfgDir, "config.toml"))
	if err != nil || cfg.Serial != "adb-AAAA1111-bbb._adb-tls-connect._tcp" || cfg.Workspace != e.ws || cfg.Agent != "claude" || cfg.ADBPath != fakeADB(t) {
		t.Fatalf("config %+v %v", cfg, err)
	}
	agents, _ := os.ReadFile(filepath.Join(e.ws, "AGENTS.md"))
	if !strings.Contains(string(agents), "Pixel 9 Pro XL") || !strings.Contains(string(agents), "Android: `17` (SDK 37)") {
		t.Fatalf("AGENTS.md profile:\n%s", agents)
	}
	if _, err := os.Stat(filepath.Join(e.ws, "device.json")); err != nil {
		t.Fatal(err)
	}
}

func TestSetupSkipsPairingForAKnownPhone(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	cfg := config.Default()
	cfg.Serial = "adb-AAAA1111-bbb._adb-tls-connect._tcp"
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 || !strings.Contains(errOut, "already paired") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	calls, _ := testutil.ReadLog(e.log)
	for _, c := range calls {
		if strings.HasPrefix(c.Joined(), "pair") || strings.HasPrefix(c.Joined(), "mdns") {
			t.Fatalf("pairing attempted: %q", c.Joined())
		}
	}
}

// After `adb usb`, setup checks the port again before reading the profile.
func TestSetupClosesAnOpenTCPPort(t *testing.T) {
	setVar(t, &usbDropWait, 100*time.Millisecond)
	rules := append([]testutil.FakeRule{
		{Match: tcpPortRule, Stdout: "5555\n", Times: 1},
		{Match: "^usb$", Stdout: "restarting in USB mode\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 || !strings.Contains(errOut, "adb usb") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	var steps []string
	for _, c := range loggedCalls(t, e.log) {
		if c == "usb" || c == "shell getprop service.adb.tcp.port" || c == "shell getprop ro.product.model" {
			steps = append(steps, c)
		}
	}
	if want := "shell getprop service.adb.tcp.port|usb|shell getprop service.adb.tcp.port|shell getprop ro.product.model"; strings.Join(steps, "|") != want {
		t.Fatalf("steps %q", steps)
	}
}

// A port that `adb usb` does not close stops setup: nothing is saved.
func TestSetupStopsWhenThePortStaysOpen(t *testing.T) {
	setVar(t, &usbDropWait, 100*time.Millisecond)
	rules := append([]testutil.FakeRule{{Match: tcpPortRule, Stdout: "5555\n"}}, phoneRules()...)
	e := newSetupEnv(t, rules)
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitFailure || !strings.Contains(errOut, "DEVICE_PORT_OPEN") || !strings.Contains(errOut, "adbuddy setup") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if exists(filepath.Join(e.cfgDir, "config.toml")) {
		t.Fatal("config saved with the port open")
	}
}

func TestSetupFailsCleanly(t *testing.T) {
	rules := append([]testutil.FakeRule{
		{Match: "^pair ", Stdout: "Failed: Wrong password or connection was dropped.\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n000000\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitFailure || !strings.Contains(errOut, "PAIRING_FAILED") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(e.cfgDir, "config.toml")); err == nil {
		t.Fatal("config saved after a failed setup")
	}
	t.Setenv("ADBUDDY_SOURCE", "agent")
	if _, errOut, code := runWithInput(t, "", "setup"); code != ExitFailure || !strings.Contains(errOut, "AGENT_SESSION") {
		t.Fatalf("agent session: %d %q", code, errOut)
	}
}

func TestPhoneGuideDependsOnTheMode(t *testing.T) {
	const plain = "Keep the phone on the same Wi-Fi network as this PC."
	if g := phoneGuide(false); !strings.Contains(g, plain+"\n") || strings.Contains(g, "Tailscale") {
		t.Fatalf("QR guide: %q", g)
	}
	if g := phoneGuide(true); !strings.Contains(g, "reachable over Tailscale") || strings.Contains(g, plain+"\n") {
		t.Fatalf("code guide: %q", g)
	}
	e := newSetupEnv(t, phoneRules())
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 || !strings.Contains(errOut, "reachable over Tailscale") || strings.Contains(errOut, plain+"\n") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestSetupCodeProtocolFaultSaysToKeepThePopupOpen(t *testing.T) {
	rules := append([]testutil.FakeRule{
		{Match: "^pair ", Stdout: "error: protocol fault (couldn't read status message)\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitFailure || !strings.Contains(errOut, "PAIRING_FAILED") || !strings.Contains(errOut, "Keep the pairing popup open") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestSetupReportsDownloadFailure(t *testing.T) {
	newSetupEnv(t, phoneRules())
	_, errOut, code := runWithInput(t, "", "setup")
	if code != ExitTransient || !strings.Contains(errOut, "TOOLS_DOWNLOAD_FAILED") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

// A download that does not match Google's checksum is not a network hiccup:
// retrying does not help.
func TestSetupReportsChecksumMismatch(t *testing.T) {
	newSetupEnv(t, phoneRules())
	archive := []byte("not the real platform-tools")
	manifest := fmt.Sprintf(`<?xml version='1.0' encoding='utf-8'?>
<sdk:sdk-repository xmlns:sdk="http://schemas.android.com/sdk/android/repo/repository2/03" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <channel id="channel-0">stable</channel>
  <remotePackage path="platform-tools">
    <revision><major>37</major><minor>0</minor><micro>0</micro></revision>
    <channelRef ref="channel-0"/>
    <archives><archive><complete><size>%d</size><checksum type="sha1">%s</checksum><url>pt.zip</url></complete><host-os>%s</host-os></archive></archives>
  </remotePackage>
</sdk:sdk-repository>`, len(archive), strings.Repeat("0", 40), platformtools.HostOS(runtime.GOOS))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/"+platformtools.ManifestName) {
			io.WriteString(w, manifest)
			return
		}
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ADBUDDY_REPOSITORY_URL", srv.URL+"/repo/")
	_, errOut, code := runWithInput(t, "", "setup")
	if code != ExitFailure || !strings.Contains(errOut, "TOOLS_CHECKSUM_MISMATCH") || strings.Contains(errOut, "internet connection") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

// adb start-server failing: its own words, made printable, and never "<nil>".
func TestSetupReportsStartServerFailure(t *testing.T) {
	e := newSetupEnv(t, []testutil.FakeRule{{Match: "^start-server$", Exit: 1, Stderr: "cannot bind 'tcp:5038'\x1b[2J\n"}})
	_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitFailure || !strings.Contains(errOut, "ADB_FAILED") || !strings.Contains(errOut, "cannot bind") ||
		strings.Contains(errOut, "<nil>") || strings.Contains(errOut, "\x1b") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	// An adb that does not answer in time is reported, not waited for.
	setVar(t, &startServerWait, time.Nanosecond)
	_, errOut, code = runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitFailure || !strings.Contains(errOut, "ADB_FAILED") || !strings.Contains(errOut, "did not answer") {
		t.Fatalf("timeout: exit %d stderr %q", code, errOut)
	}
}

// setup does not rewrite the environment and config.toml under a running serve.
func TestSetupRefusesWhileServeRuns(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	data := shortDir(t)
	t.Setenv("ADBUDDY_DATA_DIR", data)
	fakeDaemon(t, data, nil, ipc.Exit{})
	_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitFailure || !strings.Contains(errOut, "ALREADY_RUNNING") || !strings.Contains(errOut, "adbuddy serve") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if calls, _ := testutil.ReadLog(e.log); len(calls) > 0 || exists(e.ws) || exists(filepath.Join(e.cfgDir, "config.toml")) {
		t.Fatalf("setup went on: adb calls %d", len(calls))
	}
}

// `adbuddy` alone starts setup only for a person at a terminal; a script, a
// CI job or --json gets NOT_CONFIGURED (78) and nothing is downloaded.
func TestBareAdbuddyStartsSetupOnlyInATerminal(t *testing.T) {
	newSetupEnv(t, phoneRules())
	for _, args := range [][]string{nil, {"--json"}} {
		out, errOut, code := runWithInput(t, "", args...)
		if code != ExitConfig || !strings.Contains(out+errOut, "NOT_CONFIGURED") || strings.Contains(errOut, "starting") || strings.Contains(errOut, "platform-tools") {
			t.Fatalf("%v without a terminal: exit %d stdout %q stderr %q", args, code, out, errOut)
		}
	}
	setVar(t, &isTerminal, func(any) bool { return true })
	if out, errOut, code := runWithInput(t, "", "--json"); code != ExitConfig || !strings.Contains(out, "NOT_CONFIGURED") {
		t.Fatalf("--json at a terminal: exit %d stdout %q stderr %q", code, out, errOut)
	}
	_, errOut, code := runWithInput(t, "")
	if code != ExitTransient || !strings.Contains(errOut, "not set up yet") || !strings.Contains(errOut, "TOOLS_DOWNLOAD_FAILED") {
		t.Fatalf("at a terminal: exit %d stderr %q", code, errOut)
	}

	e := newSetupEnv(t, phoneRules())
	config.Save(filepath.Join(e.cfgDir, "config.toml"), config.Default())
	out, _, code := runWithInput(t, "")
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("configured: exit %d stdout %q", code, out)
	}
}

// The address of --code is checked before anything is asked or run.
func TestSetupCodeAddressMustBeIPAndPort(t *testing.T) {
	for _, bad := range []string{"192.0.2.10", "phone.local:37000", "192.0.2.10:0", "192.0.2.10:65536", "192.0.2.10:-1", "192.0.2.10:port"} {
		e := newSetupEnv(t, phoneRules())
		_, errOut, code := runWithInput(t, bad+"\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
		if code != ExitUsage || !strings.Contains(errOut, "IP address and port") || strings.Contains(errOut, "pairing code: ") {
			t.Fatalf("%q: exit %d stderr %q", bad, code, errOut)
		}
		for _, c := range loggedCalls(t, e.log) {
			if strings.HasPrefix(c, "pair") {
				t.Fatalf("%q: adb pair called", bad)
			}
		}
	}
}

// The help of serve and env init tells where the defaults come from.
func TestHelpTellsTheDefaults(t *testing.T) {
	out, _, _ := run(t, "serve", "--help")
	for _, want := range []string{"adb_path in config.toml, else the adb installed by adbuddy setup", "workspace in config.toml, else ~/ADBuddy/default"} {
		if !strings.Contains(out, want) {
			t.Errorf("serve --help lacks %q", want)
		}
	}
	if out, _, _ := run(t, "env", "init", "--help"); !strings.Contains(out, "workspace in config.toml, else ~/ADBuddy/default") {
		t.Errorf("env init --help: %s", out)
	}
}

func TestSetupCodeMustBeSixDigits(t *testing.T) {
	for _, bad := range []string{"12345", "1234567", "12a456", "123 456"} {
		e := newSetupEnv(t, phoneRules())
		_, errOut, code := runWithInput(t, "192.0.2.10:37000\n"+bad+"\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
		if code != ExitUsage || !strings.Contains(errOut, "6 digits") {
			t.Fatalf("%q: exit %d stderr %q", bad, code, errOut)
		}
		calls, _ := testutil.ReadLog(e.log)
		for _, c := range calls {
			if strings.HasPrefix(c.Joined(), "pair") {
				t.Fatalf("%q: adb pair called", bad)
			}
		}
	}
}

// Ctrl+C while setup waits for the phone exits with 130, whatever step it
// was in. Each case cancels once stderr shows that step, so it never depends
// on how fast the machine is.
func TestSetupInterruptedWhileWaitingForThePhone(t *testing.T) {
	setVar(t, &usbDropWait, 100*time.Millisecond)
	cases := map[string]struct {
		rules    []testutil.FakeRule
		input    string
		args     []string
		block    bool   // stdin never answers
		known    bool   // config.toml has the phone's serial
		cancelOn string // stderr text of the step to interrupt
	}{
		// The QR code is shown and nobody scans it.
		"qr": {rules: []testutil.FakeRule{{Match: "^start-server$"}, {Match: "^mdns services$", Stdout: "List of discovered mdns services\n"}},
			cancelOn: "Waiting for the phone"},
		// Paired with the code, and the phone never connects.
		"connect": {rules: []testutil.FakeRule{
			{Match: "^start-server$"},
			{Match: "^devices$", Stdout: "List of devices attached\n"},
			{Match: "^mdns services$", Stdout: "List of discovered mdns services\n"},
			{Match: `^pair 192\.0\.2\.10:37000$`, Stdout: "Successfully paired to 192.0.2.10:37000 [guid=adb-AAAA1111-bbb]\n", EchoStdin: true},
		}, input: "192.0.2.10:37000\n123456\n", args: []string{"--code"}, cancelOn: "Paired. Connecting"},
		// Nobody types the IP address and port.
		"prompt": {rules: []testutil.FakeRule{{Match: "^start-server$"}}, args: []string{"--code"}, block: true,
			cancelOn: "type the IP address and port"},
		// The phone paired before does not show up.
		"known": {rules: []testutil.FakeRule{{Match: "^start-server$"}, {Match: "^devices$", Stdout: phoneGone}},
			known: true, cancelOn: "Looking for your phone"},
		// The phone does not come back after `adb usb`.
		"invariant": {rules: []testutil.FakeRule{
			{Match: "^start-server$"},
			{Match: "^devices$", Stdout: phoneOnline, Times: 1},
			{Match: "^devices$", Stdout: phoneGone},
			{Match: tcpPortRule, Stdout: "5555\n"},
			serialnoRule,
		}, known: true, cancelOn: "Closed adb's network port"},
		// Nobody answers in the app.
		"consent": {rules: appRules("pending"), input: "192.0.2.10:37000\n123456\n", args: []string{"--code"}, cancelOn: "Open the phone"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newSetupEnv(t, c.rules)
			if c.known {
				cfg := config.Default()
				cfg.Serial = phoneSerial
				if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stdin io.Reader = strings.NewReader(c.input)
			if c.block {
				r, w := io.Pipe()
				t.Cleanup(func() { w.Close() })
				stdin = r
			}
			var out, errb syncBuffer
			cancelWhen(t, &errb, c.cancelOn, cancel)
			args := append([]string{"setup", "--adb", fakeADB(t), "--workspace", e.ws}, c.args...)
			done := make(chan int, 1)
			go func() { done <- mainContext(ctx, args, stdin, &out, &errb) }()
			select {
			case code := <-done:
				if code != ExitInterrupt || !strings.Contains(errb.String(), "INTERRUPTED") {
					t.Fatalf("exit %d, stderr %q", code, errb.String())
				}
				if strings.Contains(errb.String(), "Could not ask in the app") {
					t.Fatalf("Ctrl+C reported as a failure of the app: %q", errb.String())
				}
				if c.known && strings.Contains(errb.String(), "Prepare your phone") {
					t.Fatalf("went on to pairing after Ctrl+C: %q", errb.String())
				}
			case <-time.After(15 * time.Second):
				t.Fatal("setup did not stop")
			}
		})
	}
}

// loadSetupConfig reads the config.toml setup wrote.
func loadSetupConfig(t *testing.T, e setupEnv) config.Config {
	t.Helper()
	cfg, err := config.Load(filepath.Join(e.cfgDir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// setVar sets a package variable for one test.
func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// A relative --workspace or --adb is saved as an absolute path: serve, run
// and journal read config.toml from any folder.
func TestSetupSavesAbsolutePaths(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	dir := t.TempDir()
	t.Chdir(dir)
	relADB, err := filepath.Rel(dir, fakeADB(t))
	if err != nil {
		t.Skip("the fake adb is on another volume:", err)
	}
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", relADB, "--workspace", "relws")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	cfg := loadSetupConfig(t, e)
	if !filepath.IsAbs(cfg.Workspace) || !sameDir(cfg.Workspace, filepath.Join(dir, "relws")) {
		t.Fatalf("workspace %q", cfg.Workspace)
	}
	if !filepath.IsAbs(cfg.ADBPath) || !sameDir(cfg.ADBPath, fakeADB(t)) {
		t.Fatalf("adb_path %q", cfg.ADBPath)
	}
}

// Running setup again with the same phone keeps the environment chosen
// before; another phone gets its own.
func TestSetupAgainKeepsTheWorkspace(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	custom := filepath.Join(t.TempDir(), "custom")
	if _, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", custom); code != 0 {
		t.Fatalf("first setup: exit %d stderr %q", code, errOut)
	}
	if _, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t)); code != 0 || !strings.Contains(errOut, "already paired") {
		t.Fatalf("second setup: exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Workspace != custom {
		t.Fatalf("workspace moved to %q", cfg.Workspace)
	}

	// Another phone: the configured one is not around, this one is paired.
	cfg := loadSetupConfig(t, e)
	cfg.Serial = "adb-OTHER-ccc._adb-tls-connect._tcp"
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	setVar(t, &knownPhoneWait, 0)
	if _, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t)); code != 0 {
		t.Fatalf("other phone: exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Workspace != filepath.Join(home, "ADBuddy", "pixel-9-pro-xl") {
		t.Fatalf("other phone's workspace %q", cfg.Workspace)
	}
}

// The saved serial is the phone's mDNS name, which survives a new wireless
// debugging port, even when this time it connected with `adb connect ip:port`.
func TestSetupSavesTheStableSerial(t *testing.T) {
	rules := append([]testutil.FakeRule{
		{Match: "^devices$", Stdout: "List of devices attached\n192.0.2.10:41000\tdevice\n"},
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\nadb-AAAA1111-bbb\t_adb-tls-connect._tcp\t192.0.2.10:41000\n"},
		{Match: `^connect 192\.0\.2\.10:41000$`, Stdout: "connected to 192.0.2.10:41000\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	setVar(t, &autoConnectWait, 0)
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != "adb-AAAA1111-bbb._adb-tls-connect._tcp" {
		t.Fatalf("serial %q", cfg.Serial)
	}
	// Final review of M1b, R2-4: the JSON tells the saved serial from the
	// one of this session.
	var env struct{ Data setupResult }
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Data.Serial != "adb-AAAA1111-bbb._adb-tls-connect._tcp" || env.Data.Active != "192.0.2.10:41000" {
		t.Fatalf("stdout %s (%v)", out, err)
	}
}

// A phone known only by ip:port (no guid to name it) is not saved: that
// serial stops working when wireless debugging picks a new port.
func TestSetupDoesNotSaveAnAddressAsSerial(t *testing.T) {
	rules := append([]testutil.FakeRule{
		{Match: "^devices$", Stdout: "List of devices attached\n192.0.2.10:41000\tdevice\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	cfg := config.Default()
	cfg.Serial = "192.0.2.10:41000"
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != "" {
		t.Fatalf("serial %q", cfg.Serial)
	}
	if !strings.Contains(errOut, "not saving") {
		t.Fatalf("no warning: %q", errOut)
	}
	// Final review of M1b, R2-4: "serial" is only the saved one.
	var env struct{ Data setupResult }
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Data.Serial != "" || env.Data.Active != "192.0.2.10:41000" {
		t.Fatalf("stdout %s (%v)", out, err)
	}
}

// The configured agent is kept, with a note, when it is not in PATH and
// --agent is not given.
func TestSetupKeepsTheConfiguredAgent(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	cfg := config.Default()
	cfg.Serial, cfg.Agent = "adb-AAAA1111-bbb._adb-tls-connect._tcp", "codex"
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if got := loadSetupConfig(t, e).Agent; got != "codex" {
		t.Fatalf("agent %q", got)
	}
	if !strings.Contains(errOut, "codex is not in PATH") {
		t.Fatalf("no note: %q", errOut)
	}
}

// Running setup again finds the phone saved by its mDNS name under the
// serial adb lists it with, without pairing again, and keeps the name.
func TestSetupFindsTheKnownPhoneByItsGuid(t *testing.T) {
	setVar(t, &autoConnectWait, 0)
	for name, c := range guidCases {
		t.Run(name, func(t *testing.T) {
			e := newSetupEnv(t, append(append([]testutil.FakeRule{}, c.rules...), phoneRules()...))
			cfg := config.Default()
			cfg.Serial = phoneSerial
			if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
				t.Fatal(err)
			}
			_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
			if code != 0 || !strings.Contains(errOut, "already paired") || strings.Contains(errOut, "Prepare your phone") {
				t.Fatalf("exit %d stderr %q", code, errOut)
			}
			if got := loadSetupConfig(t, e).Serial; got != phoneSerial {
				t.Fatalf("saved serial %q", got)
			}
			if !phoneCallsUse(t, e.log, c.active) {
				t.Fatalf("calls to the phone not sent to %s: %q", c.active, loggedCalls(t, e.log))
			}
		})
	}
}

// An environment in config.toml without a serial (the phone was known only
// by its address) is kept when setup runs again.
func TestSetupKeepsTheWorkspaceWithoutASerial(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	custom := filepath.Join(t.TempDir(), "custom")
	cfg := config.Default()
	cfg.Workspace = custom
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t)); code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if got := loadSetupConfig(t, e).Workspace; got != custom {
		t.Fatalf("workspace moved to %q", got)
	}
}

// --adb with a bare name that is not in PATH is a usage error, not a path
// in the current folder.
func TestSetupADBNotInPath(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	_, errOut, code := runWithInput(t, "", "setup", "--adb", "no-such-adb", "--workspace", e.ws)
	if code != ExitUsage || !strings.Contains(errOut, "not found in PATH") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if exists(filepath.Join(e.cfgDir, "config.toml")) {
		t.Fatal("config saved")
	}
}
