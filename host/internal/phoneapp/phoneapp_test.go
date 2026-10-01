package phoneapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

type fakeADB struct {
	answers map[string][]string // joined args -> successive stdouts (the last one repeats)
	reply   func(key string) (string, bool)
	hang    map[string]bool // joined args -> block until the context ends
	hangAll bool
	calls   []string
	left    []time.Duration // time left before each call's deadline (-1: none)
	stdin   string          // what the last call read from stdin
}

func (f *fakeADB) Run(ctx context.Context, _ string, args []string, stdin io.Reader, stdout, _ io.Writer) (int, error) {
	key := strings.Join(args, " ")
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		f.stdin = string(b)
	}
	f.calls = append(f.calls, key)
	if dl, ok := ctx.Deadline(); ok {
		f.left = append(f.left, time.Until(dl))
	} else {
		f.left = append(f.left, -1)
	}
	if f.hangAll || f.hang[key] {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	if f.reply != nil {
		if out, ok := f.reply(key); ok {
			io.WriteString(stdout, out)
			return 0, nil
		}
	}
	outs, ok := f.answers[key]
	if !ok {
		return 1, nil
	}
	io.WriteString(stdout, outs[0])
	if len(outs) > 1 {
		f.answers[key] = outs[1:]
	}
	return 0, nil
}

// set changes a package variable for one test.
func set[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

// within fails the test if fn does not return in d.
func within(t *testing.T, d time.Duration, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatal("still waiting: no time limit")
		return nil
	}
}

const (
	pingKey  = "shell content call --uri content://tech.hybriq.adbuddy.bridge --method ping"
	writeKey = "shell content write --uri content://tech.hybriq.adbuddy.bridge/handshake"
)

var testToken = strings.Repeat("0123456789abcdef", 4)

func TestPackageMatchesClassify(t *testing.T) {
	if Package != classify.ADBuddyPackage {
		t.Fatalf("phoneapp.Package %q != classify.ADBuddyPackage %q", Package, classify.ADBuddyPackage)
	}
}

func TestInstalledVersion(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"shell dumpsys package tech.hybriq.adbuddy": {"Packages:\r\n  Package [tech.hybriq.adbuddy] (1a2b):\r\n    versionCode=3 minSdk=30\r\n    versionName=0.2.0\r\n    User 0: ceDataInode=4242 installed=true hidden=false suspended=false stopped=false notLaunched=false enabled=0 instant=false virtual=false\r\n"}}}
	v, err := InstalledVersion(context.Background(), f)
	if err != nil || v != "0.2.0" {
		t.Fatalf("%q %v", v, err)
	}
	f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{"Unable to find package: tech.hybriq.adbuddy\n"}
	if v, _ := InstalledVersion(context.Background(), f); v != "" {
		t.Fatalf("not installed: %q", v)
	}
	// After `pm uninstall -k --user 0` the package (and its data) stays on
	// the phone, but not for user 0.
	f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{"Packages:\n" +
		"  Package [tech.hybriq.adbuddy] (5d3c1a2):\n" +
		"    userId=10321\n" +
		"    pkg=Package{8f1e2b3 tech.hybriq.adbuddy}\n" +
		"    codePath=/data/app/~~Zx9==/tech.hybriq.adbuddy-Qw1==\n" +
		"    versionCode=3 minSdk=30 targetSdk=35\n" +
		"    versionName=0.2.0\n" +
		"    flags=[ HAS_CODE ALLOW_CLEAR_USER_DATA ]\n" +
		"    User 0: ceDataInode=4242 installed=false hidden=false suspended=false distractionFlags=0 stopped=true notLaunched=true enabled=0 instant=false virtual=false\n" +
		"      runtime permissions:\n"}
	if v, err := InstalledVersion(context.Background(), f); v != "" || err != nil {
		t.Fatalf("uninstalled for user 0: %q %v", v, err)
	}
}

