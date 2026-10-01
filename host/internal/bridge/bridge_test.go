package bridge

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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
	if o.Token == "" {
		o.Token = token
	}
	return startRaw(t, l, o)
}

// startRaw starts a server with o as given (an empty token stays empty).
func startRaw(t *testing.T, l net.Listener, o Options) *harness {
	t.Helper()
	h := &harness{t: t, msgs: make(chan Message, 100)}
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

// dial connects and sends hello, repeating the session of the server's
// welcome as the app does; it returns the connection.
func (h *harness) dial(tok string, lastSeq uint64) *websocket.Conn {
	h.t.Helper()
	return h.dialHello(Hello{Protocol: ProtocolVersion, Token: tok, LastSeq: lastSeq, Session: "s_TEST", AppVersion: "test"})
}

// dialHello connects and sends hl.
func (h *harness) dialHello(hl Hello) *websocket.Conn {
	h.t.Helper()
	c := h.connect(nil)
	send(h.t, c, TypeHello, hl)
	return c
}

// connect opens the socket without sending anything.
func (h *harness) connect(o *websocket.DialOptions) *websocket.Conn {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, h.url, o)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { c.CloseNow() })
	return c
}

func (h *harness) port() string {
	_, p, _ := net.SplitHostPort(strings.TrimSuffix(strings.TrimPrefix(h.url, "ws://"), "/v1"))
	return p
}

func sendRaw(t *testing.T, c *websocket.Conn, typ websocket.MessageType, data []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, typ, data); err != nil {
		t.Fatal(err)
	}
}

// closeStatus reads until the connection ends and returns its close code.
func closeStatus(t *testing.T, c *websocket.Conn) websocket.StatusCode {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, _, err := c.Read(ctx)
		if err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

func wantMsg(t *testing.T, h *harness, typ string) Message {
	t.Helper()
	select {
	case m := <-h.msgs:
		if m.Type != typ {
			t.Fatalf("got %+v, want %s", m, typ)
		}
		return m
	case <-time.After(3 * time.Second):
		t.Fatalf("%s not delivered", typ)
	}
	return Message{}
}

// calls records the calls to OnConnect.
type calls struct {
	mu sync.Mutex
	v  []bool
}

func (c *calls) add(b bool) { c.mu.Lock(); c.v = append(c.v, b); c.mu.Unlock() }

func (c *calls) get() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]bool(nil), c.v...)
}

