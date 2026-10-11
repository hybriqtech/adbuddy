package latbench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// Phone is a small fake Android for the agent to work on without an emulator:
// the host's daemon talks to it through its two doors (the executor that stands
// for adb and the function that stands for the ADBuddy app), and it answers at
// once. The Settings home is a real recording (uidump/testdata/api34); the
// other screens are written in its style. It has a launcher, Settings, Network &
// internet and Settings' search. What it does not know it answers with nothing
// and exit 0, and the commands are in Log so a run can be checked.
type Phone struct {
	mu     sync.Mutex
	screen string // home, settings, network, search
	query  string
	log    []string

	settingsHome []bridge.UIElement
}

const (
	scrHome     = "home"
	scrSettings = "settings"
	scrNetwork  = "network"
	scrSearch   = "search"
)

// NewPhone is a phone showing the launcher.
func NewPhone() (*Phone, error) {
	_, file, _, _ := runtime.Caller(0)
	xml, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "uidump", "testdata", "api34", "settings.uiautomator.txt"))
	if err != nil {
		return nil, err
	}
	d, err := uidump.Parse(xml, false, nil)
	if err != nil {
		return nil, err
	}
	return &Phone{screen: scrHome, settingsHome: d.Elements}, nil
}

// Reset puts the phone on the launcher and forgets the log.
func (p *Phone) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.screen, p.query, p.log = scrHome, "", nil
}

// Log is what the phone was asked since Reset.
func (p *Phone) Log() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.log...)
}

func el(id, text, class string, b [4]int, click bool) bridge.UIElement {
	return bridge.UIElement{ID: id, Text: text, Class: class, Bounds: b, Click: click, Enabled: true, Window: "app"}
}

// entries lays out a list of (title, summary) rows like Settings does.
func entries(rows [][2]string) []bridge.UIElement {
	out := []bridge.UIElement{el("com.android.settings:id/search_action_bar", "", "FrameLayout", [4]int{44, 224, 1036, 380}, true)}
	y := 460
	for _, r := range rows {
		t := el("android:id/title", r[0], "TextView", [4]int{192, y, 900, y + 60}, false)
		out = append(out, t)
		if r[1] != "" {
			out = append(out, el("android:id/summary", r[1], "TextView", [4]int{192, y + 62, 900, y + 110}, false))
		}
		y += 168
	}
	return out
}

func (p *Phone) elements() (pkg, activity string, els []bridge.UIElement) {
	switch p.screen {
	case scrSettings:
		return "com.android.settings", "com.android.settings.homepage.SettingsHomepageActivity", p.settingsHome
	case scrNetwork:
		return "com.android.settings", "com.android.settings.Settings$NetworkDashboardActivity", append(
			[]bridge.UIElement{el("com.android.settings:id/action_bar", "Network & internet", "TextView", [4]int{132, 120, 800, 220}, false)},
			entries([][2]string{{"Internet", "Wi‑Fi"}, {"SIMs", "Mobile data off"}, {"Airplane mode", ""}, {"Hotspot & tethering", "Off"}, {"Data Saver", "Off"}, {"VPN", "None"}, {"Private DNS", "Automatic"}, {"Adaptive connectivity", "On"}})[1:]...)
	case scrSearch:
		field := el("com.google.android.settings.intelligence:id/open_search_view_edit_text", p.query, "EditText", [4]int{150, 140, 900, 260}, true)
		field.Edit, field.Focused, field.Hint = true, true, "Search settings"
		els := []bridge.UIElement{field}
		if p.query == "" {
			els = append(els, el("android:id/title", "Recent searches", "TextView", [4]int{60, 330, 900, 400}, false))
		} else if strings.Contains(strings.ToLower(p.query), "wifi") || strings.Contains(strings.ToLower(p.query), "wi-fi") {
			for i, r := range [][2]string{{"Wi‑Fi", "Network & internet"}, {"Wi‑Fi preferences", "Wi‑Fi > Wi‑Fi preferences"}, {"Wi‑Fi hotspot", "Hotspot & tethering"}, {"Saved networks", "Wi‑Fi > Saved networks"}} {
				els = append(els, el("android:id/title", r[0], "TextView", [4]int{192, 340 + i*160, 900, 400 + i*160}, true), el("android:id/summary", r[1], "TextView", [4]int{192, 402 + i*160, 900, 450 + i*160}, false))
			}
		} else {
			els = append(els, el("android:id/title", "No results found", "TextView", [4]int{60, 330, 900, 400}, false))
		}
		return "com.google.android.settings.intelligence", "com.google.android.settings.intelligence.search.SearchActivity", els
	}
	els = []bridge.UIElement{}
	for i, n := range []string{"Phone", "Messages", "Chrome", "Camera", "Photos", "Settings", "Play Store", "YouTube"} {
		x := 90 + (i%4)*250
		y := 1700 + (i/4)*330
		e := el("", n, "TextView", [4]int{x, y, x + 200, y + 100}, true)
		e.Desc = n
		els = append(els, e)
	}
	return "com.google.android.apps.nexuslauncher", "com.google.android.apps.nexuslauncher.NexusLauncherActivity", els
}

