package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// yoloOn turns YOLO on until the end of the test's session.
func (e *env) yoloOn(t *testing.T) {
	t.Helper()
	e.d.Yolo().EnableSession()
}

// yoloRun is one command run in its own environment.
type yoloRun struct {
	e    *env
	s    *stream
	exit ipc.Exit
}

// yoloPair runs argv twice, each in a fresh environment prepared by setup:
// without YOLO (the behaviour of today) and with it.
func yoloPair(t *testing.T, setup func(*env), argv ...string) (off, on yoloRun) {
	t.Helper()
	for _, yolo := range []bool{false, true} {
		e := newEnv(t)
		if setup != nil {
			setup(e)
		}
		if yolo {
			e.yoloOn(t)
		}
		s, exit := e.exec(t, argv...)
		if yolo {
			on = yoloRun{e, s, exit}
		} else {
			off = yoloRun{e, s, exit}
		}
	}
	return off, on
}

func (r yoloRun) ran(sub string) bool { return r.e.ranContains(sub) }

// warned reports whether the agent was told, on stderr, what YOLO let through.
func (r yoloRun) warned(sub string) bool {
	out := r.s.errOut.String()
	return strings.Contains(out, "⚠ YOLO: ") && strings.Contains(out, sub)
}

func (r yoloRun) stderr() string { return r.s.errOut.String() }

// Mutation (each case): in exec, make `redLifted` false.
func TestYoloRunsARedWithoutAsking(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		ran  string
	}{
		{"red", []string{"shell", "rm", "/sdcard/x"}, "shell rm /sdcard/x"},
		{"special", []string{"shell", "reboot"}, "shell reboot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			off, on := yoloPair(t, func(e *env) { e.conf.decision = confirm.Deny }, tc.argv...)
			if off.exit.Code != ExitDenied || len(off.e.conf.asked) != 1 || off.ran(tc.ran) {
				t.Fatalf("without YOLO: %+v asked %d ran %q", off.exit, len(off.e.conf.asked), off.e.adb.ran())
			}
			if on.exit.Code != 0 || len(on.e.conf.asked) != 0 || !on.ran(tc.ran) || !on.warned("asked for your confirmation") {
				t.Fatalf("with YOLO: %+v asked %d ran %q stderr %q", on.exit, len(on.e.conf.asked), on.e.adb.ran(), on.stderr())
			}
			if j := on.e.last(t); j.Status != journal.StatusOK || !j.Yolo || j.Level != "red" || j.Source != journal.SourceAgent {
				t.Fatalf("journal %+v", j)
			}
		})
	}
}

// The precheck raises it to 🔴 (the package is not a system app).
func TestYoloRunsWhatThePrecheckRaisesToRed(t *testing.T) {
	setup := func(e *env) {
		e.conf.decision = confirm.Deny
		e.adb.shell["pm list packages -s com.user"] = ""
	}
	off, on := yoloPair(t, setup, "shell", "pm", "uninstall", "--user", "0", "com.user")
	if off.exit.Code != ExitDenied || len(off.e.conf.asked) != 1 {
		t.Fatalf("without YOLO: %+v asked %d", off.exit, len(off.e.conf.asked))
	}
	if on.exit.Code != 0 || len(on.e.conf.asked) != 0 || !on.ran("pm uninstall --user 0 com.user") || !on.warned("asked for your confirmation") {
		t.Fatalf("with YOLO: %+v asked %d ran %q stderr %q", on.exit, len(on.e.conf.asked), on.e.adb.ran(), on.stderr())
	}
}

