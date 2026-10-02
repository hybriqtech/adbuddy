package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
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
	return doctorEnvWith(t, rules, nil)
}

// doctorEnvWith is doctorEnv with a config that edit may change first.
func doctorEnvWith(t *testing.T, rules []testutil.FakeRule, edit func(*config.Config)) setupEnv {
	t.Helper()
	e := newSetupEnv(t, rules)
	cfg := config.Default()
	cfg.Serial, cfg.ADBPath, cfg.Workspace = doctorSerial, fakeADB(t), e.ws
	if edit != nil {
		edit(&cfg)
	}
	if err := config.Save(filepath.Join(e.cfgDir, "config.toml"), cfg); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if _, err := workspace.Init(workspace.Options{Dir: e.ws, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	return e
}

// doctorJSON reads the report from the envelope: {"ok":true} when ready or
// degraded, {"ok":false,"error":{"code":"DOCTOR_BROKEN"}} with the report
// as data when broken.
func doctorJSON(t *testing.T, out string) doctorReport {
	t.Helper()
	var env struct {
		OK    bool
		Error *struct{ Code string }
		Data  doctorReport
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("%v %q", err, out)
	}
	broken := env.Data.Verdict == "broken"
	switch {
	case env.Data.Verdict == "":
		t.Fatalf("no report: %q", out)
	case broken && (env.OK || env.Error == nil || env.Error.Code != "DOCTOR_BROKEN"):
		t.Fatalf("a broken report must be a DOCTOR_BROKEN failure: %q", out)
	case !broken && (!env.OK || env.Error != nil):
		t.Fatalf("a %s report must be ok: %q", env.Data.Verdict, out)
	}
	return env.Data
}

// setWait shortens one of doctor's time limits for a test.
func setWait(t *testing.T, v *time.Duration, d time.Duration) {
	t.Helper()
	old := *v
	*v = d
	t.Cleanup(func() { *v = old })
}

// adbCalls is every adb call the fake saw, one per line.
func adbCalls(t *testing.T, e setupEnv) string {
	t.Helper()
	calls, _ := testutil.ReadLog(e.log)
	var joined []string
	for _, c := range calls {
		joined = append(joined, c.Joined())
	}
	return strings.Join(joined, "\n")
}

// doctorServe fakes a running serve on e's environment, in a data folder
// short enough for the socket.
func doctorServe(t *testing.T, e setupEnv) {
	t.Helper()
	data := shortDir(t)
	t.Setenv("ADBUDDY_DATA_DIR", data)
	fakeDaemon(t, data, ipc.Ping{Version: Version, Session: "s_1", Serial: doctorSerial, Journal: workspace.JournalDir(e.ws)}, ipc.Exit{})
}

// channelRule answers `adb reverse --list` with the channel of chat port 47123 open.
var channelRule = testutil.FakeRule{Match: `^reverse --list$`, Stdout: "host-19 tcp:47123 tcp:47123\n"}

func withChatPort(c *config.Config) { c.ChatPort = 47123 }

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

// Review of task 7, I1: --fix never restarts the adb server under a running
// serve, says so, and doctor does not offer --fix then.
func TestDoctorFixLeavesTheServerOfServeAlone(t *testing.T) {
	e := doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n"}))
	doctorServe(t, e)
	out, _, code := runWithInput(t, "", "doctor", "--fix", "--json")
	rep := doctorJSON(t, out)
	c := statusOf(rep, "adb server")
	if code != 1 || len(rep.Fixed) != 0 || c.Status != checkFail || c.Run != "" || !strings.Contains(c.Detail, "serve is running") {
		t.Fatalf("exit %d %+v", code, rep)
	}
	if s := statusOf(rep, "serve"); s.Status != checkOK {
		t.Fatalf("serve: %+v", s)
	}
	if all := adbCalls(t, e); strings.Contains(all, "kill-server") || strings.Contains(all, "start-server") {
		t.Fatalf("restarted the server of serve:\n%s", all)
	}
	// Without --fix it is not offered either.
	out, _, _ = runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "adb server"); strings.Contains(c.Run+c.Guidance, "--fix") || c.Guidance == "" {
		t.Fatalf("%+v", c)
	}
}

// I2: --fix only reports what it fixed.
func TestDoctorFixThatDoesNotHelp(t *testing.T) {
	e := doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n"}))
	out, _, code := runWithInput(t, "", "doctor", "--fix", "--json")
	rep := doctorJSON(t, out)
	c := statusOf(rep, "adb server")
	if code != 1 || len(rep.Fixed) != 0 || c.Status != checkFail || !strings.Contains(c.Detail, "still does not answer") {
		t.Fatalf("exit %d %+v", code, rep)
	}
	if all := adbCalls(t, e); !strings.Contains(all, "start-server") {
		t.Fatalf("not even tried:\n%s", all)
	}
}

func TestDoctorFixWhenStartServerFails(t *testing.T) {
	doctorEnv(t, doctorRules(
		testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n", Times: 1},
		testutil.FakeRule{Match: `^start-server$`, Exit: 1, Stderr: "could not bind\n"},
	))
	out, _, _ := runWithInput(t, "", "doctor", "--fix", "--json")
	if rep := doctorJSON(t, out); len(rep.Fixed) != 0 {
		t.Fatalf("announced a fix that failed: %+v", rep)
	}
}

func TestDoctorFixWithNothingToFix(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	out, _, code := runWithInput(t, "", "doctor", "--fix", "--json")
	rep := doctorJSON(t, out)
	if code != 0 || len(rep.Fixed) != 0 {
		t.Fatalf("exit %d %+v", code, rep)
	}
	if all := adbCalls(t, e); strings.Contains(all, "kill-server") || strings.Contains(all, "start-server") {
		t.Fatalf("restarted a healthy server:\n%s", all)
	}
}

func TestDoctorFixWithoutAdb(t *testing.T) {
	e := doctorEnvWith(t, doctorRules(), func(c *config.Config) { c.ADBPath = filepath.Join(t.TempDir(), "no-such-adb") })
	out, _, code := runWithInput(t, "", "doctor", "--fix", "--json")
	rep := doctorJSON(t, out)
	if code != 1 || len(rep.Fixed) != 0 || statusOf(rep, "adb").Status != checkFail {
		t.Fatalf("exit %d %+v", code, rep)
	}
	if c := statusOf(rep, "adb server"); c.Status != checkSkip || c.Run != "" {
		t.Fatalf("adb server: %+v", c)
	}
	if strings.Contains(out, "--fix") {
		t.Fatalf("offers --fix without adb: %s", out)
	}
	if all := adbCalls(t, e); all != "" {
		t.Fatalf("called adb:\n%s", all)
	}
}

// I3: what the agent's session cannot run is something to ask the user.
func TestDoctorAgentAsksTheUser(t *testing.T) {
	rules := doctorRules(testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,com.other,10124\n"})
	doctorEnv(t, rules)
	// The person: Run lines.
	out, _, _ := runWithInput(t, "", "doctor")
	for _, want := range []string{"    Run: adbuddy setup\n", "    Run: adbuddy serve\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("person: missing %q in\n%s", want, out)
		}
	}
	// The agent: the same commands, as something to ask.
	t.Setenv("ADBUDDY_SOURCE", "agent")
	out, _, _ = runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	for name, cmd := range map[string]string{"phone app": "adbuddy setup", "serve": "adbuddy serve"} {
		c := statusOf(rep, name)
		if c.Run != "" || !strings.HasPrefix(c.Guidance, "Ask the user to run "+cmd) {
			t.Errorf("agent %s: %+v", name, c)
		}
	}
	out, _, _ = runWithInput(t, "", "doctor")
	if strings.Contains(out, "Run: adbuddy") || !strings.Contains(out, "    Guidance: Ask the user to run adbuddy setup") {
		t.Fatalf("agent, text:\n%s", out)
	}
}

func TestDoctorAgentRunsWhatIsSafe(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	os.Remove(filepath.Join(e.ws, "AGENTS.md"))
	t.Setenv("ADBUDDY_SOURCE", "agent")
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "environment"); c.Status != checkFail || !strings.HasPrefix(c.Run, "adbuddy env init --dir ") {
		t.Fatalf("%+v", c)
	}
}

