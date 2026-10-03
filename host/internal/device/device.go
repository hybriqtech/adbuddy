// Package device reads the phone's profile (spec §3.1 step 8) and applies
// the host invariant on connection (spec §3.3). Every command here only
// reads, except `adb usb` in EnsureNoTCPPort.
package device

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// ADB runs adb against the active phone (implemented by adb.Runner with Serial set).
type ADB interface {
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

func shell(ctx context.Context, a ADB, cmd string) (string, error) {
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"shell", cmd}, nil, &out, &errb)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("adb shell %s: exit %d: %s", cmd, code, classify.Printable(strings.TrimSpace(errb.String())))
	}
	return strings.TrimSpace(strings.ReplaceAll(out.String(), "\r", "")), nil
}

// Profile describes the phone for the agent.
type Profile struct {
	Name         string    `json:"name"` // folder name: model in lower case with dashes
	Model        string    `json:"model"`
	Manufacturer string    `json:"manufacturer"`
	Brand        string    `json:"brand"`
	Android      string    `json:"android"`
	SDK          int       `json:"sdk"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	Density      int       `json:"density"`
	Launcher     string    `json:"launcher"`
	Locale       string    `json:"locale"`
	Google       bool      `json:"google"` // a Google phone (Pixel) or another maker's
	CollectedAt  time.Time `json:"collectedAt"`
}

var (
	sizeRe    = regexp.MustCompile(`(Physical|Override) size: (\d+)x(\d+)`)
	densityRe = regexp.MustCompile(`(Physical|Override) density: (\d+)`)
	slugRe    = regexp.MustCompile(`[^a-z0-9]+`)
	// activityRe is a component as `resolve-activity --brief` prints it.
	activityRe = regexp.MustCompile(`^[\w.]+/[\w.$]+$`)
)

// maxRunes bounds every value from the phone in AGENTS.md and the folder name.
const maxRunes = 120

func truncate(s string) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes]) + "…"
}

// screenNumber reads a size or density; more than 5 digits counts as 0.
func screenNumber(s string) int {
	if len(s) > 5 {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

// launcher is the last line of `resolve-activity --brief` that names an
// activity. The chooser (no default launcher set) is not a launcher.
func launcher(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if !activityRe.MatchString(l) {
			continue
		}
		if strings.HasPrefix(l, "android/") && strings.HasSuffix(l, "ResolverActivity") {
			return ""
		}
		return l
	}
	return ""
}

// Collect reads the profile with read-only commands.
func Collect(ctx context.Context, a ADB) (Profile, error) {
	var p Profile
	props := []struct {
		key string
		dst *string
	}{
		{"ro.product.model", &p.Model},
		{"ro.product.manufacturer", &p.Manufacturer},
		{"ro.product.brand", &p.Brand},
		{"ro.build.version.release", &p.Android},
		{"persist.sys.locale", &p.Locale},
	}
	for _, pr := range props {
		v, err := shell(ctx, a, "getprop "+pr.key)
		if err != nil {
			return Profile{}, err
		}
		*pr.dst = v
	}
	if p.Model == "" {
		return Profile{}, fmt.Errorf("the phone did not report its model")
	}
	if p.Locale == "" {
		p.Locale, _ = shell(ctx, a, "getprop ro.product.locale")
	}
	sdk, err := shell(ctx, a, "getprop ro.build.version.sdk")
	if err != nil {
		return Profile{}, err
	}
	p.SDK, _ = strconv.Atoi(sdk)
	if out, err := shell(ctx, a, "wm size"); err == nil {
		for _, m := range sizeRe.FindAllStringSubmatch(out, -1) {
			p.Width, p.Height = screenNumber(m[2]), screenNumber(m[3]) // an override, listed last, wins
		}
	}
	if out, err := shell(ctx, a, "wm density"); err == nil {
		for _, m := range densityRe.FindAllStringSubmatch(out, -1) {
			p.Density = screenNumber(m[2])
		}
	}
	if out, err := shell(ctx, a, "cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.HOME"); err == nil {
		p.Launcher = launcher(out)
	}
	p.Google = strings.EqualFold(p.Manufacturer, "Google")
	p.Name = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(p.Model), "-"), "-")
	if len(p.Name) > maxRunes { // only ASCII is left after slugRe
		p.Name = strings.Trim(p.Name[:maxRunes], "-")
	}
	if p.Name == "" {
		p.Name = "phone"
	}
	p.CollectedAt = time.Now().UTC()
	return p, nil
}

// Markdown is the profile section of AGENTS.md.
// The values come from the phone and end up in the agent's instructions:
// they are escaped and kept inside code spans, as data.
func (p Profile) Markdown() string {
	maker := "another maker (not Google)"
	if p.Google {
		maker = "Google"
	}
	v := func(s string) string {
		return "`" + strings.ReplaceAll(classify.Printable(truncate(s)), "`", "'") + "`"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- Model: %s (%s, brand %s; %s)\n", v(p.Model), v(p.Manufacturer), v(p.Brand), maker)
	fmt.Fprintf(&b, "- Android: %s (SDK %d)\n", v(p.Android), p.SDK)
	if p.Width > 0 {
		fmt.Fprintf(&b, "- Screen: %dx%d px", p.Width, p.Height)
		if p.Density > 0 {
			fmt.Fprintf(&b, ", density %d dpi", p.Density)
		}
		b.WriteString("\n")
	}
	if p.Launcher != "" {
		fmt.Fprintf(&b, "- Default launcher: %s\n", v(p.Launcher))
	}
	if p.Locale != "" {
		fmt.Fprintf(&b, "- Language: %s\n", v(p.Locale))
	}
	fmt.Fprintf(&b, "- Full profile: `device.json` (collected %s)", p.CollectedAt.Format("2006-01-02"))
	return b.String()
}

