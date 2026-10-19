package daemon

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// Task 7b: turning YOLO on takes away the 🔴 question that was waiting, and
// nothing answered on the phone while YOLO is on leaves a permit behind.

// phoneApp is a connected app that keeps the questions it is shown.
type phoneApp struct {
	reqs    chan bridge.ConfirmRequest
	notices atomic.Int32
}

func (a *phoneApp) Connected() bool { return true }
func (a *phoneApp) Publish(typ string, payload any) error {
	switch p := payload.(type) {
	case bridge.ConfirmRequest:
		a.reqs <- p
	case bridge.Notice:
		a.notices.Add(1)
	}
	return nil
}

func (a *phoneApp) next(t *testing.T) bridge.ConfirmRequest {
	t.Helper()
	select {
	case r := <-a.reqs:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the phone was never asked")
	}
	return bridge.ConfirmRequest{}
}

func (a *phoneApp) notAsked(t *testing.T) {
	t.Helper()
	select {
	case r := <-a.reqs:
		t.Fatalf("the phone was asked %q", r.Title)
	case <-time.After(300 * time.Millisecond):
	}
}

// phoneConfirmer wires a real bridge.Confirmer to the daemon the way serve
// does (cli/phone_yolo.go): it asks YOLO, and hears of every change.
func (e *env) phoneConfirmer(t *testing.T) (*bridge.Confirmer, *phoneApp) {
	t.Helper()
	e.d.o.OnEntry = func(journal.Entry) {} // several goroutines
	app := &phoneApp{reqs: make(chan bridge.ConfirmRequest, 64)}
	c := bridge.NewConfirmer(app, &fakeConfirmer{decision: confirm.Deny})
	e.d.o.Confirmer = c
	c.SetYoloActive(e.d.Yolo().Active)
	e.d.Yolo().Subscribe(func(ch YoloChange) {
		if ch.State.Active {
			c.YoloStarted()
		} else {
			c.YoloEnded()
		}
	})
	return c, app
}

func answerPhone(c *bridge.Confirmer, id, decision string) {
	raw, _ := json.Marshal(bridge.ConfirmReply{ID: id, Decision: decision})
	c.Reply(raw)
}

const redRm = "/sdcard/x"

// lastAgentEntry is the last record of the agent's commands (not of YOLO's own events).
func lastAgentEntry(t *testing.T, e *env) journal.Entry {
	t.Helper()
	all, _, err := e.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Source == journal.SourceAgent {
			return all[i]
		}
	}
	t.Fatal("no record of the agent")
	return journal.Entry{}
}

// Mutations: remove `o.Yolo.Subscribe(d.yoloWithdraw)` from New (the command
// then waits for an answer that never comes); remove `c.YoloEnded()` from
// phoneConfirmer's observer is not needed here: the permit is refused already.
func TestYoloOnWithdrawsThePendingQuestionAndNothingIsKept(t *testing.T) {
	// Without YOLO: allow_session runs the command and is kept (today's behaviour).
	t.Run("without YOLO", func(t *testing.T) {
		e := newEnv(t)
		c, app := e.phoneConfirmer(t)
		res := e.execAsync("shell", "rm", redRm)
		answerPhone(c, app.next(t).ID, "allow_session")
		if r := waitResult(t, res); r.exit.Code != 0 || !e.ranContains("rm "+redRm) {
			t.Fatalf("%+v ran %q", r.exit, e.adb.ran())
		}
		if r := waitResult(t, e.execAsync("shell", "rm", redRm)); r.exit.Code != 0 {
			t.Fatalf("the kept permit was not used: %+v", r.exit)
		}
		app.notAsked(t)
	})
	t.Run("with YOLO", func(t *testing.T) {
		e := newEnv(t)
		c, app := e.phoneConfirmer(t)
		res := e.execAsync("shell", "rm", redRm)
		q := app.next(t)
		e.d.Yolo().EnableSession()
		r := waitResult(t, res)
		if r.exit.Code != ExitDenied || e.ranContains("rm "+redRm) {
			t.Fatalf("%+v ran %q", r.exit, e.adb.ran())
		}
		if out := r.s.errOut.String(); !strings.Contains(out, YoloWithdrawnMessage) || strings.Contains(out, "paused") {
			t.Fatalf("stderr %q", out)
		}
		if j := lastAgentEntry(t, e); j.Status != journal.StatusBlocked || j.Reason != YoloWithdrawnMessage || j.Level != "red" {
			t.Fatalf("journal %+v", j)
		}
		if p := c.Pending(); len(p) != 0 {
			t.Fatalf("still pending: %+v", p)
		}
		// The app's late (or injected) answer is stale: it approves nothing.
		before := app.notices.Load()
		answerPhone(c, q.ID, "allow_session")
		if app.notices.Load() != before+1 || e.ranContains("rm "+redRm) {
			t.Fatal("a late answer was not told it is stale, or ran the command")
		}
		// The agent repeats it under YOLO: it runs with no question.
		if r := waitResult(t, e.execAsync("shell", "rm", redRm)); r.exit.Code != 0 || !e.ranContains("rm "+redRm) {
			t.Fatalf("under YOLO: %+v", r.exit)
		}
		app.notAsked(t)
		// YOLO ends: the same 🔴 asks again, nothing was kept.
		e.d.Yolo().Off(YoloOffApp)
		res = e.execAsync("shell", "rm", redRm)
		again := app.next(t)
		if again.ID == q.ID {
			t.Fatal("same id")
		}
		answerPhone(c, again.ID, "deny")
		if r := waitResult(t, res); r.exit.Code != ExitDenied {
			t.Fatalf("%+v", r.exit)
		}
	})
}

