package daemon

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// The notice is taken off the screen while adbuddy screenshot captures it
// (plan 2026-10-12-p1-pantalla-oculta, T3).

type appCall struct {
	op  string
	adb int // adb commands that had run when the op reached the app
}

// noticeOps records, in order, the notice and hideNotice ops the app gets.
func noticeOps(e *env, a *fakeApp) *[]appCall {
	var order []appCall
	a.onCall = func(op string) {
		if op == bridge.UIOpNotice || op == bridge.UIOpHideNotice {
			order = append(order, appCall{op, len(ranOnPhone(e))})
		}
	}
	return &order
}

func resultOf(t *testing.T, s *stream) ScreenshotResult {
	t.Helper()
	if len(s.data) != 1 {
		t.Fatalf("data %q", s.data)
	}
	var r ScreenshotResult
	if err := json.Unmarshal(s.data[0], &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestScreenshotTakesTheNoticeOffForTheCapture(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	order := noticeOps(e, a)
	s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	// notice and hideNotice before screencap, the notice again right after it.
	want := []appCall{{bridge.UIOpNotice, 0}, {bridge.UIOpHideNotice, 0}, {bridge.UIOpNotice, 1}}
	if len(*order) != len(want) {
		t.Fatalf("ops %+v, want %+v", *order, want)
	}
	for i := range want {
		if (*order)[i] != want[i] {
			t.Fatalf("ops %+v, want %+v", *order, want)
		}
	}
	var hide string
	for _, c := range a.called() {
		if strings.HasPrefix(c, bridge.UIOpHideNotice+" ") {
			hide = c
		}
	}
	if hide != `hideNotice {"ms":2000}` {
		t.Fatalf("hideNotice call %q", hide)
	}
	if r := resultOf(t, s); r.NoticeInImage {
		t.Fatalf("result %+v: the notice was taken off, nothing to tell", r)
	}
}

func TestScreenshotWithoutTheVeilSaysSo(t *testing.T) {
	for _, c := range []struct {
		name     string
		err      error
		inImage  bool // the notice may be in the image
		restored bool // the notice was asked again after the capture
	}{
		{"an older app", &UIError{Code: bridge.CodeUnsupported, Text: "old"}, true, false},
		{"the app failed", &UIError{Code: bridge.CodeFailed, Text: "no"}, true, true},
		{"the app hung", ErrUITimeout, true, true},
		{"accessibility off", &UIError{Code: bridge.CodeUnavailable, Text: "off"}, false, false},
		{"no app", ErrNoApp, false, false},
	} {
		e := newEnv(t)
		a := e.withApp()
		a.errs[bridge.UIOpHideNotice] = c.err
		e.adb.out["exec-out screencap -p"] = pngBytes(t)
		s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
		if exit.Code != 0 || !screenRead(e) {
			t.Errorf("%s: exit %+v, the capture must go on", c.name, exit)
			continue
		}
		if r := resultOf(t, s); r.NoticeInImage != c.inImage {
			t.Errorf("%s: noticeInImage %v, want %v", c.name, r.NoticeInImage, c.inImage)
		}
		want := 1
		if c.restored {
			want = 2
		}
		if n := noticeCalls(a); n != want {
			t.Errorf("%s: the notice was asked %d times, want %d", c.name, n, want)
		}
	}
}

func TestScreenshotWithNoAppHasNoNoticeToTellOf(t *testing.T) {
	e := newEnv(t)
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	if r := resultOf(t, s); r.NoticeInImage {
		t.Fatalf("result %+v", r)
	}
}

// A pause stops the capture, as it stops any read, wherever it is found: the
// app says so, or the host knows once the notice is off.
func TestScreenshotPausedWhileTheNoticeComesOffIsBlocked(t *testing.T) {
	for _, how := range []string{"the app says so", "the host knows"} {
		e := newEnv(t)
		a := e.withApp()
		var paused atomic.Bool
		e.d.o.Paused = func() bool { return paused.Load() }
		e.adb.out["exec-out screencap -p"] = pngBytes(t)
		if how == "the app says so" {
			a.errs[bridge.UIOpHideNotice] = &UIError{Code: bridge.CodeProtected, Text: "The agent is paused from the phone."}
		} else {
			a.onCall = func(op string) {
				if op == bridge.UIOpHideNotice {
					paused.Store(true)
				}
			}
		}
		s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
		if exit.Code != ExitDenied {
			t.Errorf("%s: exit %+v", how, exit)
		}
		if screenRead(e) || s.out.Len() != 0 {
			t.Errorf("%s: adb ran %q, %d bytes out", how, ranOnPhone(e), s.out.Len())
		}
		if n := noticeCalls(a); n != 1 {
			t.Errorf("%s: the notice was asked %d times: a paused session gets no notice back", how, n)
		}
	}
}

// A slow veil leaves the guard old: a protected app that came to the front
// meanwhile stops the capture, and the notice is put back.
func TestScreenshotLooksAtTheGuardAgainAfterASlowVeil(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	a.onCall = func(op string) {
		if op == bridge.UIOpHideNotice {
			time.Sleep(baselineSlack + 100*time.Millisecond)
			a.mu.Lock()
			a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.example.bank"}
			a.mu.Unlock()
		}
	}
	s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "is a protected app") {
		t.Fatalf("exit %+v", exit)
	}
	if screenRead(e) || s.out.Len() != 0 {
		t.Fatalf("adb ran %q, %d bytes out", ranOnPhone(e), s.out.Len())
	}
	if n := noticeCalls(a); n != 2 {
		t.Fatalf("the notice was asked %d times, want 2 (it is put back)", n)
	}
}

// hideNotice is for screenshot only: the agent cannot ask for the notice to
// go, and then act.
func TestTheAgentCannotAskToHideTheNotice(t *testing.T) {
	if _, ok := helpers[bridge.UIOpHideNotice]; ok {
		t.Fatal("hideNotice is a helper")
	}
	e := newEnv(t)
	a := e.withApp()
	_, exit := e.helper(t, bridge.UIOpHideNotice, bridge.UIHideNoticeArgs{Ms: 2000})
	if exit.ErrorCode != "UI_INVALID" {
		t.Fatalf("exit %+v", exit)
	}
	for _, c := range a.called() {
		if strings.HasPrefix(c, bridge.UIOpHideNotice) {
			t.Fatalf("the app got %q", c)
		}
	}
}

// The agent's own raw reads keep the notice on the screen (and in the image):
// only adbuddy screenshot takes it off.
func TestRawReadsDoNotHideTheNotice(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	order := noticeOps(e, a)
	if s, exit := e.exec(t, "exec-out", "screencap", "-p"); exit.Code != 0 {
		t.Fatalf("exit %+v %q", exit, s.errOut.String())
	}
	if len(*order) != 1 || (*order)[0].op != bridge.UIOpNotice {
		t.Fatalf("ops %+v, want only the notice", *order)
	}
}

// The guard's age counts from when it said yes, notice included: a slow
// notice (not a slow veil) leaves the guard old too.
func TestScreenshotLooksAtTheGuardAgainAfterASlowNotice(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	slow := true
	a.onCall = func(op string) {
		if op == bridge.UIOpNotice && slow {
			slow = false
			time.Sleep(baselineSlack + 100*time.Millisecond)
			a.mu.Lock()
			a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.example.bank"}
			a.mu.Unlock()
		}
	}
	s, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot))
	if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "is a protected app") {
		t.Fatalf("exit %+v", exit)
	}
	if screenRead(e) || s.out.Len() != 0 {
		t.Fatalf("adb ran %q, %d bytes out", ranOnPhone(e), s.out.Len())
	}
}