// WriteJSON saves device.json in the environment: atomically, and 0600 even
// if the file was already there with other permissions.
func WriteJSON(dir string, p Profile) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".device-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "device.json"))
}

// TCP is adbd's TCP port as its properties read (spec §3.3, VB-41). Both
// fields come from the phone: trimmed, cut to maxPortRunes and printable, so
// they can be shown as they are.
type TCP struct {
	// Now is the port adbd listens on, "" when none: service.adb.tcp.port,
	// or persist.adb.tcp.port when service.adb.tcp.port is empty (AOSP adbd
	// takes persist at its start only then, as after a reboot). Any value but
	// empty or 0 counts as a port, even one that is not a number: never a
	// false "closed".
	Now string
	// Persist is persist.adb.tcp.port when it names a port ("" when empty or
	// 0). `adb usb` does not clear it and it survives a reboot (measured,
	// VB-41): adbd takes it again at its next start with service.adb.tcp.port
	// empty. ADBuddy never sets it and cannot clear it: the shell user may
	// not (setprop exits 1) and the host never runs su.
	Persist string
}

// maxPortRunes bounds a port property as the host keeps and shows it.
const maxPortRunes = 16

// ReadTCPWait bounds one ReadTCP where a caller has a longer budget for more
// than the read (the remote controller and `remote off` before `adb usb`, the
// watcher of the connection): a hung getprop must not eat it. A variable so
// a test can shorten it; read it before starting goroutines.
var ReadTCPWait = 10 * time.Second

// portValue is a property read as a port: "" when empty or 0; otherwise the
// value, cut to maxPortRunes and printable.
func portValue(v string) string {
	if v == "" || v == "0" {
		return ""
	}
	if utf8.RuneCountInString(v) > maxPortRunes {
		v = string([]rune(v)[:maxPortRunes]) + "…"
	}
	return classify.Printable(v)
}

// ReadTCP reads adbd's TCP port: service.adb.tcp.port, then
// persist.adb.tcp.port. It only reads. If either read fails the error says
// so: the port is unknown then, never closed.
func ReadTCP(ctx context.Context, a ADB) (TCP, error) {
	service, err := shell(ctx, a, "getprop service.adb.tcp.port")
	if err != nil {
		return TCP{}, err
	}
	persist, err := shell(ctx, a, "getprop persist.adb.tcp.port")
	if err != nil {
		return TCP{}, err
	}
	t := TCP{Now: portValue(service), Persist: portValue(persist)}
	if service == "" {
		t.Now = t.Persist
	}
	return t, nil
}

// EnsureNoTCPPort applies the host invariant (spec §3.3): if adbd listens on
// a TCP port (TCP.Now) other than keep (remote mode's 5555, "" when the mode
// is off), `adb usb` closes it: adbd sets service.adb.tcp.port to 0 and
// restarts, and with it at 0 ignores persist.adb.tcp.port until the next
// reboot. It returns what it read and whether it ran `adb usb`: adbd
// restarts, so the caller must wait for the phone and read the port again. A
// persist port with service.adb.tcp.port at 0 is nothing to close now
// (`adb usb` would only restart adbd again, at every start): the caller warns
// about it. A read that fails is an error, and nothing runs.
func EnsureNoTCPPort(ctx context.Context, a ADB, keep string) (TCP, bool, error) {
	t, err := ReadTCP(ctx, a)
	if err != nil || t.Now == "" || keep != "" && t.Now == keep {
		return t, false, err
	}
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"usb"}, nil, &out, &errb)
	if err != nil {
		return t, false, err
	}
	if code != 0 {
		return t, false, fmt.Errorf("adb usb: %s", classify.Printable(strings.TrimSpace(errb.String()+" "+out.String())))
	}
	return t, true, nil
}