func TestDoctorAgentAsksForFix(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n"}))
	t.Setenv("ADBUDDY_SOURCE", "agent")
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "adb server"); c.Run != "" || !strings.HasPrefix(c.Guidance, "Ask the user to run adbuddy doctor --fix") {
		t.Fatalf("%+v", c)
	}
}

// I4: Ctrl+C stops doctor without a report made of failures.
func TestDoctorCtrlC(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, SleepMs: 5000}))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	out, _, code := runCtx(t, ctx, "doctor", "--json")
	if code != ExitInterrupt || strings.Contains(out, "verdict") || !strings.Contains(out, "INTERRUPTED") {
		t.Fatalf("exit %d %q", code, out)
	}
}

func TestDoctorCtrlCDuringFix(t *testing.T) {
	doctorEnv(t, doctorRules(
		testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n", Times: 1},
		testutil.FakeRule{Match: `^start-server$`, SleepMs: 5000},
	))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(1500*time.Millisecond, cancel)
	out, _, code := runCtx(t, ctx, "doctor", "--fix")
	if code != ExitInterrupt || strings.Contains(out, "Verdict") {
		t.Fatalf("exit %d %q", code, out)
	}
}

// I5: broken is a failure envelope that still carries the report.
func TestDoctorBrokenJSONEnvelope(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n"}))
	out, _, code := runWithInput(t, "", "doctor", "--json")
	if code != 1 || !strings.HasPrefix(out, `{"ok":false,"error":{"code":"DOCTOR_BROKEN"`) || !strings.Contains(out, `"data":{"verdict":"broken"`) {
		t.Fatalf("exit %d %s", code, out)
	}
}

