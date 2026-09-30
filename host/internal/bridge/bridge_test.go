package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type harness struct {
	t    *testing.T
	srv  *Server
	url  string
	msgs chan Message
}

func start(t *testing.T, o Options) *harness {
	t.Helper()
	l, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, msgs: make(chan Message, 100)}
	if o.Token == "" {
		o.Token = token
	}
	o.Session = "s_TEST"
	o.OnMessage = func(m Message) { h.msgs <- m }
	h.srv = New(o)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.srv.Serve(ctx, l); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	h.url = "ws://" + l.Addr().String() + "/v1"
	return h
}

// dial connects and sends hello; it returns the connection.
func (h *harness) dial(tok string, lastSeq uint64) *websocket.Conn {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { c.CloseNow() })
	send(h.t, c, TypeHello, Hello{Protocol: ProtocolVersion, Token: tok, LastSeq: lastSeq, AppVersion: "test"})
	return c
}

func send(t *testing.T, c *websocket.Conn, typ string, payload any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(Message{V: ProtocolVersion, Type: typ, TS: time.Now(), Payload: raw})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func recv(t *testing.T, c *websocket.Conn) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func waitConnected(t *testing.T, s *Server, want bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for s.Connected() != want {
		if time.Now().After(deadline) {
			t.Fatalf("Connected() never became %v", want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestWrongTokenIsRejected(t *testing.T) {
	h := start(t, Options{})
	for _, tok := range []string{"", "nope", strings.Repeat("0", 64)} {
		c := h.dial(tok, 0)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, _, err := c.Read(ctx)
		cancel()
		if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
			t.Errorf("token %q: %v, want policy violation", tok, err)
		}
	}
	if h.srv.Connected() {
		t.Fatal("an app without the token counts as connected")
	}
}

func TestNoHelloIsRejected(t *testing.T) {
	h := start(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send(t, c, TypeChatSend, map[string]string{"text": "hi"})
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("%v, want policy violation", err)
	}
}

func TestForeignOriginIsRejected(t *testing.T) {
	h := start(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, h.url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}}})
	if err == nil {
		t.Fatal("a web page from another origin could open the socket")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestWelcomeEventsAndResume(t *testing.T) {
	h := start(t, Options{HostVersion: "1.2.3"})
	h.srv.Publish(TypeNotice, Notice{Level: "info", Text: "one"})
	c := h.dial(token, 0)
	w := recv(t, c)
	var wel Welcome
	json.Unmarshal(w.Payload, &wel)
	if w.Type != TypeWelcome || wel.Protocol != ProtocolVersion || wel.Session != "s_TEST" || wel.FromSeq != 1 || wel.HostVersion != "1.2.3" {
		t.Fatalf("welcome %+v %+v", w, wel)
	}
	if m := recv(t, c); m.Type != TypeNotice || m.Seq != 1 {
		t.Fatalf("backlog %+v", m)
	}
	waitConnected(t, h.srv, true)
	h.srv.Publish(TypeNotice, Notice{Level: "info", Text: "two"})
	if m := recv(t, c); m.Seq != 2 {
		t.Fatalf("live event %+v", m)
	}
	c.Close(websocket.StatusNormalClosure, "")
	waitConnected(t, h.srv, false)
	h.srv.Publish(TypeNotice, Notice{Level: "info", Text: "three"})

	c2 := h.dial(token, 2)
	w2 := recv(t, c2)
	json.Unmarshal(w2.Payload, &wel)
	if wel.FromSeq != 3 {
		t.Fatalf("resume from %d, want 3", wel.FromSeq)
	}
	m := recv(t, c2)
	var n Notice
	json.Unmarshal(m.Payload, &n)
	if m.Seq != 3 || n.Text != "three" {
		t.Fatalf("resumed %+v %q", m, n.Text)
	}
}

func TestTooOldOrFutureSeqGetsASnapshot(t *testing.T) {
	h := start(t, Options{Snapshot: func() Snapshot {
		return Snapshot{State: SessionState{State: "idle", Paused: true}, Pending: []ConfirmRequest{}, Journal: []json.RawMessage{}}
	}})
	for i := 0; i < BufferSize+5; i++ {
		h.srv.Publish(TypeNotice, Notice{Text: "x"})
	}
	for _, last := range []uint64{1, BufferSize * 10} {
		c := h.dial(token, last)
		recv(t, c) // welcome
		m := recv(t, c)
		var s Snapshot
		json.Unmarshal(m.Payload, &s)
		if m.Type != TypeSnapshot || !s.State.Paused {
			t.Fatalf("lastSeq %d: %+v", last, m)
		}
		c.Close(websocket.StatusNormalClosure, "")
		waitConnected(t, h.srv, false)
	}
}

func TestNewConnectionReplacesTheOld(t *testing.T) {
	h := start(t, Options{})
	c1 := h.dial(token, 0)
	recv(t, c1)
	waitConnected(t, h.srv, true)
	c2 := h.dial(token, 0)
	recv(t, c2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := c1.Read(ctx); err == nil {
		t.Fatal("the old connection is still open")
	}
	h.srv.Publish(TypeNotice, Notice{Text: "to c2"})
	if m := recv(t, c2); m.Type != TypeNotice {
		t.Fatalf("%+v", m)
	}
}

func TestAppMessagesAreDelivered(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	send(t, c, TypePause, struct{}{})
	select {
	case m := <-h.msgs:
		if m.Type != TypePause {
			t.Fatalf("%+v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pause not delivered")
	}
}

// fakeApp records published requests and answers them.
type fakeApp struct {
	mu        sync.Mutex
	connected bool
	published []ConfirmRequest
	onPublish func(ConfirmRequest)
}

func (f *fakeApp) Connected() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.connected }
func (f *fakeApp) Publish(typ string, payload any) error {
	r := payload.(ConfirmRequest)
	f.mu.Lock()
	f.published = append(f.published, r)
	cb := f.onPublish
	f.mu.Unlock()
	if cb != nil {
		go cb(r)
	}
	return nil
}

type fixed confirm.Decision

func (d fixed) Confirm(context.Context, confirm.Request) confirm.Decision { return confirm.Decision(d) }

func reply(c *Confirmer, id, decision string) {
	raw, _ := json.Marshal(ConfirmReply{ID: id, Decision: decision})
	c.Reply(raw)
}

func TestConfirmOnThePhone(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Allow))
	req := confirm.Request{ID: "j_1", Title: "Restart", Key: "reboot\x00shell\x00reboot", ExpiresAt: time.Now().Add(time.Minute)}
	for decision, want := range map[string]confirm.Decision{"allow": confirm.Allow, "deny": confirm.Deny, "bogus": confirm.Deny} {
		app.onPublish = func(r ConfirmRequest) { reply(c, r.ID, decision) }
		if got := c.Confirm(context.Background(), req); got != want {
			t.Errorf("%s: %v want %v", decision, got, want)
		}
	}
	if len(app.published) != 3 || app.published[0].Title != "Restart" || !app.published[0].AllowSession {
		t.Fatalf("%+v", app.published)
	}
}

func TestAllowSessionCoversOnlyTheSameCommand(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Deny))
	app.onPublish = func(r ConfirmRequest) { reply(c, r.ID, "allow_session") }
	a := confirm.Request{ID: "j_1", Key: "k1", ExpiresAt: time.Now().Add(time.Minute)}
	if c.Confirm(context.Background(), a) != confirm.Allow {
		t.Fatal("first")
	}
	app.onPublish = nil
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	a.ID = "j_2"
	if c.Confirm(ctx, a) != confirm.Allow {
		t.Fatal("the same command was asked again")
	}
	b := confirm.Request{ID: "j_3", Key: "k2", ExpiresAt: time.Now().Add(time.Minute)}
	if got := c.Confirm(ctx, b); got != confirm.Timeout {
		t.Fatalf("another command was allowed without asking: %v", got)
	}
	// A special command is never allowed for the session.
	app.onPublish = func(r ConfirmRequest) {
		if r.AllowSession {
			t.Error("allow_session offered for a special command")
		}
		reply(c, r.ID, "allow_session")
	}
	s := confirm.Request{ID: "j_4", Key: "k3", Special: true, ExpiresAt: time.Now().Add(time.Minute)}
	c.Confirm(context.Background(), s)
	app.onPublish = nil
	s.ID = "j_5"
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if c.Confirm(ctx2, s) != confirm.Timeout {
		t.Fatal("a special command was remembered for the session")
	}
}

