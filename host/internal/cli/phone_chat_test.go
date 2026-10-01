package cli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/pty"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// fakeAgentPTY is an agent in memory: the test writes its output and reads
// what was typed (as fakePTY in internal/session).
type fakeAgentPTY struct {
	out   *io.PipeReader
	outW  *io.PipeWriter
	mu    sync.Mutex
	typed strings.Builder
	done  chan struct{}
	once  sync.Once
}

func newFakeAgentPTY() *fakeAgentPTY {
	r, w := io.Pipe()
	return &fakeAgentPTY{out: r, outW: w, done: make(chan struct{})}
}

func (f *fakeAgentPTY) Read(b []byte) (int, error) { return f.out.Read(b) }
func (f *fakeAgentPTY) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typed.Write(b)
	return len(b), nil
}
func (f *fakeAgentPTY) Resize(int, int) error { return nil }
func (f *fakeAgentPTY) Pid() int              { return 1 }
func (f *fakeAgentPTY) Wait(ctx context.Context) (int, error) {
	select {
	case <-f.done:
		return 0, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (f *fakeAgentPTY) Hangup() error { f.exit(); return nil }
func (f *fakeAgentPTY) Kill() error   { f.exit(); return nil }
func (f *fakeAgentPTY) exit()         { f.once.Do(func() { f.outW.Close(); close(f.done) }) }
func (f *fakeAgentPTY) said() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.typed.String()
}

// chatRig is a link with an app connected and a session whose agents are in
// memory; each start makes a new one.
type chatRig struct {
	l   *phoneLink
	c   *websocket.Conn
	m   *session.Manager
	mu  sync.Mutex
	pts []*fakeAgentPTY
}

func newChatRig(t *testing.T) *chatRig {
	t.Helper()
	l, c := linkedApp(t)
	r := &chatRig{l: l, c: c}
	r.m = session.New(session.Options{
		Dir:  t.TempDir(),
		Look: func(name string) (string, error) { return "/bin/" + name, nil },
		Env:  func(string) []string { return nil },
		Start: func(pty.Options) (pty.PTY, error) {
			f := newFakeAgentPTY()
			r.mu.Lock()
			r.pts = append(r.pts, f)
			r.mu.Unlock()
			return f, nil
		},
		Quiet:  50 * time.Millisecond,
		OnInfo: l.sessionInfo,
	})
	l.useSession(r.m)
	t.Cleanup(r.m.Stop)
	return r
}

// start starts the agent and takes the session.state it publishes.
func (r *chatRig) start(t *testing.T, name string) {
	t.Helper()
	if err := r.m.Start(name, false); err != nil {
		t.Fatal(err)
	}
	r.read(t, bridge.TypeSessionState)
}

func (r *chatRig) pty(i int) *fakeAgentPTY {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pts[i]
}

// read returns the next message of the wanted type, skipping the state
// changes the agent's life publishes meanwhile.
func (r *chatRig) read(t *testing.T, typ string) bridge.Message {
	t.Helper()
	for range 20 {
		m := readApp(t, r.c)
		if m.Type == typ {
			return m
		}
		if m.Type != bridge.TypeSessionState {
			t.Fatalf("got %s %s, want %s", m.Type, m.Payload, typ)
		}
	}
	t.Fatalf("no %s", typ)
	return bridge.Message{}
}

func (r *chatRig) readError(t *testing.T, rid, code string) bridge.ErrorReply {
	t.Helper()
	m := r.read(t, bridge.TypeError)
	var er bridge.ErrorReply
	if err := json.Unmarshal(m.Payload, &er); err != nil {
		t.Fatal(err)
	}
	if m.RID != rid || er.Code != code {
		t.Fatalf("got error %q for %q (%s), want %q for %q", er.Code, m.RID, er.Text, code, rid)
	}
	return er
}

func chatMessageOf(t *testing.T, m bridge.Message) bridge.ChatMessage {
	t.Helper()
	var cm bridge.ChatMessage
	if err := json.Unmarshal(m.Payload, &cm); err != nil {
		t.Fatal(err)
	}
	return cm
}

// A chat.send is typed into the agent as a paste and comes back as a
// chat.message with the request's rid; sending the same id again confirms
// the message again and types nothing.
func TestChatSendTypesTheMessageOnce(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")

	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_1", Text: "hello agent"})
	m := r.read(t, bridge.TypeChatMessage)
	cm := chatMessageOf(t, m)
	if m.RID != "r_1" || cm.ID != "m_1" || cm.Role != "user" || cm.Text != "hello agent" || cm.Conversation == "" || cm.Attachments == nil {
		t.Fatalf("got rid %q %+v", m.RID, cm)
	}
	if got := r.pty(0).said(); got != "hello agent\r" {
		t.Fatalf("typed %q", got)
	}

	sendApp(t, r.c, bridge.TypeChatSend, "r_2", bridge.ChatSend{ID: "m_1", Text: "hello agent"})
	m = r.read(t, bridge.TypeChatMessage)
	if m.RID != "r_2" || chatMessageOf(t, m).ID != "m_1" {
		t.Fatalf("retry: got rid %q %s", m.RID, m.Payload)
	}
	if got := r.pty(0).said(); got != "hello agent\r" {
		t.Fatalf("typed again: %q", got)
	}
}

// Two chat.send with the same id at once type the message once.
func TestChatSendWithTheSameIDAtOnceTypesOnce(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	payload, _ := json.Marshal(bridge.ChatSend{ID: "m_same", Text: "once only"})
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.l.onChatSend(bridge.Message{Type: bridge.TypeChatSend, RID: "r_" + string(rune('a'+i)), Payload: payload})
		}()
	}
	wg.Wait()
	if n := strings.Count(r.pty(0).said(), "once only"); n != 1 {
		t.Fatalf("typed %d times: %q", n, r.pty(0).said())
	}
	// After it, a retry gets the confirmation of the one that was typed.
	sendApp(t, r.c, bridge.TypeChatSend, "r_late", bridge.ChatSend{ID: "m_same", Text: "once only"})
	for {
		m := readApp(t, r.c)
		if m.Type == bridge.TypeChatMessage && m.RID == "r_late" {
			break
		}
	}
	if n := strings.Count(r.pty(0).said(), "once only"); n != 1 {
		t.Fatalf("typed %d times", n)
	}
}