// I6: a read that fails is "could not check", not a false "missing".
func TestDoctorAppReadsThatFail(t *testing.T) {
	fail := func(match string) testutil.FakeRule {
		return testutil.FakeRule{Match: match, Exit: 1, Stderr: "error: closed\n"}
	}
	cases := map[string][]testutil.FakeRule{
		"package": {
			{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n", Times: 1},
			fail(`^shell dumpsys package tech\.hybriq\.adbuddy$`),
		},
		"accessibility": {fail(`^shell settings get secure enabled_accessibility_services$`)},
		"battery":       {fail(`^shell dumpsys deviceidle whitelist$`)},
	}
	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			doctorEnv(t, doctorRules(rules...))
			out, _, code := runWithInput(t, "", "doctor", "--json")
			c := statusOf(doctorJSON(t, out), "phone app")
			if code != 0 || c.Status != checkWarn || !strings.HasPrefix(c.Detail, "could not check: ") || strings.Contains(c.Detail, "without") ||
				c.Guidance != "Unlock the phone and run adbuddy doctor again." || c.Run != "" {
				t.Fatalf("exit %d %+v", code, c)
			}
		})
	}
}

// Minors.
func TestDoctorDoesNotCreateTheJournal(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	jdir := workspace.JournalDir(e.ws)
	if err := os.RemoveAll(jdir); err != nil {
		t.Fatal(err)
	}
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "environment"); c.Status != checkOK || !strings.Contains(c.Detail, "created on first use") {
		t.Fatalf("%+v", c)
	}
	if _, err := os.Stat(jdir); !os.IsNotExist(err) {
		t.Fatalf("doctor created the journal folder: %v", err)
	}
	// With the folder there, nothing is written into it.
	os.MkdirAll(jdir, 0o700)
	runWithInput(t, "", "doctor", "--json")
	if files, _ := os.ReadDir(jdir); len(files) != 0 {
		t.Fatalf("doctor wrote into the journal: %v", files)
	}
}

