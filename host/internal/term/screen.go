// Package term keeps the agent's terminal as the host sees it (spec M3 §4.2):
// an emulator with the current screen, the tail of the raw output, and the
// stream position of protocol spec §7 (epoch and offset).
package term

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/x/vt"
)

// TailSize is how much raw output the host keeps to resume a terminal without
// a snapshot (protocol spec §7: at least 256 KiB).
const TailSize = 256 << 10

var (
	pasteOn  = []byte("\x1b[?2004h")
	pasteOff = []byte("\x1b[?2004l")
)

// Screen is one PTY process's terminal. It is safe for concurrent use.
type Screen struct {
	mu     sync.Mutex
	em     *vt.Emulator
	epoch  string
	offset int64  // bytes written so far: the position of the next one
	tail   []byte // the last TailSize bytes, ending at offset
	paste  bool   // the program turned bracketed paste on
	hidden bool   // the program hid the cursor
	carry  []byte // the end of the last write, for a mode sequence split in two
	cols   int
	rows   int
}

// New returns the screen of a new PTY process (a new epoch).
func New(epoch string, cols, rows int) *Screen {
	em := vt.NewEmulator(cols, rows)
	// The emulator answers the program's queries (cursor position…) on its
	// read side: the host drops them, the real terminals answer (spec M3
	// §4.2). Unread, they would block the emulator.
	go io.Copy(io.Discard, em)
	s := &Screen{em: em, epoch: epoch, cols: cols, rows: rows}
	// Called from em.Write, with mu held.
	em.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { s.hidden = !visible }})
	return s
}

// Close releases the emulator.
func (s *Screen) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.em.Close()
}

// Write feeds output of the PTY and returns the offset of its first byte.
func (s *Screen) Write(p []byte) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := s.offset
	s.em.Write(p)
	s.offset += int64(len(p))
	s.tail = append(s.tail, p...)
	if over := len(s.tail) - TailSize; over > 0 {
		s.tail = append(s.tail[:0:0], s.tail[over:]...)
	}
	s.scanPaste(p)
	return at
}

// scanPaste follows the bracketed paste mode, also when its sequence comes
// split between two writes.
func (s *Screen) scanPaste(p []byte) {
	buf := append(s.carry, p...)
	on, off := bytes.LastIndex(buf, pasteOn), bytes.LastIndex(buf, pasteOff)
	if on >= 0 || off >= 0 {
		s.paste = on > off
	}
	keep := len(pasteOn) - 1
	if len(buf) < keep {
		keep = len(buf)
	}
	s.carry = append(s.carry[:0], buf[len(buf)-keep:]...)
}

// Resize changes the emulator's size, as the PTY's.
func (s *Screen) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.em.Resize(cols, rows)
	s.cols, s.rows = cols, rows
}

// Epoch names the PTY process; Offset is the position of the next byte.
func (s *Screen) Epoch() string { return s.epoch }

func (s *Screen) Offset() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offset
}

// Size is the terminal's size.
func (s *Screen) Size() (cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cols, s.rows
}

// BracketedPaste reports whether the program turned bracketed paste on.
func (s *Screen) BracketedPaste() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paste
}

// Since returns the output from offset on when epoch is this one and those
// bytes are still in the tail; otherwise ok is false and a snapshot is due.
func (s *Screen) Since(epoch string, offset int64) (data []byte, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := s.offset - int64(len(s.tail))
	if epoch != s.epoch || offset < start || offset > s.offset {
		return nil, false
	}
	return append([]byte(nil), s.tail[offset-start:]...), true
}

// Modes are the terminal modes the program set, each one on or off, as the
// sequences that set them in a terminal: the alternate screen, the cursor's
// visibility and bracketed paste.
func (s *Screen) Modes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []byte(s.altMode() + s.inputModes())
}

func (s *Screen) altMode() string {
	if s.em.IsAltScreen() {
		return "\x1b[?1049h"
	}
	return "\x1b[?1049l"
}

func (s *Screen) inputModes() string {
	m := "\x1b[?25h"
	if s.hidden {
		m = "\x1b[?25l"
	}
	if s.paste {
		return m + "\x1b[?2004h"
	}
	return m + "\x1b[?2004l"
}

// Snapshot is the current screen as bytes that, written to a terminal of its
// size, draw it and set its modes (Modes), at the offset it corresponds to
// (spike S4: Render separates lines with a bare \n, which a terminal does not
// take as a new line at column 0). The alternate screen comes first: it
// changes the screen that is drawn on.
func (s *Screen) Snapshot() (data []byte, offset int64, cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := strings.ReplaceAll(s.em.Render(), "\n", "\r\n")
	pos := s.em.CursorPosition()
	snap := s.altMode() + "\x1b[0m\x1b[2J\x1b[H" + r + fmt.Sprintf("\x1b[0m\x1b[%d;%dH", pos.Y+1, pos.X+1) + s.inputModes()
	return []byte(snap), s.offset, s.cols, s.rows
}

// Text is the screen's plain text, for detecting what the agent shows (the
// trust dialog, spec M3 §3.3).
func (s *Screen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.em.String()
}
