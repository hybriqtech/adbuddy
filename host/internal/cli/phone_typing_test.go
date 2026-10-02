package cli

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

// stuckRig is a chatRig whose agent does not read its input: each Write into
// its terminal waits until open is called (or the agent ends).
func stuckRig(t *testing.T) (r *chatRig, open func()) {
	t.Helper()
	r = newChatRig(t)
	r.gate, r.entered = make(chan struct{}), make(chan struct{}, 1)
	var once sync.Once
	open = func() { once.Do(func() { close(r.gate) }) }
	t.Cleanup(open)
	r.start(t, "claude")
	return r, open
}

// typeAndWait sends a terminal.input and waits until the agent's terminal is
// stuck in its Write.
func (r *chatRig) typeAndWait(t *testing.T, data string) {
	t.Helper()
	sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte(data)})
	select {
	case <-r.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("the typing never reached the terminal")
	}
}

// V08: with the agent's terminal stuck (it does not read its input), the
// channel's reader goes on: the phone's typing beyond the queue is dropped,
// with one notice for the run of drops; a chat.send is told busy and its id
// is free again; and a confirm.reply is handled while the terminal is still
// stuck, so the person can still answer a red command.
func TestTheChannelGoesOnWhileTheAgentDoesNotRead(t *testing.T) {
	r, open := stuckRig(t)
	r.typeAndWait(t, "<0>")
	for n := 1; n <= 16; n++ { // the queue, session.writeQueue
		sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte("<q>")})
	}
	sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte("\x1b")})
	sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte("[A")})
	sendApp(t, r.c, bridge.TypeInterrupt, "", bridge.Empty{})
	n := r.read(t, bridge.TypeNotice)
	var notice bridge.Notice
	if err := json.Unmarshal(n.Payload, &notice); err != nil {
		t.Fatal(err)
	}
	if notice.Text != r.l.lang.T("notice.typing_dropped") {
		t.Fatalf("notice %q", notice.Text)
	}

	// A chat.send: busy at once (the queue is full), and only one notice
	// came for the three drops (read fails on another one).
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_1", Text: "hello agent"})
	er := r.readError(t, "r_1", bridge.CodeBusy)
	if er.Text != "The agent is not reading its input. Try again in a moment." {
		t.Fatalf("text %q", er.Text)
	}

	// A red command's question is answered while the terminal is stuck.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	decided := make(chan confirm.Decision, 1)
	go func() {
		decided <- r.l.Confirmer().Confirm(ctx, confirm.Request{ID: "j_1", Title: "Restart", ExpiresAt: time.Now().Add(time.Minute)})
	}()
	req := r.read(t, bridge.TypeConfirmReq)
	if !strings.Contains(string(req.Payload), `"j_1"`) {
		t.Fatalf("the question: %s", req.Payload)
	}
	sendApp(t, r.c, bridge.TypeConfirmReply, "", bridge.ConfirmReply{ID: "j_1", Decision: "allow"})
	select {
	case d := <-decided:
		if d != confirm.Allow {
			t.Fatalf("decided %v", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the confirm.reply was not handled while the terminal was stuck")
	}
	select {
	case <-r.gate:
		t.Fatal("the terminal is not stuck any more")
	default:
	}

	// The terminal reads again: what was queued is typed, what was dropped
	// never; the same message id goes through now.
	open()
	want := "<0>" + strings.Repeat("<q>", 16)
	waitUntil(t, "the queued typing", func() bool { return r.pty(0).said() == want })
	sendApp(t, r.c, bridge.TypeChatSend, "r_2", bridge.ChatSend{ID: "m_1", Text: "hello agent"})
	m := r.read(t, bridge.TypeChatMessage)
	if m.RID != "r_2" {
		t.Fatalf("rid %q", m.RID)
	}
	if got := r.pty(0).said(); got != want+"hello agent\r" {
		t.Fatalf("typed %q", got)
	}
	// The phone's typing goes in again (the drop ended with the queue).
	sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte("z")})
	waitUntil(t, "typing after the drop", func() bool { return strings.HasSuffix(r.pty(0).said(), "\rz") })
}

// A chat.send queued behind a stuck Write while a question comes: the writer
// asks agentBusy again before typing, so nothing is typed, the answer is
// CodeBusy with the reason, and the id is free again.
func TestChatSendAsksAgainWhenItIsTyped(t *testing.T) {
	r, open := stuckRig(t)
	r.typeAndWait(t, "a")
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_1", Text: "hello agent"})
	// The reader checks agentBusy, queues the paste and waits for it (up to
	// session.writeWait): the question comes once it is queued, so that only
	// the writer's own check can stop it.
	waitUntil(t, "the paste in the writer's queue", func() bool { return r.m.Queued() == 1 })
	r.l.adapter.mu.Lock()
	r.l.adapter.attention = time.Now().Add(time.Minute)
	r.l.adapter.mu.Unlock()
	open()
	er := r.readError(t, "r_1", bridge.CodeBusy)
	if er.Text != busyPending {
		t.Fatalf("text %q", er.Text)
	}
	if got := r.pty(0).said(); got != "a" {
		t.Fatalf("typed %q", got)
	}
	r.l.chat.mu.Lock()
	inflight, sent := r.l.chat.inflight["m_1"], r.l.chat.sent["m_1"]
	r.l.chat.mu.Unlock()
	if inflight || sent {
		t.Fatalf("inflight %v sent %v after the busy answer", inflight, sent)
	}
}