// Attachments go to the agent as paths of the environment, and into the
// message as what they are.
func TestChatSendWithAttachments(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	a := bridge.Attachment{ID: strings.Repeat("a", 32), Name: "x.png", MIME: "image/png", Size: 3, SHA256: "s", Thumb: "dGh1bWI="}
	r.l.chat.attachments.put(a, "inbox/x.png")

	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_1", Text: "look", Attachments: []string{a.ID}})
	cm := chatMessageOf(t, r.read(t, bridge.TypeChatMessage))
	if len(cm.Attachments) != 1 || cm.Attachments[0].ID != a.ID || cm.Attachments[0].Thumb != a.Thumb {
		t.Fatalf("message %+v", cm)
	}
	if got := r.pty(0).said(); got != "look\n\nAdjuntos:\n- inbox/x.png\r" {
		// Without bracketed paste the agent's line has no new lines.
		if got != "look Adjuntos: - inbox/x.png\r" {
			t.Fatalf("typed %q", got)
		}
	}
}

// Every way a chat.send fails answers with its code, and types nothing.
func TestChatSendErrors(t *testing.T) {
	r := newChatRig(t)

	// Without an agent.
	sendApp(t, r.c, bridge.TypeChatSend, "r_0", bridge.ChatSend{ID: "m_0", Text: "x"})
	r.readError(t, "r_0", bridge.CodeNoAgent)

	r.start(t, "claude")
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "not a valid id!", Text: "x"})
	r.readError(t, "r_1", bridge.CodeInvalid)
	sendApp(t, r.c, bridge.TypeChatSend, "r_1b", bridge.ChatSend{ID: strings.Repeat("i", 65), Text: "x"})
	r.readError(t, "r_1b", bridge.CodeInvalid)
	sendApp(t, r.c, bridge.TypeChatSend, "r_2", bridge.ChatSend{ID: "m_2", Text: strings.Repeat("x", 64<<10+1)})
	r.readError(t, "r_2", bridge.CodeTooLarge)
	sendApp(t, r.c, bridge.TypeChatSend, "r_3", bridge.ChatSend{ID: "m_3", Text: "x", Attachments: make([]string, 11)})
	r.readError(t, "r_3", bridge.CodeTooLarge)
	sendApp(t, r.c, bridge.TypeChatSend, "r_4", bridge.ChatSend{ID: "m_4", Text: "x", Attachments: []string{strings.Repeat("b", 32)}})
	r.readError(t, "r_4", bridge.CodeNotFound)

	// The agent shows a question the message's Enter would answer.
	r.pty(0).outW.Write([]byte("Do you want to proceed?"))
	waitUntil(t, "the question on the screen", func() bool { return r.m.Busy() != "" })
	sendApp(t, r.c, bridge.TypeChatSend, "r_5", bridge.ChatSend{ID: "m_5", Text: "x"})
	er := r.readError(t, "r_5", bridge.CodeBusy)
	if er.Text != r.m.Busy() {
		t.Fatalf("text %q", er.Text)
	}

	// Nothing was typed, and each failure freed its id: the same ids work
	// once the cause is gone.
	if got := r.pty(0).said(); got != "" {
		t.Fatalf("typed %q", got)
	}
	r.m.Stop()
	r.read(t, bridge.TypeSessionState)
	sendApp(t, r.c, bridge.TypeChatSend, "r_6", bridge.ChatSend{ID: "m_5", Text: "x"})
	r.readError(t, "r_6", bridge.CodeNoAgent)
}

