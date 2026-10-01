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

// TCPPort returns the TCP port adbd listens on, or "" when it listens on
// none (service.adb.tcp.port empty or 0). It only reads.
func TCPPort(ctx context.Context, a ADB) (string, error) {
	port, err := shell(ctx, a, "getprop service.adb.tcp.port")
	if err != nil || port == "0" {
		return "", err
	}
	return port, nil
}

// EnsureNoTCPPort applies the host invariant (spec §3.3): if adbd listens on
// a TCP port (service.adb.tcp.port neither empty nor 0) and remote mode is
// not on, `adb usb` closes it. adbd restarts, so the caller must wait for
// the phone again when this returns true. keep is a port that must stay open
// (remote mode's 5555); another open port is still closed.
func EnsureNoTCPPort(ctx context.Context, a ADB, keep string) (bool, error) {
	port, err := TCPPort(ctx, a)
	if err != nil || port == "" || keep != "" && port == keep {
		return false, err
	}
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"usb"}, nil, &out, &errb)
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("adb usb: %s", classify.Printable(strings.TrimSpace(errb.String()+" "+out.String())))
	}
	return true, nil
}
