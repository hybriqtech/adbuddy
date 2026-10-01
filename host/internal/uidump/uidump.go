// Package uidump reads the screen without the ADBuddy app (M5 spec §5): the
// XML of `uiautomator dump`, the app in the foreground from `dumpsys
// activity activities`, and whether ADBuddy's accessibility service is on
// (then uiautomator must not run: it would disconnect the service). It also
// finds the element a tap by text or id means, in the same order as the app
// (spec §3.2).
package uidump

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// AppPackage is ADBuddy's app: never shown nor touched (spec §4.2).
const AppPackage = "tech.hybriq.adbuddy"

// SystemUI is the package of the status bar and the notifications.
const SystemUI = "com.android.systemui"

// Limits of a dump (spec §3.1).
const (
	MaxElements = 2000
	MaxDepth    = 75
)

// node is an element of uiautomator's XML.
type node struct {
	Text       string `xml:"text,attr"`
	ID         string `xml:"resource-id,attr"`
	Class      string `xml:"class,attr"`
	Package    string `xml:"package,attr"`
	Desc       string `xml:"content-desc,attr"`
	Hint       string `xml:"hint,attr"`
	Checkable  string `xml:"checkable,attr"`
	Checked    string `xml:"checked,attr"`
	Clickable  string `xml:"clickable,attr"`
	Enabled    string `xml:"enabled,attr"`
	Focused    string `xml:"focused,attr"`
	Scrollable string `xml:"scrollable,attr"`
	LongClick  string `xml:"long-clickable,attr"`
	Password   string `xml:"password,attr"`
	Bounds     string `xml:"bounds,attr"`
	Nodes      []node `xml:"node"`
}

type hierarchy struct {
	Rotation int    `xml:"rotation,attr"`
	Nodes    []node `xml:"node"`
}

var boundsRE = regexp.MustCompile(`^\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]$`)

func (n node) bounds() [4]int {
	m := boundsRE.FindStringSubmatch(n.Bounds)
	if m == nil {
		return [4]int{}
	}
	var b [4]int
	for i := range b {
		b[i], _ = strconv.Atoi(m[i+1])
	}
	return b
}

func (n node) edit() bool {
	return strings.HasSuffix(n.Class, "EditText") || strings.HasSuffix(n.Class, "AutoCompleteTextView")
}
func (n node) click() bool    { return n.Clickable == "true" || n.LongClick == "true" }
func (n node) password() bool { return n.Password == "true" }

// usable: something the agent can do with it; informative: something to read.
func (n node) usable() bool {
	return n.click() || n.edit() || n.Scrollable == "true" || n.Checkable == "true"
}

func (n node) informative() bool {
	return !n.password() && (n.Text != "" || n.Hint != "") || n.Desc != ""
}

func empty(b [4]int) bool { return b[2] <= b[0] || b[3] <= b[1] }

// parse reads uiautomator's output: the XML, maybe followed by
// "UI hierchary dumped to: /dev/tty" when it was dumped to the terminal.
func parse(out []byte) (hierarchy, error) {
	var h hierarchy
	start := bytes.Index(out, []byte("<?xml"))
	if start < 0 {
		start = bytes.Index(out, []byte("<hierarchy"))
	}
	end := bytes.LastIndex(out, []byte("</hierarchy>"))
	if start < 0 || end < start {
		return h, errors.New("uiautomator did not return a screen layout")
	}
	if err := xml.Unmarshal(out[start:end+len("</hierarchy>")], &h); err != nil {
		return h, fmt.Errorf("uiautomator's screen layout is unreadable: %w", err)
	}
	return h, nil
}

