package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

func appRules(consent string) []testutil.FakeRule {
	provider := `^shell content call --uri content://tech\.hybriq\.adbuddy\.bridge --method `
	return append([]testutil.FakeRule{
		{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n    versionName=0.2.0\n"},
		{Match: provider + `requestSetup --extra `, Stdout: "Result: Bundle[{ok=true}]\n"},
		{Match: `^shell am start -n tech\.hybriq\.adbuddy/\.MainActivity -a tech\.hybriq\.adbuddy\.action\.SETUP$`, Stdout: "Starting: Intent\n"},
		{Match: provider + `getSetupConsent$`, Stdout: "Result: Bundle[{consent=" + consent + "}]\n"},
		{Match: `^shell pm grant tech\.hybriq\.adbuddy android\.permission\.WRITE_SECURE_SETTINGS$`},
		{Match: `^shell dumpsys deviceidle whitelist \+tech\.hybriq\.adbuddy$`, Stdout: "Added: tech.hybriq.adbuddy\n"},
		{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "com.x/.Svc\n"},
		{Match: `^shell settings put secure `},
	}, phoneRules()...)
}

func setupJSON(t *testing.T, out string) setupResult {
	t.Helper()
	var env struct {
		OK   bool
		Data setupResult
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK {
		t.Fatalf("%v %q", err, out)
	}
	return env.Data
}

func TestSetupSetsUpTheApp(t *testing.T) {
	e := newSetupEnv(t, appRules("granted"))
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	r := setupJSON(t, out).App
	if !r.Installed || r.Version != "0.2.0" || r.Consent != "granted" || !r.Accessibility {
		t.Fatalf("%+v\n%s", r, errOut)
	}
	calls, _ := testutil.ReadLog(e.log)
	var joined []string
	for _, c := range calls {
		joined = append(joined, c.Joined())
	}
	all := strings.Join(joined, "\n")
	for _, want := range []string{
		"shell pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS",
		"shell dumpsys deviceidle whitelist +tech.hybriq.adbuddy",
		"shell settings put secure enabled_accessibility_services com.x/.Svc:tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService",
		"shell settings put secure accessibility_enabled 1",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in\n%s", want, all)
		}
	}
	cfg, err := config.Load(filepath.Join(e.cfgDir, "config.toml"))
	if err != nil || cfg.ChatPort < 20000 || cfg.ChatPort >= 60000 {
		t.Fatalf("chat port %d %v", cfg.ChatPort, err)
	}
}

func TestSetupWithoutConsentChangesNothingOnThePhone(t *testing.T) {
	e := newSetupEnv(t, appRules("denied"))
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if r := setupJSON(t, out).App; r.Consent != "denied" || r.Accessibility {
		t.Fatalf("%+v", r)
	}
	calls, _ := testutil.ReadLog(e.log)
	for _, c := range calls {
		if j := c.Joined(); strings.Contains(j, "pm grant") || strings.Contains(j, "settings put") || strings.Contains(j, "deviceidle") {
			t.Fatalf("changed the phone without consent: %q", j)
		}
	}
}

func TestSetupWithoutReleaseSkipsTheApp(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if r := setupJSON(t, out).App; r.Installed || r.Consent != "skipped" {
		t.Fatalf("%+v", r)
	}
}
