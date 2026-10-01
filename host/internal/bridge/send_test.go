package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// sendRID sends a message with a request id.
func sendRID(t *testing.T, c *websocket.Conn, typ, rid string, payload any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(Message{V: ProtocolVersion, Type: typ, RID: rid, TS: time.Now(), Payload: raw})
	sendRaw(t, c, websocket.MessageText, data)
}

// quiet fails if c gets anything within a short while.
func quiet(t *testing.T, c *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, data, err := c.Read(ctx); err == nil {
		t.Fatalf("got %s", data)
	}
}

// Each method sends only its class (protocol spec §3): a stream message
// published as an event would be resent after a reconnection.
func TestMethodsRefuseOtherClasses(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	for _, err := range []error{
		h.srv.Publish(TypeUIRequest, UIRequest{}),
		h.srv.Publish(TypeError, ErrorReply{}),
		h.srv.Publish("made.up", Empty{}),
		h.srv.Send(TypeNotice, Notice{}),
		h.srv.Send(TypeError, ErrorReply{}),
		h.srv.Reply(Message{}, TypeTerminalOutput, TerminalOutput{}),
		h.srv.Reply(Message{}, TypeNotice, Notice{}),
	} {
		if !errors.Is(err, ErrClass) {
			t.Errorf("got %v, want ErrClass", err)
		}
	}
	quiet(t, c)
}

