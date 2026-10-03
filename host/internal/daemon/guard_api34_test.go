package daemon

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The guard's decision on real outputs of the API 34 emulator (T5 of the
// manual validations plan): the fake adb answers each read of the guard with
// what the emulator printed in a state (uidump/testdata/api34, recorded by
// emutest's guard tests, which also ask the real daemon on the emulator).

// api34Sample reads the read name of state, "" when it was not recorded.
func api34Sample(t *testing.T, state, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "uidump", "testdata", "api34", state+"."+name+".txt"))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// api34Env is an env whose phone is the emulator in state, without the app.
// dumpsys power is the state's, or settings' (the screen on and unlocked).
func api34Env(t *testing.T, state string) *env {
	t.Helper()
	e := newEnv(t)
	for cmd, name := range map[string]string{
		fixedFocusCmd:                 "focus",
		fixedWindowsCmd:               "windows",
		"dumpsys activity activities": "activities",
	} {
		out := api34Sample(t, state, name)
		if out == "" {
			t.Fatalf("no %s sample for %s", name, state)
		}
		e.adb.shell[cmd] = out
	}
	power := api34Sample(t, state, "power")
	if power == "" {
		power = api34Sample(t, "settings", "power")
	}
	e.adb.shell["dumpsys power"] = power
	return e
}

func TestTheGuardOnAPI34Screens(t *testing.T) {
	var (
		screencap = []string{"exec-out", "screencap", "-p"}
		tap       = []string{"shell", "input", "tap", "540", "1200"}
		back      = []string{"shell", "input", "keyevent", "4"}
		home      = []string{"shell", "input", "keyevent", "3"}
		wake      = []string{"shell", "input", "keyevent", "224"}
	)
	const (
		adbuddy = "ADBuddy itself is in front"
		shade   = "the notification shade is open"
		runs    = ""
	)
	cases := []struct {
		state string
		argv  []string
		why   string // "" (runs): it runs
	}{
		// Nothing over the app: the wallpaper's window behind it is a fixed
		// one (the T5 finding, fixed), so the command runs.
		{"settings", screencap, runs},
		{"settings", tap, runs},
		{"keyboard", screencap, runs},
		{"headsup-gone", screencap, runs},
		{"screen-off", screencap, runs},
		{"screen-off", wake, runs},
		{"doze", screencap, runs},
		{"doze", wake, runs},

		{"adbuddy", screencap, adbuddy},
		{"adbuddy", tap, adbuddy},
		{"adbuddy", back, adbuddy},
		{"adbuddy", home, runs},

		{"shade", screencap, shade},
		{"shade", tap, shade},
		{"shade", back, runs},
		{"shade", home, runs},
		{"quick-settings", screencap, shade},
		{"quick-settings", back, runs},

		{"locked-off", screencap, shade},
		{"locked-off", wake, runs},
		{"locked", screencap, shade},
		{"locked", tap, shade},
		{"locked", wake, shade},
		{"locked", back, runs},
		{"locked", home, runs},

		{"volume-dialog", screencap, "a system window is on screen (VolumeDialogImpl"},
		{"headsup", screencap, "a system window is on screen (NotificationShade"},
		{"bubble-collapsed", screencap, "a system window is on screen (Bubbles!"},

		// ADBuddy in the other half: blocked, and Home is no way out there.
		{"split", screencap, "tech.hybriq.adbuddy is a protected app"},
		{"split", home, "tech.hybriq.adbuddy is a protected app"},
		// The open bubble is ADBuddy in front: Home leaves it.
		{"bubble-expanded", screencap, adbuddy},
		{"bubble-expanded", tap, adbuddy},
		{"bubble-expanded", home, runs},
		{"bubble-expanded-a11y", screencap, adbuddy},
	}
	for _, c := range cases {
		e := api34Env(t, c.state)
		s, exit := e.exec(t, c.argv...)
		ran := ranGuarded(e)
		if c.why == runs {
			if exit.Code != 0 || len(ran) != 1 {
				t.Errorf("%s %q: exit %d %q, ran %q; want it to run", c.state, c.argv, exit.Code, s.errOut.String(), ran)
			}
			continue
		}
		if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), c.why) || len(ran) != 0 {
			t.Errorf("%s %q: exit %d %q, ran %q; want blocked because %q", c.state, c.argv, exit.Code, s.errOut.String(), ran, c.why)
		}
	}
}

// Reasons of the guard without the app on the API 34 samples.
const (
	api34ADBuddy   = "tech.hybriq.adbuddy is a protected app: ADBuddy itself is in front"
	api34Split     = "tech.hybriq.adbuddy is a protected app; "
	api34Settings  = "com.android.settings is a protected app; "
	api34Shade     = "the notification shade is open; "
	api34System    = "a system window is on screen ("
	api34Wallpaper = "com.android.systemui.wallpapers.ImageWallpaper"
)

