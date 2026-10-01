package uidump

import (
	"regexp"
	"strings"
)

// The notification shade and the quick settings are not an app: a code can be
// on them with any app in front. These readers say what dumpsys window shows
// of them. The formats were written from memory of Android 12-15 and are not
// verified on Android 17 (validations 14 and 17 of the M5b plan): whatever
// they do not understand is reported as not ok, and the guard fails closed.

// focusLineRE is a line about the focused window: mCurrentFocus=null, or
// mCurrentFocus=Window{<hash> u<user> <title>}, where the title is the
// package/activity of an app or the name of a system window.
var focusLineRE = regexp.MustCompile(`^\s*mCurrentFocus=(null|Window\{\S+ u\d+ ([^}]+)\})\s*$`)

// ParseFocus reads every mCurrentFocus of `dumpsys window | grep -E
// 'mCurrentFocus'`, one per display, as the title of the focused window ("" for
// null). It is not ok when there is no such line, or when one of them is not
// understood.
func ParseFocus(out string) (focus []string, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "mCurrentFocus") {
			continue
		}
		m := focusLineRE.FindStringSubmatch(line)
		if m == nil {
			return nil, false
		}
		focus = append(focus, strings.TrimSpace(m[2]))
	}
	return focus, len(focus) > 0
}

// IsShade reports whether a focused window's title is the notification shade
// or the status bar, which holds the expanded quick settings. The package is
// not in the line, so only the title is compared. null is not the shade.
func IsShade(focus string) bool {
	return focus == "NotificationShade" || focus == "StatusBar"
}

var (
	windowHeaderRE = regexp.MustCompile(`^\s*Window #\d+ Window\{\S+ u\d+ (.+)\}:\s*$`)
	windowPkgRE    = regexp.MustCompile(`\bpackage=(\S+)`)
	windowVisRE    = regexp.MustCompile(`\bmViewVisibility=(0x[0-9a-fA-F]+)\b`)
)

// SystemUIPackage owns the shade, the status bar and the heads-up
// notifications.
const SystemUIPackage = "com.android.systemui"

// fixedSystemUI are the windows of the system interface that are always
// there and show no text of anyone's.
var fixedSystemUI = map[string]bool{
	"StatusBar": true, "NavigationBar0": true, "NavigationBar1": true,
	"ScreenDecorOverlay": true, "ScreenDecorOverlayBottom": true,
}

// ParseSystemUIWindows reads `dumpsys window windows` and returns the titles
// of the windows of com.android.systemui that are visible (mViewVisibility=0x0)
// and not in the fixed list: a heads-up notification, the shade, a dialog.
// It is not ok when no window is found, or when a window of systemui does not
// say whether it is visible, or a window does not say whose it is.
func ParseSystemUIWindows(out string) (unknown []string, ok bool) {
	type window struct{ title, pkg, vis string }
	var all []window
	for _, line := range strings.Split(out, "\n") {
		if m := windowHeaderRE.FindStringSubmatch(line); m != nil {
			all = append(all, window{title: strings.TrimSpace(m[1])})
			continue
		}
		if len(all) == 0 {
			continue
		}
		w := &all[len(all)-1]
		if m := windowPkgRE.FindStringSubmatch(line); m != nil && w.pkg == "" {
			w.pkg = m[1]
		}
		if m := windowVisRE.FindStringSubmatch(line); m != nil && w.vis == "" {
			w.vis = m[1]
		}
	}
	if len(all) == 0 {
		return nil, false
	}
	for _, w := range all {
		if w.pkg == "" { // cannot tell whose it is
			return nil, false
		}
		if w.pkg != SystemUIPackage || fixedSystemUI[w.title] {
			continue
		}
		if w.vis == "" {
			return nil, false
		}
		if w.vis == "0x0" || w.vis == "0x00" {
			unknown = append(unknown, w.title)
		}
	}
	return unknown, true
}
