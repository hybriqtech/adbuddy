package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// V23: a retry of a message that was typed, once it has left the history
// (a new conversation, or more than keptChat messages later), is not typed
// again: it is told invalid, and nothing is added to the new conversation.
func TestChatSendRetryAfterSessionNewIsNotTypedAgain(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_1", Text: "hello agent"})
	r.read(t, bridge.TypeChatMessage)

	sendApp(t, r.c, bridge.TypeSessionNew, "r_new", bridge.SessionNew{})
	nextState(t, r)

	sendApp(t, r.c, bridge.TypeChatSend, "r_2", bridge.ChatSend{ID: "m_1", Text: "hello agent"})
	r.readError(t, "r_2", bridge.CodeInvalid)
	for i := range 2 {
		if strings.Contains(r.pty(i).said(), "hello agent\r") && i == 1 {
			t.Fatalf("typed again in the new conversation: %q", r.pty(i).said())
		}
	}
	if s := r.l.snapshot(); len(s.Chat) != 0 {
		t.Fatalf("the retry entered the new conversation: %+v", s.Chat)
	}
}

func TestChatSendRetryBeyondTheHistoryIsNotTypedAgain(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_old", Text: "only once"})
	first := chatMessageOf(t, r.read(t, bridge.TypeChatMessage))
	for i := range keptChat + 5 {
		r.l.addMessage(bridge.ChatMessage{ID: fmt.Sprintf("a_%d", i), Role: "assistant", Text: "x", Conversation: first.Conversation})
	}
	r.l.chat.mu.Lock()
	_, still := r.l.chat.findLocked("m_old")
	r.l.chat.mu.Unlock()
	if still {
		t.Fatal("m_old is still in the history")
	}
	sendApp(t, r.c, bridge.TypeChatSend, "r_2", bridge.ChatSend{ID: "m_old", Text: "only once"})
	for {
		m := readApp(t, r.c)
		if m.RID == "r_2" {
			if m.Type != bridge.TypeError || !strings.Contains(string(m.Payload), bridge.CodeInvalid) {
				t.Fatalf("got %s %s", m.Type, m.Payload)
			}
			break
		}
	}
	if n := strings.Count(r.pty(0).said(), "only once"); n != 1 {
		t.Fatalf("typed %d times: %q", n, r.pty(0).said())
	}
}

// The set of typed ids has a fixed size: the oldest leave first.
func TestSentIDsAreBounded(t *testing.T) {
	var c chatState
	for i := range maxSentIDs + 10 {
		c.markSentLocked(fmt.Sprintf("m_%d", i))
	}
	if len(c.sent) != maxSentIDs || len(c.sentOrder) != maxSentIDs {
		t.Fatalf("%d ids, %d in order", len(c.sent), len(c.sentOrder))
	}
	if c.sent["m_0"] || c.sent["m_9"] || !c.sent["m_10"] || !c.sent[fmt.Sprintf("m_%d", maxSentIDs+9)] {
		t.Fatal("the oldest ids did not leave first")
	}
}

// V41: the snapshot trusts the size kept with each message instead of
// serializing them all again under the server's lock.
func TestChatSnapshotUsesTheStoredSizes(t *testing.T) {
	l := &phoneLink{}
	l.conf = bridge.NewConfirmer(noApp{}, nil)
	l.setAreas(l.chatArea())
	for i := range 3 {
		l.addMessage(bridge.ChatMessage{ID: fmt.Sprintf("m_%d", i), Role: "assistant", Text: "short", Conversation: "c_1"})
	}
	if len(l.chat.sizes) != 3 || l.chat.sizes[0] < 20 {
		t.Fatalf("sizes %v", l.chat.sizes)
	}
	l.chat.sizes[0] = maxChatSnapshot // as if the first one weighed that much
	if s := l.snapshot(); len(s.Chat) != 2 || s.Chat[0].ID != "m_1" {
		t.Fatalf("the stored size was not used: %+v", s.Chat)
	}
	// A replaced message keeps its size current.
	l.addMessage(bridge.ChatMessage{ID: "m_1", Role: "assistant", Text: strings.Repeat("y", 1000), Conversation: "c_1"})
	if l.chat.sizes[1] < 1000 {
		t.Fatalf("sizes %v", l.chat.sizes)
	}
}
