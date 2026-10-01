package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/session"
	"github.com/hybriqtech/adbuddy/host/internal/term"
)

// termRig is a link with an app and a running agent, grouping the terminal
// output every tick.
func termRig(t *testing.T, tick time.Duration) *chatRig {
	t.Helper()
	old := termInterval
	termInterval = tick
	t.Cleanup(func() { termInterval = old })
	r := newChatRig(t)
	r.c.SetReadLimit(2 << 20) // the app takes 1 MiB; the test reads what the bridge writes
	r.start(t, "claude")
	return r
}

// say makes the fake agent write s and waits until its screen has it.
func (r *chatRig) say(t *testing.T, s string) {
	t.Helper()
	want := r.m.Screen().Offset() + int64(len(s))
	if _, err := r.pty(0).outW.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the screen to take the output", func() bool { return r.m.Screen().Offset() >= want })
}

// nextTerm is the next terminal.* message the app gets, skipping the session
// state changes the agent's life publishes.
func (r *chatRig) nextTerm(t *testing.T) bridge.Message {
	t.Helper()
	for range 20 {
		m := readApp(t, r.c)
		if m.Type == bridge.TypeSessionState {
			continue
		}
		if !strings.HasPrefix(m.Type, "terminal.") {
			t.Fatalf("got %s %s, want a terminal message", m.Type, m.Payload)
		}
		return m
	}
	t.Fatal("no terminal message")
	return bridge.Message{}
}

