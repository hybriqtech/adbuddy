package term

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
)

// The tail keeps at least TailSize bytes and at most twice that, and stays
// in step with the offset, also with many small writes.
func TestTailBounds(t *testing.T) {
	s := New("e_1", 80, 24)
	defer s.Close()
	chunk := bytes.Repeat([]byte("y"), 1000)
	for i := 0; i < 3*TailSize/len(chunk); i++ {
		s.Write(chunk)
		s.mu.Lock()
		n := len(s.tail)
		s.mu.Unlock()
		if n > 2*TailSize || (s.Offset() >= TailSize && n < TailSize) {
			t.Fatalf("tail of %d bytes at offset %d", n, s.Offset())
		}
	}
	at := s.Write([]byte("end"))
	if got, ok := s.Since("e_1", at-int64(len(chunk))); !ok || string(got) != string(chunk)+"end" {
		t.Fatalf("Since = %d bytes, %v", len(got), ok)
	}
	if _, ok := s.Since("e_1", s.Offset()-2*TailSize-1); ok {
		t.Fatal("Since beyond twice TailSize")
	}
}

// The emulator keeps no scrollback: nobody reads it.
func TestNoScrollback(t *testing.T) {
	s := New("e_1", 20, 5)
	defer s.Close()
	for i := 0; i < 200; i++ {
		s.Write([]byte("a line that scrolls\r\n"))
	}
	s.mu.Lock()
	n := s.em.Scrollback().Len()
	s.mu.Unlock()
	if n > 1 {
		t.Fatalf("scrollback of %d lines", n)
	}
}

// Close ends the goroutine that drains the emulator's answers before it
// closes the emulator, and the screen takes writes after it.
func TestCloseEndsTheDrain(t *testing.T) {
	s := New("e_1", 20, 5)
	s.Write([]byte("\x1b[6n")) // a query: the emulator answers on its pipe
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.drained:
	case <-time.After(5 * time.Second):
		t.Fatal("the drain goroutine still runs after Close")
	}
	s.Write([]byte("after"))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// tui is output in the style of a TUI: colours, absolute moves, a partial
// repaint and Unicode.
var tui = []byte("\x1b[2J\x1b[H\x1b[1;38;2;200;100;0mClaude\x1b[0m banner\r\n" +
	"\x1b[5;1H❯ Responde solo: ÑANDÚ42\r\n" +
	"\x1b[7;3H\x1b[32m● ÑANDÚ42\x1b[0m\r\n" +
	"\x1b[10;1H────────────\x1b[11;1H❯ \x1b[11;3Hdraft\x1b[2D")

// redraw writes data to a clean emulator of the size.
func redraw(data []byte, cols, rows int) *vt.Emulator {
	em := vt.NewEmulator(cols, rows)
	go io.Copy(io.Discard, em)
	em.Write(data)
	return em
}

// sameScreen checks that the redrawn emulator shows the screen of s: text,
// styles and cursor.
func sameScreen(t *testing.T, s *Screen, got *vt.Emulator) {
	t.Helper()
	s.mu.Lock()
	text, render, cur := s.em.String(), s.em.Render(), s.em.CursorPosition()
	s.mu.Unlock()
	if got.String() != text {
		t.Errorf("redrawn text:\n%s\nwant:\n%s", got.String(), text)
	}
	if got.Render() != render {
		t.Errorf("redrawn with styles:\n%q\nwant:\n%q", got.Render(), render)
	}
	if got.CursorPosition() != cur {
		t.Errorf("redrawn cursor at %v, want %v", got.CursorPosition(), cur)
	}
}

// A snapshot written to a clean terminal draws the same screen (spike S4).
func TestSnapshotRedrawsTheScreen(t *testing.T) {
	s := New("e_1", 60, 14)
	defer s.Close()
	s.Write(tui)
	data, off, cols, rows := s.Snapshot()
	if off != int64(len(tui)) || cols != 60 || rows != 14 {
		t.Fatalf("offset %d size %dx%d", off, cols, rows)
	}
	em := redraw(data, cols, rows)
	defer em.Close()
	sameScreen(t, s, em)
	if !strings.Contains(s.Text(), "● ÑANDÚ42") {
		t.Fatalf("screen %q", s.Text())
	}
}

