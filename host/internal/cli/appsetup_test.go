package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

const testAppPath = "/data/app/~~Ab==/tech.hybriq.adbuddy-Cd==/base.apk"

// appRules answers like a phone whose app (signed with testCert) says
// consent.
func appRules(consent string, over ...testutil.FakeRule) []testutil.FakeRule {
	return appRulesFor(consent, testAPK(testCert), over...)
}

// appRulesFor answers like a phone with apk installed at testAppPath, whose
// app says consent. The first rule that matches answers, so a test puts its
// changes in front (over).
func appRulesFor(consent string, apk []byte, over ...testutil.FakeRule) []testutil.FakeRule {
	provider := `^shell content call --uri content://tech\.hybriq\.adbuddy\.bridge --method `
	rules := append([]testutil.FakeRule{}, over...)
	rules = append(rules, appReadRules(apk)...)
	return append(append(rules, []testutil.FakeRule{
		{Match: `^install -r `, Stdout: "Performing Streamed Install\nSuccess\n"},
		{Match: provider + `requestSetup --extra `, Stdout: "Result: Bundle[{ok=true}]\n"},
		{Match: `^shell am start -n tech\.hybriq\.adbuddy/\.MainActivity -a tech\.hybriq\.adbuddy\.action\.SETUP$`, Stdout: "Starting: Intent\n"},
		{Match: provider + `getSetupConsent$`, Stdout: "Result: Bundle[{consent=" + consent + "}]\n"},
		{Match: `^shell pm grant tech\.hybriq\.adbuddy android\.permission\.WRITE_SECURE_SETTINGS$`},
		{Match: `^shell dumpsys deviceidle whitelist \+tech\.hybriq\.adbuddy$`, Stdout: "Added: tech.hybriq.adbuddy\n"},
		{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "com.x/.Svc\n"},
		{Match: `^shell settings put secure `},
	}...), phoneRules()...)
}

// appReadRules answers what the host reads of the installed app: its
// versionCode and versionName, and its path, stat and bytes (pm path, stat
// and base64).
func appReadRules(apk []byte) []testutil.FakeRule {
	return []testutil.FakeRule{
		{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n  Package [tech.hybriq.adbuddy] (1a2b):\n    versionCode=1 minSdk=30 targetSdk=37\n    versionName=0.2.0\n"},
		{Match: `^shell pm path tech\.hybriq\.adbuddy$`, Stdout: "package:" + testAppPath + "\n"},
		{Match: `^shell stat -c `, Stdout: strconv.Itoa(len(apk)) + " 1 1\n"},
		{Match: `^shell base64 `, Stdout: base64.StdEncoding.EncodeToString(apk) + "\n"},
	}
}

// The tests that reach the app pin testCert as the release certificate.
func init() { releaseCert = apktest.SHA256(testCert) }

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

// notInstalled answers "the app is not installed" (times > 0: only that
// many times, then the app is there).
func notInstalled(times int) testutil.FakeRule {
	return testutil.FakeRule{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n", Times: times}
}

// Test certificates: tests that reach the app pin testCert (pinTestCert).
var (
	testCert  = apktest.Cert("hybriqtech")
	otherCert = apktest.Cert("someone else")
)

// testAPK is an APK signed (v2 and v3) with cert.
func testAPK(cert []byte) []byte {
	return apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: cert}}}, apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: cert}}})
}

// pinTestCert makes setup trust testCert as the release certificate.
func pinTestCert(t *testing.T) { setVar(t, &releaseCert, apktest.SHA256(testCert)) }

// release serves an APK signed with testCert and pins testCert.
func release(t *testing.T, status int, badSum bool) string {
	t.Helper()
	return releaseSignedBy(t, status, badSum, testCert)
}

// releaseSignedBy serves an APK signed with cert (status != 200: an error;
// badSum: a .sha256 that does not match) and pins testCert.
func releaseSignedBy(t *testing.T, status int, badSum bool, cert []byte) string {
	t.Helper()
	pinTestCert(t)
	apk := testAPK(cert)
	sum := sha256.Sum256(apk)
	if badSum {
		sum = sha256.Sum256([]byte("another file"))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "nope", status)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  adbuddy.apk\n"))
			return
		}
		w.Write(apk)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

// writeAPK writes an APK signed with testCert, as --apk takes it.
func writeAPK(t *testing.T) string { return writeAPKSignedBy(t, testCert) }

