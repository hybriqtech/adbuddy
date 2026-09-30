package bridge

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

// recorder is a Fallback that records what it was asked.
type recorder struct {
	mu    sync.Mutex
	d     confirm.Decision
	asked []string
}

func (r *recorder) Confirm(_ context.Context, q confirm.Request) confirm.Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, q.ID)
	return r.d
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.asked)
}

// notes records the Notify calls.
type notes struct {
	mu sync.Mutex
	v  []string
}

func (n *notes) add(title string, moved bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := "still on the phone: "
	if moved {
		s = "moved: "
	}
	n.v = append(n.v, s+title)
}

func (n *notes) get() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string{}, n.v...)
}

// confirmWithin runs Confirm with a long expiry and fails if it takes more
// than a few seconds: a question must not wait for an app that is gone.
func confirmWithin(t *testing.T, c *Confirmer, r confirm.Request) confirm.Decision {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	got := make(chan confirm.Decision, 1)
	go func() { got <- c.Confirm(ctx, r) }()
	select {
	case d := <-got:
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("the question kept waiting for the phone")
	}
	return confirm.Deny
}

// T1 review, M7: the app disconnects while its question waits; the question
// goes to the Fallback (the terminal, or deny) at once.
func TestQuestionMovesToTheFallbackWhenTheAppLeaves(t *testing.T) {
	for _, want := range []confirm.Decision{confirm.Allow, confirm.Nobody} {
		app := &fakeApp{connected: true}
		fb := &recorder{d: want}
		c := NewConfirmer(app, fb)
		n := &notes{}
		c.Notify = n.add
		app.onPublish = func(ConfirmRequest) {
			app.mu.Lock()
			app.connected = false
			app.mu.Unlock()
			c.AppGone()
		}
		req := confirm.Request{ID: "j_1", Title: "Restart", ExpiresAt: time.Now().Add(time.Minute)}
		if got := confirmWithin(t, c, req); got != want {
			t.Fatalf("got %v, want the fallback's %v", got, want)
		}
		if fb.count() != 1 {
			t.Fatalf("fallback asked %d times", fb.count())
		}
		if p := c.Pending(); len(p) != 0 {
			t.Fatalf("still pending on the phone: %+v", p)
		}
		if got := n.get(); len(got) != 1 || got[0] != "moved: Restart" {
			t.Fatalf("notes %q", got)
		}
		reply(c, "j_1", "allow") // the old app's late answer: ignored
	}
}

// leavingApp is connected when asked first, and gone by the time the
// question is published: its OnConnect(false) ran before the question was
// pending, so AppGone had nothing to move.
type leavingApp struct {
	mu    sync.Mutex
	calls int
}

func (a *leavingApp) Connected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	return a.calls == 1
}

func (a *leavingApp) Publish(string, any) error { return nil }

func TestQuestionMovesWhenTheAppLeftBeforeItWasPending(t *testing.T) {
	fb := &recorder{d: confirm.Allow}
	c := NewConfirmer(&leavingApp{}, fb)
	if got := confirmWithin(t, c, confirm.Request{ID: "j_1", ExpiresAt: time.Now().Add(time.Minute)}); got != confirm.Allow || fb.count() != 1 {
		t.Fatalf("got %v, fallback asked %d", got, fb.count())
	}
}

// If the app is back when the question would move (it reconnected at once),
// the question stays on the phone, which shows it again, and the terminal
// is told it still waits there.
func TestQuestionStaysOnThePhoneWhenTheAppIsBack(t *testing.T) {
	app := &fakeApp{connected: true}
	fb := &recorder{d: confirm.Deny}
	c := NewConfirmer(app, fb)
	n := &notes{}
	c.Notify = n.add
	app.onPublish = func(r ConfirmRequest) {
		c.AppGone() // gone and back: Connected is true again
		deadline := time.Now().Add(5 * time.Second)
		for len(n.get()) == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		reply(c, r.ID, "allow")
	}
	if got := confirmWithin(t, c, confirm.Request{ID: "j_1", Title: "Restart", ExpiresAt: time.Now().Add(time.Minute)}); got != confirm.Allow {
		t.Fatalf("got %v, want the phone's allow", got)
	}
	if fb.count() != 0 {
		t.Fatal("the fallback was asked too")
	}
	if got := n.get(); len(got) != 1 || got[0] != "still on the phone: Restart" {
		t.Fatalf("notes %q", got)
	}
	// And a later disconnection still moves it.
	app.onPublish = func(ConfirmRequest) {
		app.mu.Lock()
		app.connected = false
		app.mu.Unlock()
		c.AppGone()
	}
	fb.d = confirm.Allow
	if got := confirmWithin(t, c, confirm.Request{ID: "j_2", Title: "Restart", ExpiresAt: time.Now().Add(time.Minute)}); got != confirm.Allow || fb.count() != 1 {
		t.Fatalf("got %v, fallback asked %d", got, fb.count())
	}
}

// The question goes back and forth: gone, back, gone again.
func TestQuestionMovesOnTheSecondDisconnection(t *testing.T) {
	app := &fakeApp{connected: true}
	fb := &recorder{d: confirm.Allow}
	c := NewConfirmer(app, fb)
	n := &notes{}
	c.Notify = n.add
	app.onPublish = func(ConfirmRequest) {
		c.AppGone() // back at once
		deadline := time.Now().Add(5 * time.Second)
		for len(n.get()) == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		app.mu.Lock()
		app.connected = false
		app.mu.Unlock()
		c.AppGone()
	}
	if got := confirmWithin(t, c, confirm.Request{ID: "j_1", Title: "Restart", ExpiresAt: time.Now().Add(time.Minute)}); got != confirm.Allow || fb.count() != 1 {
		t.Fatalf("got %v, fallback asked %d", got, fb.count())
	}
	if got := n.get(); len(got) != 2 || got[1] != "moved: Restart" {
		t.Fatalf("notes %q", got)
	}
}

// lockingApp needs the Confirmer's lock in Connected, as the server does:
// it answers Connected under its own lock, and under that same lock a
// snapshot calls Pending. Connected called with the Confirmer's lock held
// would deadlock against a reconnecting app.
type lockingApp struct {
	c         *Confirmer
	connected atomic.Bool
}

func (a *lockingApp) Connected() bool {
	a.c.Pending()
	return a.connected.Load()
}

func (a *lockingApp) Publish(string, any) error {
	go func() {
		a.connected.Store(false)
		a.c.AppGone()
	}()
	return nil
}

func TestConnectedIsNeverAskedUnderTheConfirmersLock(t *testing.T) {
	app := &lockingApp{}
	app.connected.Store(true)
	fb := &recorder{d: confirm.Allow}
	c := NewConfirmer(app, fb)
	app.c = c
	if got := confirmWithin(t, c, confirm.Request{ID: "j_1", ExpiresAt: time.Now().Add(time.Minute)}); got != confirm.Allow || fb.count() != 1 {
		t.Fatalf("got %v, fallback asked %d", got, fb.count())
	}
}