// Parse turns uiautomator's output into the dump the app would give (spec
// §3.1), as far as the XML allows: no activity, every window is "app" but
// the system UI's, and a password field never carries its text. The nodes of
// ADBuddy and of the packages hidden reports (the protected apps; nil: none)
// are left out, as in another half of a split screen. all keeps every
// visible node.
func Parse(out []byte, all bool, hidden func(string) bool) (bridge.UIDump, error) {
	hide := hider(hidden)
	h, err := parse(out)
	if err != nil {
		return bridge.UIDump{}, err
	}
	d := bridge.UIDump{Rotation: h.Rotation, Elements: []bridge.UIElement{}}
	if len(h.Nodes) > 0 {
		root := h.Nodes[0]
		b := root.bounds()
		d.Width, d.Height = b[2], b[3]
		d.Package = root.Package
		if hide(d.Package) {
			d.Package = ""
		}
	}
	var walk func(n node, depth int)
	walk = func(n node, depth int) {
		if depth > MaxDepth {
			d.Truncated = true
			return
		}
		if hide(n.Package) {
			return
		}
		b := n.bounds()
		if !empty(b) && (all || n.usable() || n.informative()) {
			if len(d.Elements) >= MaxElements {
				d.Truncated = true
				return
			}
			d.Elements = append(d.Elements, element(n, b))
		}
		for _, c := range n.Nodes {
			walk(c, depth+1)
		}
	}
	for _, n := range h.Nodes {
		walk(n, 0)
	}
	return d, nil
}

func element(n node, b [4]int) bridge.UIElement {
	e := bridge.UIElement{
		ID: n.ID, Text: n.Text, Desc: n.Desc, Hint: n.Hint,
		Class: n.Class[strings.LastIndex(n.Class, ".")+1:], Bounds: b,
		Click: n.click(), Edit: n.edit(), Scroll: n.Scrollable == "true", Focused: n.Focused == "true",
		Enabled: n.Enabled != "false", Password: n.password(), Window: "app",
	}
	if n.Checkable == "true" {
		checked := n.Checked == "true"
		e.Checked = &checked
	}
	if e.Password {
		e.Text, e.Hint = "", ""
	}
	if n.Package == SystemUI {
		e.Window = "system"
	}
	return e
}

// Errors of Find.
var (
	ErrNotFound  = errors.New("no element matches")
	ErrProtected = errors.New("the element belongs to ADBuddy")
)

// AmbiguousError lists the bounds of the elements that match, at most 5.
type AmbiguousError struct {
	N      int
	Bounds [][4]int
}

func (e *AmbiguousError) Error() string {
	parts := make([]string, len(e.Bounds))
	for i, b := range e.Bounds {
		parts[i] = FormatBounds(b)
	}
	return fmt.Sprintf("%d elements match: %s. Use --id or --xy.", e.N, strings.Join(parts, ", "))
}

// FormatBounds writes bounds as (left,top)-(right,bottom).
func FormatBounds(b [4]int) string {
	return fmt.Sprintf("(%d,%d)-(%d,%d)", b[0], b[1], b[2], b[3])
}

