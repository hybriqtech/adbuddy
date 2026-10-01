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

// TailSize is how much raw output the host keeps at least to resume a
// terminal without a snapshot (protocol spec §7: at least 256 KiB).
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
	tail   []byte // the last bytes written, ending at offset: at least TailSize (once written), at most twice that
	paste  bool   // the program turned bracketed paste on
	hidden bool   // the program hid the cursor
	carry  []byte // the end of the last write, for a mode sequence split in two
	cols   int
	rows   int
	closed bool
	// drained is closed when the goroutine that drops the emulator's
	// answers has ended.
	drained chan struct{}
}

// New returns the screen of a new PTY process (a new epoch).
func New(epoch string, cols, rows int) *Screen {
	em := vt.NewEmulator(cols, rows)
	// Nobody reads the scrollback, and its default keeps 10 000 lines (0
	// means that default too).
	em.SetScrollbackSize(1)
	s := &Screen{em: em, epoch: epoch, cols: cols, rows: rows, drained: make(chan struct{})}
	// The emulator answers the program's queries (cursor position…) on its
	// read side: the host drops them, the real terminals answer (spec M3
	// §4.2). Unread, they would block the emulator.
	go func() {
		defer close(s.drained)
		_, _ = io.Copy(io.Discard, em)
	}()
	// Called from em.Write, with mu held.
	em.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { s.hidden = !visible }})
	return s
}

// Close releases the emulator. Emulator.Close sets a flag that its Read
// checks without a lock, so the goroutine that reads it ends first: closing
// the answers' pipe gives it io.EOF.
func (s *Screen) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if c, ok := s.em.InputPipe().(io.Closer); ok {
		_ = c.Close()
		<-s.drained
	}
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
	// Cut back to TailSize only once it doubled, in place: not a copy of
	// the whole tail on each write.
	if len(s.tail) > 2*TailSize {
		n := copy(s.tail, s.tail[len(s.tail)-TailSize:])
		s.tail = s.tail[:n]
	}
	s.scanPaste(p)
	return at
}

// scanPaste follows the bracketed paste mode, also when its sequence comes
// split between two writes. It only sees the mode on its own (ESC[?2004h
// and ESC[?2004l): not inside a combined sequence such as ESC[?25;2004h,
// nor turned off by a reset (RIS, ESC c).
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

// Epoch names the PTY process whose output this is (protocol spec §7).
func (s *Screen) Epoch() string { return s.epoch }

// Offset is how many bytes of output were written so far: the position the
// next byte will have.
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
//
// It does not carry everything a terminal keeps: the active style (the
// snapshot ends with ESC[0m, so text the program writes next without setting
// its style again comes out plain), the scroll region (ESC[r) and, while the
// alternate screen is on, the main screen under it (leaving the alternate
// screen shows an empty one until the program repaints). The modes it does
// carry are those of Modes.
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

// PlainSnapshot is Snapshot without styles: the screen's text (trailing
// spaces trimmed), the cursor and the modes, as bytes with no SGR. It is what
// a screen too heavy to send styled becomes. If the text is still over
// maxData bytes (maxData > 0), its first lines are left out; the cursor moves
// up with them.
func (s *Screen) PlainSnapshot(maxData int) (data []byte, offset int64, cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := strings.Split(s.em.String(), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	pos := s.em.CursorPosition()
	head := s.altMode() + "\x1b[0m\x1b[2J\x1b[H"
	tail := s.inputModes()
	size := len(head) + len(tail) + 32
	for _, l := range lines {
		size += len(l) + 2
	}
	dropped := 0
	for maxData > 0 && size > maxData && dropped < len(lines)-1 {
		size -= len(lines[dropped]) + 2
		dropped++
	}
	body := strings.Join(lines[dropped:], "\r\n")
	y := max(pos.Y-dropped, 0)
	snap := head + body + fmt.Sprintf("\x1b[%d;%dH", y+1, pos.X+1) + tail
	return []byte(snap), s.offset, s.cols, s.rows
}
