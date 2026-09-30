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
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// phoneRules answers the read-only commands setup sends to the phone.
func phoneRules() []testutil.FakeRule {
	return []testutil.FakeRule{
		{Match: "^start-server$"},
		{Match: "^devices$", Stdout: "List of devices attached\nadb-AAA-bbb._adb-tls-connect._tcp\tdevice\n"},
		{Match: "^pair 192\\.0\\.2\\.10:37000$", Stdout: "Enter pairing code: Successfully paired to 192.0.2.10:37000 [guid=adb-AAA-bbb]\n", EchoStdin: true},
		{Match: "^shell getprop service\\.adb\\.tcp\\.port$", Stdout: "\n"},
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
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK || env.Data.Serial != "adb-AAA-bbb._adb-tls-connect._tcp" || env.Data.Agent != "claude" || env.Data.Device.Model != "Pixel 9 Pro XL" {
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
	if err != nil || cfg.Serial != "adb-AAA-bbb._adb-tls-connect._tcp" || cfg.Workspace != e.ws || cfg.Agent != "claude" || cfg.ADBPath != fakeADB(t) {
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
	cfg.Serial = "adb-AAA-bbb._adb-tls-connect._tcp"
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

func TestSetupReportsDownloadFailure(t *testing.T) {
	newSetupEnv(t, phoneRules())
	t.Setenv("ADBUDDY_REPOSITORY_URL", "http://127.0.0.1:1/")
	_, errOut, code := runWithInput(t, "", "setup")
	if code != ExitTransient || !strings.Contains(errOut, "TOOLS_DOWNLOAD_FAILED") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestBareAdbuddyStartsSetupWhenNotConfigured(t *testing.T) {
	newSetupEnv(t, phoneRules())
	t.Setenv("ADBUDDY_REPOSITORY_URL", "http://127.0.0.1:1/")
	_, errOut, code := runWithInput(t, "")
	if code != ExitTransient || !strings.Contains(errOut, "not set up yet") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	e := newSetupEnv(t, phoneRules())
	config.Save(filepath.Join(e.cfgDir, "config.toml"), config.Default())
	out, _, code := runWithInput(t, "")
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Fatalf("configured: exit %d stdout %q", code, out)
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
			{Match: `^pair 192\.0\.2\.10:37000$`, Stdout: "Successfully paired to 192.0.2.10:37000 [guid=adb-AAA-bbb]\n", EchoStdin: true},
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
		}, known: true, cancelOn: "Closed adb's network port"},
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
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\nadb-AAA-bbb\t_adb-tls-connect._tcp\t192.0.2.10:41000\n"},
		{Match: `^connect 192\.0\.2\.10:41000$`, Stdout: "connected to 192.0.2.10:41000\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	setVar(t, &autoConnectWait, 0)
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != "adb-AAA-bbb._adb-tls-connect._tcp" {
		t.Fatalf("serial %q", cfg.Serial)
	}
	if strings.Contains(out, "192.0.2.10") {
		t.Fatalf("stdout reports the address: %s", out)
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
	_, errOut, code := runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Serial != "" {
		t.Fatalf("serial %q", cfg.Serial)
	}
	if !strings.Contains(errOut, "not saving") {
		t.Fatalf("no warning: %q", errOut)
	}
}

// The configured agent is kept, with a note, when it is not in PATH and
// --agent is not given.
func TestSetupKeepsTheConfiguredAgent(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	cfg := config.Default()
	cfg.Serial, cfg.Agent = "adb-AAA-bbb._adb-tls-connect._tcp", "codex"
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
