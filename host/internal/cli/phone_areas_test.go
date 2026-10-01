package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// sendApp sends a message as the app, with rid when not empty.
func sendApp(t *testing.T, c *websocket.Conn, typ, rid string, payload any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	data, _ := json.Marshal(bridge.Message{V: bridge.ProtocolVersion, Type: typ, RID: rid, TS: time.Now(), Payload: raw})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

// linkedApp starts a link and connects to it as the app.
func linkedApp(t *testing.T) (*phoneLink, *websocket.Conn) {
	t.Helper()
	e := linkEnv(t)
	f := &appPhone{}
	l, _, err := startLink(t, e, f, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	c := openApp(t, e.cfg.ChatPort, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: f.handedTokens()[0], AppVersion: "0.3.0"})
	if m := readApp(t, c); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	return l, c
}

// A type this host does not handle gets an error with the request's rid, or
// the notice an M2 app understands when it has no rid: once per type on a
// connection, so a repeating app does not push real events out of the buffer.
func TestUnhandledTypesAreAnswered(t *testing.T) {
	_, c := linkedApp(t)

	sendApp(t, c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "m_1", Text: "hello"})
	m := readApp(t, c)
	if m.Type != bridge.TypeError || m.RID != "r_1" || m.Seq != 0 {
		t.Fatalf("got %s rid %q seq %d, want an error for r_1", m.Type, m.RID, m.Seq)
	}
	var er bridge.ErrorReply
	if err := json.Unmarshal(m.Payload, &er); err != nil || er.Code != bridge.CodeUnsupported || !strings.Contains(er.Text, "chat.send") {
		t.Fatalf("payload %s", m.Payload)
	}

	long := "made.up." + strings.Repeat("x", 200)
	for range 3 {
		sendApp(t, c, long, "", bridge.Empty{})
	}
	sendApp(t, c, "other.type", "", bridge.Empty{})
	m = readApp(t, c)
	if m.Type != bridge.TypeNotice || m.Seq == 0 || !strings.Contains(string(m.Payload), "made.up.") || strings.Contains(string(m.Payload), strings.Repeat("x", 65)) {
		t.Fatalf("got %s seq %d %s, want one short notice event", m.Type, m.Seq, m.Payload)
	}
	if m = readApp(t, c); !strings.Contains(string(m.Payload), "other.type") {
		t.Fatalf("got %s, want the notice of other.type (one per type)", m.Payload)
	}

	// At most maxNotices types per connection: 2 told so far.
	for i := range maxNotices + 5 {
		sendApp(t, c, fmt.Sprintf("type.%d", i), "", bridge.Empty{})
	}
	for i := range maxNotices - 2 {
		if m = readApp(t, c); !strings.Contains(string(m.Payload), fmt.Sprintf("type.%d ", i)) {
			t.Fatalf("notice %d: %s", i, m.Payload)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, data, err := c.Read(ctx); err == nil {
		t.Fatalf("a notice past the limit: %s", data)
	}
}

// Pause and resume still work through the confirm area.
func TestPauseThroughTheAreas(t *testing.T) {
	l, c := linkedApp(t)
	sendApp(t, c, bridge.TypePause, "", bridge.Empty{})
	if m := readApp(t, c); m.Type != bridge.TypeSessionState || !strings.Contains(string(m.Payload), `"paused":true`) {
		t.Fatalf("got %s %s", m.Type, m.Payload)
	}
	if !l.Paused() {
		t.Fatal("not paused")
	}
}

// The areas of the milestones handle different types.
func TestAreasDoNotShareTypes(t *testing.T) {
	l := &phoneLink{}
	defer func() {
		if r := recover(); r != nil {
			t.Fatal(r)
		}
	}()
	l.setAreas(l.confirmArea(), l.chatArea(), l.agentArea(), l.uiArea())

	defer func() {
		if recover() == nil {
			t.Fatal("two areas with the same type were accepted")
		}
	}()
	l.setAreas(l.confirmArea(), l.confirmArea())
}

// The server reaches each area: attach with each connection, also one that
// replaces another, gone when the app leaves, and frames.
func TestAreasHearOfConnections(t *testing.T) {
	var mu sync.Mutex
	var attached, gone, frames int
	test := area{
		attach: func() { mu.Lock(); attached++; mu.Unlock() },
		gone:   func() { mu.Lock(); gone++; mu.Unlock() },
		frame:  func(bridge.FrameID, int64, []byte) { mu.Lock(); frames++; mu.Unlock() },
	}
	count := func() (int, int, int) { mu.Lock(); defer mu.Unlock(); return attached, gone, frames }

	e := linkEnv(t)
	f := &appPhone{}
	l, _, err := startLink(t, e, f, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	l.setAreas(append(l.areas, test)...)
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	hello := bridge.Hello{Protocol: bridge.ProtocolVersion, Token: f.handedTokens()[0], AppVersion: "0.3.0"}
	c1 := openApp(t, e.cfg.ChatPort, hello)
	readApp(t, c1)
	var id bridge.FrameID
	frame, _ := bridge.EncodeFrame(id, 0, []byte("x"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c1.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the frame of the first connection", func() bool { _, _, fr := count(); return fr == 1 })
	c2 := openApp(t, e.cfg.ChatPort, hello)
	readApp(t, c2)
	waitUntil(t, "two attaches", func() bool { a, _, _ := count(); return a == 2 })
	// A replacement is not "the app left".
	time.Sleep(200 * time.Millisecond)
	if _, g, _ := count(); g != 0 {
		t.Fatalf("gone called %d times on a replacement", g)
	}
	c2.Close(websocket.StatusNormalClosure, "")
	waitUntil(t, "gone", func() bool { _, g, _ := count(); return g == 1 })
}

// What is kept per connection starts over: after reconnecting, a type told by
// notice is told again.
func TestNoticesStartOverWithEachConnection(t *testing.T) {
	e := linkEnv(t)
	f := &appPhone{}
	if _, _, err := startLink(t, e, f, &recording{}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	hello := bridge.Hello{Protocol: bridge.ProtocolVersion, Token: f.handedTokens()[0], AppVersion: "0.3.0"}
	c1 := openApp(t, e.cfg.ChatPort, hello)
	w := readApp(t, c1)
	var welcome bridge.Welcome
	if err := json.Unmarshal(w.Payload, &welcome); err != nil {
		t.Fatal(err)
	}

	sendApp(t, c1, "again.type", "", bridge.Empty{})
	first := readApp(t, c1)
	if first.Type != bridge.TypeNotice || !strings.Contains(string(first.Payload), "again.type") {
		t.Fatalf("got %s %s", first.Type, first.Payload)
	}
	sendApp(t, c1, "again.type", "", bridge.Empty{})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, data, err := c1.Read(ctx); err == nil {
		t.Fatalf("a second notice for the same type: %s", data)
	}

	hello.LastSeq, hello.Session = first.Seq, welcome.Session
	c2 := openApp(t, e.cfg.ChatPort, hello)
	if m := readApp(t, c2); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	sendApp(t, c2, "again.type", "", bridge.Empty{})
	m := readApp(t, c2)
	if m.Type != bridge.TypeNotice || m.Seq <= first.Seq || !strings.Contains(string(m.Payload), "again.type") {
		t.Fatalf("got %s seq %d %s, want a new notice after seq %d", m.Type, m.Seq, m.Payload, first.Seq)
	}
}
