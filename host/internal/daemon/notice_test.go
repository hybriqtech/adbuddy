package daemon

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// The notice before the host reads the screen by adb (plan screen-overlay-frame T2).

const serviceBound = "Bound services:{Service[label=ADBuddy, " + uidump.AppPackage + "/.a11y.AdbuddyAccessibilityService]}"

func noticeCalls(a *fakeApp) (n int) {
	for _, c := range a.called() {
		if strings.HasPrefix(c, bridge.UIOpNotice) {
			n++
		}
	}
	return n
}

func screenRead(e *env) bool {
	for _, r := range ranOnPhone(e) {
		if strings.Contains(r, "screencap") || strings.Contains(r, "screenrecord") || strings.Contains(r, "uiautomator") {
			return true
		}
	}
	return false
}

func TestScreenshotAsksTheAppForTheNoticeFirst(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	var order []string
	a.onCall = func(op string) {
		if op == bridge.UIOpNotice {
			order = append(order, strings.Join(ranOnPhone(e), "|"))
		}
	}
	if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if len(order) != 1 || strings.Contains(order[0], "screencap") {
		t.Fatalf("the notice must come before screencap: %q", order)
	}
	if r := ranOnPhone(e); len(r) != 1 || r[0] != "exec-out screencap -p" {
		t.Fatalf("adb ran %q", r)
	}
}

func TestRawReadsAskForTheNotice(t *testing.T) {
	for _, argv := range [][]string{
		{"exec-out", "screencap", "-p"},
		{"shell", "screencap", "-p", "/sdcard/s.png"},
		{"shell", "uiautomator", "dump", "/sdcard/u.xml"},
		{"shell", "dumpsys", "activity", "top"},
		{"shell", "screenrecord", "/sdcard/r.mp4"}, // red: the notice comes after the Allow
	} {
		e := newEnv(t)
		a := e.withApp()
		var asked, ranAtNotice []int
		a.onCall = func(op string) {
			if op == bridge.UIOpNotice {
				asked = append(asked, len(e.conf.asked))
				ranAtNotice = append(ranAtNotice, len(ranOnPhone(e)))
			}
		}
		if s, exit := e.exec(t, argv...); exit.Code != 0 {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
			continue
		}
		if len(asked) != 1 {
			t.Errorf("%q: the notice was asked %d times", argv, len(asked))
			continue
		}
		if ranAtNotice[0] != 0 {
			t.Errorf("%q: adb ran before the notice", argv)
		}
		if argv[1] == "screenrecord" && asked[0] != 1 {
			t.Errorf("%q: the notice came before the Allow (%d questions)", argv, asked[0])
		}
		if argv[1] != "screenrecord" && asked[0] != 0 {
			t.Errorf("%q: asked the user for a green read", argv)
		}
	}
}

func TestNoNoticePossibleStillReads(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"no app", ErrNoApp},
		{"unavailable", &UIError{Code: bridge.CodeUnavailable, Text: "off"}},
		{"unsupported", &UIError{Code: bridge.CodeUnsupported, Text: "old"}},
	}
	for _, c := range cases {
		e := newEnv(t)
		a := e.withApp()
		a.errs[bridge.UIOpNotice] = c.err
		e.adb.out["exec-out screencap -p"] = pngBytes(t)
		if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 0 {
			t.Errorf("%s: %+v", c.name, exit)
		}
		if s, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 0 {
			t.Errorf("%s: raw: %+v %q", c.name, exit, s.errOut.String())
		}
	}
	// unsupported reads even with the service bound: the app answers, it is not stuck.
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpNotice] = &UIError{Code: bridge.CodeUnsupported, Text: "old"}
	e.adb.shell["dumpsys accessibility"] = serviceBound
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 0 {
		t.Errorf("unsupported with the service bound: %+v", exit)
	}
	// No app at all.
	e = newEnv(t)
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 0 {
		t.Errorf("no app at all: %+v", exit)
	}
}

func TestStuckAppWithServiceBoundBlocks(t *testing.T) {
	off := &UIError{Code: bridge.CodeUnavailable, Text: "off"}
	bound := func(e *env) { e.adb.shell["dumpsys accessibility"] = serviceBound }
	cannotCheck := func(e *env) { delete(e.adb.shell, "dumpsys accessibility") }
	cases := []struct {
		name  string
		err   error
		setup func(e *env)
	}{
		{"no app, service bound", ErrNoApp, bound},
		{"timeout, service bound", ErrUITimeout, bound},
		{"unavailable, service bound", off, bound},
		{"no app, dumpsys accessibility fails", ErrNoApp, cannotCheck},
		{"unavailable, dumpsys accessibility fails", off, cannotCheck},
		{"timeout", ErrUITimeout, func(e *env) {}},
	}
	for _, c := range cases {
		e := newEnv(t)
		a := e.withApp()
		a.errs[bridge.UIOpNotice] = c.err
		c.setup(e)
		e.adb.out["exec-out screencap -p"] = pngBytes(t)
		if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.HasPrefix(exit.Error, "Blocked by ADBuddy:") {
			t.Errorf("%s: %+v", c.name, exit)
		}
		if _, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 126 {
			t.Errorf("%s: raw: %+v", c.name, exit)
		}
		if screenRead(e) {
			t.Errorf("%s: adb read the screen: %q", c.name, ranOnPhone(e))
		}
	}
}

func TestAppCannotDrawNoticeBlocksTheScreenshot(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpNotice] = &UIError{Code: bridge.CodeFailed, Text: "no overlay"}
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	_, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.HasPrefix(exit.Error, "Blocked by ADBuddy:") {
		t.Fatalf("%+v", exit)
	}
	if _, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 126 {
		t.Fatalf("raw: %+v", exit)
	}
	if screenRead(e) {
		t.Fatalf("adb ran %q", ranOnPhone(e))
	}
}

func TestAppPausedAnswerBlocks(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpNotice] = &UIError{Code: bridge.CodeProtected, Text: "paused"}
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" {
		t.Fatalf("%+v", exit)
	}
	if screenRead(e) {
		t.Fatalf("adb ran %q", ranOnPhone(e))
	}
}

func TestPauseDuringTheNoticeBlocksTheScreenshot(t *testing.T) {
	for _, raw := range []bool{false, true} {
		e := newEnv(t)
		a := e.withApp()
		paused := false
		e.d.o.Paused = func() bool { return paused }
		a.onCall = func(op string) {
			if op == bridge.UIOpNotice {
				paused = true
			}
		}
		e.adb.out["exec-out screencap -p"] = pngBytes(t)
		var code int
		if raw {
			_, exit := e.exec(t, "exec-out", "screencap", "-p")
			code = exit.Code
		} else {
			_, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
			code = exit.Code
		}
		if code != 126 {
			t.Errorf("raw=%v: exit %d", raw, code)
		}
		if screenRead(e) {
			t.Errorf("raw=%v: adb ran %q", raw, ranOnPhone(e))
		}
	}
}

func TestNoticeComesAfterTheGuard(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.example.bank"}
	if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 126 || !strings.Contains(exit.Error, "is a protected app") {
		t.Fatalf("%+v", exit)
	}
	if s, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 126 || !strings.Contains(s.errOut.String(), "is a protected app") {
		t.Fatalf("raw: %+v %q", exit, s.errOut.String())
	}
	if n := noticeCalls(a); n != 0 {
		t.Fatalf("the notice was asked %d times with a protected app in front", n)
	}
}
