package daemon

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// The notification shade and the quick settings count as a protected app for
// the guard (M5b plan, Task 10): a code can be on them while any app is in
// front. Sample dumpsys output is written by hand (validations 14 and 17).

const (
	fixedFocusCmd   = "dumpsys window | grep -E 'mCurrentFocus'"
	fixedWindowsCmd = "dumpsys window windows"

	focusApp   = "  mCurrentFocus=Window{1a2b3c u0 com.android.settings/com.android.settings.Settings}\n"
	focusShade = "  mCurrentFocus=Window{4b0f3a2 u0 NotificationShade}\n"

	windowsQuiet = "WINDOW MANAGER WINDOWS\n" +
		"  Window #0 Window{1 u0 StatusBar}:\n    mOwnerUid=10123 showForAllUsers=true package=com.android.systemui appop=NONE\n    mViewVisibility=0x0 mHaveFrame=true\n" +
		"  Window #1 Window{2 u0 NavigationBar0}:\n    mOwnerUid=10123 showForAllUsers=true package=com.android.systemui appop=NONE\n    mViewVisibility=0x0 mHaveFrame=true\n" +
		"  Window #2 Window{3 u0 com.android.settings/com.android.settings.Settings}:\n    mOwnerUid=10150 package=com.android.settings appop=NONE\n    mViewVisibility=0x0 mHaveFrame=true\n"
	windowsHeadsUp = windowsQuiet +
		"  Window #3 Window{4 u0 HeadsUp}:\n    mOwnerUid=10123 package=com.android.systemui appop=NONE\n    mViewVisibility=0x0 mHaveFrame=true\n"

	powerAwake  = "POWER MANAGER\n  mWakefulness=Awake\n"
	powerAsleep = "POWER MANAGER\n  mWakefulness=Asleep\n"
)

// shadeBlocked are the commands the guard stops while the shade is open.
var shadeBlocked = [][]string{
	{"exec-out", "screencap", "-p"},
	{"shell", "uiautomator", "dump"},
	{"shell", "input", "tap", "1", "1"},
	{"shell", "input", "swipe", "1", "1", "2", "2"},
	{"shell", "am", "start", "-n", "com.x/.Y"},
	{"shell", "input", "keyevent", "66"},
	{"shell", "input", "keyevent", "--longpress", "3"},
	{"shell", "input", "keyevent", "--duration", "500", "4"},
	{"shell", "input", "keyevent", "3", "4"},
	{"shell", "input", "-d", "0", "keyevent", "4"},
	{"shell", "cmd", "input", "keyevent", "4"},
}

// ranGuarded lists what reached adb of the commands the guard protects.
func ranGuarded(e *env) []string {
	var out []string
	for _, r := range e.adb.ran() {
		if strings.HasPrefix(r, "precheck:") {
			continue
		}
		if strings.Contains(r, "input") || strings.Contains(r, "screencap") || strings.Contains(r, "uiautomator") || strings.Contains(r, "am start") {
			out = append(out, r)
		}
	}
	return out
}

func TestGuardBlocksWhileTheShadeIsOpen(t *testing.T) {
	for _, app := range []bool{false, true} {
		e := newEnv(t)
		if app {
			e.withApp()
		}
		e.adb.shell[fixedFocusCmd] = focusShade
		e.adb.shell["dumpsys power"] = powerAwake
		for _, argv := range shadeBlocked {
			s, exit := e.exec(t, argv...)
			if exit.Code != 126 || !strings.Contains(s.errOut.String(), "Blocked by ADBuddy: the notification shade is open; ") && !strings.Contains(s.errOut.String(), "must run alone") {
				t.Errorf("app=%v %q: %+v %q", app, argv, exit, s.errOut.String())
			}
		}
		for _, op := range []string{bridge.UIOpDump, bridge.UIOpClick, bridge.UIOpSetText, OpScreenshot} {
			var args any = bridge.UIDumpArgs{}
			switch op {
			case bridge.UIOpClick:
				args = xy(1, 1)
			case bridge.UIOpSetText:
				args = bridge.UISetTextArgs{Text: "a"}
			case OpScreenshot:
				args = ScreenshotArgs{Scale: 1, Format: "png"}
			}
			if _, exit := e.helper(t, op, args); exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "the notification shade is open") {
				t.Errorf("app=%v %s: %+v", app, op, exit)
			}
		}
		if r := ranGuarded(e); len(r) != 0 {
			t.Errorf("app=%v: adb ran %q", app, r)
		}
	}
}