func payloadOf[T any](t *testing.T, m bridge.Message) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(m.Payload, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

var syncs atomic.Int64

// settle checks that nothing from the terminal reaches the app for a while:
// after some ticks it sends a chat.send and expects its chat.message next
// (a read that times out would close the socket).
func (r *chatRig) settle(t *testing.T) {
	t.Helper()
	time.Sleep(5 * termInterval)
	id := fmt.Sprintf("m_settle%d", syncs.Add(1))
	sendApp(t, r.c, bridge.TypeChatSend, "r_"+id, bridge.ChatSend{ID: id, Text: "sync"})
	for range 20 {
		m := readApp(t, r.c)
		switch {
		case m.Type == bridge.TypeSessionState:
		case (m.Type == bridge.TypeChatMessage || m.Type == bridge.TypeError) && m.RID == "r_"+id:
			return
		default:
			t.Fatalf("got %s %s, want nothing from the terminal", m.Type, m.Payload)
		}
	}
	t.Fatal("no answer to the sync")
}

func (r *chatRig) watch(t *testing.T, epoch string, offset int64) {
	t.Helper()
	sendApp(t, r.c, bridge.TypeTerminalWatch, "", bridge.TerminalWatch{Epoch: epoch, Offset: offset})
}

// A watch from an offset the host still has gets only the new bytes; from an
// old one (another epoch, nothing, or lost) it gets the screen.
func TestTerminalWatchResumesFromTheTailOrSnapshots(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.say(t, "hello world")
	epoch := r.m.Screen().Epoch()
	r.m.ResizePhone(100, 30)

	r.watch(t, epoch, 5)
	m := r.nextTerm(t)
	out := payloadOf[bridge.TerminalOutput](t, m)
	if m.Type != bridge.TypeTerminalOutput || out.Epoch != epoch || out.Offset != 5 || string(out.Data) != " world" || m.Seq != 0 {
		t.Fatalf("got %s %+v", m.Type, out)
	}
	// It also learns the size, which changed while it was not watching.
	m = r.nextTerm(t)
	if sz := payloadOf[bridge.TerminalSize](t, m); m.Type != bridge.TypeTerminalSize || sz.Epoch != epoch || sz.Offset != 11 || sz.Cols != 100 || sz.Rows != 30 {
		t.Fatalf("got %s %s, want the size 100x30 at 11", m.Type, m.Payload)
	}
	r.settle(t)

	// A watch from nothing, and from another epoch, get the screen.
	for _, w := range []bridge.TerminalWatch{{}, {Epoch: "e_other", Offset: 3}, {Epoch: epoch, Offset: 99}} {
		r.watch(t, w.Epoch, w.Offset)
		m = r.nextTerm(t)
		snap := payloadOf[bridge.TerminalSnapshot](t, m)
		if m.Type != bridge.TypeTerminalSnapshot || snap.Epoch != epoch || snap.Offset != 11 ||
			snap.Cols != 100 || snap.Rows != 30 || !strings.Contains(string(snap.Data), "hello world") {
			t.Fatalf("watch %+v: got %s %+v", w, m.Type, snap)
		}
	}
	r.settle(t)
}

// A watch from an offset the app already has everything up to sends nothing
// but keeps the output coming.
func TestTerminalWatchUpToDateThenLive(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.say(t, "abc")
	epoch := r.m.Screen().Epoch()
	r.watch(t, epoch, 3)
	// The app learns the size it has to draw with.
	m := r.nextTerm(t)
	if sz := payloadOf[bridge.TerminalSize](t, m); m.Type != bridge.TypeTerminalSize || sz.Epoch != epoch || sz.Offset != 3 || sz.Cols != session.DefaultCols || sz.Rows != session.DefaultRows {
		t.Fatalf("got %s %s", m.Type, m.Payload)
	}
	r.settle(t)
	r.say(t, "def")
	out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Offset != 3 || string(out.Data) != "def" {
		t.Fatalf("got %+v", out)
	}
}

// Without an agent a watch sends nothing; the screen goes out when it starts.
func TestTerminalWatchWithoutAnAgent(t *testing.T) {
	old := termInterval
	termInterval = 50 * time.Millisecond
	t.Cleanup(func() { termInterval = old })
	r := newChatRig(t)
	r.watch(t, "", 0)
	r.settle(t)
	r.start(t, "claude")
	m := r.nextTerm(t)
	if m.Type != bridge.TypeTerminalSnapshot || payloadOf[bridge.TerminalSnapshot](t, m).Epoch != r.m.Screen().Epoch() {
		t.Fatalf("got %s %s", m.Type, m.Payload)
	}
}

// Live output is grouped by tick and continues from the offset of the last.
func TestTerminalLiveOutputIsGrouped(t *testing.T) {
	r := termRig(t, 300*time.Millisecond)
	r.watch(t, "", 0)
	if m := r.nextTerm(t); m.Type != bridge.TypeTerminalSnapshot {
		t.Fatalf("got %s", m.Type)
	}
	r.say(t, "a")
	r.say(t, "b")
	r.say(t, "c")
	out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Offset != 0 || string(out.Data) != "abc" {
		t.Fatalf("got %+v, want one piece abc at 0", out)
	}
	r.say(t, "d")
	out = payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Offset != 3 || string(out.Data) != "d" {
		t.Fatalf("got %+v", out)
	}
}

// A big burst goes in pieces of at most 64 KiB, in order and with no gaps.
func TestTerminalOutputGoesInChunks(t *testing.T) {
	r := termRig(t, 300*time.Millisecond)
	r.watch(t, "", 0)
	r.nextTerm(t)
	r.say(t, strings.Repeat("x", 150<<10))
	var got, at int
	for got < 150<<10 {
		out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
		if len(out.Data) > 64<<10 || int(out.Offset) != at {
			t.Fatalf("piece of %d at %d, want at most 64 KiB at %d", len(out.Data), out.Offset, at)
		}
		got += len(out.Data)
		at += len(out.Data)
	}
}

// Resuming from the tail sends it in pieces too: Since can give 512 KiB.
func TestTerminalResumeGoesInChunks(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.say(t, strings.Repeat("y", 200<<10))
	r.watch(t, r.m.Screen().Epoch(), 0)
	var got int
	for got < 200<<10 {
		out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
		if len(out.Data) > 64<<10 || int(out.Offset) != got {
			t.Fatalf("piece of %d at %d, want at most 64 KiB at %d", len(out.Data), out.Offset, got)
		}
		got += len(out.Data)
	}
}

// What the Manager hands the subscription before the watch's own start (the
// output that came between Since and the subscription) is neither repeated
// nor lost: the bytes below next are dropped, a gap asks for the screen.
func TestTerminalNoDuplicatesNoGaps(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.say(t, "hello")
	epoch := r.m.Screen().Epoch()
	r.watch(t, epoch, 5) // up to date: next = 5
	if m := r.nextTerm(t); m.Type != bridge.TypeTerminalSize {
		t.Fatalf("got %s", m.Type)
	}
	r.settle(t)

	feed := func(off int64, s string) {
		r.l.onTermOutput(session.Output{Epoch: epoch, Offset: off, Data: []byte(s)})
	}
	feed(0, "hello") // already there
	feed(3, "lo!!")  // two bytes it has, two new: offset 5..7
	feed(7, "?")
	out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Offset != 5 || string(out.Data) != "!!?" {
		t.Fatalf("got %+v, want !!? at 5", out)
	}
	r.settle(t)

	// A gap: 8 is next, 20 comes. The screen, which has "hello" only, is sent.
	feed(20, "zz")
	m := r.nextTerm(t)
	snap := payloadOf[bridge.TerminalSnapshot](t, m)
	if m.Type != bridge.TypeTerminalSnapshot || snap.Offset != 5 {
		t.Fatalf("got %s %+v", m.Type, snap)
	}
	// And it continues from the snapshot's offset.
	r.say(t, "!")
	out = payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Offset != 5 || string(out.Data) != "!" {
		t.Fatalf("got %+v", out)
	}
}

// Output that arrives with the real session between the watch and the first
// tick is sent once.
func TestTerminalOutputRightAfterTheWatch(t *testing.T) {
	r := termRig(t, 200*time.Millisecond)
	r.say(t, "one")
	r.watch(t, "", 0)
	// The snapshot is the screen at offset 3, with "one".
	m := r.nextTerm(t)
	snap := payloadOf[bridge.TerminalSnapshot](t, m)
	if m.Type != bridge.TypeTerminalSnapshot || snap.Offset != 3 || !strings.Contains(string(snap.Data), "one") {
		t.Fatalf("got %s %+v", m.Type, snap)
	}
	// What comes next continues at 3: once, with no gap.
	r.say(t, "two")
	out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Offset != 3 || string(out.Data) != "two" {
		t.Fatalf("got %+v, want two at 3", out)
	}
	r.settle(t)
}

// More output than the host keeps for a tick gives up on the pieces and sends
// the screen instead.
func TestTerminalOverflowSendsASnapshot(t *testing.T) {
	r := termRig(t, 500*time.Millisecond)
	r.say(t, "ab")
	r.watch(t, r.m.Screen().Epoch(), 2)
	r.nextTerm(t) // the size
	chunk := []byte(strings.Repeat("z", 600<<10))
	epoch := r.m.Screen().Epoch()
	r.l.onTermOutput(session.Output{Epoch: epoch, Offset: 2, Data: chunk})
	r.l.onTermOutput(session.Output{Epoch: epoch, Offset: 2 + int64(len(chunk)), Data: chunk})
	m := r.nextTerm(t)
	snap := payloadOf[bridge.TerminalSnapshot](t, m)
	if m.Type != bridge.TypeTerminalSnapshot || snap.Offset != 2 {
		t.Fatalf("got %s %d bytes, want a snapshot at 2", m.Type, len(m.Payload))
	}
	r.settle(t)
}

// A watch whose connection is gone (SendTo says ErrNoApp) ends.
func TestTerminalWatchEndsWhenItsConnectionIsGone(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.say(t, "x")
	w := &termWatch{msg: bridge.Message{Type: bridge.TypeTerminalWatch}, done: make(chan struct{}), snapDue: true}
	r.l.term.mu.Lock()
	r.l.term.w = w
	r.l.term.mu.Unlock()
	r.l.termTick(w)
	if r.watched() {
		t.Fatal("the watch of a gone connection is still on")
	}
	select {
	case <-w.done:
	default:
		t.Fatal("its tick goroutine was not told to end")
	}
}

// A size change under pressure is not sent: the snapshot that follows has
// it. And after a pressure the snapshot waits until the queue is down to
// half the limit.
func TestTerminalSizeUnderPressureAndHysteresis(t *testing.T) {
	level := pressureLevel(t)
	r := termRig(t, 50*time.Millisecond)
	r.watch(t, "", 0)
	r.nextTerm(t)

	level.Store(int64(termBacklogLimit + 1))
	r.m.ResizePhone(100, 30)
	time.Sleep(4 * termInterval)
	level.Store(int64(termBacklogLimit/2 + 1)) // eased, but not enough
	r.settle(t)
	level.Store(int64(termBacklogLimit / 2))
	m := r.nextTerm(t)
	snap := payloadOf[bridge.TerminalSnapshot](t, m)
	if m.Type != bridge.TypeTerminalSnapshot || snap.Cols != 100 || snap.Rows != 30 {
		t.Fatalf("got %s %s, want a snapshot of 100x30", m.Type, m.Payload)
	}
	r.settle(t) // and no terminal.size after it
}

// pressureLevel makes the app's queue look as long as the returned value says
// (negative: the real one).
func pressureLevel(t *testing.T) *atomic.Int64 {
	t.Helper()
	level := &atomic.Int64{}
	level.Store(-1)
	old := backlogOf
	backlogOf = func(s *bridge.Server) int {
		if n := level.Load(); n >= 0 {
			return int(n)
		}
		return old(s)
	}
	t.Cleanup(func() { backlogOf = old })
	return level
}

// A screen the PC made huge and full of styles goes as a plain snapshot, so
// that the message does not pass what the app takes (1 MiB).
func TestTerminalHugeStyledScreenGoesPlain(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.m.AttachPC("pc_1", 400, 200)
	var b strings.Builder
	for y := range 200 {
		for x := range 400 {
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm%c", x%256, y%256, (x+y)%256, (x*3)%256, (y*5)%256, (x+y*2)%256, 'a'+rune((x+y)%26))
		}
		b.WriteString("\x1b[0m\r\n")
	}
	r.say(t, b.String())
	r.watch(t, "", 0)
	m := r.nextTerm(t)
	snap := payloadOf[bridge.TerminalSnapshot](t, m)
	if m.Type != bridge.TypeTerminalSnapshot || len(m.Payload) >= 1<<20 || snap.Cols != 400 || snap.Rows != 200 {
		t.Fatalf("got %s of %d bytes %dx%d", m.Type, len(m.Payload), snap.Cols, snap.Rows)
	}
	if strings.Contains(string(snap.Data), "38;2;") || !strings.Contains(string(snap.Data), "abcdefghij") {
		t.Fatal("the snapshot is not plain text")
	}
	t.Logf("plain snapshot payload: %d bytes", len(m.Payload))
}

// Another process of the agent sends its own screen.
func TestTerminalNewEpochSendsASnapshot(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	r.say(t, "old")
	first := r.m.Screen().Epoch()
	r.watch(t, "", 0)
	r.nextTerm(t)

	if err := r.m.Start("claude", false); err != nil {
		t.Fatal(err)
	}
	m := r.nextTerm(t)
	snap := payloadOf[bridge.TerminalSnapshot](t, m)
	if m.Type != bridge.TypeTerminalSnapshot || snap.Epoch == first || snap.Epoch != r.m.Screen().Epoch() || snap.Offset != 0 {
		t.Fatalf("got %s %+v (first epoch %s)", m.Type, snap, first)
	}
	r.pty(1).outW.Write([]byte("new"))
	out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Epoch != snap.Epoch || out.Offset != 0 || string(out.Data) != "new" {
		t.Fatalf("got %+v", out)
	}
}

// A size change is noticed at the tick and goes after the output before it.
func TestTerminalSizeComesAfterThePreviousOutput(t *testing.T) {
	r := termRig(t, 300*time.Millisecond)
	r.watch(t, "", 0)
	r.nextTerm(t)
	r.say(t, "xyz")
	r.m.ResizePhone(100, 30)

	m := r.nextTerm(t)
	if out := payloadOf[bridge.TerminalOutput](t, m); m.Type != bridge.TypeTerminalOutput || string(out.Data) != "xyz" {
		t.Fatalf("got %s %s, want the output first", m.Type, m.Payload)
	}
	m = r.nextTerm(t)
	sz := payloadOf[bridge.TerminalSize](t, m)
	if m.Type != bridge.TypeTerminalSize || sz.Cols != 100 || sz.Rows != 30 || sz.Offset != 3 || sz.Epoch != r.m.Screen().Epoch() {
		t.Fatalf("got %s %+v", m.Type, sz)
	}
	r.settle(t) // and no more of it
}

// terminal.unwatch, a new connection and the app going away end the
// subscription; a watch of a connection that is not current is ignored.
func TestTerminalSubscriptionEnds(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	end := map[string]func(){
		"unwatch": func() { sendApp(t, r.c, bridge.TypeTerminalUnwatch, "", bridge.Empty{}) },
		"attach":  r.l.onAttach,
		"gone":    r.l.onGone,
	}
	for _, name := range []string{"unwatch", "attach", "gone"} {
		r.watch(t, "", 0)
		r.nextTerm(t)
		end[name]()
		waitUntil(t, name+" to end the watch", func() bool { return !r.watched() })
		r.say(t, name)
		r.settle(t)
	}

	// A watch that did not come on the current connection.
	r.l.onTermWatch(bridge.Message{Type: bridge.TypeTerminalWatch, Payload: json.RawMessage(`{}`)})
	r.settle(t)
	r.l.term.mu.Lock()
	w := r.l.term.w
	r.l.term.mu.Unlock()
	if w != nil {
		t.Fatal("a watch of no connection subscribed")
	}
}

// terminal.input reaches the agent as it is, up to 64 KiB; more is dropped.
func TestTerminalInputLimits(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte("\x1b[A\x03ls")})
	waitUntil(t, "the input", func() bool { return r.pty(0).said() == "\x1b[A\x03ls" })

	sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte(strings.Repeat("q", 64<<10+1))})
	sendApp(t, r.c, bridge.TypeTerminalInput, "", bridge.TerminalInput{Data: []byte(strings.Repeat("k", 64<<10))})
	waitUntil(t, "the 64 KiB input", func() bool { return len(r.pty(0).said()) == 6+64<<10 })
	if strings.Contains(r.pty(0).said(), "q") {
		t.Fatal("an input over 64 KiB was typed")
	}
	// Garbage is dropped too, and the channel goes on.
	sendApp(t, r.c, bridge.TypeTerminalInput, "", map[string]any{"data": 5})
	r.settle(t)
}