func writeAPKSignedBy(t *testing.T, cert []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(p, testAPK(cert), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func phoneCalls(t *testing.T, e setupEnv) string {
	t.Helper()
	calls, _ := testutil.ReadLog(e.log)
	var joined []string
	for _, c := range calls {
		joined = append(joined, c.Joined())
	}
	return strings.Join(joined, "\n")
}

func assertUntouched(t *testing.T, all string) {
	t.Helper()
	for _, bad := range []string{"pm grant", "settings put", "deviceidle"} {
		if strings.Contains(all, bad) {
			t.Errorf("changed the phone without consent (%s):\n%s", bad, all)
		}
	}
}

func TestSetupSetsUpTheApp(t *testing.T) {
	e := newSetupEnv(t, appRules("granted"))
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	r := setupJSON(t, out).App
	if !r.Installed || r.Version != "0.2.0" || r.Consent != "granted" || !r.Permission || !r.Battery || !r.Accessibility {
		t.Fatalf("%+v\n%s", r, errOut)
	}
	if strings.Contains(errOut, "⚠") || !strings.Contains(errOut, "is ready") || !strings.Contains(errOut, "waiting up to 5 minutes") {
		t.Errorf("stderr: %s", errOut)
	}
	if !strings.Contains(errOut, "already on were kept: com.x/.Svc") {
		t.Errorf("no note about the services kept: %s", errOut)
	}
	all := phoneCalls(t, e)
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

// Every way the app part of setup can go: setup ends well (exit 0) and says
// what happened; without consent, nothing changes on the phone.
func TestSetupAppBranches(t *testing.T) {
	type want struct {
		installed, permission, battery, a11y bool
		version, consent                     string
	}
	fail := func(match, stderr string) testutil.FakeRule {
		return testutil.FakeRule{Match: match, Exit: 1, Stderr: stderr}
	}
	cases := []struct {
		name     string
		consent  string
		over     []testutil.FakeRule
		args     func(t *testing.T) []string
		wait     time.Duration
		want     want
		stderr   []string // texts that must be there
		noStderr []string // texts that must not be there
		noChange bool     // no consent: nothing changed on the phone
		calls    []string // adb calls that must be there
		apk      []byte   // the app on the phone (nil: signed with testCert)
	}{
		{name: "apk", consent: "granted", over: []testutil.FakeRule{notInstalled(1)},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPK(t)} },
			want:   want{true, true, true, true, "0.2.0", "granted"},
			stderr: []string{"Installing the ADBuddy app", "is ready"}, noStderr: []string{"⚠"}, calls: []string{"install -r "}},
		{name: "apk missing", consent: "granted",
			args:   func(t *testing.T) []string { return []string{"--apk", filepath.Join(t.TempDir(), "nope.apk")} },
			want:   want{consent: "skipped"},
			stderr: []string{"skipping the app", "Run `adbuddy setup` again"}, noChange: true},
		{name: "download", consent: "granted", over: []testutil.FakeRule{notInstalled(1)},
			args:   func(t *testing.T) []string { t.Setenv("ADBUDDY_RELEASE_URL", release(t, 200, false)); return nil },
			want:   want{true, true, true, true, "0.2.0", "granted"},
			stderr: []string{"Getting the ADBuddy app", "Installing the ADBuddy app", "is ready"}, noStderr: []string{"⚠"}, calls: []string{"install -r "}},
		{name: "no release", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args:   func(t *testing.T) []string { t.Setenv("ADBUDDY_RELEASE_URL", release(t, 404, false)); return nil },
			want:   want{consent: "skipped"},
			stderr: []string{"no published version of the ADBuddy app yet", "--apk <file>"}, noStderr: []string{"⚠"}, noChange: true},
		{name: "bad checksum", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args:   func(t *testing.T) []string { t.Setenv("ADBUDDY_RELEASE_URL", release(t, 200, true)); return nil },
			want:   want{consent: "skipped"},
			stderr: []string{"does not match its checksum", "nothing was installed", "--apk <file>"}, noChange: true},
		{name: "other signer", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args: func(t *testing.T) []string {
				t.Setenv("ADBUDDY_RELEASE_URL", releaseSignedBy(t, 200, false, otherCert))
				return nil
			},
			want:   want{consent: "skipped"},
			stderr: []string{"not signed by hybriqtech", "nothing was installed", "--apk <file>"}, noChange: true},
		{name: "unpinned", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args: func(t *testing.T) []string {
				t.Setenv("ADBUDDY_RELEASE_URL", release(t, 200, false))
				setVar(t, &releaseCert, "")
				return nil
			},
			want:   want{consent: "skipped"},
			stderr: []string{"no pinned signing certificate", "--apk <file>"}, noStderr: []string{"⚠", "Getting the ADBuddy app"}, noChange: true},
		{name: "release url not allowed", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args: func(t *testing.T) []string {
				pinTestCert(t)
				t.Setenv("ADBUDDY_RELEASE_URL", "http://example.com/")
				return nil
			},
			want:   want{consent: "skipped"},
			stderr: []string{"ADBUDDY_RELEASE_URL must be https://"}, noChange: true},
		{name: "development build on the phone", consent: "granted",
			over: []testutil.FakeRule{notInstalled(0), {Match: `^install -r `, Exit: 1,
				Stderr: "adb: failed to install app.apk: Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package tech.hybriq.adbuddy signatures do not match newer version; ignoring!]\n"}},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPK(t)} },
			want:   want{consent: "skipped"},
			stderr: []string{"signed with another key", "Uninstall it on the phone", "adbuddy setup"}, noStderr: []string{"Could not install the app"}, noChange: true},
		{name: "installed app signed by someone else", consent: "granted", apk: testAPK(otherCert),
			want:   want{installed: true, version: "0.2.0", consent: "skipped"},
			stderr: []string{"not signed by hybriqtech", "--apk <that APK>"}, noChange: true},
		{name: "nothing trusted", consent: "granted",
			args:   func(t *testing.T) []string { setVar(t, &releaseCert, ""); return nil },
			want:   want{installed: true, version: "0.2.0", consent: "skipped"},
			stderr: []string{"trusts no signing certificate"}, noChange: true},
		{name: "apk of a development build", consent: "granted", apk: testAPK(otherCert), over: []testutil.FakeRule{notInstalled(1)},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPKSignedBy(t, otherCert)} },
			want:   want{true, true, true, true, "0.2.0", "granted"},
			stderr: []string{"Trusting this development build of the app (SHA-256 ", " of ", "development build you trusted"}, calls: []string{"install -r --no-incremental "}},
		{name: "a release forgets the development build", consent: "granted",
			args: func(t *testing.T) []string {
				if _, err := phoneapp.TrustDevAPK(os.Getenv("ADBUDDY_DATA_DIR"), strings.Repeat("a", 64)); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			want:   want{true, true, true, true, "0.2.0", "granted"},
			stderr: []string{"no longer trusts the development build"}},
		{name: "apk that is not signed", consent: "granted",
			args: func(t *testing.T) []string {
				p := filepath.Join(t.TempDir(), "x.apk")
				os.WriteFile(p, []byte("not an apk"), 0o600)
				return []string{"--apk", p}
			},
			want:   want{consent: "skipped"},
			stderr: []string{"not a signed APK", "skipping the app"}, noChange: true},
		{name: "install fails", consent: "granted",
			over:   []testutil.FakeRule{notInstalled(0), {Match: `^install -r `, Stdout: "Failure [INSTALL_FAILED_OLDER_SDK]\n"}},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPK(t)} },
			want:   want{consent: "skipped"},
			stderr: []string{"Could not install the app", "INSTALL_FAILED_OLDER_SDK", "--apk <file>"}, noChange: true},
		{name: "not there after install", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPK(t)} },
			want:   want{consent: "skipped"},
			stderr: []string{"does not appear as installed", "Run `adbuddy setup` again"}, noChange: true},
		{name: "consent denied", consent: "denied",
			want:   want{true, false, false, false, "0.2.0", "denied"},
			stderr: []string{"You said no in the app"}, noStderr: []string{"⚠"}, noChange: true},
		{name: "consent timeout", consent: "pending", wait: 200 * time.Millisecond,
			want:   want{true, false, false, false, "0.2.0", "timeout"},
			stderr: []string{"No answer in the app"}, noChange: true},
		{name: "consent error", consent: "granted",
			over:   []testutil.FakeRule{fail(`getSetupConsent$`, "boom")},
			want:   want{true, false, false, false, "0.2.0", "skipped"},
			stderr: []string{"Could not ask in the app", "boom", "Run `adbuddy setup` again"}, noChange: true},
		{name: "check fails", consent: "granted",
			over:   []testutil.FakeRule{fail(`^shell dumpsys package `, "boom")},
			want:   want{consent: "skipped"},
			stderr: []string{"Could not check the ADBuddy app", "boom", "Run `adbuddy setup` again"}, noChange: true},
		{name: "grant fails", consent: "granted",
			over:   []testutil.FakeRule{fail(`^shell pm grant `, "denied by policy")},
			want:   want{true, false, true, true, "0.2.0", "granted"},
			stderr: []string{"Could not give the app the permission", "Run `adbuddy setup` again"}, noStderr: []string{"is ready"}},
		{name: "battery fails", consent: "granted",
			over:   []testutil.FakeRule{{Match: `^shell dumpsys deviceidle whitelist `, Stdout: "Unknown command\n"}},
			want:   want{true, true, false, true, "0.2.0", "granted"},
			stderr: []string{"Could not exempt the app from battery", "Run `adbuddy setup` again"}, noStderr: []string{"is ready"}},
		{name: "accessibility fails", consent: "granted",
			over:   []testutil.FakeRule{fail(`^shell settings put secure `, "not allowed")},
			want:   want{true, true, true, false, "0.2.0", "granted"},
			stderr: []string{"Could not turn on the app's accessibility service", "Turn it on by hand"}, noStderr: []string{"is ready"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			apk := c.apk
			if apk == nil {
				apk = testAPK(testCert)
			}
			e := newSetupEnv(t, appRulesFor(c.consent, apk, c.over...))
			if c.wait > 0 {
				setVar(t, &consentWait, c.wait)
			}
			args := []string{"setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json"}
			if c.args != nil {
				args = append(args, c.args(t)...)
			}
			out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", args...)
			if code != 0 {
				t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
			}
			r := setupJSON(t, out).App
			got := want{r.Installed, r.Permission, r.Battery, r.Accessibility, r.Version, r.Consent}
			if got != c.want {
				t.Errorf("app %+v, want %+v\n%s", got, c.want, errOut)
			}
			for _, s := range c.stderr {
				if !strings.Contains(errOut, s) {
					t.Errorf("stderr lacks %q:\n%s", s, errOut)
				}
			}
			for _, s := range c.noStderr {
				if strings.Contains(errOut, s) {
					t.Errorf("stderr has %q:\n%s", s, errOut)
				}
			}
			all := phoneCalls(t, e)
			if c.noChange {
				assertUntouched(t, all)
			}
			for _, s := range c.calls {
				if !strings.Contains(all, s) {
					t.Errorf("missing call %q in\n%s", s, all)
				}
			}
		})
	}
}