func TestShadeLetsTheClosingKeysThrough(t *testing.T) {
	for _, key := range []string{"3", "4", "KEYCODE_HOME", "KEYCODE_BACK"} {
		e := newEnv(t)
		e.adb.shell[fixedFocusCmd] = focusShade
		e.adb.shell["dumpsys power"] = powerAwake
		if s, exit := e.exec(t, "shell", "input", "keyevent", key); exit.Code != 0 {
			t.Errorf("%s: %+v %q", key, exit, s.errOut.String())
		}
		// As one shell argument, too.
		if s, exit := e.exec(t, "shell", "input keyevent "+key); exit.Code != 0 {
			t.Errorf("%s one argument: %+v %q", key, exit, s.errOut.String())
		}
	}
	// Not chained to anything else.
	e := newEnv(t)
	e.adb.shell[fixedFocusCmd] = focusShade
	e.adb.shell["dumpsys power"] = powerAwake
	for _, argv := range [][]string{
		{"shell", "input keyevent 4 ; input tap 1 1"},
		{"shell", "input keyevent 4 && input tap 1 1"},
		{"shell", "input keyevent 4; screencap -p"},
		{"shell", "input keyevent 4 & input tap 1 1"},
	} {
		if s, exit := e.exec(t, argv...); exit.Code != 126 {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	if r := ranGuarded(e); len(r) != 0 {
		t.Errorf("adb ran %q", r)
	}
}

func TestShadeWithTheScreenOffLetsTheAgentWakeIt(t *testing.T) {
	// The lock screen lives in NotificationShade on Android 12+: with the
	// screen off the shade rule does not apply.
	e := newEnv(t)
	e.adb.shell[fixedFocusCmd] = focusShade
	e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)\n"
	e.adb.shell["dumpsys power"] = powerAsleep
	for _, key := range []string{"224", "26"} {
		if s, exit := e.exec(t, "shell", "input", "keyevent", key); exit.Code != 0 {
			t.Fatalf("%s with the screen off: %+v %q", key, exit, s.errOut.String())
		}
	}
	// With the screen on, 224 is a key like any other.
	e.adb.shell["dumpsys activity activities"] = settingsInFront
	e.adb.shell["dumpsys power"] = powerAwake
	if s, exit := e.exec(t, "shell", "input", "keyevent", "224"); exit.Code != 126 || !strings.Contains(s.errOut.String(), "the notification shade is open") {
		t.Fatalf("224 with the screen on: %+v %q", exit, s.errOut.String())
	}
}

func TestGuardFailsClosedWhenTheShadeIsUnknown(t *testing.T) {
	for name, out := range map[string]string{"unreadable": "Unknown command\n", "no line": "", "garbage": "  mCurrentFocus=garbage\n"} {
		e := newEnv(t)
		e.adb.shell[fixedFocusCmd] = out
		e.adb.shell["dumpsys power"] = powerAwake
		s, exit := e.exec(t, "shell", "input", "tap", "1", "1")
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), "could not tell whether the notification shade is open") {
			t.Errorf("%s: %+v %q", name, exit, s.errOut.String())
		}
	}
	// The command failing is unknown too.
	e := newEnv(t)
	delete(e.adb.shell, fixedFocusCmd)
	e.adb.shell["dumpsys power"] = powerAwake
	if s, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 126 || !strings.Contains(s.errOut.String(), "could not tell whether the notification shade is open") {
		t.Errorf("failing: %+v %q", exit, s.errOut.String())
	}
	// Unknown with the screen off: the agent may still wake it.
	e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n"
	e.adb.shell["dumpsys power"] = powerAsleep
	if s, exit := e.exec(t, "shell", "input", "keyevent", "224"); exit.Code != 0 {
		t.Errorf("screen off: %+v %q", exit, s.errOut.String())
	}
}

