package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if !strings.Contains(string(agents), "Pixel 9 Pro XL") || !strings.Contains(string(agents), "Android 17 (SDK 37)") {
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

func TestSetupClosesAnOpenTCPPort(t *testing.T) {
	rules := append([]testutil.FakeRule{
		{Match: "^shell getprop service\\.adb\\.tcp\\.port$", Stdout: "5555\n"},
		{Match: "^usb$", Stdout: "restarting in USB mode\n"},
	}, phoneRules()...)
	e := newSetupEnv(t, rules)
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 || !strings.Contains(errOut, "adb usb") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	calls, _ := testutil.ReadLog(e.log)
	usb := false
	for _, c := range calls {
		usb = usb || c.Joined() == "usb"
	}
	if !usb {
		t.Fatal("adb usb not run")
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
