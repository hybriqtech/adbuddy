package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

func protectSettings(e *env) {
	e.d.o.Protected = func(p string) bool { return p == "com.android.settings" }
}

// lastUndo is the newest undo record of the journal.
func lastUndo(t *testing.T, e *env) journal.Entry {
	t.Helper()
	all, _, err := e.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Source == journal.SourceUndo {
			return all[i]
		}
	}
	t.Fatal("no undo record in the journal")
	return journal.Entry{}
}

// The user's protected apps are not enforced under YOLO. Mutation: in
// guardForeground, turn the `case d.userProtected(pkg) && !d.lift(...)` into
// `case d.userProtected(pkg):` returning the reason.
func TestYoloLiftsTheUsersProtectedApps(t *testing.T) {
	for _, argv := range [][]string{
		{"shell", "input", "tap", "1", "1"},
		{"exec-out", "screencap", "-p"},
	} {
		off, on := yoloPair(t, protectSettings, argv...)
		if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "com.android.settings is a protected app") || len(ranOnPhone(off.e)) != 0 {
			t.Fatalf("%v without YOLO: %+v %q ran %q", argv, off.exit, off.stderr(), ranOnPhone(off.e))
		}
		if on.exit.Code != 0 || !on.warned("com.android.settings is a protected app") || len(ranOnPhone(on.e)) != 1 {
			t.Fatalf("%v with YOLO: %+v %q ran %q", argv, on.exit, on.stderr(), ranOnPhone(on.e))
		}
	}
}

func TestYoloLiftsAProtectedAppInTheOtherHalfOfASplitScreen(t *testing.T) {
	setup := func(e *env) {
		e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n  topResumedActivity=ActivityRecord{1 u0 com.android.settings/.Settings t1}\n    mResumedActivity: ActivityRecord{2 u0 com.example.bank/.Main t2}\n"
		e.d.o.Protected = func(p string) bool { return p == "com.example.bank" }
	}
	off, on := yoloPair(t, setup, "shell", "input", "tap", "1", "1")
	if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "com.example.bank is a protected app") {
		t.Fatalf("without YOLO: %+v %q", off.exit, off.stderr())
	}
	if on.exit.Code != 0 || !on.warned("com.example.bank is a protected app") {
		t.Fatalf("with YOLO: %+v %q", on.exit, on.stderr())
	}
}

// The name of an app that opens a protected one: `am start` and monkey.
func TestYoloLiftsOpeningAProtectedApp(t *testing.T) {
	setup := func(e *env) { e.d.o.Protected = func(p string) bool { return p == "com.example.bank" } }
	argv := []string{"shell", "monkey", "-p", "com.example.bank", "-c", "android.intent.category.LAUNCHER", "1"}
	off, on := yoloPair(t, setup, argv...)
	if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "com.example.bank is a protected app") {
		t.Fatalf("without YOLO: %+v %q", off.exit, off.stderr())
	}
	if on.exit.Code != 0 || !on.warned("com.example.bank is a protected app") || !on.ran("monkey -p com.example.bank") {
		t.Fatalf("with YOLO: %+v %q ran %q", on.exit, on.stderr(), on.e.adb.ran())
	}
}

// The notification shade and system windows can hold a 2FA code: the guard
// blocks without YOLO and lets it through with it.
func TestYoloLiftsTheShade(t *testing.T) {
	off, on := yoloPair(t, func(e *env) { e.adb.shell[fixedFocusCmd] = focusShade }, "shell", "input", "tap", "1", "1")
	if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "the notification shade is open") {
		t.Fatalf("without YOLO: %+v %q", off.exit, off.stderr())
	}
	if on.exit.Code != 0 || !on.warned("the notification shade is open") {
		t.Fatalf("with YOLO: %+v %q", on.exit, on.stderr())
	}
}