// terminal.resize takes 1..1000 per side, and the host brings it to 200×100.
func TestTerminalResizeLimits(t *testing.T) {
	r := termRig(t, 50*time.Millisecond)
	size := func() [2]int { c, rw := r.m.Screen().Size(); return [2]int{c, rw} }
	for _, bad := range []bridge.TerminalResize{{Cols: 0, Rows: 30}, {Cols: 80, Rows: 0}, {Cols: 1001, Rows: 30}, {Cols: 80, Rows: 1001}, {Cols: -1, Rows: 5}} {
		sendApp(t, r.c, bridge.TypeTerminalResize, "", bad)
	}
	r.settle(t)
	if got := size(); got != [2]int{session.DefaultCols, session.DefaultRows} {
		t.Fatalf("an invalid resize applied: %v", got)
	}
	for _, c := range []struct {
		in   bridge.TerminalResize
		want [2]int
	}{
		{bridge.TerminalResize{Cols: 80, Rows: 24}, [2]int{80, 24}},
		{bridge.TerminalResize{Cols: 1, Rows: 1}, [2]int{1, 1}},
		{bridge.TerminalResize{Cols: 1000, Rows: 1000}, [2]int{200, 100}},
		{bridge.TerminalResize{Cols: 500, Rows: 40}, [2]int{200, 40}},
		{bridge.TerminalResize{Cols: 90, Rows: 400}, [2]int{90, 100}},
	} {
		sendApp(t, r.c, bridge.TypeTerminalResize, "", c.in)
		waitUntil(t, fmt.Sprintf("%+v to give %v", c.in, c.want), func() bool { return size() == c.want })
	}
}

