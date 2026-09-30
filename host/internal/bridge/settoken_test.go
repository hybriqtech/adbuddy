package bridge

import (
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

// T5 review, I2: each handover of the channel has a new token. SetToken
// makes the server take only the new one and drops the app connected with
// the old one, so a token captured earlier is worth nothing.
func TestSetTokenReplacesTheToken(t *testing.T) {
	h := start(t, Options{})
	old := h.dial(token, 0)
	if m := recv(t, old); m.Type != TypeWelcome {
		t.Fatalf("got %+v", m)
	}
	waitConnected(t, h.srv, true)
	fresh := strings.Repeat("ab", 32)
	h.srv.SetToken(fresh)
	if st := closeStatus(t, old); st != websocket.StatusPolicyViolation {
		t.Fatalf("the connection with the old token: close %v", st)
	}
	waitConnected(t, h.srv, false)
	if st := closeStatus(t, h.dial(token, 0)); st != websocket.StatusPolicyViolation {
		t.Fatalf("old token: close %v, want 1008", st)
	}
	c := h.dial(fresh, 0)
	if m := recv(t, c); m.Type != TypeWelcome {
		t.Fatalf("new token: got %+v", m)
	}
	waitConnected(t, h.srv, true)
	// Setting the same token again keeps the app connected.
	h.srv.SetToken(fresh)
	time.Sleep(100 * time.Millisecond)
	if !h.srv.Connected() {
		t.Fatal("the app with the current token was dropped")
	}
}

// T5 review, minor 1: an answer to a question that is no longer on the
// phone (answered, or moved to the terminal) is told so.
func TestReplyToAnUnknownQuestionIsTold(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Allow))
	reply(c, "j_gone", "allow")
	app.mu.Lock()
	defer app.mu.Unlock()
	if len(app.notices) != 1 || app.notices[0].Text != "That question was already answered or moved to the PC terminal." || app.notices[0].Level != "info" {
		t.Fatalf("notices %+v", app.notices)
	}
	if len(app.published) != 0 {
		t.Fatalf("published %+v", app.published)
	}
}

// T5 re-review, M1: a connection revoked by SetToken dispatches nothing more
// (confirm.reply, pause, resume), even while its close is still under way.
func TestARevokedConnectionDispatchesNothing(t *testing.T) {
	h := start(t, Options{})
	old := h.dial(token, 0)
	if m := recv(t, old); m.Type != TypeWelcome {
		t.Fatalf("got %+v", m)
	}
	waitConnected(t, h.srv, true)
	h.srv.SetToken(strings.Repeat("cd", 32))
	// The old app does not read, so it never answers the close: its socket
	// stays open for a while, and it still writes.
	send(t, old, TypeConfirmReply, ConfirmReply{ID: "j_1", Decision: "allow"})
	send(t, old, TypePause, struct{}{})
	send(t, old, TypeResume, struct{}{})
	select {
	case m := <-h.msgs:
		t.Fatalf("the revoked connection dispatched %+v", m)
	case <-time.After(500 * time.Millisecond):
	}
}

// The same for a message the reader had already taken when SetToken came:
// it is not dispatched after the revocation.
func TestAMessageReadBeforeTheRevocationIsNotDispatched(t *testing.T) {
	held, release := make(chan struct{}), make(chan struct{})
	old := beforeDispatch
	beforeDispatch = func(m Message) {
		if m.Type == TypeConfirmReply {
			close(held)
			<-release
		}
	}
	t.Cleanup(func() { beforeDispatch = old })
	h := start(t, Options{})
	c := h.dial(token, 0)
	if m := recv(t, c); m.Type != TypeWelcome {
		t.Fatalf("got %+v", m)
	}
	waitConnected(t, h.srv, true)
	send(t, c, TypeConfirmReply, ConfirmReply{ID: "j_1", Decision: "allow"})
	select {
	case <-held:
	case <-time.After(3 * time.Second):
		t.Fatal("the reply never reached the reader")
	}
	h.srv.SetToken(strings.Repeat("cd", 32))
	close(release)
	select {
	case m := <-h.msgs:
		t.Fatalf("the revoked connection dispatched %+v", m)
	case <-time.After(500 * time.Millisecond):
	}
}

// T3 re-review 2, minor: a revoked connection that still writes keeps being
// closed as revoked ("token changed", 1008), not as going away.
func TestARevokedConnectionThatWritesIsClosedAsRevoked(t *testing.T) {
	for i := 0; i < 20; i++ {
		held, release := make(chan struct{}), make(chan struct{})
		old := beforeDispatch
		beforeDispatch = func(m Message) {
			if m.Type == TypeConfirmReply {
				close(held)
				<-release
			}
		}
		h := start(t, Options{})
		beforeDispatch = old
		c := h.dial(token, 0)
		if m := recv(t, c); m.Type != TypeWelcome {
			t.Fatalf("got %+v", m)
		}
		waitConnected(t, h.srv, true)
		send(t, c, TypeConfirmReply, ConfirmReply{ID: "j_1", Decision: "allow"})
		select {
		case <-held:
		case <-time.After(3 * time.Second):
			t.Fatal("the reply never reached the reader")
		}
		h.srv.SetToken(strings.Repeat("cd", 32))
		close(release)
		if st := closeStatus(t, c); st != websocket.StatusPolicyViolation {
			t.Fatalf("attempt %d: close %v, want 1008 (token changed)", i, st)
		}
	}
}