func TestCallParsesTheBundle(t *testing.T) {
	key := "shell content call --uri content://tech.hybriq.adbuddy.bridge --method requestSetup --extra 'host:s:My PC'"
	f := &fakeADB{answers: map[string][]string{key: {"Result: Bundle[{ok=true, appVersion=0.2.0}]\r\n"}}}
	res, err := Call(context.Background(), f, "requestSetup", "host:s:My PC")
	if err != nil || res["appVersion"] != "0.2.0" {
		t.Fatal(res, err, f.calls)
	}
	f.answers[key] = []string{"Result: Bundle[{error=not the shell user}]\n"}
	if _, err := Call(context.Background(), f, "requestSetup", "host:s:My PC"); err == nil || !strings.Contains(err.Error(), "not the shell user") {
		t.Fatalf("refusal: %v", err)
	}
	f.answers[key] = []string{"Error while accessing provider\n"}
	if _, err := Call(context.Background(), f, "requestSetup", "host:s:My PC"); err == nil {
		t.Fatal("no bundle accepted")
	}
}

// sentHandshake reads the JSON the fake got on stdin.
func sentHandshake(t *testing.T, f *fakeADB) (h struct {
	Token string `json:"token"`
	Port  int    `json:"port"`
	Host  string `json:"host"`
	Nonce string `json:"nonce"`
}) {
	t.Helper()
	if err := json.Unmarshal([]byte(f.stdin), &h); err != nil {
		t.Fatalf("stdin %q: %v", f.stdin, err)
	}
	return h
}