// api34Before is what the guard without the app decided on every sample of
// uidump/testdata/api34 before the wallpaper's window of API 34 was listed as
// a fixed one (recorded with the code of T5): for each state, with no app of
// the user protected and with Settings protected. Every state was blocked.
var api34Before = map[string][2]string{
	"settings":             {api34System + api34Wallpaper + ")", api34Settings},
	"adbuddy":              {api34ADBuddy, api34ADBuddy},
	"keyboard":             {api34System + api34Wallpaper + ")", api34System + api34Wallpaper + ")"},
	"split":                {api34Split, api34Settings},
	"screen-off":           {api34System + api34Wallpaper + ")", api34Settings},
	"doze":                 {api34System + api34Wallpaper + ")", api34Settings},
	"headsup-gone":         {api34System + api34Wallpaper + ")", api34Settings},
	"shade":                {api34Shade, api34Settings},
	"quick-settings":       {api34Shade, api34Settings},
	"locked-off":           {api34Shade, api34Settings},
	"locked":               {api34Shade, api34Settings},
	"headsup":              {api34System + "NotificationShade, " + api34Wallpaper + ")", api34Settings},
	"volume-dialog":        {api34System + "VolumeDialogImpl, " + api34Wallpaper + ")", api34Settings},
	"bubble-collapsed":     {api34System + "Bubbles!, " + api34Wallpaper + ")", api34Settings},
	"bubble-expanded":      {api34ADBuddy, api34ADBuddy},
	"bubble-expanded-a11y": {api34ADBuddy, api34ADBuddy},
}

// withoutWallpaper is what before becomes once the wallpaper's window is a
// fixed one: the same reason without it, and "" (it runs) where it was the
// only reason.
func withoutWallpaper(before string) string {
	list, ok := strings.CutPrefix(before, api34System)
	if !ok {
		return before
	}
	var left []string
	for _, w := range strings.Split(strings.TrimSuffix(list, ")"), ", ") {
		if w != api34Wallpaper {
			left = append(left, w)
		}
	}
	if len(left) == 0 {
		return ""
	}
	return api34System + strings.Join(left, ", ") + ")"
}

// The wallpaper's window of API 34 listed as a fixed one unblocks exactly the
// screens it alone blocked: on every real sample (all of them, by their
// windows read), with a protected app or ADBuddy in front, the shade or the
// lock screen open, a heads-up, the volume dialog or the bubble on screen, the
// guard without the app still blocks for the same reason, minus the wallpaper.
// What reads or touches the screen is judged so: a screenshot, a tap, a dump.
func TestTheWallpaperUnblocksOnlyWhatItAloneBlocked(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "uidump", "testdata", "api34", "*.windows.txt"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no samples: %v", err)
	}
	seen := map[string]bool{}
	for _, f := range files {
		state := strings.TrimSuffix(filepath.Base(f), ".windows.txt")
		seen[state] = true
		before, ok := api34Before[state]
		if !ok {
			t.Errorf("%s: a sample with no decision before the fix; add it to api34Before", state)
			continue
		}
		for i, protectSettings := range []bool{false, true} {
			want := withoutWallpaper(before[i])
			for _, argv := range [][]string{
				{"exec-out", "screencap", "-p"},
				{"shell", "input", "tap", "540", "1200"},
				{"shell", "uiautomator", "dump"},
			} {
				e := api34Env(t, state)
				if protectSettings {
					e.d.o.Protected = func(pkg string) bool { return pkg == "com.android.settings" }
				}
				s, exit := e.exec(t, argv...)
				ran := ranGuarded(e)
				if want == "" {
					if exit.Code != 0 || len(ran) != 1 {
						t.Errorf("%s (Settings protected %v) %q: exit %d %q; want it to run (before: %q)", state, protectSettings, argv, exit.Code, s.errOut.String(), before[i])
					}
					continue
				}
				if exit.Code != ExitDenied || !strings.HasPrefix(s.errOut.String(), "Blocked by ADBuddy: "+want) || len(ran) != 0 {
					t.Errorf("%s (Settings protected %v) %q: exit %d %q, ran %q; want blocked because %q", state, protectSettings, argv, exit.Code, s.errOut.String(), ran, want)
				}
			}
		}
	}
	for state := range api34Before {
		if !seen[state] {
			t.Errorf("%s: in api34Before but no windows sample", state)
		}
	}
	// Only the states with nothing over an unprotected app run now.
	var unblocked []string
	for state, before := range api34Before {
		for i, b := range before {
			if withoutWallpaper(b) == "" {
				unblocked = append(unblocked, state+map[int]string{0: "", 1: " (Settings protected)"}[i])
			}
		}
	}
	slices.Sort(unblocked)
	if want := []string{"doze", "headsup-gone", "keyboard", "keyboard (Settings protected)", "screen-off", "settings"}; !slices.Equal(unblocked, want) {
		t.Errorf("unblocked %q, want %q", unblocked, want)
	}
}