// Find returns the bounds of what a tap by id or text means in uiautomator's
// output (spec §3.2): the nodes with that exact id; else with that exact
// text or description; else the same ignoring case. Each one stands for its
// nearest clickable ancestor (itself if it is clickable, itself if none is).
// Several different targets are an *AmbiguousError, and a match inside
// ADBuddy or a package hidden reports is ErrProtected.
func Find(out []byte, id, text string, hidden func(string) bool) ([4]int, error) {
	hide := hider(hidden)
	h, err := parse(out)
	if err != nil {
		return [4]int{}, err
	}
	type found struct {
		target node
		app    bool // inside ADBuddy or a hidden app
	}
	var exact, byText, folded []found
	var walk func(n node, clickable *node, depth int)
	walk = func(n node, clickable *node, depth int) {
		if depth > MaxDepth {
			return
		}
		if n.click() {
			c := n
			clickable = &c
		}
		if !empty(n.bounds()) {
			t := n
			if clickable != nil {
				t = *clickable
			}
			f := found{target: t, app: hide(n.Package) || hide(t.Package)}
			switch {
			case id != "" && n.ID == id:
				exact = append(exact, f)
			case text != "" && !n.password() && (n.Text == text || n.Desc == text):
				byText = append(byText, f)
			case text != "" && !n.password() && (strings.EqualFold(n.Text, text) || strings.EqualFold(n.Desc, text)):
				folded = append(folded, f)
			}
		}
		for _, c := range n.Nodes {
			walk(c, clickable, depth+1)
		}
	}
	for _, n := range h.Nodes {
		walk(n, nil, 0)
	}
	for _, set := range [][]found{exact, byText, folded} {
		if len(set) == 0 {
			continue
		}
		var targets [][4]int
		seen := map[[4]int]bool{}
		for _, f := range set {
			if f.app {
				return [4]int{}, ErrProtected
			}
			b := f.target.bounds()
			if !seen[b] {
				seen[b] = true
				targets = append(targets, b)
			}
		}
		if len(targets) == 1 {
			return targets[0], nil
		}
		return [4]int{}, &AmbiguousError{N: len(targets), Bounds: targets[:min(5, len(targets))]}
	}
	return [4]int{}, ErrNotFound
}

// Center is the middle of bounds.
func Center(b [4]int) (x, y int) { return (b[0] + b[2]) / 2, (b[1] + b[3]) / 2 }

// hider is hidden plus ADBuddy itself.
func hider(hidden func(string) bool) func(string) bool {
	return func(pkg string) bool { return pkg == AppPackage || hidden != nil && hidden(pkg) }
}

// resumedRE finds the resumed activities in `dumpsys activity activities`:
// topResumedActivity (Android 10+), mResumedActivity (older, one per stack)
// or ResumedActivity (some makers, one per display), each as
// ActivityRecord{<hash> u<user> <package>/<activity> t<task>}.
var resumedRE = regexp.MustCompile(`(?m)^\s*(?:topResumedActivity|mResumedActivity|ResumedActivity)\s*[=:]\s*ActivityRecord\{\S+ u\d+ ([A-Za-z0-9_.]+)/([A-Za-z0-9_.$]+)`)

// Activity is a resumed activity, written in full (".Settings" becomes
// "com.android.settings.Settings").
type Activity struct{ Package, Name string }

// Resumed reads every resumed activity from `dumpsys activity activities`,
// the top one first: with a split screen, a picture in picture or a second
// display there are several. None means none is resumed, or the output
// changed: check Asleep before trusting it.
func Resumed(dumpsys string) []Activity {
	var all []Activity
	seen := map[Activity]bool{}
	for _, m := range resumedRE.FindAllStringSubmatch(dumpsys, -1) {
		a := Activity{m[1], m[2]}
		if strings.HasPrefix(a.Name, ".") {
			a.Name = a.Package + a.Name
		}
		if !seen[a] {
			seen[a] = true
			all = append(all, a)
		}
	}
	return all
}

// Asleep reports whether `dumpsys power` says the screen is off: there is a
// mWakefulness line and every one says Asleep or Dozing. The only case where
// no resumed activity means nothing is in front. Every line must say it, so
// that a line injected by text in the output (a wakelock label with a line
// break) cannot hide the real Awake.
func Asleep(dumpsysPower string) bool {
	n := 0
	for _, line := range strings.Split(dumpsysPower, "\n") {
		line = strings.TrimSpace(line)
		v, found := strings.CutPrefix(line, "mWakefulness=")
		if !found {
			continue
		}
		if v != "Asleep" && v != "Dozing" {
			return false
		}
		n++
	}
	return n > 0
}

// ServiceOn reports whether `dumpsys accessibility` lists ADBuddy's service
// (enabled or bound): uiautomator would then disconnect it.
func ServiceOn(dumpsys string) bool {
	return strings.Contains(dumpsys, AppPackage+"/")
}
