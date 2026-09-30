package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

const doctorSerial = "adb-AAAA1111-bbb._adb-tls-connect._tcp"

// doctorRules answer like a healthy phone with the app set up; extra rules go first.
func doctorRules(extra ...testutil.FakeRule) []testutil.FakeRule {
	return append(extra,
		testutil.FakeRule{Match: `^version$`, Stdout: "Android Debug Bridge version 1.0.41\nVersion 37.0.1-15733141\n"},
		testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n" + doctorSerial + "\tdevice\n"},
		testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "AAAA1111\n"},
		testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "\n"},
		testutil.FakeRule{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n      android.permission.WRITE_SECURE_SETTINGS: granted=true\n"},
		testutil.FakeRule{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService\n"},
		testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,tech.hybriq.adbuddy,10123\n"},
		testutil.FakeRule{Match: `^(kill-server|start-server)$`},
	)
}

// doctorEnv writes a config for a set-up phone and creates its environment.
func doctorEnv(t *testing.T, rules []testutil.FakeRule) setupEnv {
	t.Helper()
	e := newSetupEnv(t, rules)
	cfg := config.Default()
	cfg.Serial, cfg.ADBPath, cfg.Workspace = doctorSerial, fakeADB(t), e.ws
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if _, err := workspace.Init(workspace.Options{Dir: e.ws, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	return e
}

func doctorJSON(t *testing.T, out string) doctorReport {
	t.Helper()
	var env struct {
		OK   bool
		Data doctorReport
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK {
		t.Fatalf("%v %q", err, out)
	}
	return env.Data
}

func statusOf(rep doctorReport, name string) check {
	for _, c := range rep.Checks {
		if c.Name == name {
			return c
		}
	}
	return check{Status: "missing"}
}

func TestDoctorHealthyPhoneWithoutServeIsDegraded(t *testing.T) {
	doctorEnv(t, doctorRules())
	out, errOut, code := runWithInput(t, "", "doctor", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	rep := doctorJSON(t, out)
	for _, name := range []string{"config", "adb", "adb server", "phone", "identity", "network port", "phone app", "environment"} {
		if c := statusOf(rep, name); c.Status != checkOK {
			t.Errorf("%s: %+v", name, c)
		}
	}
	if c := statusOf(rep, "serve"); c.Status != checkWarn || c.Guidance == "" {
		t.Errorf("serve: %+v", c)
	}
	if rep.Verdict != "degraded" {
		t.Fatalf("verdict %s", rep.Verdict)
	}
}

func TestDoctorOnlyReads(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	runWithInput(t, "", "doctor")
	calls, _ := testutil.ReadLog(e.log)
	for _, c := range calls {
		j := c.Joined()
		for _, bad := range []string{"put", "grant", "connect", "usb", "install", "reverse tcp", "kill-server", "whitelist +"} {
			if strings.Contains(j, bad) {
				t.Errorf("doctor changed something: %q", j)
			}
		}
	}
}

func TestDoctorBrokenCases(t *testing.T) {
	cases := map[string]struct {
		rule     testutil.FakeRule
		name     string
		wantHelp string
	}{
		"no phone":    {testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n"}, "phone", "Wireless debugging"},
		"old adb":     {testutil.FakeRule{Match: `^version$`, Stdout: "Version 35.0.2-1\n"}, "adb", "adbuddy setup"},
		"wrong phone": {testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "ZZZZ9999\n"}, "identity", "adbuddy connect"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doctorEnv(t, doctorRules(tc.rule))
			out, _, code := runWithInput(t, "", "doctor", "--json")
			if code != 1 {
				t.Fatalf("exit %d, want 1\n%s", code, out)
			}
			rep := doctorJSON(t, out)
			c := statusOf(rep, tc.name)
			if rep.Verdict != "broken" || c.Status != checkFail || !strings.Contains(c.Run+c.Guidance, tc.wantHelp) {
				t.Fatalf("%s %+v", rep.Verdict, c)
			}
		})
	}
}

func TestDoctorAppNotSetUpIsOnlyAWarning(t *testing.T) {
	doctorEnv(t, doctorRules(
		testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,com.other,10124\n"},
	))
	out, _, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	c := statusOf(rep, "phone app")
	if code != 0 || c.Status != checkWarn || !strings.Contains(c.Detail, "battery exemption") || c.Run != "adbuddy setup" {
		t.Fatalf("exit %d %+v", code, c)
	}
}

func TestDoctorWithoutSetup(t *testing.T) {
	newSetupEnv(t, doctorRules())
	out, _, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	if code != 1 || statusOf(rep, "config").Run != "adbuddy setup" {
		t.Fatalf("exit %d %+v", code, rep)
	}
}

func TestDoctorHumanOutput(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n"}))
	out, _, _ := runWithInput(t, "", "doctor")
	for _, want := range []string{"✓ adb: 37.0.1", "✗ phone:", "    Guidance: Turn on Wireless debugging", "Verdict: broken"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestDoctorFixRestartsTheAdbServer(t *testing.T) {
	e := doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n", Times: 1}))
	out, _, code := runWithInput(t, "", "doctor", "--fix", "--json")
	rep := doctorJSON(t, out)
	if code != 0 || len(rep.Fixed) != 1 || statusOf(rep, "adb server").Status != checkOK {
		t.Fatalf("exit %d %+v", code, rep)
	}
	calls, _ := testutil.ReadLog(e.log)
	var joined []string
	for _, c := range calls {
		joined = append(joined, c.Joined())
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, "kill-server") || !strings.Contains(all, "start-server") {
		t.Fatalf("server not restarted:\n%s", all)
	}
}

func TestDoctorFixIsForThePersonOnly(t *testing.T) {
	doctorEnv(t, doctorRules())
	t.Setenv("ADBUDDY_SOURCE", "agent")
	_, errOut, code := runWithInput(t, "", "doctor", "--fix")
	if code != 1 || !strings.Contains(errOut, "AGENT_SESSION") {
		t.Fatalf("exit %d %q", code, errOut)
	}
	if _, _, code := runWithInput(t, "", "doctor", "--json"); code != 0 {
		t.Fatalf("the agent cannot run doctor: exit %d", code)
	}
}

// The accessibility service may be listed in its short form (pkg/.Cls), as
// Android writes it when the person turns it on in Settings.
func TestDoctorAcceptsTheShortAccessibilityName(t *testing.T) {
	doctorEnv(t, doctorRules(
		testutil.FakeRule{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "com.other/.Svc:tech.hybriq.adbuddy/.a11y.AgentAccessibilityService\n"},
	))
	out, _, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	if c := statusOf(rep, "phone app"); code != 0 || c.Status != checkOK {
		t.Fatalf("exit %d %+v", code, c)
	}
}

func TestDoctorMissingAccessibilityIsAWarning(t *testing.T) {
	doctorEnv(t, doctorRules(
		testutil.FakeRule{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "null\n"},
	))
	out, _, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	c := statusOf(rep, "phone app")
	if code != 0 || c.Status != checkWarn || !strings.Contains(c.Detail, "accessibility service") {
		t.Fatalf("exit %d %+v", code, c)
	}
}