// A notice the app cannot show does not stop a read of the screen under YOLO
// (the notice is still asked for, so the person sees it when it can be shown).
func TestYoloLiftsTheNoticeRefusal(t *testing.T) {
	setup := func(e *env) {
		a := e.withApp()
		a.errs[bridge.UIOpNotice] = ErrUITimeout
	}
	off, on := yoloPair(t, setup, "exec-out", "screencap", "-p")
	if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "did not answer in time") {
		t.Fatalf("without YOLO: %+v %q", off.exit, off.stderr())
	}
	if on.exit.Code != 0 || !on.warned("did not answer in time") || !on.ran("exec-out screencap -p") {
		t.Fatalf("with YOLO: %+v %q ran %q", on.exit, on.stderr(), on.e.adb.ran())
	}
}

// ADBuddy's own screen stays protected: otherwise the agent could tap its way
// to the YOLO switch or the pause. Mutation: in guardForeground, make the
// `pkg == uidump.AppPackage && pkg == f.first()` case lift.
func TestYoloKeepsADBuddyProtected(t *testing.T) {
	for _, app := range []bool{false, true} {
		for _, argv := range [][]string{
			{"shell", "input", "tap", "1", "1"},
			{"exec-out", "screencap", "-p"},
			{"shell", "input", "keyevent", "66"},
		} {
			e := newEnv(t)
			withADBuddyInFront(e, app)
			e.yoloOn(t)
			s, exit := e.exec(t, argv...)
			if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), fromADBuddy) || len(ranOnPhone(e)) != 0 {
				t.Errorf("app %v %v: %+v %q ran %q", app, argv, exit, s.errOut.String(), ranOnPhone(e))
			}
		}
	}
}

func TestYoloKeepsADBuddyProtectedInASplitScreen(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["dumpsys activity activities"] = "ACTIVITY MANAGER ACTIVITIES\n  topResumedActivity=ActivityRecord{1 u0 com.android.settings/.Settings t1}\n    mResumedActivity: ActivityRecord{2 u0 tech.hybriq.adbuddy/.MainActivity t2}\n"
	e.yoloOn(t)
	s, exit := e.exec(t, "shell", "input", "tap", "1", "1")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "tech.hybriq.adbuddy is a protected app") {
		t.Fatalf("%+v %q", exit, s.errOut.String())
	}
}

// What stops a command from opening ADBuddy behind the guard's back stays.
func TestYoloKeepsTheAloneRuleForScreenCommands(t *testing.T) {
	e := newEnv(t)
	e.yoloOn(t)
	s, exit := e.exec(t, "shell", "input tap 1 1; input tap 2 2")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "must run alone") || len(ranOnPhone(e)) != 0 {
		t.Fatalf("%+v %q ran %q", exit, s.errOut.String(), ranOnPhone(e))
	}
}

// Not knowing what is in front, ADBuddy could be: it still fails closed.
func TestYoloDoesNotGuessWhatIsInFront(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["dumpsys activity activities"] = "garbage"
	e.yoloOn(t)
	s, exit := e.exec(t, "shell", "input", "tap", "1", "1")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "could not tell which app is in the foreground") {
		t.Fatalf("%+v %q", exit, s.errOut.String())
	}
}

// The helpers (adbuddy tap, type, screen, screenshot) go through the same guard.
func TestYoloHelpersActInAProtectedApp(t *testing.T) {
	var offApp, onApp *fakeApp
	var off, on struct {
		s    *stream
		exit ipc.Exit
	}
	for _, yolo := range []bool{false, true} {
		e := newEnv(t)
		a := e.withApp("com.example.bank")
		// The app marks the bank as protected, except under YOLO with the link up (task A5): then it stops.
		a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.example.bank", Protected: !yolo}
		a.results[bridge.UIOpClick] = bridge.UIClicked{Clicked: true, Bounds: [4]int{1, 1, 2, 2}, How: "gesture"}
		if yolo {
			e.yoloOn(t)
		}
		s, exit := e.helper(t, bridge.UIOpClick, xy(1, 1), "adbuddy", "tap", "--xy", "1", "1")
		if yolo {
			on.s, on.exit, onApp = s, exit, a
		} else {
			off.s, off.exit, offApp = s, exit, a
		}
	}
	if off.exit.Code != ExitDenied || off.exit.ErrorCode != "UI_BLOCKED" || strings.Contains(strings.Join(offApp.called(), "|"), "click") {
		t.Fatalf("without YOLO: %+v calls %q", off.exit, offApp.called())
	}
	if on.exit.Code != 0 || !strings.Contains(on.s.errOut.String(), "⚠ YOLO: com.example.bank is a protected app") || !strings.Contains(strings.Join(onApp.called(), "|"), "click") {
		t.Fatalf("with YOLO: %+v %q calls %q", on.exit, on.s.errOut.String(), onApp.called())
	}
}

