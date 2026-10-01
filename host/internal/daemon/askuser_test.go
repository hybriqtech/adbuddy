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

type askResult struct {
	d   confirm.Decision
	err error
}

// askAsync asks the user a question of serve's own (`adbuddy remote on`) in
// the background.
func (e *env) askAsync(title string, timeout time.Duration) chan askResult {
	ch := make(chan askResult, 1)
	go func() {
		d, err := e.d.AskUser(context.Background(), confirm.Request{ID: "r_1", Title: title, RawCommand: "adbuddy remote on", Special: true}, timeout)
		ch <- askResult{d, err}
	}()
	return ch
}

func waitAsk(t *testing.T, ch chan askResult) askResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the question did not end")
	}
	return askResult{}
}

const portTitle = "Open adb's network port 5555 for 12 hours"

// Final review C1: a question of serve's own blocks the agent exactly like a
// 🔴 one: an `input tap` could press Allow on the phone.
func TestAskUserBlocksTheAgentWhileItWaits(t *testing.T) {
	for _, answer := range []confirm.Decision{confirm.Allow, confirm.Deny} {
		t.Run(answer.String(), func(t *testing.T) {
			e := newEnv(t)
			e.d.o.OnEntry = func(journal.Entry) {}
			held := newHeld()
			e.d.o.Confirmer = held
			before := time.Now()
			ask := e.askAsync(portTitle, time.Minute)
			q := asked(t, held)
			if q.Title != portTitle || q.Key != "" || !q.Special || q.ID != "r_1" ||
				q.ExpiresAt.Before(before.Add(50*time.Second)) || q.ExpiresAt.After(time.Now().Add(70*time.Second)) {
				t.Fatalf("asked %+v", q)
			}
			want := `Blocked by ADBuddy: waiting for your answer to "` + portTitle + `"`
			for _, argv := range [][]string{{"shell", "input", "tap", "1", "1"}, {"shell", "getprop", "ro.product.model"}, {"shell", "reboot"}} {
				r := waitResult(t, e.execAsync(argv...))
				if r.exit.Code != ExitDenied || strings.TrimSpace(r.s.errOut.String()) != want {
					t.Fatalf("%v while asking: %+v %q", argv, r.exit, r.s.errOut.String())
				}
			}
			if len(held.asked) != 0 {
				t.Fatal("a second question was asked")
			}
			u := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
			if u.Code != ExitDenied || u.ErrorCode != "WAITING_FOR_ANSWER" || !strings.Contains(u.Error, portTitle) {
				t.Fatalf("undo while asking: %+v", u)
			}
			if ran := e.adb.ran(); len(ran) != 0 {
				t.Fatalf("adb ran while asking: %q", ran)
			}
			held.answer <- answer
			if r := waitAsk(t, ask); r.err != nil || r.d != answer {
				t.Fatalf("answer %+v", r)
			}
			// Answered: commands run again, and nothing stays in flight.
			if _, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 0 {
				t.Fatalf("tap after the answer: %+v", exit)
			}
			e.d.askMu.Lock()
			defer e.d.askMu.Unlock()
			if e.d.asking != nil || len(e.d.inFlight) != 0 {
				t.Fatalf("asking %v, in flight %d", e.d.asking, len(e.d.inFlight))
			}
		})
	}
}

// The question expires timeout after it shows up.
func TestAskUserExpires(t *testing.T) {
	e := newEnv(t)
	held := newHeld()
	e.d.o.Confirmer = held
	ask := e.askAsync(portTitle, 50*time.Millisecond)
	asked(t, held)
	if r := waitAsk(t, ask); r.err != nil || r.d != confirm.Timeout {
		t.Fatalf("%+v", r)
	}
	if _, exit := e.exec(t, "shell", "getprop", "ro.product.model"); exit.Code != 0 {
		t.Fatalf("after it expired: %+v", exit)
	}
}