func TestDoctorJournalNotWritable(t *testing.T) {
	cases := map[string]func(t *testing.T, jdir string){
		// This month's file, the one the journal appends to, is read-only.
		"read-only file": func(t *testing.T, jdir string) {
			f := filepath.Join(jdir, time.Now().UTC().Format("2006-01")+".jsonl")
			if err := os.WriteFile(f, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(f, 0o400); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(f, 0o600) })
		},
		"not a folder": func(t *testing.T, jdir string) {
			if err := os.Remove(jdir); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(jdir, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		// A link (a junction on Windows) in place of the folder: the
		// journal refuses it.
		"a link": func(t *testing.T, jdir string) {
			if err := os.Remove(jdir); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()
			if err := os.Symlink(target, jdir); err != nil {
				if runtime.GOOS != "windows" {
					t.Fatal(err)
				}
				// A junction, with System32's cmd: the PATH of these tests
				// is the fake adb's.
				cmd := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
				if out, err := exec.Command(cmd, "/c", "mklink", "/J", jdir, target).CombinedOutput(); err != nil {
					t.Skipf("mklink /J: %v %s", err, out)
				}
			}
		},
		// This month's file is not a regular file: the journal refuses it.
		"month file not regular": func(t *testing.T, jdir string) {
			if err := os.Mkdir(filepath.Join(jdir, time.Now().UTC().Format("2006-01")+".jsonl"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			e := doctorEnv(t, doctorRules())
			spoil(t, workspace.JournalDir(e.ws))
			out, _, code := runWithInput(t, "", "doctor", "--json")
			if c := statusOf(doctorJSON(t, out), "environment"); code != 1 || c.Status != checkFail || !strings.Contains(c.Detail, "journal cannot be written") || c.Guidance == "" {
				t.Fatalf("exit %d %+v", code, c)
			}
		})
	}
}

func TestDoctorSkipsWhatDependsOnTheServer(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n"}))
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	for _, name := range []string{"phone", "identity", "network port", "phone app"} {
		if c := statusOf(rep, name); c.Status != checkSkip || c.Detail != "not checked: adb server down" {
			t.Errorf("%s: %+v", name, c)
		}
	}
	if c := statusOf(rep, "adb server"); c.Run != "adbuddy doctor --fix" {
		t.Errorf("adb server: %+v", c)
	}
	var names []string
	for _, c := range rep.Checks {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, ","); got != "config,adb,adb server,serve,phone,identity,network port,phone app,environment,claude hooks" {
		t.Errorf("order: %s", got)
	}
}

func TestDoctorBatteryExemptionOfAnotherPackage(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "system,com.android.shell,2000\nuser,tech.hybriq.adbuddy2,10124\n"}))
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "phone app"); c.Status != checkWarn || !strings.Contains(c.Detail, "battery exemption") {
		t.Fatalf("%+v", c)
	}
}

func TestDoctorWithServeRunning(t *testing.T) {
	e := doctorEnvWith(t, doctorRules(channelRule), withChatPort)
	doctorServe(t, e)
	out, errOut, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	if code != 0 || rep.Verdict != "ready" || statusOf(rep, "serve").Status != checkOK {
		t.Fatalf("exit %d %+v %s", code, rep, errOut)
	}
}

func TestDoctorChannelClosed(t *testing.T) {
	e := doctorEnvWith(t, doctorRules(testutil.FakeRule{Match: `^reverse --list$`}), withChatPort)
	doctorServe(t, e)
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	c := statusOf(doctorJSON(t, out), "phone app")
	if c.Status != checkWarn || c.Guidance != "Wait 15 seconds and run adbuddy doctor again; if it stays closed, open the ADBuddy app." {
		t.Fatalf("%+v", c)
	}
}

func TestDoctorChannelUnknown(t *testing.T) {
	e := doctorEnvWith(t, doctorRules(testutil.FakeRule{Match: `^reverse --list$`, Exit: 1, Stderr: "error: closed\n"}), withChatPort)
	doctorServe(t, e)
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "phone app"); c.Status != checkWarn || c.Guidance == "" {
		t.Fatalf("%+v", c)
	}
}

func TestDoctorServeThatDoesNotAnswer(t *testing.T) {
	doctorEnv(t, doctorRules())
	setWait(t, &doctorPingWait, 300*time.Millisecond)
	data := shortDir(t)
	t.Setenv("ADBUDDY_DATA_DIR", data)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, _ ipc.Request, _ *ipc.Conn) ipc.Exit {
		<-ctx.Done()
		return ipc.Exit{}
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	out, _, code := runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "serve"); code != 0 || c.Status != checkWarn || !strings.Contains(c.Detail, "does not answer") || c.Guidance == "" {
		t.Fatalf("exit %d %+v", code, c)
	}
}