func TestYoloHelpersKeepADBuddyProtected(t *testing.T) {
	e := newEnv(t)
	withADBuddyInFront(e, true)
	e.yoloOn(t)
	_, exit := e.helper(t, bridge.UIOpClick, xy(1, 1), "adbuddy", "tap", "--xy", "1", "1")
	if exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" {
		t.Fatalf("%+v", exit)
	}
}

// What the app itself marks as protected keeps blocking under YOLO: ADBuddy in
// the other half of a split screen reaches the host only as that flag (the app
// says one package), and a window the phone cannot read could be anything.
// Mutation: in guardForeground, add `&& !d.lift(ctx, screenProtectedReason(f.first()))`
// to the `f.appProtected` condition.
func TestYoloKeepsWhatTheAppMarksAsProtected(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpForeground] = bridge.UIForeground{Package: "com.android.settings", Protected: true}
	e.yoloOn(t)
	for _, argv := range [][]string{{"shell", "input", "tap", "1", "1"}, {"exec-out", "screencap", "-p"}} {
		s, exit := e.exec(t, argv...)
		if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "something on the screen is protected") || len(ranOnPhone(e)) != 0 {
			t.Fatalf("%v: %+v %q ran %q", argv, exit, s.errOut.String(), ranOnPhone(e))
		}
	}
	if _, exit := e.helper(t, bridge.UIOpClick, xy(1, 1), "adbuddy", "tap", "--xy", "1", "1"); exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" {
		t.Fatalf("helper: %+v", exit)
	}
}

// A question of serve's own keeps the helpers and undo refused too (see
// TestYoloKeepsTheDoorsClosedWhileServesOwnQuestionWaits).
func TestYoloKeepsHelpersAndUndoClosedWhileServesOwnQuestionWaits(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	e.withApp().results[bridge.UIOpClick] = bridge.UIClicked{Clicked: true, Bounds: [4]int{1, 1, 2, 2}, How: "gesture"}
	e.forge(t, "j_f1", putArgv, "shell", "settings", "put", "global", "a", "1")
	held := newHeld()
	e.d.o.Confirmer = held
	e.yoloOn(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.d.AskUser(context.Background(), confirm.Request{ID: "q", Title: "Open port 5555"}, 0)
	}()
	asked(t, held)
	if _, exit := e.helper(t, bridge.UIOpClick, xy(1, 1), "adbuddy", "tap", "--xy", "1", "1"); exit.Code != ExitDenied || exit.ErrorCode != "UI_BLOCKED" {
		t.Fatalf("helper: %+v", exit)
	}
	if _, exit, _ := e.undoLast(t); exit.Code != ExitDenied || exit.ErrorCode != "WAITING_FOR_ANSWER" {
		t.Fatalf("undo: %+v", exit)
	}
	held.answer <- confirm.Deny
	<-done
}