// Another question waiting no longer stops commands. Mutation: remove
// `&& !d.lift(...)` from the waitingFor gate (and, for the second case, the
// lift in the admit gate).
func TestYoloDoesNotWaitForAnotherQuestion(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"green", []string{"shell", "getprop", "ro.product.model"}},
		{"yellow", []string{"shell", "settings", "put", "global", "k", "2"}},
	} {
		for _, yolo := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/off", true: "/on"}[yolo], func(t *testing.T) {
				e := newEnv(t)
				e.d.o.OnEntry = func(journal.Entry) {}
				e.adb.shell["settings get global k"] = "1\n"
				held := newHeld()
				e.d.o.Confirmer = held
				red := e.execAsync("shell", "reboot") // asks, and waits for the answer
				asked(t, held)
				if yolo {
					e.yoloOnKeepingQuestion(t)
				}
				s, exit := e.exec(t, tc.argv...)
				out := s.errOut.String()
				if yolo {
					if exit.Code != 0 || !strings.Contains(out, "⚠ YOLO: a question was waiting for your answer") {
						t.Fatalf("with YOLO: %+v %q", exit, out)
					}
				} else if exit.Code != ExitDenied || !strings.Contains(out, `waiting for your answer to "Restart the phone"`) {
					t.Fatalf("without YOLO: %+v %q", exit, out)
				}
				held.answer <- confirm.Deny
				waitResult(t, red)
			})
		}
	}
}

// A question of serve's own (adbuddy remote on) is the person's: while it waits
// the agent's commands stay refused even under YOLO (an `input tap` could press
// Allow on the phone and approve it for them). Mutation: in liftQuestion, remove
// the `if d.waitingOnOwn() { return false }`.
func TestYoloKeepsTheDoorsClosedWhileServesOwnQuestionWaits(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	held := newHeld()
	e.d.o.Confirmer = held
	e.yoloOn(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.d.AskUser(context.Background(), confirm.Request{ID: "q", Title: "Open port 5555"}, 0)
	}()
	asked(t, held)
	for _, argv := range [][]string{
		{"shell", "input", "tap", "1", "1"},
		{"shell", "getprop", "ro.product.model"},
		{"shell", "settings", "put", "global", "k", "2"},
		{"shell", "rm", "/sdcard/x"},
	} {
		s, exit := e.exec(t, argv...)
		if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), `waiting for your answer to "Open port 5555"`) {
			t.Fatalf("%q under YOLO while serve's question waits: %+v %q", argv, exit, s.errOut.String())
		}
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("adb ran %q while the person's question waited", e.adb.ran())
	}
	held.answer <- confirm.Deny
	<-done
}

// A 🔴 that YOLO runs is a change in flight: a question of serve's own (remote
// on) waits for it, as it waits for any change. Mutation: in exec, make the
// admit condition `if level != classify.Red {` (without `|| redLifted`).
func TestYoloRedStillCountsAsAChangeInFlight(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	g := newGated(e.adb, "reboot")
	e.d.o.ADB = g
	held := newHeld()
	e.d.o.Confirmer = held
	e.yoloOn(t)
	red := e.execAsync("shell", "reboot") // runs without asking, and takes its time on the phone
	g.waitStarted(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.d.AskUser(context.Background(), confirm.Request{ID: "q", Title: "a question of serve's own"}, 0)
	}()
	notAsked(t, held, "a change was running")
	close(g.open)
	if r := waitResult(t, red); r.exit.Code != 0 {
		t.Fatalf("the 🔴 under YOLO: %+v", r.exit)
	}
	asked(t, held)
	held.answer <- confirm.Deny
	<-done
}

// adb options that reach another server are dropped, not passed on: the command
// runs on the active phone through ADBuddy's own adb server.
func TestYoloDropsAForbiddenOptionAndRuns(t *testing.T) {
	off, on := yoloPair(t, nil, "-a", "shell", "ls")
	if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "is not allowed") || len(off.e.adb.ran()) != 0 {
		t.Fatalf("without YOLO: %+v %q", off.exit, off.stderr())
	}
	if on.exit.Code != 0 || !on.warned("-a") || !on.ran("shell ls") || on.ran("-a") {
		t.Fatalf("with YOLO: %+v %q ran %q", on.exit, on.stderr(), on.e.adb.ran())
	}
}

// The phone named is not the active one: the host's adb only reaches the
// active phone, so that is where it runs (and the agent is told).
func TestYoloRunsOnTheActivePhoneWhenAnotherIsNamed(t *testing.T) {
	off, on := yoloPair(t, nil, "-s", "other", "shell", "ls")
	if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "only the active phone can be used") {
		t.Fatalf("without YOLO: %+v %q", off.exit, off.stderr())
	}
	if on.exit.Code != 0 || !on.warned("other") || !on.warned("ran on the active phone") || !on.ran("shell ls") || on.ran("other") {
		t.Fatalf("with YOLO: %+v %q ran %q", on.exit, on.stderr(), on.e.adb.ran())
	}
}