func TestDoctorHungAdb(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, SleepMs: 5000}))
	setWait(t, &doctorCallWait, 300*time.Millisecond)
	start := time.Now()
	out, _, code := runWithInput(t, "", "doctor", "--json")
	c := statusOf(doctorJSON(t, out), "adb server")
	if code != 1 || c.Status != checkFail || !strings.Contains(c.Detail, "no answer within") {
		t.Fatalf("exit %d %+v", code, c)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestDoctorTimeLimit(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^version$`, SleepMs: 5000}))
	setWait(t, &doctorTotalWait, 300*time.Millisecond)
	start := time.Now()
	out, _, code := runWithInput(t, "", "doctor", "--json")
	c := statusOf(doctorJSON(t, out), "adb")
	if code != 1 || c.Status != checkFail || !strings.Contains(c.Detail, "time limit") {
		t.Fatalf("exit %d %+v", code, c)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestDoctorNetworkPortOpen(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "5555\n"}))
	out, _, code := runWithInput(t, "", "doctor", "--json")
	c := statusOf(doctorJSON(t, out), "network port")
	if code != 0 || c.Status != checkWarn || !strings.Contains(c.Detail, "5555") {
		t.Fatalf("exit %d %+v", code, c)
	}
	if strings.Contains(c.Guidance, "later version") || strings.Contains(c.Detail, "remote mode") {
		t.Fatalf("%+v", c)
	}
}

// Final review, minor 1: with the remote mode on, doctor says so and until
// when, and that with serve stopped nothing closes the port; it does not
// reassure that serve closes it.
func TestDoctorNetworkPortOpenInRemoteMode(t *testing.T) {
	cases := map[string]struct {
		until time.Time
		want  string
	}{
		"on":      {until: time.Now().Add(5 * time.Hour), want: "remote mode on until "},
		"expired": {until: time.Now().Add(-time.Hour), want: "remote mode expired at "},
	}
	// Re-review M2: an expiry edited by hand beyond 12 hours counts as
	// expired, as the controller has it; doctor does not say "on until" it.
	t.Run("beyond the lifetime", func(t *testing.T) {
		far := time.Now().Add(20 * time.Hour)
		doctorEnvWith(t, doctorRules(testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "5555\n"}),
			func(cfg *config.Config) {
				cfg.TailscaleIP, cfg.RemoteUntil = "100.64.0.5", far.UTC().Format(time.RFC3339)
			})
		out, _, _ := runWithInput(t, "", "doctor", "--json")
		ch := statusOf(doctorJSON(t, out), "network port")
		if ch.Status != checkWarn || !strings.Contains(ch.Detail, "remote mode expired at ") || strings.Contains(ch.Detail, "on until") {
			t.Fatalf("%+v", ch)
		}
	})
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doctorEnvWith(t, doctorRules(testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "5555\n"}),
				func(cfg *config.Config) {
					cfg.TailscaleIP, cfg.RemoteUntil = "100.64.0.5", c.until.UTC().Format(time.RFC3339)
				})
			out, _, code := runWithInput(t, "", "doctor", "--json")
			ch := statusOf(doctorJSON(t, out), "network port")
			if code != 0 || ch.Status != checkWarn || !strings.Contains(ch.Detail, c.want+c.until.Local().Format("15:04")) {
				t.Fatalf("exit %d %+v", code, ch)
			}
			if strings.Contains(ch.Detail+ch.Guidance, "serve closes it") || strings.Contains(ch.Guidance, "later version") ||
				!strings.Contains(ch.Guidance, "Nothing closes it while serve is stopped") || !strings.Contains(ch.Guidance, "adbuddy remote off") {
				t.Fatalf("%+v", ch)
			}
		})
	}
}

func TestDoctorIdentityThatCannotBeChecked(t *testing.T) {
	t.Run("guid without serial number", func(t *testing.T) {
		const serial = "adb-abc-x1._adb-tls-connect._tcp"
		doctorEnvWith(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n" + serial + "\tdevice\n"}),
			func(c *config.Config) { c.Serial = serial })
		out, _, _ := runWithInput(t, "", "doctor", "--json")
		if c := statusOf(doctorJSON(t, out), "identity"); c.Status != checkWarn || c.Guidance == "" || strings.Contains(c.Guidance, "Nothing to do") {
			t.Fatalf("%+v", c)
		}
	})
	t.Run("only an address", func(t *testing.T) {
		const addr = "192.0.2.10:37000"
		doctorEnvWith(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n" + addr + "\tdevice\n"}),
			func(c *config.Config) { c.Serial, c.Address = "", addr })
		out, _, _ := runWithInput(t, "", "doctor", "--json")
		rep := doctorJSON(t, out)
		c := statusOf(rep, "identity")
		if statusOf(rep, "config").Status != checkOK || c.Status != checkWarn || strings.Contains(c.Detail, "no phone is set up") || !strings.Contains(c.Guidance, addr) {
			t.Fatalf("%+v", rep)
		}
	})
}

func TestDoctorSerialNumberUnreadable(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Exit: 1, Stderr: "error: closed\n"}))
	out, _, code := runWithInput(t, "", "doctor", "--json")
	if c := statusOf(doctorJSON(t, out), "identity"); code != 0 || c.Status != checkWarn || !strings.Contains(c.Detail, "could not read") {
		t.Fatalf("exit %d %+v", code, c)
	}
}

func TestDoctorEnvironmentIncomplete(t *testing.T) {
	for name, f := range map[string]string{"AGENTS.md": "AGENTS.md", "shim": filepath.Join(".adbuddy", "bin", exeName("adb"))} {
		t.Run(name, func(t *testing.T) {
			e := doctorEnv(t, doctorRules())
			if err := os.Remove(filepath.Join(e.ws, f)); err != nil {
				t.Fatal(err)
			}
			out, _, code := runWithInput(t, "", "doctor", "--json")
			c := statusOf(doctorJSON(t, out), "environment")
			if code != 1 || c.Status != checkFail || !strings.Contains(c.Detail, f) || !strings.HasPrefix(c.Run, "adbuddy env init --dir ") {
				t.Fatalf("exit %d %+v", code, c)
			}
		})
	}
}

func TestDoctorAdbVersions(t *testing.T) {
	cases := map[string]struct{ out, status string }{
		"unknown":    {"Android Debug Bridge version 1.0.41\n", checkWarn},
		"exactly 37": {"Version 37.0.0-1\n", checkOK},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^version$`, Stdout: tc.out}))
			out, _, _ := runWithInput(t, "", "doctor", "--json")
			if c := statusOf(doctorJSON(t, out), "adb"); c.Status != tc.status {
				t.Fatalf("%+v", c)
			}
		})
	}
}