// A snapshot of the biggest screen the host allows (200x100), every cell with a truecolor fg+bg style
// of its own, still fits in a message of the bridge (1 MiB).
func TestTerminalSnapshotOfTheBiggestScreenFits(t *testing.T) {
	s := term.New("e_big", termCols, termRows)
	defer s.Close()
	var b strings.Builder
	for y := range termRows {
		for x := range termCols {
			fmt.Fprintf(&b, "[1;4;38;2;%d;%d;%d;48;2;%d;%d;%dm%c", x%256, y%256, (x+y)%256, (x*3)%256, (y*5)%256, (x+y*2)%256, 'a'+rune((x+y)%26))
		}
		b.WriteString("\x1b[0m\r\n")
	}
	s.Write([]byte(b.String()))
	data, off, cols, rows := s.Snapshot()
	raw, err := json.Marshal(bridge.Message{V: bridge.ProtocolVersion, Type: bridge.TypeTerminalSnapshot, Payload: mustJSON(t,
		bridge.TerminalSnapshot{Epoch: "e_big", Offset: off, Cols: cols, Rows: rows, Data: data})})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot data %d bytes, message %d bytes", len(data), len(raw))
	if len(raw) >= 1<<20 {
		t.Fatalf("the message weighs %d bytes: it does not fit in 1 MiB", len(raw))
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// With the app's queue pressed nothing goes out; when it eases, a snapshot.
func TestTerminalPressureSendsASnapshotAfterwards(t *testing.T) {
	var high atomic.Bool
	oldB := backlogOf
	backlogOf = func(s *bridge.Server) int {
		if high.Load() {
			return termBacklogLimit + 1
		}
		return oldB(s)
	}
	t.Cleanup(func() { backlogOf = oldB })
	r := termRig(t, 50*time.Millisecond)
	r.watch(t, "", 0)
	r.nextTerm(t)

	high.Store(true)
	r.say(t, "while pressed ")
	r.say(t, "more")
	time.Sleep(4 * termInterval)
	high.Store(false)
	m := r.nextTerm(t)
	snap := payloadOf[bridge.TerminalSnapshot](t, m)
	if m.Type != bridge.TypeTerminalSnapshot || snap.Offset != 18 || !strings.Contains(string(snap.Data), "while pressed more") {
		t.Fatalf("got %s %+v, want a snapshot with what came while pressed", m.Type, snap)
	}
	r.say(t, "!")
	out := payloadOf[bridge.TerminalOutput](t, r.nextTerm(t))
	if out.Offset != 18 || string(out.Data) != "!" {
		t.Fatalf("got %+v", out)
	}
}

// A watch made while the queue is pressed gets nothing until it eases.
func TestTerminalWatchUnderPressure(t *testing.T) {
	var high atomic.Bool
	high.Store(true)
	oldB := backlogOf
	backlogOf = func(s *bridge.Server) int {
		if high.Load() {
			return termBacklogLimit + 1
		}
		return oldB(s)
	}
	t.Cleanup(func() { backlogOf = oldB })
	r := termRig(t, 50*time.Millisecond)
	r.say(t, "hi")
	r.watch(t, "", 0)
	time.Sleep(4 * termInterval)
	high.Store(false)
	if m := r.nextTerm(t); m.Type != bridge.TypeTerminalSnapshot {
		t.Fatalf("got %s", m.Type)
	}
}

// watched reports whether a terminal subscription is on.
func (r *chatRig) watched() bool {
	r.l.term.mu.Lock()
	defer r.l.term.mu.Unlock()
	return r.l.term.w != nil
}
