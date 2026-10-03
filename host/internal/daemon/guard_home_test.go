package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// V44 (ruling A): with ADBuddy itself in front (the user chatting with the
// agent on the phone) there are two ways out: Home alone and, when the app
// says what is on screen, opening another app with monkey's launcher form.
// Nothing else that touches or reads the screen goes through, and neither is
// a way out of any other protected app.

// adbuddyResumed is dumpsys with ADBuddy's chat in front.
const adbuddyResumed = "ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)\n  topResumedActivity=ActivityRecord{1 u0 tech.hybriq.adbuddy/.MainActivity t1}\n"

// fromADBuddy is in the reason the guard gives with ADBuddy in front.
const fromADBuddy = "To leave it, press Home alone (input keyevent 3)"

// openByPackage is the other way out, which the reason offers only when the
// app says what is on screen.
const openByPackage = "monkey -p <package> -c android.intent.category.LAUNCHER 1"

// openGame opens a game that is not protected from ADBuddy's chat.
var openGame = []string{"shell", "monkey", "-p", "com.supercell.clashroyale", "-c", "android.intent.category.LAUNCHER", "1"}

// withADBuddyInFront puts ADBuddy in front: as the app says it (it marks its
// own screen as protected) or, without the app, as dumpsys says it.
func withADBuddyInFront(e *env, app bool) {
	if app {
		a := e.withApp()
		a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "tech.hybriq.adbuddy", Activity: "tech.hybriq.adbuddy.MainActivity", Protected: true}
		return
	}
	e.adb.shell["dumpsys activity activities"] = adbuddyResumed
}

// ranOnPhone lists what reached adb other than the guard's own queries.
func ranOnPhone(e *env) []string {
	var out []string
	for _, r := range e.adb.ran() {
		if !strings.HasPrefix(r, "precheck:") {
			out = append(out, r)
		}
	}
	return out
}

// helperArgs are valid args for each screen helper the guard covers.
func helperArgs(op string) any {
	switch op {
	case bridge.UIOpClick:
		return xy(1, 1)
	case bridge.UIOpSetText:
		return bridge.UISetTextArgs{Text: "a"}
	case OpScreenshot:
		return ScreenshotArgs{Scale: 1, Format: "png"}
	}
	return bridge.UIDumpArgs{}
}

func TestHomeLeavesADBuddy(t *testing.T) {
	for _, app := range []bool{true, false} {
		for _, c := range []struct {
			argv []string
			ran  string // what reaches adb
		}{
			{[]string{"shell", "input", "keyevent", "3"}, "shell input keyevent 3"},
			{[]string{"shell", "input", "keyevent", "KEYCODE_HOME"}, "shell input keyevent KEYCODE_HOME"},
			{[]string{"shell", "input keyevent 3"}, "shell input keyevent 3"},
			{[]string{"shell", "/system/bin/input keyevent 3"}, "shell /system/bin/input keyevent 3"},
		} {
			e := newEnv(t)
			withADBuddyInFront(e, app)
			s, exit := e.exec(t, c.argv...)
			if exit.Code != 0 {
				t.Errorf("app=%v %q: %+v %q", app, c.argv, exit, s.errOut.String())
				continue
			}
			if r := ranOnPhone(e); len(r) != 1 || r[0] != c.ran {
				t.Errorf("app=%v %q: adb ran %q", app, c.argv, r)
			}
			if j := e.last(t); j.Status != journal.StatusOK || j.Rule != "ui.input" {
				t.Errorf("app=%v %q: journal %+v", app, c.argv, j)
			}
		}
	}
}

func TestOpeningAnotherAppLeavesADBuddy(t *testing.T) {
	for _, c := range []struct {
		argv []string
		ran  string // what reaches adb
	}{
		{openGame, "shell monkey -p com.supercell.clashroyale -c android.intent.category.LAUNCHER 1"},
		{[]string{"shell", "monkey -p com.whatsapp -c android.intent.category.LAUNCHER 1"}, "shell monkey -p com.whatsapp -c android.intent.category.LAUNCHER 1"},
		{[]string{"shell", "/system/bin/monkey -p com.android.settings -c android.intent.category.LAUNCHER 1"}, "shell /system/bin/monkey -p com.android.settings -c android.intent.category.LAUNCHER 1"},
	} {
		e := newEnv(t)
		withADBuddyInFront(e, true)
		s, exit := e.exec(t, c.argv...)
		if exit.Code != 0 {
			t.Errorf("%q: %+v %q", c.argv, exit, s.errOut.String())
			continue
		}
		if r := ranOnPhone(e); len(r) != 1 || r[0] != c.ran {
			t.Errorf("%q: adb ran %q", c.argv, r)
		}
		if j := e.last(t); j.Status != journal.StatusOK || j.Rule != "ui.monkey" {
			t.Errorf("%q: journal %+v", c.argv, j)
		}
	}
}