func (c *calls) waitFor(t *testing.T, want []bool, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !reflect.DeepEqual(c.get(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("OnConnect calls %v, want %v", c.get(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
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
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("%v (response %+v), want 403", err, resp)
	}
}

// DNS rebinding: a page on evil.example whose name now points to 127.0.0.1
// sends Host and Origin evil.example:<port>, which match each other.
func TestForeignHostIsRejected(t *testing.T) {
	h := start(t, Options{})
	host := "evil.example:" + h.port()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, resp, err := websocket.Dial(ctx, h.url, &websocket.DialOptions{Host: host, HTTPHeader: http.Header{"Origin": []string{"http://" + host}}})
	if err == nil {
		t.Fatal("a page from a rebound name could open the socket")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("%v (response %+v), want 403", err, resp)
	}
	// The names the app uses through adb reverse still work.
	for _, ok := range []string{"127.0.0.1:" + h.port(), "localhost:" + h.port()} {
		c := h.connect(&websocket.DialOptions{Host: ok})
		send(t, c, TypeHello, Hello{Protocol: ProtocolVersion, Token: token, Session: "s_TEST"})
		if m := recv(t, c); m.Type != TypeWelcome {
			t.Fatalf("host %s: %+v", ok, m)
		}
		c.Close(websocket.StatusNormalClosure, "")
		waitConnected(t, h.srv, false)
	}
}

func TestSilentClientIsClosedWithPolicyViolation(t *testing.T) {
	old := helloTimeout
	helloTimeout = 100 * time.Millisecond
	t.Cleanup(func() { helloTimeout = old })
	h := start(t, Options{})
	c := h.connect(nil)
	if got := closeStatus(t, c); got != websocket.StatusPolicyViolation {
		t.Fatalf("close %v, want policy violation", got)
	}
}

func TestEmptyServerTokenRejectsEveryone(t *testing.T) {
	l, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	h := startRaw(t, l, Options{})
	c := h.dialHello(Hello{Protocol: ProtocolVersion, Token: "", AppVersion: "test"})
	if got := closeStatus(t, c); got != websocket.StatusPolicyViolation {
		t.Fatalf("close %v, want policy violation", got)
	}
	if h.srv.Connected() {
		t.Fatal("an empty token lets an app in")
	}
}

func TestOversizedHelloIsRejected(t *testing.T) {
	h := start(t, Options{})
	c := h.dialHello(Hello{Protocol: ProtocolVersion, Token: token, AppVersion: strings.Repeat("x", 5<<10)})
	if got := closeStatus(t, c); got != websocket.StatusMessageTooBig {
		t.Fatalf("close %v, want message too big", got)
	}
}

func TestOversizedMessageClosesTheConnection(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	sendRaw(t, c, websocket.MessageText, []byte(`{"v":1,"type":"chat.send","payload":"`+strings.Repeat("x", maxMessage)+`"}`))
	if got := closeStatus(t, c); got != websocket.StatusMessageTooBig {
		t.Fatalf("close %v, want message too big", got)
	}
	waitConnected(t, h.srv, false)
}

func TestMalformedMessagesAreIgnored(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	sendRaw(t, c, websocket.MessageText, []byte(`{not json`))
	sendRaw(t, c, websocket.MessageText, []byte(`{"v":1,"type":"pause","ts":"yesterday","payload":{}}`))
	wantMsg(t, h, TypePause)
	send(t, c, TypeResume, struct{}{})
	wantMsg(t, h, TypeResume)
	if !h.srv.Connected() {
		t.Fatal("a malformed message cut the channel")
	}
}

func TestBinaryFramesAreIgnored(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	sendRaw(t, c, websocket.MessageBinary, []byte{0, 1, 2, 3})
	send(t, c, TypePause, struct{}{})
	wantMsg(t, h, TypePause)
}

// A restarted host numbers its events again from 1: an app that saw 3 events
// of the previous host must not take events 4 and 5 of this one as the rest.
func TestRestartedHostSendsASnapshot(t *testing.T) {
	h := start(t, Options{})
	for i := 0; i < 5; i++ {
		h.srv.Publish(TypeNotice, Notice{Text: "x"})
	}
	// Same host: resume after 3, no snapshot.
	c := h.dialHello(Hello{Protocol: ProtocolVersion, Token: token, LastSeq: 3, Session: "s_TEST"})
	var wel Welcome
	json.Unmarshal(recv(t, c).Payload, &wel)
	if wel.FromSeq != 4 {
		t.Fatalf("same session: from %d, want 4", wel.FromSeq)
	}
	for _, want := range []uint64{4, 5} {
		if m := recv(t, c); m.Type != TypeNotice || m.Seq != want {
			t.Fatalf("same session: %+v, want notice %d", m, want)
		}
	}
	c.Close(websocket.StatusNormalClosure, "")
	waitConnected(t, h.srv, false)

	// Another host (the session the app remembers is not this one): snapshot.
	c = h.dialHello(Hello{Protocol: ProtocolVersion, Token: token, LastSeq: 3, Session: "s_OLD"})
	json.Unmarshal(recv(t, c).Payload, &wel)
	m := recv(t, c)
	if m.Type != TypeSnapshot || wel.FromSeq != m.Seq {
		t.Fatalf("other session: welcome %+v, then %+v; want a snapshot", wel, m)
	}
}

func TestReplacingAConnectionKeepsTheAppConnected(t *testing.T) {
	var got calls
	h := start(t, Options{OnConnect: got.add})
	c1 := h.dial(token, 0)
	recv(t, c1)
	waitConnected(t, h.srv, true)
	c2 := h.dial(token, 0)
	recv(t, c2)
	closeStatus(t, c1)                 // the old connection is closed…
	time.Sleep(200 * time.Millisecond) // …and its handler has had time to finish
	if v := got.get(); !reflect.DeepEqual(v, []bool{true}) || !h.srv.Connected() {
		t.Fatalf("after replacing: OnConnect %v, connected %v; want [true] and connected", v, h.srv.Connected())
	}
	c2.Close(websocket.StatusNormalClosure, "")
	got.waitFor(t, []bool{true, false}, 3*time.Second)
	waitConnected(t, h.srv, false)
}

// A client that does not keep up is dropped at once, and counts as gone.
func TestSlowClientIsDropped(t *testing.T) {
	s := New(Options{})
	cancelled := false
	s.client = &client{send: make(chan outItem), cancel: func() { cancelled = true }}
	s.Publish(TypeNotice, Notice{Text: "x"})
	if !cancelled || s.Connected() {
		t.Fatalf("cancelled %v, connected %v; want cancelled and not connected", cancelled, s.Connected())
	}
}

func TestSlowClientOverTheNetwork(t *testing.T) {
	var got calls
	h := start(t, Options{OnConnect: got.add})
	c := h.dial(token, 0)
	recv(t, c)
	waitConnected(t, h.srv, true)
	big := strings.Repeat("x", 16<<10)
	for i := 0; i < 20000 && h.srv.Connected(); i++ { // the app stops reading
		h.srv.Publish(TypeNotice, Notice{Text: big})
	}
	if h.srv.Connected() {
		t.Fatal("a client that does not read is never dropped")
	}
	c.CloseNow()
	got.waitFor(t, []bool{true, false}, 15*time.Second)
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
	notices   []Notice
	onPublish func(ConfirmRequest)
}

func (f *fakeApp) Connected() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.connected }
func (f *fakeApp) Publish(typ string, payload any) error {
	if n, ok := payload.(Notice); ok {
		f.mu.Lock()
		f.notices = append(f.notices, n)
		f.mu.Unlock()
		return nil
	}
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
	if got := c.Confirm(context.Background(), s); got != confirm.Allow {
		t.Fatalf("j_4: %v, want allow", got)
	}
	app.onPublish = nil
	s.ID = "j_5"
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if c.Confirm(ctx2, s) != confirm.Timeout {
		t.Fatal("a special command was remembered for the session")
	}
}

func TestConfirmRejectsEmptyOrDuplicateIDs(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Allow))
	short, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if got := c.Confirm(short, confirm.Request{ExpiresAt: time.Now().Add(time.Minute)}); got != confirm.Deny {
		t.Fatalf("empty id: %v, want deny", got)
	}
	if len(app.published) != 0 {
		t.Fatalf("empty id was asked: %+v", app.published)
	}
	asked := make(chan struct{})
	app.onPublish = func(ConfirmRequest) { close(asked) }
	first := make(chan confirm.Decision)
	req := confirm.Request{ID: "j_1", Title: "first", ExpiresAt: time.Now().Add(time.Minute)}
	go func() { first <- c.Confirm(context.Background(), req) }()
	<-asked
	app.mu.Lock()
	app.onPublish = nil
	app.mu.Unlock()
	req.Title = "second"
	if got := c.Confirm(short, req); got != confirm.Deny {
		t.Fatalf("duplicate id: %v, want deny", got)
	}
	if p := c.Pending(); len(p) != 1 || p[0].Title != "first" {
		t.Fatalf("the duplicate replaced the first question: %+v", p)
	}
	reply(c, "j_1", "allow")
	if got := <-first; got != confirm.Allow {
		t.Fatalf("first: %v, want allow", got)
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if len(app.published) != 1 {
		t.Fatalf("published %d, want 1", len(app.published))
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

// Final review, I1: PublishSnapshot sends the current state as a numbered
// event, to the app connected now and, kept like any event, to one that
// resumes after it.
func TestPublishSnapshotIsAnEvent(t *testing.T) {
	var state atomic.Value
	state.Store("idle")
	h := start(t, Options{Snapshot: func() Snapshot {
		return Snapshot{State: SessionState{State: state.Load().(string)}, Pending: []ConfirmRequest{}, Journal: []json.RawMessage{}}
	}})
	c := h.dial(token, 0)
	recv(t, c) // welcome
	waitConnected(t, h.srv, true)
	h.srv.Publish(TypeNotice, Notice{Text: "one"})
	recv(t, c)
	h.srv.PublishSnapshot()
	var snap Snapshot
	if m := recv(t, c); m.Type != TypeSnapshot || m.Seq != 2 || json.Unmarshal(m.Payload, &snap) != nil || snap.State.State != "idle" {
		t.Fatalf("live: %+v", m)
	}
	c.Close(websocket.StatusNormalClosure, "")
	waitConnected(t, h.srv, false)
	state.Store("working")
	h.srv.PublishSnapshot()
	c = h.dial(token, 2)
	recv(t, c) // welcome
	if m := recv(t, c); m.Type != TypeSnapshot || m.Seq != 3 || json.Unmarshal(m.Payload, &snap) != nil || snap.State.State != "working" {
		t.Fatalf("resumed: %+v", m)
	}
}