func TestConfirmWithoutAppUsesFallback(t *testing.T) {
	c := NewConfirmer(&fakeApp{}, fixed(confirm.Allow))
	if c.Confirm(context.Background(), confirm.Request{ID: "j_1"}) != confirm.Allow {
		t.Fatal("fallback not used")
	}
}

func TestConfirmCancelAndLateReply(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Allow))
	ctx, cancel := context.WithCancel(context.Background())
	app.onPublish = func(ConfirmRequest) { cancel() }
	if c.Confirm(ctx, confirm.Request{ID: "j_1", ExpiresAt: time.Now().Add(time.Minute)}) != confirm.Deny {
		t.Fatal("cancelled request not denied")
	}
	reply(c, "j_1", "allow") // late: ignored, no panic
	if len(c.Pending()) != 0 {
		t.Fatal("pending not cleaned")
	}
}

func TestAnOldAppIsToldToUpdate(t *testing.T) {
	rejected := make(chan Hello, 1)
	h := start(t, Options{OnReject: func(hl Hello) { rejected <- hl }})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, h.url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send(t, c, TypeHello, Hello{Token: token, AppVersion: "0.1.0"}) // no protocol: an app older than the field
	_, _, err = c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusCode(CloseIncompatible) {
		t.Fatalf("%v, want close %d", err, CloseIncompatible)
	}
	select {
	case hl := <-rejected:
		if hl.AppVersion != "0.1.0" {
			t.Fatalf("%+v", hl)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnReject not called")
	}
	if h.srv.Connected() {
		t.Fatal("an incompatible app counts as connected")
	}
}

func TestAnOldAppWithoutTheTokenLearnsNothing(t *testing.T) {
	h := start(t, Options{})
	c := h.dial("wrong", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := c.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("%v: a wrong token must be rejected before the version check", err)
	}
}