// The helpers that change something do not wait for a question either, and
// run without a journal (they are told).
func TestYoloHelpersRunWithAQuestionWaitingAndNoJournal(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	a := e.withApp()
	a.results[bridge.UIOpClick] = bridge.UIClicked{Clicked: true, Bounds: [4]int{1, 1, 2, 2}, How: "gesture"}
	held := newHeld()
	e.d.o.Confirmer = held
	red := e.execAsync("shell", "reboot")
	asked(t, held)
	if _, exit := e.helper(t, bridge.UIOpClick, xy(1, 1), "adbuddy", "tap", "--xy", "1", "1"); exit.Code != ExitDenied {
		t.Fatalf("without YOLO the tap went through a waiting question: %+v", exit)
	}
	e.yoloOn(t)
	breakJournal(t)(e)
	s, exit := e.helper(t, bridge.UIOpClick, xy(1, 1), "adbuddy", "tap", "--xy", "1", "1")
	if exit.Code != 0 || !strings.Contains(s.errOut.String(), "a question was waiting") || !strings.Contains(s.errOut.String(), "without a record") {
		t.Fatalf("with YOLO: %+v %q", exit, s.errOut.String())
	}
	held.answer <- confirm.Deny
	waitResult(t, red)
}

// The notifications of the protected apps are shown under YOLO; ADBuddy's never.
func TestYoloShowsTheNotificationsOfProtectedApps(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	a.results[bridge.UIOpNotifications] = bridge.UINotifications{Notifications: []bridge.UINotification{
		{Key: "k1", Package: "com.example.bank", Title: "Bank", Text: bankCode, PostedAt: posted},
		{Key: "k2", Package: "tech.hybriq.adbuddy", Title: "The agent wants to", Text: "adb shell reboot", PostedAt: posted},
	}}
	read := func() bridge.UINotifications {
		s, exit := e.helper(t, bridge.UIOpNotifications, struct{}{}, "adbuddy", "notifications")
		if exit.Code != 0 || len(s.data) != 1 {
			t.Fatalf("%+v %q", exit, s.data)
		}
		var n bridge.UINotifications
		if err := json.Unmarshal(s.data[0], &n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := read(); n.Notifications[0].Text != "" {
		t.Fatalf("without YOLO the bank's text came out: %+v", n.Notifications[0])
	}
	e.yoloOn(t)
	n := read()
	if n.Notifications[0].Text != bankCode {
		t.Fatalf("with YOLO the bank's text is hidden: %+v", n.Notifications[0])
	}
	if n.Notifications[1].Text != "" || n.Notifications[1].Title != "" {
		t.Fatalf("ADBuddy's notification came out: %+v", n.Notifications[1])
	}
}

// An undo whose inverse is 🔴 runs without asking.
func TestYoloUndoRunsARedInverse(t *testing.T) {
	delInv := []string{"shell", "settings", "delete", "global", "a"}
	for _, yolo := range []bool{false, true} {
		e := newEnv(t)
		e.conf.decision = confirm.Deny
		e.forge(t, "j_f1", putArgv, delInv...)
		if yolo {
			e.yoloOn(t)
		}
		res, exit, s := e.undoLast(t)
		if !yolo {
			if len(e.conf.asked) != 1 || e.ranContains("settings delete global a") || exit.Code != ExitDenied {
				t.Fatalf("without YOLO: asked %d ran %q %+v", len(e.conf.asked), e.adb.ran(), exit)
			}
			continue
		}
		if len(e.conf.asked) != 0 || !e.ranContains("settings delete global a") || exit.Code != 0 || len(res.Undone) != 1 {
			t.Fatalf("with YOLO: asked %d ran %q %+v %+v", len(e.conf.asked), e.adb.ran(), exit, res)
		}
		if !strings.Contains(s.errOut.String(), "⚠ YOLO:") {
			t.Fatalf("the agent was not told: %q", s.errOut.String())
		}
		if u := lastUndo(t, e); u.Level != "red" || u.Rule != "undo-yolo" || !u.Yolo {
			t.Fatalf("undo entry %+v", u)
		}
	}
}

// serve's own questions (`adbuddy remote on`) are the person's, not the
// agent's: YOLO does not answer them. Mutation: lift in AskUser.
func TestYoloDoesNotSkipServesOwnQuestions(t *testing.T) {
	e := newEnv(t)
	held := newHeld()
	e.d.o.Confirmer = held
	e.yoloOn(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.d.AskUser(context.Background(), confirm.Request{ID: "q", Title: "Open port 5555"}, 0)
	}()
	asked(t, held)
	held.answer <- confirm.Deny
	<-done
}
