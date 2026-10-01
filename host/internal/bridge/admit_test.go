package bridge

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Admit is asked after the token and before anything is announced: a
// refused app is closed like one without the token, and neither OnConnect
// nor OnAttach hear of it.
func TestAdmitDecidesAfterTheToken(t *testing.T) {
	var asked, attached atomic.Int32
	var admit atomic.Bool
	var conns calls
	h := start(t, Options{
		Admit:     func(ctx context.Context, hl Hello) bool { asked.Add(1); return admit.Load() },
		OnConnect: conns.add,
		OnAttach:  func() { attached.Add(1) },
	})
	if st := closeStatus(t, h.dial("nope", 0)); st != websocket.StatusPolicyViolation {
		t.Fatalf("wrong token: %v", st)
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("Admit asked %d times before the token", n)
	}
	if st := closeStatus(t, h.dial(token, 0)); st != websocket.StatusPolicyViolation {
		t.Fatalf("refused: %v", st)
	}
	time.Sleep(50 * time.Millisecond)
	if asked.Load() != 1 || h.srv.Connected() || len(conns.get()) != 0 || attached.Load() != 0 {
		t.Fatalf("refused app: asked %d, connected %v, OnConnect %v, OnAttach %d", asked.Load(), h.srv.Connected(), conns.get(), attached.Load())
	}
	admit.Store(true)
	c := h.dial(token, 0)
	defer c.CloseNow()
	if m := recv(t, c); m.Type != TypeWelcome {
		t.Fatalf("admitted: got %q, want a welcome", m.Type)
	}
	conns.waitFor(t, []bool{true}, 5*time.Second)
	if attached.Load() != 1 {
		t.Fatalf("OnAttach %d times", attached.Load())
	}
}

// An Admit that does not answer in time rejects the app (fail closed).
func TestAdmitThatHangsRejects(t *testing.T) {
	old := admitTimeout
	admitTimeout = 50 * time.Millisecond
	t.Cleanup(func() { admitTimeout = old })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	h := start(t, Options{Admit: func(ctx context.Context, hl Hello) bool { <-release; return true }})
	if st := closeStatus(t, h.dial(token, 0)); st != websocket.StatusPolicyViolation {
		t.Fatalf("hung Admit: %v", st)
	}
	if h.srv.Connected() {
		t.Fatal("an app counts as connected while Admit hangs")
	}
}

// Without Admit every app with the token gets in, as before.
func TestNoAdmitAdmitsEveryone(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	defer c.CloseNow()
	if m := recv(t, c); m.Type != TypeWelcome {
		t.Fatalf("got %q, want a welcome", m.Type)
	}
}
