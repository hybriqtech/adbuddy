package daemon

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// heldConfirmer does not answer until told: the person is still reading the
// question on the phone.
type heldConfirmer struct {
	asked  chan confirm.Request
	answer chan confirm.Decision
}

func newHeld() *heldConfirmer {
	return &heldConfirmer{asked: make(chan confirm.Request, 4), answer: make(chan confirm.Decision)}
}

func (c *heldConfirmer) Confirm(ctx context.Context, r confirm.Request) confirm.Decision {
	c.asked <- r
	select {
	case d := <-c.answer:
		return d
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return confirm.Timeout
		}
		return confirm.Deny
	}
}

type result struct {
	s    *stream
	exit ipc.Exit
}

// execAsync runs argv in the background, as a second process of the agent.
func (e *env) execAsync(argv ...string) chan result {
	ch := make(chan result, 1)
	go func() {
		s := &stream{}
		exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: argv, Cwd: e.ws, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
		ch <- result{s, exit}
	}()
	return ch
}

func waitResult(t *testing.T, ch chan result) result {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the command did not finish")
	}
	return result{}
}

// entryOf returns the last journal record of id.
func entryOf(t *testing.T, e *env, id string) journal.Entry {
	t.Helper()
	all, _, err := e.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].ID == id {
			return all[i]
		}
	}
	t.Fatalf("no journal entry %s", id)
	return journal.Entry{}
}

// Controller finding: while a 🔴 question waits for an answer, every other
// command of the shim is refused without reaching adb (an `input tap` could
// press Allow on the phone), and undo too. Once answered, they pass again.
func TestCommandsAreBlockedWhileAQuestionWaits(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {} // called from several goroutines here
	held := newHeld()
	e.d.o.Confirmer = held
	red := e.execAsync("shell", "reboot")
	var q confirm.Request
	select {
	case q = <-held.asked:
	case <-time.After(5 * time.Second):
		t.Fatal("the 🔴 was not asked")
	}
	want := `Blocked by ADBuddy: waiting for your answer to "Restart the phone"`

	tap, getprop := e.execAsync("shell", "input", "tap", "1", "1"), e.execAsync("shell", "getprop", "ro.product.model")
	for name, ch := range map[string]chan result{"tap": tap, "getprop": getprop} {
		r := waitResult(t, ch)
		if r.exit.Code != ExitDenied || strings.TrimSpace(r.s.errOut.String()) != want {
			t.Fatalf("%s while a question waits: %+v %q", name, r.exit, r.s.errOut.String())
		}
	}
	another := waitResult(t, e.execAsync("shell", "reboot"))
	if another.exit.Code != ExitDenied || strings.TrimSpace(another.s.errOut.String()) != want {
		t.Fatalf("second 🔴: %+v %q", another.exit, another.s.errOut.String())
	}
	if len(held.asked) != 0 {
		t.Fatal("a second question was asked")
	}
	if ran := e.adb.ran(); len(ran) != 0 {
		t.Fatalf("adb ran while a question waited: %q", ran)
	}
	u := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	if u.Code != ExitDenied || u.ErrorCode != "WAITING_FOR_ANSWER" || !strings.Contains(u.Error, `"Restart the phone"`) {
		t.Fatalf("undo while a question waits: %+v", u)
	}
	// The refused commands are journaled as blocked, with what the
	// classifier says about them.
	all, _, err := e.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	levels := map[string]string{}
	for _, j := range all {
		if j.ID == q.ID {
			continue
		}
		if j.Status != journal.StatusBlocked || j.Rule == "" || j.Description == "" || !strings.Contains(j.Reason, "waiting for your answer") {
			t.Fatalf("refused command journaled as %+v", j)
		}
		levels[j.Rule] = j.Level
	}
	if levels["ui.input"] != "yellow" || levels["sys.getprop"] != "green" || levels["sys.reboot"] != "red" {
		t.Fatalf("levels %v", levels)
	}

	held.answer <- confirm.Allow
	if r := waitResult(t, red); r.exit.Code != 0 {
		t.Fatalf("the allowed 🔴: %+v %q", r.exit, r.s.errOut.String())
	}
	if j := entryOf(t, e, q.ID); j.Status != journal.StatusOK {
		t.Fatalf("the allowed 🔴 journaled as %+v", j)
	}
	for _, argv := range [][]string{{"shell", "input", "tap", "1", "1"}, {"shell", "getprop", "ro.product.model"}} {
		if _, exit := e.exec(t, argv...); exit.Code != 0 {
			t.Fatalf("%q after the answer: %+v", argv, exit)
		}
	}
	// input tap is guarded: the guard reads the foreground first (M5 spec §4.3).
	if ran := e.adb.ran(); strings.Join(ran, "|") != "shell reboot|precheck: dumpsys activity activities|shell input tap 1 1|shell getprop ro.product.model" {
		t.Fatalf("adb ran %q", ran)
	}
}