// Send reaches the connected app with no number and fails without one.
func TestSendIsNotAnEvent(t *testing.T) {
	h := start(t, Options{})
	if err := h.srv.Send(TypeTerminalOutput, TerminalOutput{Epoch: "e_1"}); !errors.Is(err, ErrNoApp) {
		t.Fatalf("Send without an app: %v, want ErrNoApp", err)
	}
	c := h.dial(token, 0)
	recv(t, c) // welcome
	if err := h.srv.Send(TypeTerminalOutput, TerminalOutput{Epoch: "e_1", Data: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if m := recv(t, c); m.Type != TypeTerminalOutput || m.Seq != 0 || m.RID != "" {
		t.Fatalf("got %+v, want a stream message", m)
	}
}

// Reply answers on the connection the request came on, with its rid; once
// another connection replaced it, the reply goes nowhere.
func TestReplyGoesToTheConnectionThatAsked(t *testing.T) {
	h := start(t, Options{})
	c1 := h.dial(token, 0)
	recv(t, c1)
	sendRID(t, c1, TypeCaptureReq, "r_1", Empty{})
	req := wantMsg(t, h, TypeCaptureReq)
	if req.RID != "r_1" {
		t.Fatalf("rid %q", req.RID)
	}
	if err := h.srv.Reply(req, TypeError, ErrorReply{Code: CodeFailed, Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if m := recv(t, c1); m.Type != TypeError || m.RID != "r_1" || m.Seq != 0 {
		t.Fatalf("got %+v", m)
	}
	sendRID(t, c1, TypeCaptureReq, "r_2", Empty{})
	old := wantMsg(t, h, TypeCaptureReq)

	c2 := h.dial(token, 0)
	recv(t, c2)
	waitUntilReplaced(t, c1)
	if err := h.srv.Reply(old, TypeError, ErrorReply{Code: CodeFailed}); !errors.Is(err, ErrNoApp) {
		t.Fatalf("a reply to the replaced connection: %v, want ErrNoApp", err)
	}
	if h.srv.Current(old) {
		t.Fatal("Current is true for a replaced connection's message")
	}
	// A Message not given by OnMessage has no connection: it goes nowhere.
	if err := h.srv.Reply(Message{RID: "r_9"}, TypeError, ErrorReply{Code: CodeFailed}); !errors.Is(err, ErrNoApp) {
		t.Fatalf("a reply to a built request: %v, want ErrNoApp", err)
	}
	sendRID(t, c2, TypeCaptureReq, "r_3", Empty{})
	now := wantMsg(t, h, TypeCaptureReq)
	if !h.srv.Current(now) {
		t.Fatal("Current is false for the current connection's message")
	}
	if err := h.srv.Reply(Message{conn: now.conn}, TypeError, ErrorReply{Code: CodeFailed}); !errors.Is(err, ErrNoApp) {
		t.Fatalf("a reply without a rid: %v, want ErrNoApp", err)
	}
	quiet(t, c2)
}

// waitUntilReplaced reads c until the server closes it.
func waitUntilReplaced(t *testing.T, c *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return
		}
	}
}

// PublishRID keeps the rid with the event, also when it is resent.
func TestPublishRIDIsResentWithItsRID(t *testing.T) {
	h := start(t, Options{})
	if err := h.srv.PublishRID(TypeChatMessage, "r_6", ChatMessage{ID: "m_2", Role: "user"}); err != nil {
		t.Fatal(err)
	}
	c := h.dial(token, 0)
	recv(t, c)
	if m := recv(t, c); m.Type != TypeChatMessage || m.Seq != 1 || m.RID != "r_6" {
		t.Fatalf("got %+v", m)
	}
}

// What Reply and Send sent is never resent: an app back with lastSeq 0 gets
// only the events.
func TestVolatileMessagesAreNotResent(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	sendRID(t, c, TypeCaptureReq, "r_1", Empty{})
	req := wantMsg(t, h, TypeCaptureReq)
	h.srv.Publish(TypeNotice, Notice{Text: "event"})
	h.srv.Send(TypeTerminalOutput, TerminalOutput{Epoch: "e_1", Data: []byte("x")})
	h.srv.Send(TypeUIRequest, UIRequest{ID: "u_1", Op: "click", Args: json.RawMessage(`{}`)})
	h.srv.Reply(req, TypeError, ErrorReply{Code: CodeFailed})
	for _, want := range []string{TypeNotice, TypeTerminalOutput, TypeUIRequest, TypeError} {
		if m := recv(t, c); m.Type != want {
			t.Fatalf("got %s, want %s", m.Type, want)
		}
	}
	c.Close(websocket.StatusNormalClosure, "")
	waitConnected(t, h.srv, false)

	c2 := h.dial(token, 0)
	if m := recv(t, c2); m.Type != TypeWelcome {
		t.Fatalf("got %s, want welcome", m.Type)
	}
	if m := recv(t, c2); m.Type != TypeNotice || m.Seq != 1 {
		t.Fatalf("got %s seq %d, want the notice again", m.Type, m.Seq)
	}
	quiet(t, c2)
}

// A valid rid reaches OnMessage; any other drops the message.
func TestRequestIDs(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	for _, rid := range []string{strings.Repeat("r", MaxRID+1), "r 1", "ré"} {
		sendRID(t, c, TypeChatSend, rid, ChatSend{Text: "dropped"})
	}
	sendRID(t, c, TypeChatSend, strings.Repeat("r", MaxRID), ChatSend{Text: "kept"})
	m := wantMsg(t, h, TypeChatSend)
	if m.RID != strings.Repeat("r", MaxRID) || !strings.Contains(string(m.Payload), "kept") {
		t.Fatalf("got rid %q payload %s", m.RID, m.Payload)
	}
}

type frame struct {
	id   FrameID
	off  int64
	data string
}

// Valid binary frames reach OnFrame; malformed ones are dropped without
// cutting the channel.
func TestBinaryFramesReachOnFrame(t *testing.T) {
	frames := make(chan frame, 4)
	h := start(t, Options{OnFrame: func(id FrameID, off int64, data []byte) { frames <- frame{id, off, string(data)} }})
	c := h.dial(token, 0)
	recv(t, c)
	id, _ := ParseFrameID("00112233445566778899aabbccddeeff")
	sendRaw(t, c, websocket.MessageBinary, []byte{0, 1, 2, 3}) // shorter than an id
	sendRaw(t, c, websocket.MessageBinary, id[:])              // no offset nor data
	good, _ := EncodeFrame(id, 5, []byte("hello"))
	sendRaw(t, c, websocket.MessageBinary, good)
	select {
	case f := <-frames:
		if f.id != id || f.off != 5 || f.data != "hello" {
			t.Fatalf("got %+v", f)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the frame did not arrive")
	}
	send(t, c, TypePause, struct{}{})
	wantMsg(t, h, TypePause)
	if len(frames) != 0 {
		t.Fatalf("a malformed frame arrived: %+v", <-frames)
	}
}

// A frame the reader had already taken when SetToken came is not delivered
// after the revocation, like a message (settoken_test.go).
func TestAFrameReadBeforeTheRevocationIsNotDelivered(t *testing.T) {
	held, release := make(chan struct{}), make(chan struct{})
	old := beforeFrame
	beforeFrame = func() {
		close(held)
		<-release
	}
	t.Cleanup(func() { beforeFrame = old })
	var got atomic.Int32
	h := start(t, Options{OnFrame: func(FrameID, int64, []byte) { got.Add(1) }})
	c := h.dial(token, 0)
	recv(t, c)
	waitConnected(t, h.srv, true)
	var id FrameID
	f, _ := EncodeFrame(id, 0, []byte("x"))
	sendRaw(t, c, websocket.MessageBinary, f)
	select {
	case <-held:
	case <-time.After(3 * time.Second):
		t.Fatal("the frame never reached the reader")
	}
	h.srv.SetToken(strings.Repeat("cd", 32))
	close(release)
	time.Sleep(300 * time.Millisecond)
	if got.Load() != 0 {
		t.Fatal("the revoked connection delivered a frame")
	}
}

// OnAttach comes with each authenticated connection, also one that replaces
// another (OnConnect reports neither the replacement nor its end).
func TestOnAttachOnEachConnection(t *testing.T) {
	var attached atomic.Int32
	var conns calls
	h := start(t, Options{OnAttach: func() { attached.Add(1) }, OnConnect: conns.add})
	c1 := h.dial(token, 0)
	recv(t, c1)
	c2 := h.dial(token, 0)
	recv(t, c2)
	waitUntilReplaced(t, c1)
	if n := attached.Load(); n != 2 {
		t.Fatalf("OnAttach %d times, want 2", n)
	}
	if got := conns.get(); len(got) != 1 || !got[0] {
		t.Fatalf("OnConnect %v, want [true]", got)
	}
}

// A snapshot always carries lists, never null, whatever Options.Snapshot
// leaves empty: an M2 app reads them as lists.
func TestSnapshotListsAreNeverNull(t *testing.T) {
	h := start(t, Options{Snapshot: func() Snapshot { return Snapshot{} }})
	c := h.dialHello(Hello{Protocol: ProtocolVersion, Token: token, LastSeq: 5, Session: "other", AppVersion: "test"})
	recv(t, c) // welcome
	m := recv(t, c)
	if m.Type != TypeSnapshot {
		t.Fatalf("got %s, want a snapshot", m.Type)
	}
	for _, field := range []string{`"pending":[]`, `"journal":[]`, `"chat":[]`, `"permissions":[]`} {
		if !strings.Contains(string(m.Payload), field) {
			t.Errorf("snapshot %s has no %s", m.Payload, field)
		}
	}
}

// SendTo sends a stream message to the connection that asked for it, and
// nowhere once another one replaced it.
func TestSendToGoesToTheConnectionThatAsked(t *testing.T) {
	h := start(t, Options{})
	c1 := h.dial(token, 0)
	recv(t, c1)
	send(t, c1, TypeTerminalWatch, TerminalWatch{})
	watch := wantMsg(t, h, TypeTerminalWatch)
	if err := h.srv.SendTo(watch, TypeTerminalOutput, TerminalOutput{Epoch: "e_1", Data: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if m := recv(t, c1); m.Type != TypeTerminalOutput || m.Seq != 0 || m.RID != "" {
		t.Fatalf("got %+v", m)
	}
	if err := h.srv.SendTo(watch, TypeError, ErrorReply{}); !errors.Is(err, ErrClass) {
		t.Fatalf("a reply through SendTo: %v, want ErrClass", err)
	}
	if err := h.srv.SendTo(Message{}, TypeTerminalOutput, TerminalOutput{}); !errors.Is(err, ErrNoApp) {
		t.Fatalf("a built request: %v, want ErrNoApp", err)
	}

	c2 := h.dial(token, 0)
	recv(t, c2)
	waitUntilReplaced(t, c1)
	if err := h.srv.SendTo(watch, TypeTerminalOutput, TerminalOutput{Epoch: "e_1", Data: []byte("y")}); !errors.Is(err, ErrNoApp) {
		t.Fatalf("to the replaced connection: %v, want ErrNoApp", err)
	}
	quiet(t, c2)
}

// Backlog counts the messages waiting for the connected app, 0 without one.
func TestBacklogCountsTheQueue(t *testing.T) {
	s := New(Options{})
	if n := s.Backlog(); n != 0 {
		t.Fatalf("without an app: %d", n)
	}
	s.client = &client{send: make(chan outItem, sendQueue), cancel: func() {}, gen: 1}
	if n := s.Backlog(); n != 0 {
		t.Fatalf("empty queue: %d", n)
	}
	for range 5 {
		if err := s.Send(TypeTerminalOutput, TerminalOutput{Epoch: "e_1"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := s.Backlog(); n != 5 {
		t.Fatalf("got %d, want 5", n)
	}
	<-s.client.send
	if n := s.Backlog(); n != 4 {
		t.Fatalf("got %d, want 4", n)
	}
}

// readAny reads the next thing the app gets, text or binary.
func readAny(t *testing.T, c *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	typ, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("nothing for the app: %v", err)
	}
	return typ, data
}

// Frames and messages leave in the order they were queued: the frames of a
// download, an event published between them and the file.end that follows.
func TestSendFrameToKeepsTheOrder(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	recv(t, c)
	sendRID(t, c, TypeFileGet, "r_1", FileGet{})
	req := wantMsg(t, h, TypeFileGet)
	id, _ := ParseFrameID("00112233445566778899aabbccddeeff")
	ctx := context.Background()
	for i := range 3 {
		f, _ := EncodeFrame(id, int64(i), []byte{byte('a' + i)})
		if err := h.srv.SendFrameTo(ctx, req, f); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if err := h.srv.Publish(TypeNotice, Notice{Text: "between"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := h.srv.Reply(req, TypeFileEnd, FileEnd{ID: id.String(), Size: 3}); err != nil {
		t.Fatal(err)
	}
	want := []string{"frame0", "frame1", TypeNotice, "frame2", TypeFileEnd}
	for _, w := range want {
		typ, data := readAny(t, c)
		var got string
		if typ == websocket.MessageBinary {
			_, off, d, err := DecodeFrame(data)
			if err != nil || string(d) != string(rune('a'+off)) {
				t.Fatalf("frame %v %q %v", off, d, err)
			}
			got = "frame" + string(rune('0'+off))
		} else {
			var m Message
			_ = json.Unmarshal(data, &m)
			got = m.Type
		}
		if got != w {
			t.Fatalf("got %s, want %s", got, w)
		}
	}
	if err := h.srv.SendFrameTo(ctx, Message{}, []byte("x")); !errors.Is(err, ErrNoApp) {
		t.Fatalf("a built request: %v, want ErrNoApp", err)
	}
}

// With more than 4 items waiting, SendFrameTo waits: it queues nothing, never
// drops the app, and ends when there is room, with its context, when the
// connection goes or when another one replaced it.
func TestSendFrameToWaitsForRoom(t *testing.T) {
	var gone chan struct{}
	setup := func() (*Server, *client, Message) {
		s := New(Options{})
		gone = make(chan struct{})
		cl := &client{send: make(chan outItem, sendQueue), cancel: func() { t.Error("the app was dropped") }, done: gone, gen: 1}
		s.client = cl
		for range 5 {
			if err := s.Send(TypeTerminalOutput, TerminalOutput{Epoch: "e_1"}); err != nil {
				t.Fatal(err)
			}
		}
		return s, cl, Message{conn: 1}
	}
	run := func(s *Server, ctx context.Context, req Message) chan error {
		out := make(chan error, 1)
		go func() { out <- s.SendFrameTo(ctx, req, []byte("frame")) }()
		return out
	}
	stays := func(t *testing.T, out chan error, cl *client) {
		t.Helper()
		select {
		case err := <-out:
			t.Fatalf("it did not wait: %v", err)
		case <-time.After(150 * time.Millisecond):
		}
		if len(cl.send) != 5 {
			t.Fatalf("queue of %d, want 5: a frame went in", len(cl.send))
		}
	}
	finish := func(t *testing.T, out chan error) error {
		t.Helper()
		select {
		case err := <-out:
			return err
		case <-time.After(3 * time.Second):
			t.Fatal("it did not end")
		}
		return nil
	}

	t.Run("room", func(t *testing.T) {
		s, cl, req := setup()
		out := run(s, context.Background(), req)
		stays(t, out, cl)
		<-cl.send // 4 left: room, but the writer has not said so
		stays4 := len(cl.send)
		cl.signalRoom()
		if err := finish(t, out); err != nil {
			t.Fatal(err)
		}
		if stays4 != 4 || len(cl.send) != 5 {
			t.Fatalf("queue %d then %d, want 4 then 5", stays4, len(cl.send))
		}
	})
	t.Run("context", func(t *testing.T) {
		s, cl, req := setup()
		ctx, cancel := context.WithCancel(context.Background())
		out := run(s, ctx, req)
		stays(t, out, cl)
		cancel()
		if err := finish(t, out); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
		if len(cl.send) != 5 {
			t.Fatal("a frame went in")
		}
	})
	t.Run("connection gone", func(t *testing.T) {
		s, cl, req := setup()
		out := run(s, context.Background(), req)
		stays(t, out, cl)
		close(gone)
		if err := finish(t, out); !errors.Is(err, ErrNoApp) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("replaced", func(t *testing.T) {
		s, cl, req := setup()
		out := run(s, context.Background(), req)
		stays(t, out, cl)
		s.mu.Lock()
		s.client = &client{send: make(chan outItem, sendQueue), cancel: func() {}, gen: 2}
		s.mu.Unlock()
		<-cl.send
		cl.signalRoom()
		if err := finish(t, out); !errors.Is(err, ErrNoApp) {
			t.Fatalf("got %v", err)
		}
	})
}