func TestHandshakeSendsTheTokenOnStdinOnly(t *testing.T) {
	set(t, &pingEvery, time.Millisecond)
	f := &fakeADB{answers: map[string][]string{writeKey: {""}}}
	pings := 0
	f.reply = func(key string) (string, bool) {
		if key != pingKey {
			return "", false
		}
		if pings++; pings == 1 {
			return "Result: Bundle[{ok=true, port=0}]\n", true
		}
		return "Result: Bundle[{ok=true, port=47123, nonce=" + sentHandshake(t, f).Nonce + "}]\n", true
	}
	if err := Handshake(context.Background(), f, testToken, 47123, "My PC; reboot"); err != nil {
		t.Fatal(err, f.calls)
	}
	h := sentHandshake(t, f)
	if h.Token != testToken || h.Port != 47123 || h.Host != "My PC reboot" {
		t.Fatalf("stdin %q", f.stdin)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(h.Nonce) {
		t.Fatalf("nonce %q", h.Nonce)
	}
	for _, c := range f.calls {
		if strings.Contains(c, testToken) {
			t.Fatalf("the token is in a command line: %q", c)
		}
		if strings.Contains(c, h.Nonce) {
			t.Fatalf("the nonce is in a command line: %q", c)
		}
	}
	first := h.Nonce
	pings = 0
	if err := Handshake(context.Background(), f, testToken, 47123, "My PC"); err != nil {
		t.Fatal(err, f.calls)
	}
	if sentHandshake(t, f).Nonce == first {
		t.Fatal("the nonce is not new in each handshake")
	}
}

func TestHandshakeNeedsTheNonceAndThePort(t *testing.T) {
	set(t, &pingEvery, time.Millisecond)
	f := &fakeADB{answers: map[string][]string{writeKey: {""}}}
	nonce, port := "", "47123"
	f.reply = func(key string) (string, bool) {
		if key != pingKey {
			return "", false
		}
		n := nonce
		if n == "" {
			n = sentHandshake(t, f).Nonce
		}
		return "Result: Bundle[{ok=true, port=" + port + ", nonce=" + n + "}]\n", true
	}
	// The right port from an earlier handshake, but not this one's nonce:
	// the app did not store the new token.
	nonce = strings.Repeat("0", 32)
	if err := Handshake(context.Background(), f, testToken, 47123, "My PC"); err == nil {
		t.Fatal("a stale handshake was accepted")
	}
	nonce, port = "", "1"
	if err := Handshake(context.Background(), f, testToken, 47123, "My PC"); err == nil {
		t.Fatal("a handshake with another port was accepted")
	}
	port = "47123"
	if err := Handshake(context.Background(), f, testToken, 47123, "My PC"); err != nil {
		t.Fatal(err)
	}
}

func TestHandshakeChecksItsInputs(t *testing.T) {
	cases := []struct {
		token string
		port  int
	}{
		{"s3cret", 47123},
		{strings.ToUpper(testToken), 47123},
		{testToken[:63], 47123},
		{testToken + "0", 47123},
		{testToken[:63] + "g", 47123},
		{testToken, 0},
		{testToken, 1023},
		{testToken, 65536},
	}
	for _, c := range cases {
		f := &fakeADB{answers: map[string][]string{writeKey: {""}}}
		if err := Handshake(context.Background(), f, c.token, c.port, "My PC"); err == nil || len(f.calls) != 0 {
			t.Errorf("%q %d: %v %v", c.token, c.port, err, f.calls)
		}
	}
}

func consentCall(m string, extra ...string) string {
	s := "shell content call --uri content://tech.hybriq.adbuddy.bridge --method " + m
	for _, e := range extra {
		s += " --extra " + e
	}
	return s
}

func consentAnswers(consents ...string) map[string][]string {
	var outs []string
	for _, c := range consents {
		outs = append(outs, "Result: Bundle[{consent="+c+"}]\n")
	}
	return map[string][]string{
		consentCall("requestSetup", "'host:s:My PC'"):                                             {"Result: Bundle[{ok=true}]\n"},
		"shell am start -n tech.hybriq.adbuddy/.MainActivity -a tech.hybriq.adbuddy.action.SETUP": {"Starting: Intent\n"},
		consentCall("getSetupConsent"):                                                            outs,
	}
}

func TestAskConsent(t *testing.T) {
	f := &fakeADB{answers: consentAnswers("pending", "pending", "granted")}
	if err := AskConsent(context.Background(), f, verified(f), "My PC", time.Second, time.Millisecond); err != nil {
		t.Fatal(err, f.calls)
	}
	f = &fakeADB{answers: consentAnswers("pending", "denied")}
	if err := AskConsent(context.Background(), f, verified(f), "My PC", time.Second, time.Millisecond); !errors.Is(err, ErrConsentDenied) {
		t.Fatalf("denied: %v", err)
	}
	f = &fakeADB{answers: consentAnswers("pending")}
	if err := AskConsent(context.Background(), f, verified(f), "My PC", 20*time.Millisecond, time.Millisecond); !errors.Is(err, ErrConsentTimeout) {
		t.Fatalf("timeout: %v", err)
	}
}

func TestAskConsentCleansTheHostName(t *testing.T) {
	f := &fakeADB{answers: consentAnswers("granted")}
	if err := AskConsent(context.Background(), f, verified(f), "My PC\n", time.Second, time.Millisecond); err != nil {
		t.Fatal(err, f.calls)
	}
	f = &fakeADB{answers: consentAnswers("granted")}
	f.answers[consentCall("requestSetup", "'host:s:My PC reboot'")] = []string{"Result: Bundle[{ok=true}]\n"}
	if err := AskConsent(context.Background(), f, verified(f), "My PC; reboot", time.Second, time.Millisecond); err != nil {
		t.Fatal(err, f.calls)
	}
}

func TestAskConsentRejectsUnknownAnswers(t *testing.T) {
	for _, c := range []string{"maybe", "GRANTED", ""} {
		f := &fakeADB{answers: consentAnswers(c)}
		err := within(t, 5*time.Second, func() error {
			return AskConsent(context.Background(), f, verified(f), "My PC", time.Hour, time.Hour)
		})
		if err == nil || errors.Is(err, ErrConsentTimeout) || errors.Is(err, ErrConsentDenied) {
			t.Errorf("%q: %v", c, err)
		}
	}
}

func TestCommandsHaveATimeLimit(t *testing.T) {
	set(t, &cmdTimeout, 20*time.Millisecond)
	f := &fakeADB{hangAll: true}
	err := within(t, 5*time.Second, func() error {
		_, err := Call(context.Background(), f, "ping")
		return err
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call: %v", err)
	}
	f = &fakeADB{hangAll: true}
	err = within(t, 5*time.Second, func() error {
		return Handshake(context.Background(), f, testToken, 47123, "My PC")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Handshake: %v", err)
	}
	set(t, &cmdTimeout, time.Hour)
	set(t, &installTimeout, 20*time.Millisecond)
	f = &fakeADB{hangAll: true}
	err = within(t, 5*time.Second, func() error {
		return Install(context.Background(), f, "/tmp/a.apk")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Install: %v", err)
	}
}

func TestEachCommandHasItsOwnDeadline(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{
		"shell pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS": {""},
		"install -r --no-incremental /tmp/a.apk":                                      {"Success\n"},
	}}
	if err := Grant(context.Background(), f, verified(f)); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err != nil {
		t.Fatal(err)
	}
	if l := f.left[2]; l <= 14*time.Second || l > 15*time.Second {
		t.Errorf("Grant: %v left", l)
	}
	if l := f.left[3]; l <= 2*time.Minute || l > 3*time.Minute {
		t.Errorf("Install: %v left", l)
	}
}

func TestHandshakeHasATimeLimit(t *testing.T) {
	set(t, &cmdTimeout, time.Hour)
	set(t, &handshakeTimeout, 30*time.Millisecond)
	// The write goes through, but ping hangs.
	f := &fakeADB{answers: map[string][]string{writeKey: {""}}, hang: map[string]bool{pingKey: true}}
	err := within(t, 5*time.Second, func() error {
		return Handshake(context.Background(), f, testToken, 47123, "My PC")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hanging ping: %v", err)
	}
	// The app answers, never with this handshake, and the pings are slow.
	set(t, &pingEvery, time.Hour)
	f = &fakeADB{answers: map[string][]string{writeKey: {""}, pingKey: {"Result: Bundle[{ok=true, port=0}]\n"}}}
	err = within(t, 5*time.Second, func() error {
		return Handshake(context.Background(), f, testToken, 47123, "My PC")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("slow pings: %v", err)
	}
}

func TestAskConsentHasATimeLimit(t *testing.T) {
	set(t, &cmdTimeout, time.Hour)
	set(t, &consentGrace, 30*time.Millisecond)
	f := &fakeADB{answers: consentAnswers("pending"), hang: map[string]bool{consentCall("getSetupConsent"): true}}
	err := within(t, 5*time.Second, func() error {
		return AskConsent(context.Background(), f, verified(f), "My PC", 10*time.Millisecond, time.Millisecond)
	})
	if err == nil {
		t.Fatal("a hanging content call was waited for forever")
	}
	f = &fakeADB{hangAll: true}
	err = within(t, 5*time.Second, func() error {
		return AskConsent(context.Background(), f, verified(f), "My PC", 10*time.Millisecond, time.Millisecond)
	})
	if err == nil {
		t.Fatal("a hanging requestSetup was waited for forever")
	}
}

func TestA11yEnabled(t *testing.T) {
	long := "tech.hybriq.adbuddy/tech.hybriq.adbuddy.a11y.AgentAccessibilityService"
	short := "tech.hybriq.adbuddy/.a11y.AgentAccessibilityService"
	for list, want := range map[string]bool{
		"":                                 false,
		"null":                             false,
		long:                               true,
		short:                              true,
		strings.ToUpper(short):             true,
		"com.x/.Svc:" + short:              true,
		long + ":com.x/.Svc":               true,
		"com.x/.Svc":                       false,
		"tech.hybriq.adbuddy/.a11y":        false,
		"x" + short:                        false,
		short + "x":                        false,
		"com.x/.AgentAccessibilityService": false,
	} {
		if got := A11yEnabled(list); got != want {
			t.Errorf("%q: %v", list, got)
		}
	}
	if A11yService != long {
		t.Fatalf("A11yService %q", A11yService)
	}
}

func TestEnableAccessibilityKeepsOtherServices(t *testing.T) {
	get := "shell settings get secure enabled_accessibility_services"
	on := "shell settings put secure accessibility_enabled 1"
	short := "tech.hybriq.adbuddy/.a11y.AgentAccessibilityService"
	cases := map[string]string{
		"null":                      A11yService,
		"":                          A11yService,
		"com.x/.Svc":                "com.x/.Svc:" + A11yService,
		"com.x/.Svc:" + A11yService: "",
		"com.x/.Svc:" + short:       "",
		strings.ToUpper(short):      "",
	}
	for prev, want := range cases {
		put := "shell settings put secure enabled_accessibility_services " + want
		f := &fakeADB{answers: map[string][]string{get: {prev + "\n"}, put: {""}, on: {""}}}
		got, err := EnableAccessibility(context.Background(), f, verified(f))
		if err != nil || got != prev {
			t.Errorf("%q: %q %v %v", prev, got, err, f.calls)
		}
		wrote := false
		for _, c := range f.calls {
			wrote = wrote || strings.HasPrefix(c, "shell settings put secure enabled_accessibility_services")
		}
		if wrote != (want != "") {
			t.Errorf("%q: wrote=%v calls=%v", prev, wrote, f.calls)
		}
		if f.calls[len(f.calls)-1] != on {
			t.Errorf("%q: accessibility not turned on: %v", prev, f.calls)
		}
	}
	if got := MergeServices("com.x/.Svc:"+short, A11yService); got != "com.x/.Svc:"+short {
		t.Errorf("short form duplicated: %q", got)
	}
}

func TestAllowBackground(t *testing.T) {
	key := "shell dumpsys deviceidle whitelist +tech.hybriq.adbuddy"
	f := &fakeADB{answers: map[string][]string{key: {"Added: tech.hybriq.adbuddy\n"}}}
	if err := AllowBackground(context.Background(), f, verified(f)); err != nil {
		t.Fatal(err, f.calls)
	}
	f.answers[key] = []string{"Unknown package: tech.hybriq.adbuddy\n"}
	if err := AllowBackground(context.Background(), f, verified(f)); err == nil {
		t.Fatal("an exemption that was not added was accepted")
	}
}

func TestGrant(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"shell pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS": {""}}}
	if err := Grant(context.Background(), f, verified(f)); err != nil {
		t.Fatal(err, f.calls)
	}
	if len(f.calls) != 3 || !strings.HasPrefix(f.calls[2], "shell pm grant ") {
		t.Fatalf("calls %v", f.calls)
	}
	f = &fakeADB{answers: map[string][]string{}}
	if err := Grant(context.Background(), f, verified(f)); err == nil {
		t.Fatal("a failed grant was accepted")
	}
}

// verified makes f answer pm path and stat like a phone with the app at
// testPath, and returns the proof the functions that give the app something
// take (the same app, as Verified.still sees it).
func verified(f *fakeADB) Verified {
	if f.answers == nil {
		f.answers = map[string][]string{}
	}
	f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + testPath + "\n"}
	f.answers["shell "+shq.Join("stat", "-c", "%s %i %Y", testPath)] = []string{"100 1 1\n"}
	return Verified{path: testPath, key: "100 1 1", cert: "test"}
}

// Nothing is given to an app that is not the one checked.
func TestGivingNeedsTheCheckedApp(t *testing.T) {
	f := &fakeADB{}
	v := verified(f)
	pmPath := "shell pm path tech.hybriq.adbuddy"
	stat := "shell " + shq.Join("stat", "-c", "%s %i %Y", testPath)
	check := func(label string, v Verified) {
		t.Helper()
		if err := Grant(context.Background(), f, v); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: Grant: %v", label, err)
		}
		if err := AllowBackground(context.Background(), f, v); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: AllowBackground: %v", label, err)
		}
		if _, err := EnableAccessibility(context.Background(), f, v); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: EnableAccessibility: %v", label, err)
		}
		if err := AskConsent(context.Background(), f, v, "My PC", time.Second, time.Millisecond); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: AskConsent: %v", label, err)
		}
	}
	check("zero Verified", Verified{})
	if len(f.calls) != 0 {
		t.Fatalf("a zero Verified ran %q", f.calls)
	}
	f.answers[pmPath] = []string{"package:/data/app/~~New==/tech.hybriq.adbuddy-New==/base.apk\n"}
	check("reinstalled", v)
	f.answers[pmPath] = []string{"package:" + testPath + "\n"}
	f.answers[stat] = []string{"999 1 1\n"}
	check("updated in place", v)
	for _, c := range f.calls {
		if c != pmPath && c != stat {
			t.Fatalf("ran %q", c)
		}
	}
}