// Without the app, an app opened on top could leave ADBuddy on screen (a
// dialog, a translucent activity) and dumpsys would not tell: Home first.
func TestOpeningAnAppFromADBuddyNeedsTheApp(t *testing.T) {
	e := newEnv(t)
	withADBuddyInFront(e, false)
	s, exit := e.exec(t, openGame...)
	if exit.Code != 126 || !strings.Contains(s.errOut.String(), fromADBuddy) || strings.Contains(s.errOut.String(), "monkey") {
		t.Errorf("%+v %q", exit, s.errOut.String())
	}
	if r := ranOnPhone(e); len(r) != 0 {
		t.Errorf("adb ran %q", r)
	}
}

func TestNothingElseLeavesADBuddy(t *testing.T) {
	const byName = "tech.hybriq.adbuddy is a protected app; ask the user to do this part"
	for _, app := range []bool{true, false} {
		e := newEnv(t)
		withADBuddyInFront(e, app)
		e.d.o.Protected = func(p string) bool { return p == "com.example.bank" }
		for _, c := range []struct {
			argv []string
			want string
		}{
			// Keys that act on ADBuddy, and Home with an option, another
			// spelling or another way in.
			{[]string{"shell", "input", "keyevent", "4"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "KEYCODE_BACK"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "66"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "187"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "--longpress", "3"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "--doubletap", "3"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "HOME"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "03"}, fromADBuddy},
			// Options after an otherwise exact way out.
			{[]string{"shell", "input", "keyevent", "3", "--longpress"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "KEYCODE_HOME", "--longpress"}, fromADBuddy},
			{[]string{"shell", "monkey", "-p", "com.supercell.clashroyale", "-c", "android.intent.category.LAUNCHER", "1", "-v"}, fromADBuddy},
			{[]string{"shell", "input", "-d", "0", "keyevent", "3"}, fromADBuddy},
			{[]string{"shell", "input", "keyboard", "keyevent", "3"}, fromADBuddy},
			{[]string{"shell", "cmd", "input", "keyevent", "3"}, fromADBuddy},
			{[]string{"shell", "input", "keyevent", "3", "4"}, "must run alone"},
			// A way out chained to what would act on whatever comes next, or wrapped.
			{[]string{"shell", "input keyevent 3; input tap 1 1"}, "must run alone"},
			{[]string{"shell", "input keyevent 3 && monkey -p com.supercell.clashroyale -c android.intent.category.LAUNCHER 1"}, "must run alone"},
			{[]string{"shell", "input keyevent 3 & input tap 1 1"}, "must run alone"},
			{[]string{"shell", "watch -n 1 input keyevent 3"}, "without wrappers"},
			{[]string{"shell", "monkey -p com.supercell.clashroyale -c android.intent.category.LAUNCHER 1; input tap 1 1"}, "must run alone"},
			{[]string{"shell", "monkey -p com.supercell.clashroyale -c android.intent.category.LAUNCHER 1 && screencap -p /sdcard/a.png"}, "must run alone"},
			{[]string{"shell", "watch -n 1 monkey -p com.supercell.clashroyale -c android.intent.category.LAUNCHER 1"}, "without wrappers"},
			// Everything else that touches or reads the screen.
			{[]string{"shell", "input", "tap", "1", "1"}, fromADBuddy},
			{[]string{"shell", "input", "swipe", "1", "1", "2", "2"}, fromADBuddy},
			{[]string{"shell", "input", "text", "hi"}, fromADBuddy},
			{[]string{"exec-out", "screencap", "-p"}, fromADBuddy},
			{[]string{"shell", "uiautomator", "dump"}, fromADBuddy},
			{[]string{"shell", "dumpsys", "activity", "top"}, fromADBuddy},
			// Opening an app in any other way: am start names any activity (a
			// dialog would leave ADBuddy on screen) and resolves intents with
			// no package, which may end in ADBuddy; monkey without -p, with
			// another category or more events, or a package to decode.
			{[]string{"shell", "am", "start", "-n", "com.android.settings/.Settings"}, fromADBuddy},
			{[]string{"shell", "am", "start", "-a", "tech.hybriq.adbuddy.action.SETUP"}, fromADBuddy},
			{[]string{"shell", "am", "start", "-a", "android.intent.action.VIEW", "-d", "https://example.com"}, fromADBuddy},
			{[]string{"shell", "am", "start", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.HOME"}, fromADBuddy},
			{[]string{"shell", "monkey", "-c", "android.intent.category.LAUNCHER", "1"}, fromADBuddy},
			{[]string{"shell", "monkey", "-p", "com.supercell.clashroyale", "1"}, fromADBuddy},
			{[]string{"shell", "monkey", "-p", "com.supercell.clashroyale", "-c", "android.intent.category.MONKEY", "1"}, fromADBuddy},
			{[]string{"shell", "monkey", "-p", "com.supercell.clashroyale", "-c", "android.intent.category.LAUNCHER", "2"}, fromADBuddy},
			{[]string{"shell", "monkey", "-p", "com.supercell.clashroyale", "-p", "com.whatsapp", "-c", "android.intent.category.LAUNCHER", "1"}, fromADBuddy},
			{[]string{"shell", "monkey", "-p", "com.supercell%2Eclashroyale", "-c", "android.intent.category.LAUNCHER", "1"}, fromADBuddy},
			// Opening ADBuddy or a protected app by name keeps its own reason,
			// %-encoded too.
			{[]string{"shell", "am", "start", "-n", "tech.hybriq.adbuddy/.MainActivity"}, byName},
			{[]string{"shell", "monkey", "-p", "tech.hybriq.adbuddy", "-c", "android.intent.category.LAUNCHER", "1"}, byName},
			{[]string{"shell", "monkey", "-p", "tech.hybriq%2Eadbuddy", "-c", "android.intent.category.LAUNCHER", "1"}, byName},
			{[]string{"shell", "monkey", "-p", "com.example.bank", "-c", "android.intent.category.LAUNCHER", "1"}, "com.example.bank is a protected app; ask the user to do this part"},
		} {
			s, exit := e.exec(t, c.argv...)
			if exit.Code != 126 || !strings.Contains(s.errOut.String(), c.want) {
				t.Errorf("app=%v %q: %+v %q", app, c.argv, exit, s.errOut.String())
			}
		}
		for _, op := range []string{bridge.UIOpDump, bridge.UIOpClick, bridge.UIOpSetText, OpScreenshot} {
			if _, exit := e.helper(t, op, helperArgs(op)); exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, fromADBuddy) {
				t.Errorf("app=%v %s: %+v", app, op, exit)
			}
		}
		// The reason offers opening an app only when the app says what is on screen.
		if s, _ := e.exec(t, "shell", "input", "tap", "1", "1"); strings.Contains(s.errOut.String(), openByPackage) != app {
			t.Errorf("app=%v: %q", app, s.errOut.String())
		}
		if r := ranOnPhone(e); len(r) != 0 {
			t.Errorf("app=%v: adb ran %q", app, r)
		}
	}
}

func TestNoWayOutOfOtherProtectedApps(t *testing.T) {
	home := []string{"shell", "input", "keyevent", "3"}
	tap := []string{"shell", "input", "tap", "1", "1"}
	bank := func(p string) bool { return p == "com.example.bank" }
	for _, c := range []struct {
		name      string
		prepare   func(e *env)
		wantHome  string
		wantOther string // opening an app and a tap
	}{
		{"a protected app, by the app", func(e *env) {
			a := e.withApp("com.example.bank")
			a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.example.bank", Protected: true}
		}, "com.example.bank is a protected app", "com.example.bank is a protected app"},
		{"a protected app, by dumpsys", func(e *env) {
			e.d.o.Protected = bank
			e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n  topResumedActivity=ActivityRecord{1 u0 com.example.bank/.Main t1}\n"
		}, "com.example.bank is a protected app", "com.example.bank is a protected app"},
		{"the app says protected, without a package", func(e *env) {
			a := e.withApp()
			a.results[bridge.UIOpForeground] = bridge.UIForeground{Protected: true}
		}, "something on the screen is protected", "something on the screen is protected"},
		// After an app opened on top of ADBuddy that leaves it reachable (a
		// dialog that lets taps through around it), the app still lists
		// ADBuddy's window and says protected.
		{"another app over ADBuddy, by the app", func(e *env) {
			a := e.withApp()
			a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.supercell.clashroyale", Protected: true}
		}, "something on the screen is protected", "something on the screen is protected"},
		{"ADBuddy on top, the bank in the other half", func(e *env) {
			e.d.o.Protected = bank
			e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n  topResumedActivity=ActivityRecord{1 u0 tech.hybriq.adbuddy/.MainActivity t1}\n    mResumedActivity: ActivityRecord{2 u0 com.example.bank/.Main t2}\n"
		}, "com.example.bank is a protected app", fromADBuddy},
		{"another app on top, ADBuddy in the other half", func(e *env) {
			e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n  topResumedActivity=ActivityRecord{1 u0 com.android.settings/.Settings t1}\n    mResumedActivity: ActivityRecord{2 u0 tech.hybriq.adbuddy/.MainActivity t2}\n"
			// No way out here, so not the reason that offers one (it would loop).
		}, "tech.hybriq.adbuddy is a protected app; ask the user to do this part", "tech.hybriq.adbuddy is a protected app; ask the user to do this part"},
	} {
		e := newEnv(t)
		c.prepare(e)
		for _, x := range []struct {
			argv []string
			want string
		}{{home, c.wantHome}, {openGame, c.wantOther}, {tap, c.wantOther}} {
			s, exit := e.exec(t, x.argv...)
			if exit.Code != 126 || !strings.Contains(s.errOut.String(), x.want) {
				t.Errorf("%s: %q: %+v %q", c.name, x.argv, exit, s.errOut.String())
			}
		}
		if r := ranOnPhone(e); len(r) != 0 {
			t.Errorf("%s: adb ran %q", c.name, r)
		}
	}
}

func TestLeavingADBuddyStillFailsClosed(t *testing.T) {
	home := []string{"shell", "input", "keyevent", "3"}
	// What is in front cannot be told.
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpForeground] = ErrNoApp
	delete(e.adb.shell, "dumpsys activity activities")
	for _, argv := range [][]string{home, openGame} {
		if s, exit := e.exec(t, argv...); exit.Code != 126 || !strings.Contains(s.errOut.String(), "could not tell which app is in the foreground") {
			t.Errorf("unknown foreground: %q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	// ADBuddy in front, but whether the shade is open cannot be told.
	for _, app := range []bool{true, false} {
		e := newEnv(t)
		withADBuddyInFront(e, app)
		e.adb.shell[fixedFocusCmd] = "Unknown command\n"
		ways := [][]string{home}
		if app {
			ways = append(ways, openGame)
		}
		for _, argv := range ways {
			if s, exit := e.exec(t, argv...); exit.Code != 126 || !strings.Contains(s.errOut.String(), "could not tell whether the notification shade is open") {
				t.Errorf("app=%v unknown shade: %q: %+v %q", app, argv, exit, s.errOut.String())
			}
		}
		if r := ranOnPhone(e); len(r) != 0 {
			t.Errorf("app=%v: adb ran %q", app, r)
		}
	}
}

func TestLeavingADBuddyKeepsTheDoors(t *testing.T) {
	home := []string{"shell", "input", "keyevent", "3"}
	// The shade open over ADBuddy: Home closes it (§4.3 point 6) and goes
	// through; opening an app does not get past it.
	for _, app := range []bool{true, false} {
		e := newEnv(t)
		withADBuddyInFront(e, app)
		e.adb.shell[fixedFocusCmd] = focusShade
		if s, exit := e.exec(t, home...); exit.Code != 0 {
			t.Errorf("app=%v shade, Home: %+v %q", app, exit, s.errOut.String())
		}
		if r := ranOnPhone(e); len(r) != 1 || r[0] != "shell input keyevent 3" {
			t.Errorf("app=%v shade, Home: adb ran %q", app, r)
		}
	}
	e := newEnv(t)
	withADBuddyInFront(e, true)
	e.adb.shell[fixedFocusCmd] = focusShade
	if s, exit := e.exec(t, openGame...); exit.Code != 126 || !strings.Contains(s.errOut.String(), "the notification shade is open") {
		t.Errorf("shade: %+v %q", exit, s.errOut.String())
	}
	if r := ranOnPhone(e); len(r) != 0 {
		t.Errorf("shade: adb ran %q", r)
	}
	for _, app := range []bool{true, false} {
		ways := [][]string{home}
		if app {
			ways = append(ways, openGame)
		}
		for _, argv := range ways {
			// Paused: refused before the guard looks.
			e := newEnv(t)
			withADBuddyInFront(e, app)
			e.d.o.Paused = func() bool { return true }
			if s, exit := e.exec(t, argv...); exit.Code != 126 || !strings.Contains(s.errOut.String(), PausedMessage) {
				t.Errorf("app=%v paused: %q: %+v %q", app, argv, exit, s.errOut.String())
			}
			if r := e.adb.ran(); len(r) != 0 {
				t.Errorf("app=%v paused: %q: adb ran %q", app, argv, r)
			}
			// A 🔴 question waits: the way out does not get past the door either.
			e = newEnv(t)
			withADBuddyInFront(e, app)
			e.d.o.OnEntry = func(journal.Entry) {}
			held := newHeld()
			e.d.o.Confirmer = held
			red := e.execAsync("shell", "reboot")
			select {
			case <-held.asked:
			case <-time.After(5 * time.Second):
				t.Fatal("the 🔴 was not asked")
			}
			r := waitResult(t, e.execAsync(argv...))
			if r.exit.Code != ExitDenied || !strings.Contains(r.s.errOut.String(), "waiting for your answer") {
				t.Errorf("app=%v while a question waits: %q: %+v %q", app, argv, r.exit, r.s.errOut.String())
			}
			held.answer <- confirm.Deny
			waitResult(t, red)
			if r := ranOnPhone(e); len(r) != 0 {
				t.Errorf("app=%v question: %q: adb ran %q", app, argv, r)
			}
		}
	}
}