func TestShadeChangesNothingForAnApp(t *testing.T) {
	e := newEnv(t)
	for _, focus := range []string{focusApp, "  mCurrentFocus=null\n", focusApp + "  mCurrentFocus=null\n"} {
		e.adb.shell[fixedFocusCmd] = focus
		for _, argv := range [][]string{{"shell", "input", "tap", "1", "1"}, {"exec-out", "screencap", "-p"}, {"shell", "input", "keyevent", "66"}} {
			if s, exit := e.exec(t, argv...); exit.Code != 0 {
				t.Errorf("%q with %q: %+v %q", argv, focus, exit, s.errOut.String())
			}
		}
	}
	// One of several displays is enough.
	e.adb.shell[fixedFocusCmd] = focusApp + focusShade
	if _, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 126 {
		t.Errorf("shade on the second display: %+v", exit)
	}
	// What does not touch the screen is not asked about.
	before := len(e.adb.ran())
	if _, exit := e.exec(t, "shell", "getprop", "ro.product.model"); exit.Code != 0 {
		t.Fatalf("getprop: %+v", exit)
	}
	for _, r := range e.adb.ran()[before:] {
		if strings.Contains(r, "dumpsys") {
			t.Errorf("getprop asked %q", r)
		}
	}
}

func TestShadeIsAskedOncePerCommand(t *testing.T) {
	count := func(e *env, cmd string) int {
		n := 0
		for _, r := range e.adb.ran() {
			if r == "precheck: "+cmd {
				n++
			}
		}
		return n
	}
	e := newEnv(t)
	e.exec(t, "shell", "input", "tap", "1", "1")
	if n := count(e, fixedFocusCmd); n != 1 {
		t.Errorf("by adb: %d focus queries", n)
	}
	if n := count(e, fixedWindowsCmd); n != 1 {
		t.Errorf("by adb: %d window queries", n)
	}
	e = newEnv(t)
	e.withApp()
	e.exec(t, "shell", "input", "tap", "1", "1")
	if n := count(e, fixedFocusCmd); n != 1 {
		t.Errorf("with the app: %d focus queries", n)
	}
	// With the app the heads-up signal comes from foreground, not from here.
	if n := count(e, fixedWindowsCmd); n != 0 {
		t.Errorf("with the app: %d window queries", n)
	}
}

func TestShadeAndPauseOrder(t *testing.T) {
	e := newEnv(t)
	e.adb.shell[fixedFocusCmd] = focusShade
	e.adb.shell["dumpsys power"] = powerAwake
	e.d.o.Paused = func() bool { return true }
	s, exit := e.exec(t, "shell", "input", "tap", "1", "1")
	if exit.Code != 126 || !strings.Contains(s.errOut.String(), PausedMessage) {
		t.Fatalf("paused: %+v %q", exit, s.errOut.String())
	}
	for _, r := range e.adb.ran() {
		if strings.Contains(r, "mCurrentFocus") {
			t.Errorf("the guard looked while paused: %q", r)
		}
	}
}

// Heads-up with no app (M5b plan, Step 5b).

func TestGuardBlocksAnUnknownSystemUIWindowByADB(t *testing.T) {
	e := newEnv(t)
	for name, c := range map[string]struct {
		out     string
		blocked string
	}{
		"quiet":      {windowsQuiet, ""},
		"heads-up":   {windowsHeadsUp, "a system window is on screen (HeadsUp)"},
		"unreadable": {"Segmentation failure\n", "could not tell whether a system window is on screen"},
		"empty":      {"", "could not tell whether a system window is on screen"},
	} {
		e.adb.shell[fixedWindowsCmd] = c.out
		e.adb.shell["dumpsys power"] = powerAwake
		s, exit := e.exec(t, "shell", "input", "tap", "1", "1")
		if c.blocked == "" {
			if exit.Code != 0 {
				t.Errorf("%s: %+v %q", name, exit, s.errOut.String())
			}
			continue
		}
		if exit.Code != 126 || !strings.Contains(s.errOut.String(), c.blocked) {
			t.Errorf("%s: %+v %q", name, exit, s.errOut.String())
		}
	}
	// The helpers, too.
	e.adb.shell[fixedWindowsCmd] = windowsHeadsUp
	if _, exit := e.helper(t, bridge.UIOpClick, xy(1, 1)); exit.Code != 126 || !strings.Contains(exit.Error, "a system window is on screen") {
		t.Errorf("helper: %+v", exit)
	}
	// With the screen off nothing is on it.
	e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n"
	e.adb.shell["dumpsys power"] = powerAsleep
	e.adb.shell[fixedWindowsCmd] = "garbage"
	if s, exit := e.exec(t, "shell", "input", "keyevent", "224"); exit.Code != 0 {
		t.Errorf("screen off: %+v %q", exit, s.errOut.String())
	}
}
