// Package ipc connects the shim and the other CLI commands with the daemon
// over a local socket (spec §4.1): a Unix socket on macOS and Linux and
// AF_UNIX on Windows 10 1803+, in the user's data directory, plus a
// per-installation secret every request must present.
//
// Wire format: frames of [type byte][length uint32 big-endian][payload].
// The client sends one Request frame, then stdin frames and one stdin-EOF
// frame. The server answers with stdout/stderr/data frames and ends with
// exactly one Exit frame.
package ipc

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Frame types.
const (
	FrameRequest  byte = 'R'
	FrameStdin    byte = 'I'
	FrameStdinEOF byte = 'Z'
	FrameStdout   byte = '1'
	FrameStderr   byte = '2'
	FrameData     byte = 'D'
	FrameExit     byte = 'X'
	// FrameResize (client → server) is a terminal's new size for an attach:
	// 2 bytes of columns and 2 of rows, big-endian.
	FrameResize byte = 'W'
)

// MaxFrame bounds a frame payload.
const MaxFrame = 1 << 20

// maxSocketPath keeps below the sun_path limit (104 on macOS, 108 elsewhere).
const maxSocketPath = 100

// Request kinds.
const (
	KindExec = "exec" // run an adb command (the shim)
	KindUndo = "undo" // adbuddy undo
	KindPing = "ping" // is the daemon up? which device and session?
	// KindAttach connects a terminal to the agent's session (adbuddy attach):
	// stdin is what is typed, stdout the PTY's output, FrameResize its size.
	KindAttach = "attach"
	// KindSession starts (Argv: "new", agent) or stops (Argv: "stop") the
	// agent's session.
	KindSession = "session"
	// KindSend sends a file of the environment to the phone (adbuddy send):
	// Argv is the path and the caption.
	KindSend = "send"
	KindUI = "ui" // an agent's helper: adbuddy screen, tap, type… (M5)
)

// Request is the first frame of every connection.
type Request struct {
	Secret  string            `json:"secret"`
	Kind    string            `json:"kind"`
	Argv    []string          `json:"argv,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Target  string            `json:"target,omitempty"`  // undo: "last", "session" or an entry id
	Session string            `json:"session,omitempty"` // undo session: empty = the daemon's current one
	// UIOp and UIArgs are the helper of a KindUI request: a ui.request
	// operation (bridge.UIOp*) or "screenshot", and its args. Argv is the
	// command as the agent typed it, for the journal.
	UIOp   string          `json:"uiOp,omitempty"`
	UIArgs json.RawMessage `json:"uiArgs,omitempty"`
}

// Exit is the last frame: the exit code for the client and, for failures
// of ADBuddy itself, an error code (UPPER_SNAKE_CASE) and a message.
type Exit struct {
	Code      int    `json:"code"`
	ErrorCode string `json:"errorCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Ping is the data answered to KindPing.
type Ping struct {
	Version string `json:"version"`
	Session string `json:"session"`
	Serial  string `json:"serial"` // the serial adb lists the phone with this session
	// Identity is the phone's stable name (see daemon.Options.Identity);
	// empty from a host that predates it.
	Identity string `json:"identity,omitempty"`
	Journal  string `json:"journal"`
}

// Phone is the phone the host serves, as the agent names it in
// ANDROID_SERIAL: its identity, or its serial when there is none.
func (p Ping) Phone() string {
	if p.Identity != "" {
		return p.Identity
	}
	return p.Serial
}

// ErrNotRunning means no daemon answers on the socket.
var ErrNotRunning = errors.New("ADBuddy host not running")

// ErrAlreadyRunning means another daemon already owns the socket.
var ErrAlreadyRunning = errors.New("another ADBuddy host is already running")

// ErrClosed is the cause (context.Cause) of a request cancelled because the
// server closed (serve stopping), as opposed to the client going away.
var ErrClosed = errors.New("the ADBuddy host is stopping")

// SocketPath is the daemon socket inside the data directory.
func SocketPath(dataDir string) string { return filepath.Join(dataDir, "adbuddy.sock") }

// SecretPath is the per-installation secret inside the data directory.
func SecretPath(dataDir string) string { return filepath.Join(dataDir, "ipc.secret") }

// EnsureSecret returns the installation secret, creating it (256 random
// bits, readable only by the user) on first use.
func EnsureSecret(dataDir string) (string, error) {
	if s, err := ReadSecret(dataDir); err == nil {
		return s, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(b[:])
	f, err := os.OpenFile(SecretPath(dataDir), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return ReadSecret(dataDir) // another process won the race
	}
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(secret); err != nil {
		f.Close()
		return "", err
	}
	return secret, f.Close()
}

// ReadSecret reads the installation secret.
func ReadSecret(dataDir string) (string, error) {
	b, err := os.ReadFile(SecretPath(dataDir))
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if len(s) != 64 {
		return "", fmt.Errorf("%s is corrupt", SecretPath(dataDir))
	}
	return s, nil
}

// WriteFrame writes one frame.
func WriteFrame(w io.Writer, typ byte, payload []byte) error {
	if len(payload) > MaxFrame {
		return fmt.Errorf("frame of %d bytes exceeds %d", len(payload), MaxFrame)
	}
	var hdr [5]byte
	hdr[0] = typ
	binary.BigEndian.PutUint32(hdr[1:], uint32(len(payload)))
	if _, err := w.Write(append(hdr[:], payload...)); err != nil {
		return err
	}
	return nil
}

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > MaxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes exceeds %d", n, MaxFrame)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return hdr[0], payload, nil
}

// EncodeResize is the payload of FrameResize.
func EncodeResize(cols, rows int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b, uint16(cols))
	binary.BigEndian.PutUint16(b[2:], uint16(rows))
	return b
}

// DecodeResize reads a FrameResize payload. A size out of 1..1000 is refused.
func DecodeResize(b []byte) (cols, rows int, ok bool) {
	if len(b) != 4 {
		return 0, 0, false
	}
	cols, rows = int(binary.BigEndian.Uint16(b)), int(binary.BigEndian.Uint16(b[2:]))
	return cols, rows, cols >= 1 && cols <= 1000 && rows >= 1 && rows <= 1000
}