// breakJournal makes the journal directory a file, so nothing can be written.
func breakJournal(t *testing.T) func(*env) {
	return func(e *env) {
		t.Helper()
		// The folder may exist already (earlier records): it goes, and a file takes its place.
		if err := os.RemoveAll(e.store.Dir()); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(e.store.Dir()), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(e.store.Dir(), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		e.adb.shell["settings get global k"] = "1\n"
	}
}

// With no journal the command runs anyway and the agent is told.
func TestYoloRunsWhenTheJournalCannotBeWritten(t *testing.T) {
	off, on := yoloPair(t, breakJournal(t), "shell", "settings", "put", "global", "k", "2")
	if off.exit.Code != ExitDenied || !strings.Contains(off.stderr(), "the journal cannot be written") || off.ran("settings put") {
		t.Fatalf("without YOLO: %+v %q", off.exit, off.stderr())
	}
	if on.exit.Code != 0 || !on.ran("settings put global k 2") || !on.warned("without a record") {
		t.Fatalf("with YOLO: %+v %q ran %q", on.exit, on.stderr(), on.e.adb.ran())
	}
	if n := strings.Count(on.stderr(), "⚠ YOLO:"); n != 1 {
		t.Fatalf("the same warning %d times: %q", n, on.stderr())
	}
}

// The pause is never skipped: it blocks, is not asked about, and turns YOLO off.
// Mutation: in exec, check the pause after the red gate.
func TestYoloKeepsThePause(t *testing.T) {
	e := newEnv(t)
	e.yoloOn(t)
	e.d.o.Paused = func() bool { return true }
	s, exit := e.exec(t, "shell", "rm", "/sdcard/x")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "paused from the phone") || len(e.conf.asked) != 0 || len(e.adb.ran()) != 0 {
		t.Fatalf("exit %+v %q asked %d ran %q", exit, s.errOut.String(), len(e.conf.asked), e.adb.ran())
	}
	if e.d.Yolo().Active() {
		t.Fatal("YOLO survived the pause")
	}
	// The journal is ordered by time: the "yolo.off" event the pause wrote came after the command's entry, so
	// the blocked record is looked for by its status, not taken as the last.
	all, _, err := e.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	var blocked []journal.Entry
	for _, j := range all {
		if j.Status == journal.StatusBlocked {
			blocked = append(blocked, j)
		}
	}
	if len(blocked) != 1 || !blocked[0].Yolo {
		t.Fatalf("blocked records %+v (the entry says YOLO was on when it started)", blocked)
	}
}

func TestYoloMarksEveryJournalEntry(t *testing.T) {
	off, on := yoloPair(t, nil, "shell", "getprop", "ro.product.model")
	if off.e.last(t).Yolo {
		t.Fatal("an entry without YOLO says it had it")
	}
	if !on.e.last(t).Yolo {
		t.Fatal("an entry under YOLO does not say so")
	}
	if on.stderr() != "" {
		t.Fatalf("a green command, which nothing skipped, was told: %q", on.stderr())
	}
}

// Off again, everything is as before.
func TestYoloOffBringsTheDoorsBack(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Deny
	e.yoloOn(t)
	if _, exit := e.exec(t, "shell", "rm", "/sdcard/x"); exit.Code != 0 {
		t.Fatalf("with YOLO: %+v", exit)
	}
	e.d.Yolo().Off(YoloOffApp)
	if _, exit := e.exec(t, "shell", "rm", "/sdcard/y"); exit.Code != ExitDenied || len(e.conf.asked) != 1 {
		t.Fatalf("after off: %+v asked %d", exit, len(e.conf.asked))
	}
}

// yoloOnKeepingQuestion turns YOLO on with a question of the agent still
// waiting. Turning YOLO on withdraws that question (yoloWithdraw), so this uses
// a YOLO with no observers: it is the state the doors see when a question
// registers right after YOLO began, which is what these tests are about.
func (e *env) yoloOnKeepingQuestion(t *testing.T) {
	t.Helper()
	e.d.o.Yolo = NewYolo(nil, nil)
	e.d.o.Yolo.EnableSession()
}
