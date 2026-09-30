// Package phoneapp manages the ADBuddy app on the phone from the host (spec
// §3.1 steps 5-7): install it with `adb install`, ask for consent in the app,
// grant WRITE_SECURE_SETTINGS, enable the accessibility service without
// removing the ones already on, open the tunnel (`adb reverse`) and hand over
// the token. These commands are the host's own: they never go through the
// shim, and the agent's attempts to do the same are blocked (package classify).
package phoneapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// Identifiers shared with the app (android/): keep them in sync.
const (
	Package      = "tech.hybriq.adbuddy"
	Authority    = "tech.hybriq.adbuddy.bridge"
	Activity     = Package + "/.MainActivity"
	SetupAction  = "tech.hybriq.adbuddy.action.SETUP"
	A11yService  = Package + "/" + Package + ".a11y.AgentAccessibilityService"
	SecureWrites = "android.permission.WRITE_SECURE_SETTINGS"
)

// ADB runs adb against the active phone (an adb.Runner with Serial set).
type ADB interface {
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

func run(ctx context.Context, a ADB, args ...string) (string, error) {
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", args, nil, &out, &errb)
	if err != nil {
		return "", err
	}
	text := strings.ReplaceAll(out.String(), "\r", "")
	if code != 0 {
		return text, fmt.Errorf("adb %s: exit %d: %s", args[0], code, strings.TrimSpace(errb.String()+" "+text))
	}
	return text, nil
}

func shell(ctx context.Context, a ADB, words ...string) (string, error) {
	out, err := run(ctx, a, "shell", shq.Join(words...))
	return strings.TrimSpace(out), err
}

// InstalledVersion returns the app's versionName, or "" when it is not installed.
func InstalledVersion(ctx context.Context, a ADB) (string, error) {
	out, err := shell(ctx, a, "dumpsys", "package", Package)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "versionName="); ok {
			return v, nil
		}
	}
	return "", nil
}

// Install installs (or updates, keeping data) the APK at path.
func Install(ctx context.Context, a ADB, path string) error {
	out, err := run(ctx, a, "install", "-r", path)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "Success") {
		return fmt.Errorf("adb install: %s", strings.TrimSpace(out))
	}
	return nil
}

// bundleRe reads `content call` output: "Result: Bundle[{a=1, b=x}]".
var bundleRe = regexp.MustCompile(`Bundle\[\{(.*)\}\]`)

// Call calls a method of the app's provider and returns the result bundle.
// Only the shell user (adb) may call it; the app refuses anyone else.
func Call(ctx context.Context, a ADB, method string, extras ...string) (map[string]string, error) {
	words := []string{"content", "call", "--uri", "content://" + Authority, "--method", method}
	for _, e := range extras {
		words = append(words, "--extra", e)
	}
	out, err := shell(ctx, a, words...)
	if err != nil {
		return nil, err
	}
	m := bundleRe.FindStringSubmatch(out)
	if m == nil {
		return nil, fmt.Errorf("the app did not answer %s: %s", method, out)
	}
	res := map[string]string{}
	for _, kv := range strings.Split(m[1], ", ") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			res[k] = v
		}
	}
	if e := res["error"]; e != "" {
		return res, fmt.Errorf("the app refused %s: %s", method, e)
	}
	return res, nil
}

// Consent is the answer of the consent screen.
type Consent string

const (
	ConsentPending Consent = "pending"
	ConsentGranted Consent = "granted"
	ConsentDenied  Consent = "denied"
)

// ErrConsentDenied means the person said no in the app.
var ErrConsentDenied = errors.New("setup was refused in the app")

// ErrConsentTimeout means nobody answered in time.
var ErrConsentTimeout = errors.New("no answer in the app")