func TestSetupNoAppSkipsIt(t *testing.T) {
	e := newSetupEnv(t, appRules("granted"))
	out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--no-app", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	if !strings.Contains(out, "app: skipped") || strings.Contains(out, "not installed") {
		t.Errorf("summary: %s", out)
	}
	all := phoneCalls(t, e)
	assertUntouched(t, all)
	if strings.Contains(all, "tech.hybriq.adbuddy") {
		t.Errorf("--no-app touched the app:\n%s", all)
	}
	// And in JSON.
	e = newSetupEnv(t, appRules("granted"))
	out, _, code = runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--no-app", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
	if r := setupJSON(t, out).App; code != 0 || r.Installed || r.Consent != "skipped" {
		t.Fatalf("exit %d %+v", code, r)
	}
}

func TestSetupApkWithNoAppIsAUsageError(t *testing.T) {
	e := newSetupEnv(t, appRules("granted"))
	_, errOut, code := runWithInput(t, "", "setup", "--apk", writeAPK(t), "--no-app", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitUsage || !strings.Contains(errOut, "--apk") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestReleaseURL(t *testing.T) {
	get := func(in string) (string, error) {
		return releaseURL(&App{Getenv: func(k string) string {
			if k == "ADBUDDY_RELEASE_URL" {
				return in
			}
			return ""
		}})
	}
	for in, want := range map[string]string{
		"":                        phoneapp.ReleaseURL,
		"http://127.0.0.1:9/x":    "http://127.0.0.1:9/x/",
		"http://127.0.0.1:9/x/":   "http://127.0.0.1:9/x/",
		"http://[::1]:9/x":        "http://[::1]:9/x/",
		"http://localhost:9/x":    "http://localhost:9/x/",
		"https://example.com/dl":  "https://example.com/dl/",
		"https://example.com/dl/": "https://example.com/dl/",
	} {
		if got, err := get(in); err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"example.com",                // no scheme
		"http://example.com/",        // plain http to another host
		"file:///tmp/x",              // not a web URL
		"ftp://127.0.0.1/",           // loopback, but not http
		"http://127.0.0.1.evil.com/", // starts like loopback
		"http://127.0.0.1@evil.com/", // the host is evil.com
		"https://",                   // no host
		"https:///x",                 // no host
	} {
		if got, err := get(in); err == nil {
			t.Errorf("%q accepted as %q", in, got)
		}
	}
}

func TestFmtApp(t *testing.T) {
	for _, c := range []struct {
		r    appResult
		want string
	}{
		{appResult{Consent: "skipped", Note: "--no-app"}, "app: skipped"},
		{appResult{Consent: "skipped"}, "app: not installed"},
		{appResult{Installed: true, Version: "0.2.0", Consent: "denied"}, "app 0.2.0 (denied)"},
		{appResult{Installed: true, Version: "0.2.0", Consent: "granted", Permission: true, Battery: true, Accessibility: true}, "app 0.2.0 (granted)"},
		{appResult{Installed: true, Version: "0.2.0", Consent: "granted", Permission: true, Battery: true}, "app 0.2.0 (granted, accessibility off)"},
		{appResult{Installed: true, Version: "0.2.0", Consent: "granted", Accessibility: true}, "app 0.2.0 (granted, permission off, battery off)"},
	} {
		if got := fmtApp(c.r); got != c.want {
			t.Errorf("%q, want %q", got, c.want)
		}
	}
}