func TestTunnel(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{
		"reverse tcp:47123 tcp:47123": {""},
		"reverse --list":              {"UsbFfs tcp:47123 tcp:47123\n", "\n"},
	}}
	if err := Tunnel(context.Background(), f, 47123); err != nil {
		t.Fatal(err)
	}
	if ok, _ := TunnelOpen(context.Background(), f, 47123); !ok {
		t.Fatal("open tunnel not seen")
	}
	if ok, _ := TunnelOpen(context.Background(), f, 47123); ok {
		t.Fatal("closed tunnel seen as open")
	}
}

func TestInstall(t *testing.T) {
	key := "install -r --no-incremental /tmp/a.apk"
	f := &fakeADB{answers: map[string][]string{key: {"Performing Streamed Install\nSuccess\n"}}}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err != nil {
		t.Fatal(err)
	}
	f.answers[key] = []string{"Failure [INSTALL_FAILED_OLDER_SDK]\n"}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err == nil || errors.Is(err, ErrOtherSigner) {
		t.Fatalf("older sdk: %v", err)
	}
	f.answers[key] = []string{"Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package tech.hybriq.adbuddy signatures do not match newer version; ignoring!]\n"}
	if err := Install(context.Background(), f, "/tmp/a.apk"); !errors.Is(err, ErrOtherSigner) || !strings.Contains(err.Error(), "signatures do not match") {
		t.Fatalf("other signer: %v", err)
	}
}

