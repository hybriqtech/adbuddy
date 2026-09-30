package phoneapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeADB struct {
	answers map[string][]string // joined args -> successive stdouts (the last one repeats)
	calls   []string
	stdin   string // what the last call read from stdin
}

func (f *fakeADB) Run(_ context.Context, _ string, args []string, stdin io.Reader, stdout, _ io.Writer) (int, error) {
	key := strings.Join(args, " ")
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		f.stdin = string(b)
	}
	f.calls = append(f.calls, key)
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

func TestInstalledVersion(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"shell dumpsys package tech.hybriq.adbuddy": {"Packages:\r\n  Package [tech.hybriq.adbuddy] (1a2b):\r\n    versionCode=3 minSdk=30\r\n    versionName=0.2.0\r\n"}}}
	v, err := InstalledVersion(context.Background(), f)
	if err != nil || v != "0.2.0" {
		t.Fatalf("%q %v", v, err)
	}
	f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{"Unable to find package: tech.hybriq.adbuddy\n"}
	if v, _ := InstalledVersion(context.Background(), f); v != "" {
		t.Fatalf("not installed: %q", v)
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

func TestHandshakeSendsTheTokenOnStdinOnly(t *testing.T) {
	ping := "shell content call --uri content://tech.hybriq.adbuddy.bridge --method ping"
	f := &fakeADB{answers: map[string][]string{
		"shell content write --uri content://tech.hybriq.adbuddy.bridge/handshake": {""},
		ping: {"Result: Bundle[{ok=true, port=0}]\n", "Result: Bundle[{ok=true, port=47123}]\n"},
	}}
	pingEvery = time.Millisecond
	if err := Handshake(context.Background(), f, "s3cret", 47123, "My PC"); err != nil {
		t.Fatal(err, f.calls)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "s3cret") {
			t.Fatalf("the token is in a command line: %q", c)
		}
	}
	if !strings.Contains(f.stdin, `"token":"s3cret"`) || !strings.Contains(f.stdin, `"port":47123`) {
		t.Fatalf("stdin %q", f.stdin)
	}
	f.answers[ping] = []string{"Result: Bundle[{ok=true, port=1}]\n"}
	if err := Handshake(context.Background(), f, "s3cret", 47123, "My PC"); err == nil {
		t.Fatal("a handshake the app did not take was accepted")
	}
}

func TestAskConsent(t *testing.T) {
	call := func(m string, extra ...string) string {
		s := "shell content call --uri content://tech.hybriq.adbuddy.bridge --method " + m
		for _, e := range extra {
			s += " --extra " + e
		}
		return s
	}
	answers := func(consents ...string) map[string][]string {
		var outs []string
		for _, c := range consents {
			outs = append(outs, "Result: Bundle[{consent="+c+"}]\n")
		}
		return map[string][]string{
			call("requestSetup", "'host:s:My PC'"):                                                    {"Result: Bundle[{ok=true}]\n"},
			"shell am start -n tech.hybriq.adbuddy/.MainActivity -a tech.hybriq.adbuddy.action.SETUP": {"Starting: Intent\n"},
			call("getSetupConsent"):                                                                   outs,
		}
	}
	f := &fakeADB{answers: answers("pending", "pending", "granted")}
	if err := AskConsent(context.Background(), f, "My PC", time.Second, time.Millisecond); err != nil {
		t.Fatal(err, f.calls)
	}
	f = &fakeADB{answers: answers("pending", "denied")}
	if err := AskConsent(context.Background(), f, "My PC", time.Second, time.Millisecond); !errors.Is(err, ErrConsentDenied) {
		t.Fatalf("denied: %v", err)
	}
	f = &fakeADB{answers: answers("pending")}
	if err := AskConsent(context.Background(), f, "My PC", 20*time.Millisecond, time.Millisecond); !errors.Is(err, ErrConsentTimeout) {
		t.Fatalf("timeout: %v", err)
	}
}

func TestEnableAccessibilityKeepsOtherServices(t *testing.T) {
	get := "shell settings get secure enabled_accessibility_services"
	on := "shell settings put secure accessibility_enabled 1"
	cases := map[string]string{
		"null":                      A11yService,
		"":                          A11yService,
		"com.x/.Svc":                "com.x/.Svc:" + A11yService,
		"com.x/.Svc:" + A11yService: "",
	}
	for prev, want := range cases {
		put := "shell settings put secure enabled_accessibility_services " + want
		f := &fakeADB{answers: map[string][]string{get: {prev + "\n"}, put: {""}, on: {""}}}
		got, err := EnableAccessibility(context.Background(), f)
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
}

func TestAllowBackground(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"shell dumpsys deviceidle whitelist +tech.hybriq.adbuddy": {"Added: tech.hybriq.adbuddy\n"}}}
	if err := AllowBackground(context.Background(), f); err != nil {
		t.Fatal(err, f.calls)
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
	f := &fakeADB{answers: map[string][]string{"install -r /tmp/a.apk": {"Performing Streamed Install\nSuccess\n"}}}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err != nil {
		t.Fatal(err)
	}
	f.answers["install -r /tmp/a.apk"] = []string{"Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE]\n"}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err == nil {
		t.Fatal("failure accepted")
	}
}

func TestDownloadChecksTheSHA256(t *testing.T) {
	apk := []byte("PK fake apk")
	sum := sha256.Sum256(apk)
	good := hex.EncodeToString(sum[:])
	var sidecar string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/adbuddy.apk":
			w.Write(apk)
		case "/adbuddy.apk.sha256":
			io.WriteString(w, sidecar)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	sidecar = good + "  adbuddy.apk\n"
	p, err := Download(context.Background(), srv.Client(), srv.URL+"/", dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != string(apk) || p != filepath.Join(dir, "adbuddy.apk") {
		t.Fatalf("%s %q", p, b)
	}
	sidecar = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", t.TempDir()); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad sum: %v", err)
	}
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/none/", t.TempDir()); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("no release: %v", err)
	}
}

func TestHostName(t *testing.T) {
	for in, want := range map[string]string{"DESKTOP-AB12": "DESKTOP-AB12", "pc; reboot": "pc reboot", "$(x)": "x", "": "PC", "ñ": "PC"} {
		if got := HostName(in); got != want {
			t.Errorf("%q: %q want %q", in, got, want)
		}
	}
}