// UI is daemon.Options.UI: the ADBuddy app.
func (p *Phone) UI(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pkg, act, els := p.elements()
	p.log = append(p.log, "app "+op+" "+Clip(string(args), 60))
	switch op {
	case bridge.UIOpForeground:
		return json.Marshal(bridge.UIForeground{Package: pkg, Activity: act})
	case bridge.UIOpDump:
		return json.Marshal(bridge.UIDump{Package: pkg, Activity: act, Width: 1080, Height: 2400, Elements: els})
	case bridge.UIOpClick:
		var a bridge.UIClickArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		var hit *bridge.UIElement
		for i := range els {
			e := &els[i]
			switch {
			case a.Text != "" && (e.Text == a.Text || e.Desc == a.Text) && (e.Click || e.Text != ""):
				hit = e
			case a.ID != "" && e.ID == a.ID:
				hit = e
			case a.X != nil && a.Y != nil && *a.X >= e.Bounds[0] && *a.X < e.Bounds[2] && *a.Y >= e.Bounds[1] && *a.Y < e.Bounds[3] && e.Click:
				hit = e
			}
			if hit != nil {
				break
			}
		}
		if hit == nil && a.X != nil {
			b := [4]int{*a.X, *a.Y, *a.X + 1, *a.Y + 1}
			return json.Marshal(bridge.UIClicked{Clicked: true, Bounds: b, How: "gesture"})
		}
		if hit == nil {
			return nil, &daemon.UIError{Code: bridge.CodeNotFound, Text: "No element on the screen matches."}
		}
		p.press(hit)
		return json.Marshal(bridge.UIClicked{Clicked: true, Bounds: hit.Bounds, How: "action"})
	case bridge.UIOpSetText:
		var a bridge.UISetTextArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		if p.screen != scrSearch {
			return nil, &daemon.UIError{Code: bridge.CodeNotFound, Text: "No text field has the focus: tap one first."}
		}
		p.query = a.Text
		return json.Marshal(bridge.UITextSet{Set: true})
	}
	return nil, &daemon.UIError{Code: bridge.CodeUnsupported, Text: "not handled by the fake phone"}
}

// press is what tapping e does.
func (p *Phone) press(e *bridge.UIElement) {
	switch {
	case p.screen == scrHome && (e.Text == "Settings" || e.Desc == "Settings"):
		p.screen = scrSettings
	case p.screen == scrSettings && e.Text == "Network & internet":
		p.screen = scrNetwork
	case p.screen == scrSettings && strings.HasSuffix(e.ID, "search_action_bar") || p.screen == scrSettings && e.Text == "Search settings":
		p.screen, p.query = scrSearch, ""
	case p.screen == scrNetwork && strings.HasSuffix(e.ID, "search_action_bar"):
		p.screen, p.query = scrSearch, ""
	}
}

var focusRE = map[string]string{
	scrHome:     "com.google.android.apps.nexuslauncher/com.google.android.apps.nexuslauncher.NexusLauncherActivity",
	scrSettings: "com.android.settings/com.android.settings.Settings",
	scrNetwork:  "com.android.settings/com.android.settings.Settings$NetworkDashboardActivity",
	scrSearch:   "com.google.android.settings.intelligence/com.google.android.settings.intelligence.search.SearchActivity",
}

// Executor stands for adb: it is daemon.Executor.
func (p *Phone) Executor() daemon.Executor { return phoneADB{p} }

type phoneADB struct{ p *Phone }

func (a phoneADB) Pull(ctx context.Context, remote, local string) error { return nil }

func (a phoneADB) Shell(ctx context.Context, command string) (string, int, error) {
	out, code := a.p.shell(command)
	return out, code, nil
}

func (a phoneADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(args) > 0 && args[0] == "exec-out" {
		return a.p.execOut(args[1:], stdout, stderr), nil
	}
	if len(args) > 0 && args[0] == "shell" {
		out, code := a.p.shell(strings.Join(args[1:], " "))
		io.WriteString(stdout, out)
		return code, nil
	}
	return 0, nil
}

var (
	keyRE  = regexp.MustCompile(`^input keyevent (\S+)$`)
	tapRE  = regexp.MustCompile(`^input tap (\d+) (\d+)$`)
	textRE = regexp.MustCompile(`^input text (.+)$`)
	monkey = regexp.MustCompile(`^monkey -p (\S+) -c android\.intent\.category\.LAUNCHER 1$`)
)