// After a resize the snapshot has the new size and still redraws the screen.
func TestSnapshotAfterResize(t *testing.T) {
	s := New("e_1", 60, 14)
	defer s.Close()
	s.Write(tui)
	s.Resize(40, 12)
	s.Write([]byte("\x1b[2J\x1b[Hrepainted at 40x12"))
	data, _, cols, rows := s.Snapshot()
	if cols != 40 || rows != 12 {
		t.Fatalf("size %dx%d", cols, rows)
	}
	em := redraw(data, cols, rows)
	defer em.Close()
	sameScreen(t, s, em)
}

// Since returns what follows an offset of this epoch while it is in the tail.
func TestSince(t *testing.T) {
	s := New("e_1", 80, 24)
	defer s.Close()
	s.Write([]byte("hello "))
	at := s.Write([]byte("world"))
	if at != 6 {
		t.Fatalf("offset of the second write %d", at)
	}
	if got, ok := s.Since("e_1", 6); !ok || string(got) != "world" {
		t.Fatalf("Since(6) = %q %v", got, ok)
	}
	if got, ok := s.Since("e_1", 11); !ok || len(got) != 0 {
		t.Fatalf("Since(end) = %q %v", got, ok)
	}
	for _, c := range []struct {
		epoch string
		off   int64
	}{{"e_2", 0}, {"e_1", 12}, {"e_1", -1}} {
		if _, ok := s.Since(c.epoch, c.off); ok {
			t.Errorf("Since(%s, %d) ok", c.epoch, c.off)
		}
	}
	// Older than the tail: a snapshot is due. The tail holds between
	// TailSize and twice that.
	s.Write(bytes.Repeat([]byte("x"), 2*TailSize))
	if _, ok := s.Since("e_1", 6); ok {
		t.Fatal("Since an offset out of the tail")
	}
	if got, ok := s.Since("e_1", s.Offset()-3); !ok || string(got) != "xxx" {
		t.Fatalf("Since the end of the tail = %q %v", got, ok)
	}
}

// The bracketed paste mode follows the program, also split between writes.
func TestBracketedPaste(t *testing.T) {
	s := New("e_1", 80, 24)
	defer s.Close()
	if s.BracketedPaste() {
		t.Fatal("on at start")
	}
	s.Write([]byte("abc\x1b[?20"))
	s.Write([]byte("04hdef"))
	if !s.BracketedPaste() {
		t.Fatal("split sequence missed")
	}
	s.Write([]byte("\x1b[?2004l"))
	if s.BracketedPaste() {
		t.Fatal("still on")
	}
	s.Write([]byte("\x1b[?2004l\x1b[?2004h"))
	if !s.BracketedPaste() {
		t.Fatal("the last sequence of a write rules")
	}
}

// The snapshot carries the modes the program set (bracketed paste, hidden
// cursor, alternate screen), on or off, so a terminal that attaches gets them.
func TestSnapshotCarriesTheModes(t *testing.T) {
	s := New("e_1", 40, 10)
	defer s.Close()
	off := "\x1b[?1049l\x1b[?25h\x1b[?2004l"
	if got := string(s.Modes()); got != off {
		t.Fatalf("modes at start %q, want %q", got, off)
	}
	s.Write([]byte("main screen\x1b[?2004h\x1b[?25l\x1b[?1049h\x1b[Hin the alternate screen"))
	on := "\x1b[?1049h\x1b[?25l\x1b[?2004h"
	if got := string(s.Modes()); got != on {
		t.Fatalf("modes %q, want %q", got, on)
	}
	data, _, cols, rows := s.Snapshot()
	snap := string(data)
	if !strings.HasPrefix(snap, "\x1b[?1049h") {
		t.Fatalf("the snapshot does not enter the alternate screen first: %q", snap)
	}
	if !strings.HasSuffix(snap, "\x1b[?25l\x1b[?2004h") {
		t.Fatalf("the snapshot does not end with the modes: %q", snap)
	}
	em := redraw(data, cols, rows)
	defer em.Close()
	if !em.IsAltScreen() {
		t.Fatal("redrawn out of the alternate screen")
	}
	sameScreen(t, s, em)
	s.Write([]byte("\x1b[?1049l\x1b[?25h\x1b[?2004l"))
	if got := string(s.Modes()); got != off {
		t.Fatalf("modes after turning them off %q, want %q", got, off)
	}
}