// A rid the server does not accept gets no answer and types nothing.
func TestChatSendWithABadRIDDoesNothing(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	sendApp(t, r.c, bridge.TypeChatSend, "bad rid!", bridge.ChatSend{ID: "m_1", Text: "x"})
	sendApp(t, r.c, bridge.TypeChatSend, "r_ok", bridge.ChatSend{ID: "m_2", Text: "second"})
	m := r.read(t, bridge.TypeChatMessage)
	if m.RID != "r_ok" || chatMessageOf(t, m).ID != "m_2" {
		t.Fatalf("got rid %q %s", m.RID, m.Payload)
	}
	if got := r.pty(0).said(); got != "second\r" {
		t.Fatalf("typed %q", got)
	}
}

// session.new and session.stop answer with a session.state carrying their
// rid; a new conversation starts the history over.
func TestSessionNewAndStop(t *testing.T) {
	r := newChatRig(t)
	r.start(t, "claude")
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_1", Text: "hi"})
	r.read(t, bridge.TypeChatMessage)
	first := r.l.state().Conversation

	sendApp(t, r.c, bridge.TypeSessionNew, "r_new", bridge.SessionNew{Agent: "codex"})
	var st bridge.SessionState
	for {
		m := readApp(t, r.c)
		if m.Type == bridge.TypeSessionState && m.RID == "r_new" {
			if err := json.Unmarshal(m.Payload, &st); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if st.Agent != "codex" || st.State == session.StateStopped || st.Conversation == "" || st.Conversation == first {
		t.Fatalf("state %+v (first conversation %s)", st, first)
	}
	if s := r.l.snapshot(); len(s.Chat) != 0 {
		t.Fatalf("history of the old conversation: %+v", s.Chat)
	}

	// Without an agent in the request: the current one.
	sendApp(t, r.c, bridge.TypeSessionNew, "r_new2", bridge.SessionNew{})
	for {
		m := readApp(t, r.c)
		if m.Type == bridge.TypeSessionState && m.RID == "r_new2" {
			if err := json.Unmarshal(m.Payload, &st); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if st.Agent != "codex" {
		t.Fatalf("state %+v", st)
	}

	sendApp(t, r.c, bridge.TypeSessionStop, "r_stop", bridge.Empty{})
	for {
		m := readApp(t, r.c)
		if m.Type == bridge.TypeSessionState && m.RID == "r_stop" {
			if err := json.Unmarshal(m.Payload, &st); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if st.State != session.StateStopped {
		t.Fatalf("state %+v", st)
	}
}

// An agent that ADBuddy does not know, or one not installed, is an error.
func TestSessionNewErrors(t *testing.T) {
	r := newChatRig(t)
	sendApp(t, r.c, bridge.TypeSessionNew, "r_1", bridge.SessionNew{Agent: "../evil"})
	r.readError(t, "r_1", bridge.CodeInvalid)
	sendApp(t, r.c, bridge.TypeSessionNew, "r_2", bridge.SessionNew{Agent: "/bin/sh"})
	r.readError(t, "r_2", bridge.CodeInvalid)

	r.m = session.New(session.Options{
		Dir:    t.TempDir(),
		Look:   func(string) (string, error) { return "", io.ErrUnexpectedEOF },
		Env:    func(string) []string { return nil },
		Start:  func(pty.Options) (pty.PTY, error) { t.Fatal("started"); return nil, nil },
		OnInfo: r.l.sessionInfo,
	})
	r.l.useSession(r.m)
	sendApp(t, r.c, bridge.TypeSessionNew, "r_3", bridge.SessionNew{Agent: "codex"})
	er := r.readError(t, "r_3", bridge.CodeFailed)
	if !strings.Contains(er.Text, "codex") {
		t.Fatalf("no install hint: %q", er.Text)
	}
}

// agent.interrupt types ESC; without an agent it does nothing.
func TestAgentInterrupt(t *testing.T) {
	r := newChatRig(t)
	sendApp(t, r.c, bridge.TypeInterrupt, "", bridge.Empty{})
	r.start(t, "claude")
	sendApp(t, r.c, bridge.TypeInterrupt, "", bridge.Empty{})
	waitUntil(t, "ESC", func() bool { return r.pty(0).said() == "\x1b" })
}

// The snapshot carries the last 100 messages, without thumbnails and with a
// list of attachments always.
func TestSnapshotCarriesTheHistory(t *testing.T) {
	l := &phoneLink{}
	l.conf = bridge.NewConfirmer(noApp{}, nil)
	l.setAreas(l.chatArea())
	for i := range 130 {
		m := bridge.ChatMessage{ID: "m_" + string(rune('A'+i/26)) + string(rune('a'+i%26)), Role: "assistant", Text: "t", Conversation: "c_1"}
		if i%2 == 0 {
			m.Attachments = []bridge.Attachment{{ID: strings.Repeat("c", 32), Name: "n", Thumb: "dGh1bWI="}}
		}
		l.addMessage(m)
	}
	// An upsert replaces.
	l.addMessage(bridge.ChatMessage{ID: "m_Ea", Role: "assistant", Text: "changed", Conversation: "c_1"})
	s := l.snapshot()
	if len(s.Chat) != 100 {
		t.Fatalf("%d messages", len(s.Chat))
	}
	raw, _ := json.Marshal(s.Chat)
	if strings.Contains(string(raw), "thumb") || strings.Contains(string(raw), `"attachments":null`) {
		t.Fatalf("snapshot %.300s", raw)
	}
	changed := 0
	for _, m := range s.Chat {
		if m.Text == "changed" {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("%d changed messages", changed)
	}
	// The history itself keeps the thumbnails.
	l.chat.mu.Lock()
	kept := l.chat.history[len(l.chat.history)-2].Attachments
	l.chat.mu.Unlock()
	if len(kept) == 0 || kept[0].Thumb == "" {
		t.Fatalf("history lost its thumbnail: %+v", kept)
	}
}

// Big messages leave the oldest out so that the chat of the snapshot stays
// below 512 KiB.
func TestSnapshotChatStaysUnderTheLimit(t *testing.T) {
	l := &phoneLink{}
	l.conf = bridge.NewConfirmer(noApp{}, nil)
	l.setAreas(l.chatArea())
	for i := range 30 {
		l.addMessage(bridge.ChatMessage{ID: "m_" + string(rune('a'+i)), Role: "assistant", Text: strings.Repeat("é\"\n", 20<<10), Conversation: "c_1"})
	}
	s := l.snapshot()
	raw, _ := json.Marshal(s.Chat)
	if len(raw) >= 512<<10 || len(s.Chat) == 0 {
		t.Fatalf("%d bytes, %d messages", len(raw), len(s.Chat))
	}
	if last := s.Chat[len(s.Chat)-1].ID; last != "m_"+string(rune('a'+29)) {
		t.Fatalf("the newest was dropped: %s", last)
	}
	if len(s.Chat) >= 30 {
		t.Fatal("nothing was left out")
	}
	// Both snapshots in the welcome and after it are under the app's limit.
	if data, _ := json.Marshal(s); len(data) >= 1<<20 {
		t.Fatalf("snapshot %d bytes", len(data))
	}
}