// releaseServer serves apk and a .sha256 sidecar (sidecar() decides its
// text), and counts the requests.
func releaseServer(t *testing.T, apk []byte, sidecar func() string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/adbuddy.apk":
			w.Write(apk)
		case "/adbuddy.apk.sha256":
			io.WriteString(w, sidecar())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func sumOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestDownloadChecksTheSHA256(t *testing.T) {
	apk := signedBy(ours)
	sidecar := sumOf(apk) + "  adbuddy.apk\n"
	srv, _ := releaseServer(t, apk, func() string { return sidecar })
	dir := t.TempDir()
	p, err := Download(context.Background(), srv.Client(), srv.URL+"/", dir, apktest.SHA256(ours))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, apk) || p != filepath.Join(dir, "adbuddy.apk") {
		t.Fatalf("%s %d bytes", p, len(b))
	}
	sidecar = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", t.TempDir(), apktest.SHA256(ours)); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad sum: %v", err)
	}
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/none/", t.TempDir(), apktest.SHA256(ours)); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("no release: %v", err)
	}
}

// An APK whose checksum matches but that someone else signed is not saved.
func TestDownloadChecksTheSigner(t *testing.T) {
	apk := signedBy(other)
	srv, _ := releaseServer(t, apk, func() string { return sumOf(apk) + "  adbuddy.apk\n" })
	dir := t.TempDir()
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", dir, apktest.SHA256(ours)); !errors.Is(err, ErrSigner) {
		t.Fatalf("got %v, want ErrSigner", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "adbuddy.apk")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the APK was saved: %v", err)
	}
}

