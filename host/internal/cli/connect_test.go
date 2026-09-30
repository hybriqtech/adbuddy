package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// tsAddr is a phone reached over Tailscale (an example CGNAT address).
const tsAddr = "100.64.0.5:41234"

// addrRules answer `adb connect tsAddr` and list the phone there; serialno is
// what the phone at that address reports as ro.serialno.
func addrRules(serialno string) []testutil.FakeRule {
	return []testutil.FakeRule{
		{Match: "^start-server$"},
		{Match: `^connect 100\.64\.0\.5:41234$`, Stdout: "connected to " + tsAddr + "\n"},
		{Match: "^devices$", Stdout: "List of devices attached\n" + tsAddr + "\tdevice\n"},
		{Match: `^shell getprop ro\.serialno$`, Stdout: serialno + "\n"},
	}
}

// saveConfig writes config.toml with the phone's serial (if any) and the
// fake adb, as setup leaves it.
func saveConfig(t *testing.T, e setupEnv, serial string) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Serial, cfg.ADBPath, cfg.Workspace = serial, fakeADB(t), e.ws
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConnectSavesTheAddress(t *testing.T) {
	e := newSetupEnv(t, addrRules("AAA"))
	saveConfig(t, e, phoneSerial)
	out, errOut, code := runWithInput(t, "", "connect", tsAddr, "--json")
	if code != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	var env struct {
		OK   bool
		Data map[string]any
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK || env.Data["serial"] != tsAddr || env.Data["address"] != tsAddr || env.Data["verified"] != true {
		t.Fatalf("stdout %q (%v)", out, err)
	}
	cfg := loadSetupConfig(t, e)
	if cfg.Address != tsAddr || cfg.Serial != phoneSerial || cfg.Workspace != e.ws {
		t.Fatalf("config %+v", cfg)
	}
	if !phoneCallsUse(t, e.log, tsAddr) {
		t.Fatalf("getprop not sent to %s: %q", tsAddr, loggedCalls(t, e.log))
	}
	calls, _ := testutil.ReadLog(e.log)
	for _, c := range calls {
		if c.Port != "5038" {
			t.Fatalf("adb without ADBuddy's server port: %+v", c)
		}
		if c.Joined() == "connect "+tsAddr && c.Args[0] == "-s" {
			t.Fatalf("adb connect sent to a phone: %q", c.Args)
		}
	}

	// Without a saved phone there is nothing to check against: connected,
	// saved, not verified, and said so.
	e = newSetupEnv(t, addrRules("AAA"))
	saveConfig(t, e, "")
	out, errOut, code = runWithInput(t, "", "connect", tsAddr, "--json")
	if code != 0 || !strings.Contains(out, `"verified":false`) || !strings.Contains(errOut, "could not check") {
		t.Fatalf("no serial: exit %d stdout %q stderr %q", code, out, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Address != tsAddr {
		t.Fatalf("no serial: config %+v", cfg)
	}
}

// Another phone answers at that address: nothing is saved.
func TestConnectRefusesAnotherPhone(t *testing.T) {
	e := newSetupEnv(t, addrRules("ZZZ"))
	saveConfig(t, e, phoneSerial)
	out, errOut, code := runWithInput(t, "", "connect", tsAddr, "--json")
	if code != ExitFailure || !strings.Contains(out, `"WRONG_PHONE"`) || !strings.Contains(out, "that address is another phone; check the IP and port on the phone you set up") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
	// The other phone does not stay on ADBuddy's adb server.
	if calls := strings.Join(loggedCalls(t, e.log), "|"); !strings.Contains(calls, "disconnect "+tsAddr) {
		t.Fatalf("not disconnected: %q", calls)
	}
	if cfg := loadSetupConfig(t, e); cfg.Address != "" {
		t.Fatalf("address saved: %+v", cfg)
	}
}

func TestConnectFails(t *testing.T) {
	e := newSetupEnv(t, []testutil.FakeRule{
		{Match: "^start-server$"},
		{Match: `^connect `, Stdout: "failed to connect to '" + tsAddr + "': Connection refused\n"},
	})
	saveConfig(t, e, phoneSerial)
	_, errOut, code := runWithInput(t, "", "connect", tsAddr)
	if code != ExitTransient || !strings.Contains(errOut, "CONNECT_FAILED") || !strings.Contains(errOut, "Connection refused") || !strings.Contains(errOut, "Wireless debugging") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	if cfg := loadSetupConfig(t, e); cfg.Address != "" {
		t.Fatalf("address saved: %+v", cfg)
	}
}

// A bad address is a usage error, found before adb runs.
func TestConnectAddressMustBeIPAndPort(t *testing.T) {
	for _, bad := range []string{"100.64.0.5", "phone.local:41234", "100.64.0.5:0", "100.64.0.5:65536", "100.64.0.5:port"} {
		e := newSetupEnv(t, addrRules("AAA"))
		saveConfig(t, e, phoneSerial)
		_, errOut, code := runWithInput(t, "", "connect", bad)
		if code != ExitUsage || !strings.Contains(errOut, "IP address and port") {
			t.Fatalf("%q: exit %d stderr %q", bad, code, errOut)
		}
		if calls := loggedCalls(t, e.log); len(calls) > 0 {
			t.Fatalf("%q: adb called %q", bad, calls)
		}
	}
	if _, _, code := runWithInput(t, "", "connect"); code != ExitUsage {
		t.Fatalf("no address: exit %d", code)
	}
}

// Like setup: never from the agent's session, never under a running serve.
func TestConnectRefusesTheAgentAndARunningServe(t *testing.T) {
	e := newSetupEnv(t, addrRules("AAA"))
	saveConfig(t, e, phoneSerial)
	t.Setenv("ADBUDDY_SOURCE", "agent")
	if _, errOut, code := runWithInput(t, "", "connect", tsAddr); code != ExitFailure || !strings.Contains(errOut, "AGENT_SESSION") {
		t.Fatalf("agent: exit %d stderr %q", code, errOut)
	}
	t.Setenv("ADBUDDY_SOURCE", "")
	data := shortDir(t)
	t.Setenv("ADBUDDY_DATA_DIR", data)
	fakeDaemon(t, data, nil, ipc.Exit{})
	if _, errOut, code := runWithInput(t, "", "connect", tsAddr); code != ExitFailure || !strings.Contains(errOut, "ALREADY_RUNNING") {
		t.Fatalf("serve running: exit %d stderr %q", code, errOut)
	}
	if calls := loggedCalls(t, e.log); len(calls) > 0 {
		t.Fatalf("adb called %q", calls)
	}
	if cfg := loadSetupConfig(t, e); cfg.Address != "" {
		t.Fatalf("address saved: %+v", cfg)
	}
}

func TestConnectHelpHasAnExample(t *testing.T) {
	out, _, _ := run(t, "connect", "--help")
	if !strings.Contains(out, "adbuddy connect 100.64.0.5:41234") {
		t.Fatalf("help: %s", out)
	}
}