func TestDoctorInvalidConfig(t *testing.T) {
	e := newSetupEnv(t, doctorRules())
	os.MkdirAll(e.cfgDir, 0o700)
	if err := os.WriteFile(filepath.Join(e.cfgDir, "config.toml"), []byte("chat_port = 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, code := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	c := statusOf(rep, "config")
	if code != 1 || c.Status != checkFail || !strings.Contains(c.Guidance, "config.toml") {
		t.Fatalf("exit %d %+v", code, rep)
	}
	if s := statusOf(rep, "adb"); s.Status != checkSkip {
		t.Fatalf("adb: %+v", s)
	}
}

func TestDoctorJSONCarriesRunAndGuidance(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,com.other,10124\n"}))
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	for _, want := range []string{`"run":"adbuddy setup"`, `"guidance":"Accept the consent screen`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in %s", want, out)
		}
	}
}

func TestDoctorHelpFollowsTheOrderOfTheChecks(t *testing.T) {
	out, _, _ := runWithInput(t, "", "doctor", "--help")
	last := -1
	for _, w := range []string{"configuration", "private server", "`adbuddy serve`", "the phone (", "the phone app", "the agent environment"} {
		i := strings.Index(out, w)
		if i <= last {
			t.Fatalf("%q out of order in\n%s", w, out)
		}
		last = i
	}
}

func TestDoctorWarnsOfGlobalClaudeHooks(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(cfg, "settings.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if c := checkClaudeHooks(); c.Status != checkOK {
		t.Errorf("no settings file: %+v", c)
	}
	write(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"x"}]}]}}`)
	if c := checkClaudeHooks(); c.Status != checkOK {
		t.Errorf("an unrelated hook: %+v", c)
	}
	write(`{"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":"x"}]}],"PreToolUse":[]}}`)
	c := checkClaudeHooks()
	if c.Status != checkWarn || !strings.Contains(c.Detail, "PermissionRequest") || strings.Contains(c.Detail, "PreToolUse") || c.Guidance == "" {
		t.Errorf("a global PermissionRequest hook: %+v", c)
	}
	write(`{"hooks":{"PreToolUse":[{"hooks":[]}]}}`)
	if c := checkClaudeHooks(); c.Status != checkWarn || !strings.Contains(c.Detail, "PreToolUse") {
		t.Errorf("a global PreToolUse hook: %+v", c)
	}
	write(`{not json`)
	if c := checkClaudeHooks(); c.Status != checkOK {
		t.Errorf("unreadable settings must not alarm: %+v", c)
	}
}
