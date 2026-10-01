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
	"crypto/rand"
	"encoding/hex"
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

// Time limits. Every adb command gets its own, derived from the caller's
// context, so a phone that stops answering cannot hang setup or serve.
// Variables so the tests can shorten them.
var (
	cmdTimeout       = 15 * time.Second // any adb command but install
	installTimeout   = 3 * time.Minute  // adb install
	handshakeTimeout = 15 * time.Second // the whole Handshake: write and pings
	consentGrace     = 15 * time.Second // AskConsent: at most timeout + this
	downloadTimeout  = 5 * time.Minute  // Download without a deadline or client
)

// ADB runs adb against the active phone (an adb.Runner with Serial set).
type ADB interface {
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// runFor runs one adb command with at most d to finish.
func runFor(ctx context.Context, a ADB, d time.Duration, stdin io.Reader, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", args, stdin, &out, &errb)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil && !errors.Is(err, cerr) {
			return "", fmt.Errorf("adb %s: %w (%v)", args[0], cerr, err)
		}
		return "", fmt.Errorf("adb %s: %w", args[0], err)
	}
	text := strings.ReplaceAll(out.String(), "\r", "")
	if code != 0 {
		if cerr := ctx.Err(); cerr != nil {
			return text, fmt.Errorf("adb %s: %w", args[0], cerr)
		}
		return text, fmt.Errorf("adb %s: exit %d: %s", args[0], code, strings.TrimSpace(errb.String()+" "+text))
	}
	return text, nil
}

func run(ctx context.Context, a ADB, args ...string) (string, error) {
	return runFor(ctx, a, cmdTimeout, nil, args...)
}

func shell(ctx context.Context, a ADB, words ...string) (string, error) {
	out, err := run(ctx, a, "shell", shq.Join(words...))
	return strings.TrimSpace(out), err
}

// InstalledVersion returns the app's versionName, or "" when it is not
// installed for user 0 (after `pm uninstall -k --user 0` dumpsys still lists
// the package, with "User 0: … installed=false").
func InstalledVersion(ctx context.Context, a ADB) (string, error) {
	out, err := shell(ctx, a, "dumpsys", "package", Package)
	if err != nil {
		return "", err
	}
	version := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "versionName="); ok && version == "" {
			version = v
		}
		if strings.HasPrefix(line, "User 0:") && strings.Contains(line, " installed=false") {
			return "", nil
		}
	}
	return version, nil
}

// ErrOtherSigner means the app on the phone was signed with another key (a
// development build, say), so Android refuses to update it.
var ErrOtherSigner = errors.New("the app on the phone was signed with another key")