// Without a pinned certificate nothing is downloaded at all.
func TestDownloadNeedsAPinnedCertificate(t *testing.T) {
	srv, hits := releaseServer(t, []byte("x"), func() string { return "" })
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", t.TempDir(), ""); !errors.Is(err, ErrUnpinned) {
		t.Fatalf("got %v, want ErrUnpinned", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d requests without a pin", n)
	}
}

func TestDownloadHasATimeLimit(t *testing.T) {
	set(t, &downloadTimeout, 30*time.Millisecond)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	for _, client := range []*http.Client{nil, srv.Client()} {
		err := within(t, 5*time.Second, func() error {
			_, err := Download(context.Background(), client, srv.URL+"/", t.TempDir(), strings.Repeat("a", 64))
			return err
		})
		if err == nil {
			t.Fatalf("client %v: no error", client)
		}
	}
}

func TestHostName(t *testing.T) {
	for in, want := range map[string]string{
		"DESKTOP-AB12":                        "DESKTOP-AB12",
		"pc; reboot":                          "pc reboot",
		"$(x)":                                "x",
		"":                                    "PC",
		"ñ":                                   "PC",
		"  My PC  ":                           "My PC",
		strings.Repeat("x", 50):               strings.Repeat("x", 40),
		strings.Repeat("ñx", 45):              strings.Repeat("x", 40),
		strings.Repeat("a", 39) + " b":        strings.Repeat("a", 39),
		strings.Repeat("a", 38) + "  b":       strings.Repeat("a", 38),
		" " + strings.Repeat("a", 39) + "bcd": strings.Repeat("a", 39),
	} {
		if got := HostName(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}