// A command that got past the door before the question appeared is still
// stopped right before adb, if the question is waiting by then.
func TestACommandInFlightIsStoppedBeforeAdb(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	held := newHeld()
	e.d.o.Confirmer = held
	var red chan result
	// Right before the tap is admitted to run, the 🔴 starts asking.
	e.d.beforeRun = func(argv []string) {
		if len(argv) > 1 && argv[1] == "input" {
			red = e.execAsync("shell", "reboot")
			<-held.asked
		}
	}
	_, exit := e.exec(t, "shell", "input", "tap", "1", "1")
	if exit.Code != ExitDenied {
		t.Fatalf("tap: %+v", exit)
	}
	if ran := e.adb.ran(); len(ran) != 0 {
		t.Fatalf("adb ran: %q", ran)
	}
	held.answer <- confirm.Deny
	waitResult(t, red)
}

// T2 review: pausing from the phone withdraws the question at once; the
// command ends blocked as paused, without waiting for it to expire.
func TestPauseWithdrawsTheQuestion(t *testing.T) {
	e := newEnv(t)
	e.d.o.ConfirmTimeout = time.Hour
	var paused atomic.Bool
	e.d.o.Paused = paused.Load
	held := newHeld()
	e.d.o.Confirmer = held
	red := e.execAsync("shell", "reboot")
	q := <-held.asked
	paused.Store(true)
	e.d.CancelConfirmations()
	r := waitResult(t, red)
	if r.exit.Code != ExitDenied || strings.TrimSpace(r.s.errOut.String()) != "Blocked by ADBuddy: "+PausedMessage {
		t.Fatalf("%+v %q", r.exit, r.s.errOut.String())
	}
	if j := entryOf(t, e, q.ID); j.Status != journal.StatusBlocked || j.Reason != PausedMessage || j.Level != "red" {
		t.Fatalf("journal %+v", j)
	}
	if ran := e.adb.ran(); len(ran) != 0 {
		t.Fatalf("adb ran: %q", ran)
	}
	// Once resumed, the next 🔴 is asked as usual.
	paused.Store(false)
	next := e.execAsync("shell", "reboot")
	<-held.asked
	held.answer <- confirm.Deny
	if r := waitResult(t, next); r.exit.Code != ExitDenied || !strings.Contains(r.s.errOut.String(), "Denied by user") {
		t.Fatalf("after resume: %+v %q", r.exit, r.s.errOut.String())
	}
}

// A pause that lands just before the question is registered: the question
// is never asked.
func TestPauseJustBeforeTheQuestion(t *testing.T) {
	e := newEnv(t)
	e.d.o.ConfirmTimeout = 2 * time.Second
	e.d.o.Paused = pausedAfter(1) // free at the door, paused by the time it asks
	held := newHeld()
	e.d.o.Confirmer = held
	_, exit := e.exec(t, "shell", "reboot")
	if exit.Code != ExitDenied || len(held.asked) != 0 {
		t.Fatalf("%+v asked %d", exit, len(held.asked))
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || j.Reason != PausedMessage {
		t.Fatalf("journal %+v", j)
	}
}

// T3 re-review 2, minor: a pause that withdraws the question on screen ends
// the 🔴 blocked as paused, even if it is resumed before the answer is
// looked at; it is not "denied by user".
func TestAQuestionWithdrawnByAPauseIsPausedEvenIfResumed(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	held := newHeld()
	e.d.o.Confirmer = held
	red := e.execAsync("shell", "reboot")
	asked(t, held)
	e.d.CancelConfirmations() // paused and resumed at once: Paused() is false again
	r := waitResult(t, red)
	if want := "Blocked by ADBuddy: " + PausedMessage; r.exit.Code != ExitDenied || strings.TrimSpace(r.s.errOut.String()) != want {
		t.Fatalf("%+v %q, want %q", r.exit, r.s.errOut.String(), want)
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || j.Reason != PausedMessage {
		t.Fatalf("journal %+v", j)
	}
}

// T3 re-review 2, minor: Ctrl+C on serve while the question is on screen
// is not "denied by user" in the journal; the shim's message stays literal.
func TestServeStoppedWhileAskingIsNotDeniedByUser(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	held := newHeld()
	e.d.o.Confirmer = held
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	red := make(chan result, 1)
	go func() {
		s := &stream{}
		red <- result{s, e.d.Handle(ctx, ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "reboot"}, Cwd: e.ws}, s)}
	}()
	asked(t, held)
	cancel(ipc.ErrClosed) // what the IPC server's Close does
	r := waitResult(t, red)
	if want := "Denied by user: Restart the phone"; r.exit.Code != ExitDenied || strings.TrimSpace(r.s.errOut.String()) != want {
		t.Fatalf("%+v %q, want %q", r.exit, r.s.errOut.String(), want)
	}
	if j := e.last(t); j.Status != journal.StatusDenied || j.Reason != "abandoned: ADBuddy stopped while asking" {
		t.Fatalf("journal %+v", j)
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("ran %q", e.adb.ran())
	}
}