// AskConsent opens the consent screen (spec §3.1 step 6.1) and waits for the answer.
func AskConsent(ctx context.Context, a ADB, hostName string, timeout, every time.Duration) error {
	if _, err := Call(ctx, a, "requestSetup", "host:s:"+hostName); err != nil {
		return err
	}
	if _, err := shell(ctx, a, "am", "start", "-n", Activity, "-a", SetupAction); err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		res, err := Call(ctx, a, "getSetupConsent")
		if err != nil {
			return err
		}
		switch Consent(res["consent"]) {
		case ConsentGranted:
			return nil
		case ConsentDenied:
			return ErrConsentDenied
		}
		if time.Now().After(deadline) {
			return ErrConsentTimeout
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
}

// Grant gives the app WRITE_SECURE_SETTINGS (for "Cut access").
func Grant(ctx context.Context, a ADB) error {
	_, err := shell(ctx, a, "pm", "grant", Package, SecureWrites)
	return err
}

// AllowBackground exempts the app from battery optimisation, so it may start
// its foreground service when the host hands over the token and keep the
// channel open while the screen is off (Android 12+ restricts both).
func AllowBackground(ctx context.Context, a ADB) error {
	_, err := shell(ctx, a, "dumpsys", "deviceidle", "whitelist", "+"+Package)
	return err
}

// EnableAccessibility adds the app's service to the enabled ones without
// removing any (spec §3.1 step 6.2) and turns accessibility on. It returns
// the previous list so the caller can report it.
func EnableAccessibility(ctx context.Context, a ADB) (previous string, err error) {
	previous, err = shell(ctx, a, "settings", "get", "secure", "enabled_accessibility_services")
	if err != nil {
		return "", err
	}
	list := MergeServices(previous, A11yService)
	if list != previous {
		if _, err := shell(ctx, a, "settings", "put", "secure", "enabled_accessibility_services", list); err != nil {
			return previous, err
		}
	}
	_, err = shell(ctx, a, "settings", "put", "secure", "accessibility_enabled", "1")
	return previous, err
}

// MergeServices adds svc to a colon-separated list of components ("null"
// or empty = none) unless it is already there.
func MergeServices(list, svc string) string {
	if list == "" || list == "null" {
		return svc
	}
	for _, s := range strings.Split(list, ":") {
		if strings.EqualFold(s, svc) {
			return list
		}
	}
	return list + ":" + svc
}

// Tunnel makes 127.0.0.1:port on the phone reach 127.0.0.1:port on the PC.
func Tunnel(ctx context.Context, a ADB, port int) error {
	p := "tcp:" + strconv.Itoa(port)
	_, err := run(ctx, a, "reverse", p, p)
	return err
}

// TunnelOpen reports whether the reverse for port is in place.
func TunnelOpen(ctx context.Context, a ADB, port int) (bool, error) {
	out, err := run(ctx, a, "reverse", "--list")
	if err != nil {
		return false, err
	}
	want := "tcp:" + strconv.Itoa(port) + " tcp:" + strconv.Itoa(port)
	for _, line := range strings.Split(out, "\n") {
		if strings.HasSuffix(strings.TrimSpace(line), want) {
			return true, nil
		}
	}
	return false, nil
}

// Handshake hands the token and port to the app (spec §5.2). They travel on
// stdin (`content write`), never in a command line: on the PC and on the
// phone other processes, the agent's included, can read command lines, and
// with the token the agent could answer its own confirmations.
func Handshake(ctx context.Context, a ADB, token string, port int, hostName string) error {
	body, err := json.Marshal(map[string]any{"token": token, "port": port, "host": hostName})
	if err != nil {
		return err
	}
	var out, errb bytes.Buffer
	cmd := shq.Join("content", "write", "--uri", "content://"+Authority+"/handshake")
	code, err := a.Run(ctx, "", []string{"shell", cmd}, bytes.NewReader(body), &out, &errb)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the app refused the handshake: %s", strings.TrimSpace(errb.String()))
	}
	// Whether the app took it is checked with ping: `content write` does not
	// always report errors in its exit code.
	// The app stores the handshake on its own thread once the pipe closes:
	// give it a moment before deciding it did not take it.
	var got string
	for i := 0; i < pingTries; i++ {
		res, err := Call(ctx, a, "ping")
		if err != nil {
			return err
		}
		if got = res["port"]; got == strconv.Itoa(port) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pingEvery):
		}
	}
	return fmt.Errorf("the app did not take the handshake (port %q)", got)
}

// How long Handshake waits for the app to take the token.
var (
	pingTries = 10
	pingEvery = 200 * time.Millisecond
)
