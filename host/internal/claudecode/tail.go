package claudecode

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
)

// Limits of the reader.
const (
	// MaxLine is the longest line it reads; a longer one is skipped (the
	// transcript can carry whole attachments).
	MaxLine = 8 << 20
	// readChunk is how much one Poll reads at most.
	readChunk = 4 << 20
)

// Tailer follows a transcript that Claude Code is writing. Poll, called by
// the owner every few hundred milliseconds, returns the events of the lines
// written since the last call. Only complete lines are read: the last one,
// without its newline, waits. The file may not exist yet. Not safe for
// concurrent use.
type Tailer struct {
	path    string
	parser  *Parser
	off     int64 // where the next read starts: after the last newline read
	partial []byte
	skip    bool // inside a line longer than MaxLine, up to its newline
	started bool // the first Poll found the file
	fromEnd bool
	seen    bool // some line has been read
}

// NewTailer follows path. With fromEnd (a resumed or compacted session) what
// is already in the file is not read, only what is added; otherwise it is
// read from the start.
func NewTailer(path string, fromEnd bool) *Tailer {
	return &Tailer{path: path, parser: NewParser(), fromEnd: fromEnd}
}

// Parser gives the state of the reading: version, lines that were not JSON.
func (t *Tailer) Parser() *Parser { return t.parser }

// Started reports whether the file has been found.
func (t *Tailer) Started() bool { return t.started }

// Seen reports whether any line has been read.
func (t *Tailer) Seen() bool { return t.seen }

// Poll reads what was written since the last call. A missing file is not an
// error (it may not exist yet): it returns nothing.
func (t *Tailer) Poll() ([]Event, error) {
	f, err := os.Open(t.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("the transcript is not a regular file")
	}
	size := fi.Size()
	if !t.started {
		t.started = true
		if t.fromEnd {
			t.off = size
		}
	}
	if size < t.off { // truncated or rewritten: start again
		t.off, t.partial, t.skip = 0, nil, false
	}
	if size == t.off {
		return nil, nil
	}
	if _, err := f.Seek(t.off, io.SeekStart); err != nil {
		return nil, err
	}
	n := size - t.off
	if n > readChunk {
		n = readChunk
	}
	buf := make([]byte, n)
	got, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	buf = buf[:got]
	t.off += int64(got)

	var events []Event
	for len(buf) > 0 {
		i := bytes.IndexByte(buf, '\n')
		if i < 0 {
			if !t.skip {
				t.partial = append(t.partial, buf...)
				if len(t.partial) > MaxLine {
					t.partial, t.skip = nil, true
				}
			}
			break
		}
		line := buf[:i]
		buf = buf[i+1:]
		if t.skip {
			t.skip = false
			continue
		}
		if len(t.partial) > 0 {
			line = append(t.partial, line...)
			t.partial = nil
		}
		if len(line) > MaxLine {
			continue
		}
		t.seen = true
		events = append(events, t.parser.Feed(line)...)
	}
	return events, nil
}
