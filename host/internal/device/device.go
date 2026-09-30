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
		return "", fmt.Errorf("adb shell %s: exit %d: %s", cmd, code, strings.TrimSpace(errb.String()))
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
)

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
			p.Width, _ = strconv.Atoi(m[2]) // an override, listed last, wins
			p.Height, _ = strconv.Atoi(m[3])
		}
	}
	if out, err := shell(ctx, a, "wm density"); err == nil {
		for _, m := range densityRe.FindAllStringSubmatch(out, -1) {
			p.Density, _ = strconv.Atoi(m[2])
		}
	}
	if out, err := shell(ctx, a, "cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.HOME"); err == nil {
		lines := strings.Split(out, "\n")
		if last := strings.TrimSpace(lines[len(lines)-1]); strings.Contains(last, "/") {
			p.Launcher = last
		}
	}
	p.Google = strings.EqualFold(p.Manufacturer, "Google")
	p.Name = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(p.Model), "-"), "-")
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
		return "`" + strings.ReplaceAll(classify.Printable(s), "`", "'") + "`"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- Model: %s (%s, brand %s; %s)\n", v(p.Model), v(p.Manufacturer), v(p.Brand), maker)
	fmt.Fprintf(&b, "- Android %s (SDK %d)\n", classify.Printable(p.Android), p.SDK)
	if p.Width > 0 {
		fmt.Fprintf(&b, "- Screen: %dx%d px, density %d dpi\n", p.Width, p.Height, p.Density)
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

// WriteJSON saves device.json in the environment.
func WriteJSON(dir string, p Profile) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "device.json"), append(b, '\n'), 0o600)
}

// EnsureNoTCPPort applies the host invariant (spec §3.3): if adbd listens on
// a TCP port (service.adb.tcp.port neither empty nor 0) and remote mode is
// not on, `adb usb` closes it. adbd restarts, so the caller must wait for
// the phone again when this returns true. ADBuddy has no remote mode yet,
// so the port is always closed.
func EnsureNoTCPPort(ctx context.Context, a ADB) (bool, error) {
	port, err := shell(ctx, a, "getprop service.adb.tcp.port")
	if err != nil {
		return false, err
	}
	if port == "" || port == "0" {
		return false, nil
	}
	var out, errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"usb"}, nil, &out, &errb)
	if err != nil {
		return false, err
	}
	if code != 0 {
		return false, fmt.Errorf("adb usb: %s", strings.TrimSpace(errb.String()+" "+out.String()))
	}
	return true, nil
}