// Install installs (or updates, keeping data) the APK at path. Never
// incrementally: adb would keep reading the file on the PC after the host
// checked it.
func Install(ctx context.Context, a ADB, path string) error {
	out, err := runFor(ctx, a, installTimeout, nil, "install", "-r", "--no-incremental", path)
	if err == nil && strings.Contains(out, "Success") {
		return nil
	}
	msg := strings.TrimSpace(out)
	if err != nil {
		msg = err.Error()
	}
	if strings.Contains(msg, "INSTALL_FAILED_UPDATE_INCOMPATIBLE") {
		if err != nil {
			return fmt.Errorf("%w: %w", ErrOtherSigner, err)
		}
		return fmt.Errorf("%w: %s", ErrOtherSigner, msg)
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("adb install: %s", msg)
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

// AskConsent opens the consent screen (spec §3.1 step 6.1) and waits for the
// answer. It takes at most timeout + consentGrace, even if adb hangs. v is
// the proof that the app is ours (Verifier.Verify): nothing is asked of an
// app that is not the one checked.
func AskConsent(ctx context.Context, a ADB, v Verified, hostName string, timeout, every time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout+consentGrace)
	defer cancel()
	if err := v.still(ctx, a); err != nil {
		return err
	}
	if _, err := Call(ctx, a, "requestSetup", "host:s:"+HostName(hostName)); err != nil {
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
		switch c := Consent(res["consent"]); c {
		case ConsentGranted:
			return nil
		case ConsentDenied:
			return ErrConsentDenied
		case ConsentPending:
		default:
			return fmt.Errorf("unexpected consent from the app: %q", c)
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

// Grant gives the app WRITE_SECURE_SETTINGS (for "Cut access"), once v
// still holds.
func Grant(ctx context.Context, a ADB, v Verified) error {
	if err := v.still(ctx, a); err != nil {
		return err
	}
	_, err := shell(ctx, a, "pm", "grant", Package, SecureWrites)
	return err
}

// AllowBackground exempts the app from battery optimisation, so it may start
// its foreground service when the host hands over the token and keep the
// channel open while the screen is off (Android 12+ restricts both), once v
// still holds.
func AllowBackground(ctx context.Context, a ADB, v Verified) error {
	if err := v.still(ctx, a); err != nil {
		return err
	}
	out, err := shell(ctx, a, "dumpsys", "deviceidle", "whitelist", "+"+Package)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "Added:") {
		return fmt.Errorf("dumpsys deviceidle: %s", out)
	}
	return nil
}

// EnableAccessibility adds the app's service to the enabled ones without
// removing any (spec §3.1 step 6.2) and turns accessibility on, once v still
// holds. It returns the previous list so the caller can report it.
func EnableAccessibility(ctx context.Context, a ADB, v Verified) (previous string, err error) {
	if err := v.still(ctx, a); err != nil {
		return "", err
	}
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

// A11yEnabled reports whether the app's accessibility service is in list
// (the value of enabled_accessibility_services), in its long form
// (pkg/pkg.a11y.X) or its short one (pkg/.a11y.X), ignoring case.
func A11yEnabled(list string) bool {
	return hasComponent(list, A11yService)
}

// hasComponent reports whether the colon-separated list holds svc, in either
// form, ignoring case.
func hasComponent(list, svc string) bool {
	want := longComponent(svc)
	for _, s := range strings.Split(list, ":") {
		if strings.EqualFold(longComponent(strings.TrimSpace(s)), want) {
			return true
		}
	}
	return false
}

// longComponent turns "pkg/.Cls" into "pkg/pkg.Cls"; anything else is kept.
func longComponent(c string) string {
	if pkg, cls, ok := strings.Cut(c, "/"); ok && strings.HasPrefix(cls, ".") {
		return pkg + "/" + pkg + cls
	}
	return c
}

// MergeServices adds svc to a colon-separated list of components ("null"
// or empty = none) unless it is already there, in either form. For the app's
// service that is A11yEnabled(list).
func MergeServices(list, svc string) string {
	if list == "" || list == "null" {
		return svc
	}
	if hasComponent(list, svc) {
		return list
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

// tokenRe is the shape of the host's token: 32 random bytes in hex.
var tokenRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Handshake hands the token and port to the app (spec §5.2). They travel on
// stdin (`content write`), never in a command line: on the PC and on the
// phone other processes, the agent's included, can read command lines, and
// with the token the agent could answer its own confirmations.
//
// Each handshake carries a new nonce, and it only counts once ping returns
// that nonce and the port: after the first setup ping always had the right
// port, even when the app had not stored the new token. It takes at most
// handshakeTimeout.
func Handshake(ctx context.Context, a ADB, token string, port int, hostName string) error {
	if !tokenRe.MatchString(token) {
		return errors.New("handshake: the token is not 64 lowercase hex digits")
	}
	if port < 1024 || port > 65535 {
		return fmt.Errorf("handshake: port %d out of range", port)
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}
	nonce := hex.EncodeToString(raw[:])
	body, err := json.Marshal(map[string]any{"token": token, "port": port, "host": HostName(hostName), "nonce": nonce})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	cmd := shq.Join("content", "write", "--uri", "content://"+Authority+"/handshake")
	if _, err := runFor(ctx, a, cmdTimeout, bytes.NewReader(body), "shell", cmd); err != nil {
		return fmt.Errorf("the app refused the handshake: %w", err)
	}
	// Whether the app took it is checked with ping: `content write` does not
	// always report errors in its exit code.
	// The app stores the handshake on its own thread once the pipe closes:
	// give it a moment before deciding it did not take it.
	var got string
	fresh := false
	for i := 0; i < pingTries; i++ {
		res, err := Call(ctx, a, "ping")
		if err != nil {
			return err
		}
		got, fresh = res["port"], res["nonce"] == nonce
		if got == strconv.Itoa(port) && fresh {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pingEvery):
		}
	}
	return fmt.Errorf("the app did not take the handshake (port %q, this handshake's nonce: %v)", got, fresh)
}

// How long Handshake waits for the app to take the token.
var (
	pingTries = 10
	pingEvery = 200 * time.Millisecond
)
