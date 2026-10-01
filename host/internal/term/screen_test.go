package term

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
)

// tui is output in the style of a TUI: colours, absolute moves, a partial
// repaint and Unicode.
var tui = []byte("\x1b[2J\x1b[H\x1b[1;38;2;200;100;0mClaude\x1b[0m banner\r\n" +
	"\x1b[5;1H❯ Responde solo: ÑANDÚ42\r\n" +
	"\x1b[7;3H\x1b[32m● ÑANDÚ42\x1b[0m\r\n" +
	"\x1b[10;1H────────────\x1b[11;1H❯ \x1b[11;3Hdraft\x1b[2D")

// redraw writes data to a clean emulator of the size and returns its text.
func redraw(data []byte, cols, rows int) string {
	em := vt.NewEmulator(cols, rows)
	go io.Copy(io.Discard, em)
	em.Write(data)
	return em.String()
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
	if got := redraw(data, cols, rows); got != s.Text() {
		t.Fatalf("redrawn:\n%s\nwant:\n%s", got, s.Text())
	}
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
	if got := redraw(data, cols, rows); got != s.Text() {
		t.Fatalf("redrawn:\n%s\nwant:\n%s", got, s.Text())
	}
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
	// Older than the tail: a snapshot is due.
	s.Write(bytes.Repeat([]byte("x"), TailSize))
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
	em := vt.NewEmulator(cols, rows)
	go io.Copy(io.Discard, em)
	defer em.Close()
	em.Write(data)
	if !em.IsAltScreen() || em.String() != s.Text() {
		t.Fatalf("redrawn (alt %v):\n%s\nwant:\n%s", em.IsAltScreen(), em.String(), s.Text())
	}
	s.Write([]byte("\x1b[?1049l\x1b[?25h\x1b[?2004l"))
	if got := string(s.Modes()); got != off {
		t.Fatalf("modes after turning them off %q, want %q", got, off)
	}
}
