package bridge

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
)

// The bridge does not know which attachment ids are accepted (the handler of
// OnFrame does): frames of ids nobody accepted reach it in order with the
// messages and never cut the channel, and what the handler ignores is gone.
func TestFramesOfUnacceptedIDsDoNotCutTheChannel(t *testing.T) {
	accepted, _ := ParseFrameID("00112233445566778899aabbccddeeff")
	stranger, _ := ParseFrameID("ffeeddccbbaa99887766554433221100")
	var mu sync.Mutex
	var kept, seen []string
	h := start(t, Options{OnFrame: func(id FrameID, off int64, data []byte) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, id.String())
		if id == accepted {
			kept = append(kept, string(data))
		}
	}})
	c := h.dial(token, 0)
	recv(t, c)
	for _, f := range []struct {
		id   FrameID
		data string
	}{{stranger, "no"}, {accepted, "yes"}, {stranger, "again"}} {
		frame, err := EncodeFrame(f.id, 0, []byte(f.data))
		if err != nil {
			t.Fatal(err)
		}
		sendRaw(t, c, websocket.MessageBinary, frame)
	}
	// A message after the frames: read in order, so the frames are done.
	send(t, c, TypePause, struct{}{})
	wantMsg(t, h, TypePause)
	if !h.srv.Connected() {
		t.Fatal("frames of ids nobody accepted cut the channel")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 || len(kept) != 1 || kept[0] != "yes" {
		t.Fatalf("frames seen %v, kept %v", seen, kept)
	}
}

// OnAttach comes after OnConnect(true), in order with it, for the first app,
// and alone for one that replaces it.
func TestOnAttachComesAfterOnConnect(t *testing.T) {
	var mu sync.Mutex
	var order []string
	note := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }
	got := func() string { mu.Lock(); defer mu.Unlock(); return strings.Join(order, ",") }
	h := start(t, Options{
		OnConnect: func(b bool) {
			if b {
				note("connect")
			} else {
				note("disconnect")
			}
		},
		OnAttach: func() { note("attach") },
	})
	c1 := h.dial(token, 0)
	recv(t, c1)
	if g := got(); g != "connect,attach" {
		t.Fatalf("order %q, want connect,attach", g)
	}
	c2 := h.dial(token, 0)
	recv(t, c2)
	waitUntilReplaced(t, c1)
	if g := got(); g != "connect,attach,attach" {
		t.Fatalf("after a replacement %q, want connect,attach,attach", g)
	}
}

// OnAttach may Publish: the event reaches the app that is attaching, once,
// numbered after the welcome, and the next attach does not resend it.
func TestPublishFromOnAttachReachesTheNewApp(t *testing.T) {
	var srv atomic.Pointer[Server]
	var n atomic.Int32
	h := start(t, Options{OnAttach: func() {
		if n.Add(1) == 1 {
			_ = srv.Load().Publish(TypeNotice, Notice{Level: "info", Text: "from attach"})
		}
	}})
	srv.Store(h.srv)
	c := h.dial(token, 0)
	if m := recv(t, c); m.Type != TypeWelcome {
		t.Fatalf("got %+v, want the welcome first", m)
	}
	m := recv(t, c)
	if m.Type != TypeNotice || m.Seq != 1 || !strings.Contains(string(m.Payload), "from attach") {
		t.Fatalf("got %+v, want the notice published from OnAttach", m)
	}
	quiet(t, c) // once
	// Resuming after it: nothing is resent.
	c2 := h.dial(token, 1)
	recv(t, c2)
	waitUntilReplaced(t, c)
	quiet(t, c2)
}

// PublishSnapshot sends lists, never null, like the snapshot of a resume.
func TestPublishSnapshotListsAreNeverNull(t *testing.T) {
	h := start(t, Options{Snapshot: func() Snapshot { return Snapshot{} }})
	c := h.dial(token, 0)
	recv(t, c) // welcome
	h.srv.PublishSnapshot()
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
