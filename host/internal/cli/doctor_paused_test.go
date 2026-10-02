package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// V20: paused from the phone, the agent's doctor calls no adb (it would run
// outside the shim) and says why each check was not run; the person's doctor
// still checks the phone; both show the pause as a warning of serve.
func TestDoctorPausedAgentCallsNoAdb(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	data := shortDir(t)
	t.Setenv("ADBUDDY_DATA_DIR", data)
	fakeDaemon(t, data, ipc.Ping{Version: Version, Session: "s_1", Serial: doctorSerial, Journal: workspace.JournalDir(e.ws), Paused: true}, ipc.Exit{})

	t.Setenv("ADBUDDY_SOURCE", "agent")
	out, errOut, code := runWithInput(t, "", "doctor", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if calls := adbCalls(t, e); calls != "" {
		t.Fatalf("the paused agent's doctor called adb:\n%s", calls)
	}
	rep := doctorJSON(t, out)
	for _, name := range []string{"adb", "adb server", "phone", "identity", "network port", "phone app"} {
		if c := statusOf(rep, name); c.Status != checkSkip || !strings.Contains(c.Detail, "paused from the phone") {
			t.Errorf("%s: %+v", name, c)
		}
	}
	if c := statusOf(rep, "serve"); c.Status != checkWarn || !strings.Contains(c.Guidance, "Paused from the phone") {
		t.Errorf("serve: %+v", c)
	}
	if c := statusOf(rep, "environment"); c.Status != checkOK {
		t.Errorf("environment: %+v", c)
	}
	if rep.Verdict != "degraded" {
		t.Fatalf("verdict %s", rep.Verdict)
	}

	// The person: the phone is checked; the pause is still a warning.
	t.Setenv("ADBUDDY_SOURCE", "")
	out, _, _ = runWithInput(t, "", "doctor", "--json")
	rep = doctorJSON(t, out)
	if c := statusOf(rep, "phone"); c.Status != checkOK {
		t.Errorf("person, phone: %+v", c)
	}
	if c := statusOf(rep, "serve"); c.Status != checkWarn {
		t.Errorf("person, serve: %+v", c)
	}
	if adbCalls(t, e) == "" {
		t.Fatal("the person's doctor did not check the phone")
	}
}

// Without the pause the agent's doctor checks the phone as before.
func TestDoctorAgentWithoutPauseChecksThePhone(t *testing.T) {
	e := doctorEnv(t, doctorRules())
	doctorServe(t, e)
	t.Setenv("ADBUDDY_SOURCE", "agent")
	out, _, _ := runWithInput(t, "", "doctor", "--json")
	rep := doctorJSON(t, out)
	for _, name := range []string{"adb", "adb server", "serve", "phone", "identity", "network port"} {
		if c := statusOf(rep, name); c.Status != checkOK {
			t.Errorf("%s: %+v", name, c)
		}
	}
}