// A question of serve's own (`adbuddy remote on`) is withdrawn with an error of
// its own, and the inverse 🔴 of an undo stops the undo: both fail closed.
// Mutation: remove `o.Yolo.Subscribe(d.yoloWithdraw)` from New.
func TestYoloOnWithdrawsAskUserAndUndoQuestions(t *testing.T) {
	t.Run("AskUser", func(t *testing.T) {
		e := newEnv(t)
		e.d.o.OnEntry = func(journal.Entry) {}
		held := newHeld()
		e.d.o.Confirmer = held
		ask := e.askAsync(portTitle, time.Hour)
		asked(t, held)
		e.d.Yolo().EnableSession()
		if r := waitAsk(t, ask); !errors.Is(r.err, ErrAskYolo) || errors.Is(r.err, ErrAskPaused) || r.d == confirm.Allow {
			t.Fatalf("%+v", r)
		}
	})
	t.Run("undo", func(t *testing.T) {
		e := newEnv(t)
		e.d.o.OnEntry = func(journal.Entry) {}
		held := newHeld()
		e.d.o.Confirmer = held
		e.forge(t, "j_f1", putArgv, "shell", "settings", "delete", "global", "a")
		done := make(chan UndoResult, 1)
		exits := make(chan string, 1)
		go func() {
			_, exit, _ := e.undoLast(t)
			exits <- exit.ErrorCode + "|" + exit.Error
			done <- UndoResult{}
		}()
		asked(t, held)
		e.d.Yolo().EnableSession()
		select {
		case got := <-exits:
			if !strings.HasPrefix(got, "CONFIRMATION_WITHDRAWN|"+YoloWithdrawnMessage) || e.ranContains("settings delete") {
				t.Fatalf("%q ran %q", got, e.adb.ran())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the undo kept waiting")
		}
	})
}

// Turning YOLO on and off with a question pending, from several goroutines at
// once together with the app's answer and the pause check, never deadlocks:
// Yolo.notifyMu comes first, then the daemon's askMu, Confirmer.mu and the
// Yolo's own mu one at a time (see withdrawAsking).
func TestYoloOnOffWithAQuestionPendingDoesNotDeadlock(t *testing.T) {
	e := newEnv(t)
	c, app := e.phoneConfirmer(t)
	var paused atomic.Bool
	e.d.o.Paused = paused.Load
	for i := range 25 {
		// A different path each round: an allow_session answered with YOLO off is a
		// legitimate permit for its key, and would skip the question of the next round.
		res := e.execAsync("shell", "rm", redRm+"-"+strconv.Itoa(i))
		q := app.next(t)
		var wg sync.WaitGroup
		wg.Add(5)
		go func() { defer wg.Done(); _ = e.d.Yolo().Enable(YoloShort) }()
		go func() { defer wg.Done(); e.d.Yolo().Off(YoloOffApp) }()
		go func() { defer wg.Done(); answerPhone(c, q.ID, "allow_session") }()
		go func() { defer wg.Done(); _ = c.Pending(); _ = e.d.Yolo().Active() }()
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				paused.Store(true)
				e.d.CancelConfirmations()
				_ = e.d.paused()
				paused.Store(false)
			}
		}()
		finished := make(chan struct{})
		go func() { wg.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatalf("round %d: deadlock turning YOLO on and off with a question pending", i)
		}
		waitResult(t, res)
		e.d.Yolo().Off(YoloOffApp)
	}
}

// The race the observer cannot see: the exec read YOLO off in lift, YOLO is
// turned on, and only then is the question registered. It must not wait on the
// phone with YOLO on: it is withdrawn at once and the command can be repeated.
// Mutation: remove the `d.o.Yolo.Active()` check after beforeAsk in exec.
func TestYoloTurnedOnBetweenTheLiftAndTheQuestionWithdrawsIt(t *testing.T) {
	e := newEnv(t)
	c, app := e.phoneConfirmer(t)
	e.d.beforeAsk = func() { e.d.Yolo().EnableSession() } // the observer runs before the question exists
	r := waitResult(t, e.execAsync("shell", "rm", redRm))
	if r.exit.Code != ExitDenied || e.ranContains("rm "+redRm) || !strings.Contains(r.s.errOut.String(), YoloWithdrawnMessage) {
		t.Fatalf("%+v ran %q stderr %q", r.exit, e.adb.ran(), r.s.errOut.String())
	}
	app.notAsked(t)
	if p := c.Pending(); len(p) != 0 {
		t.Fatalf("pending: %+v", p)
	}
	if j := lastAgentEntry(t, e); j.Status != journal.StatusBlocked || j.Reason != YoloWithdrawnMessage {
		t.Fatalf("journal %+v", j)
	}
	e.d.beforeAsk = nil
	if r := waitResult(t, e.execAsync("shell", "rm", redRm)); r.exit.Code != 0 || !e.ranContains("rm "+redRm) {
		t.Fatalf("repeated under YOLO: %+v", r.exit)
	}
}