func (p *Phone) shell(cmd string) (string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cmd = strings.TrimSpace(cmd)
	if strings.Contains(cmd, "mCurrentFocus") {
		// The guard's own read: not the agent's, not logged.
		return "  mCurrentFocus=Window{d849384 u0 " + focusRE[p.screen] + "}\n", 0
	}
	p.log = append(p.log, "adb shell "+Clip(cmd, 100))
	switch {
	case keyRE.MatchString(cmd):
		switch keyRE.FindStringSubmatch(cmd)[1] {
		case "3", "KEYCODE_HOME":
			p.screen = scrHome
		case "4", "KEYCODE_BACK":
			switch p.screen {
			case scrNetwork, scrSearch:
				p.screen = scrSettings
			case scrSettings:
				p.screen = scrHome
			}
		}
		return "", 0
	case monkey.MatchString(cmd):
		if monkey.FindStringSubmatch(cmd)[1] == "com.android.settings" {
			p.screen = scrSettings
		}
		return "Events injected: 1\n## Network stats: elapsed time=12ms (0ms mobile, 0ms wifi, 12ms not connected)\n", 0
	case strings.HasPrefix(cmd, "am start") && strings.Contains(cmd, "settings"):
		p.screen = scrSettings
		return "Starting: Intent { act=android.settings.SETTINGS }\n", 0
	case tapRE.MatchString(cmd):
		m := tapRE.FindStringSubmatch(cmd)
		x, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		_, _, els := p.elements()
		for i := range els {
			if e := &els[i]; e.Click && x >= e.Bounds[0] && x < e.Bounds[2] && y >= e.Bounds[1] && y < e.Bounds[3] {
				p.press(e)
				break
			}
		}
		return "", 0
	case textRE.MatchString(cmd):
		if p.screen == scrSearch {
			p.query = strings.ReplaceAll(strings.Trim(textRE.FindStringSubmatch(cmd)[1], `'"`), "%s", " ")
		}
		return "", 0
	case strings.HasPrefix(cmd, "getprop ro.build.version.release"):
		return "14\n", 0
	case strings.HasPrefix(cmd, "getprop ro.build.version.sdk"):
		return "34\n", 0
	case strings.HasPrefix(cmd, "getprop ro.product.model"):
		return "Pixel 8\n", 0
	case strings.HasPrefix(cmd, "getprop ro.product.manufacturer"):
		return "Google\n", 0
	case strings.HasPrefix(cmd, "getprop ro.build.version.security_patch"):
		return "2025-05-05\n", 0
	case cmd == "getprop":
		return "[ro.build.version.release]: [14]\n[ro.build.version.sdk]: [34]\n[ro.product.model]: [Pixel 8]\n[ro.product.manufacturer]: [Google]\n", 0
	case strings.HasPrefix(cmd, "dumpsys battery"):
		return "Current Battery Service state:\n  AC powered: false\n  USB powered: true\n  Wireless powered: false\n  Max charging current: 500000\n  status: 2\n  health: 2\n  present: true\n  level: 87\n  scale: 100\n  voltage: 4312\n  temperature: 281\n  technology: Li-ion\n", 0
	case strings.HasPrefix(cmd, "pm list packages") || strings.HasPrefix(cmd, "cmd package list packages"):
		if strings.Contains(cmd, "-3") {
			return "package:com.whatsapp\npackage:com.spotify.music\npackage:org.mozilla.firefox\npackage:com.duolingo\n", 0
		}
		return "package:com.android.settings\npackage:com.android.systemui\npackage:com.google.android.apps.nexuslauncher\npackage:com.android.chrome\npackage:com.whatsapp\npackage:com.spotify.music\npackage:org.mozilla.firefox\npackage:com.duolingo\n", 0
	case strings.HasPrefix(cmd, "wm size"):
		return "Physical size: 1080x2400\n", 0
	case strings.HasPrefix(cmd, "wm density"):
		return "Physical density: 420\n", 0
	case strings.HasPrefix(cmd, "settings get"):
		return "null\n", 0
	}
	return "", 0
}

func (p *Phone) execOut(args []string, stdout, stderr io.Writer) int {
	p.mu.Lock()
	p.log = append(p.log, "adb exec-out "+Clip(strings.Join(args, " "), 80))
	p.mu.Unlock()
	if len(args) > 0 && args[0] == "screencap" {
		img := image.NewRGBA(image.Rect(0, 0, 540, 1200))
		for y := 0; y < 1200; y++ {
			for x := 0; x < 540; x++ {
				img.Set(x, y, color.RGBA{uint8(x / 3), uint8(y / 5), 120, 255})
			}
		}
		var b bytes.Buffer
		_ = png.Encode(&b, img)
		stdout.Write(b.Bytes())
		return 0
	}
	fmt.Fprintln(stderr, "ERROR: null root node returned by UiTestAutomationBridge.")
	return 1
}