// One question at a time, whoever asks it.
func TestAskUserIsBusyWhileAnotherQuestionWaits(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	held := newHeld()
	e.d.o.Confirmer = held
	red := e.execAsync("shell", "reboot")
	asked(t, held)
	r := waitAsk(t, e.askAsync(portTitle, time.Minute))
	if !errors.Is(r.err, ErrAskBusy) || !strings.Contains(r.err.Error(), "Restart the phone") || r.d == confirm.Allow {
		t.Fatalf("while a 🔴 waits: %+v", r)
	}
	notAsked(t, held, "a 🔴 was waiting")
	held.answer <- confirm.Deny
	waitResult(t, red)

	first := e.askAsync(portTitle, time.Minute)
	asked(t, held)
	r = waitAsk(t, e.askAsync("Another question", time.Minute))
	if !errors.Is(r.err, ErrAskBusy) || !strings.Contains(r.err.Error(), portTitle) {
		t.Fatalf("while another waits: %+v", r)
	}
	notAsked(t, held, "another question was waiting")
	held.answer <- confirm.Allow
	if r := waitAsk(t, first); r.err != nil || r.d != confirm.Allow {
		t.Fatalf("first: %+v", r)
	}
}

// Like a 🔴, the question waits for the changes in flight (a `sleep; input
// tap` already running) before it shows up, and refuses new ones meanwhile.
func TestAskUserWaitsForAChangeInFlight(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	g := newGated(e.adb, "input tap")
	e.d.o.ADB = g
	held := newHeld()
	e.d.o.Confirmer = held
	n := &notices{}
	e.d.o.OnNotice = n.add
	yellow := e.execAsync("shell", "input tap 1 1")
	g.waitStarted(t)
	ask := e.askAsync(portTitle, time.Minute)
	if !n.waitFor(`Waiting for "Interface action: tap 1 1" to finish before asking "` + portTitle + `"`) {
		t.Fatalf("not said: %q", n.get())
	}
	notAsked(t, held, "a change was running")
	r := waitResult(t, e.execAsync("shell", "getprop", "ro.product.model"))
	if r.exit.Code != ExitDenied || !strings.Contains(r.s.errOut.String(), `waiting to ask about "`+portTitle+`"`) {
		t.Fatalf("new command while waiting to ask: %+v %q", r.exit, r.s.errOut.String())
	}
	close(g.open)
	if r := waitResult(t, yellow); r.exit.Code != 0 {
		t.Fatalf("the change in flight: %+v", r.exit)
	}
	asked(t, held)
	held.answer <- confirm.Deny
	if r := waitAsk(t, ask); r.err != nil || r.d != confirm.Deny {
		t.Fatalf("%+v", r)
	}
}

// A pause withdraws the question at once (CancelConfirmations), whatever
// comes after, and a question asked while paused is never shown.
func TestPauseWithdrawsAskUser(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	var paused atomic.Bool
	e.d.o.Paused = paused.Load
	held := newHeld()
	e.d.o.Confirmer = held
	ask := e.askAsync(portTitle, time.Hour)
	asked(t, held)
	paused.Store(true)
	e.d.CancelConfirmations()
	if r := waitAsk(t, ask); !errors.Is(r.err, ErrAskPaused) || r.d == confirm.Allow {
		t.Fatalf("%+v", r)
	}
	// Asked while paused: never shown.
	if r := waitAsk(t, e.askAsync(portTitle, time.Hour)); !errors.Is(r.err, ErrAskPaused) {
		t.Fatalf("while paused: %+v", r)
	}
	notAsked(t, held, "paused")
	// Paused and resumed at once while asking: still withdrawn.
	paused.Store(false)
	ask = e.askAsync(portTitle, time.Hour)
	asked(t, held)
	e.d.CancelConfirmations()
	if r := waitAsk(t, ask); !errors.Is(r.err, ErrAskPaused) {
		t.Fatalf("withdrawn and resumed: %+v", r)
	}
	if _, exit := e.exec(t, "shell", "input", "tap", "1", "1"); exit.Code != 0 {
		t.Fatalf("tap after the pause: %+v", exit)
	}
}
