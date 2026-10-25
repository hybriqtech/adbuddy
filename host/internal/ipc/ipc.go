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
	"slices"
	"strings"
	"time"
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
	KindUI   = "ui" // an agent's helper: adbuddy screen, tap, type… (M5)
	// KindRemote turns the remote mode on or off, or reads it (adbuddy remote):
	// Argv is "on", "off" or "status". Never from the agent's session.
	KindRemote = "remote"
	// KindHook carries a Claude Code hook (adbuddy hook <event>, M4): Argv is
	// the event, stdin its payload, stdout the decision, if any.
	KindHook = "hook"
	// KindYolo turns the YOLO mode off or reads it (adbuddy yolo): Argv is
	// "off" or "status". There is no "on": only the phone app turns it on.
	// Anyone may ask, the agent included: off is always safe.
	KindYolo = "yolo"
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
	// Screen, in a KindUI request: for click and setText, also read the screen
	// once it settles (the result then carries it); for dump, wait for it to
	// settle before answering. Ignored by an older daemon.
	Screen bool `json:"screen,omitempty"`
}

// Exit is the last frame: the exit code for the client and, for failures
// of ADBuddy itself, an error code (UPPER_SNAKE_CASE) and a message.
type Exit struct {
	Code      int    `json:"code"`
	ErrorCode string `json:"errorCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

// YoloStatus is the data answered to KindYolo.
type YoloStatus struct {
	Active  bool  `json:"active"`
	Until   int64 `json:"until"` // epoch milliseconds; 0 when off or until the session ends
	Session bool  `json:"session"`
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
	// Paused: the person paused the agent from the phone (V20); false from
	// a host that predates it.
	Paused bool `json:"paused,omitempty"`
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

// secretLockPath is held while the secret is created: one creator at a time.
func secretLockPath(dataDir string) string { return filepath.Join(dataDir, "ipc.secret.lock") }

// secretWait bounds the wait for another process that is creating the secret.
const secretWait = 5 * time.Second

// secretWritten, when not nil (tests), is called with the temporary file once
// the whole secret is in it, before it takes the secret's name.
var secretWritten func(tmp string)

// EnsureSecret returns the installation secret, creating it (256 random
// bits, readable only by the user) on first use. It is written whole under a
// temporary name in the same folder (same permissions and, on Windows, the
// folder's ACL) and then renamed, so a reader sees no secret or all of it;
// creators take turns (secretLockPath) and a later one reads the first one's.
func EnsureSecret(dataDir string) (string, error) {
	if s, err := ReadSecret(dataDir); err == nil {
		return s, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	lock, err := waitLock(secretLockPath(dataDir), secretWait)
	if errors.Is(err, errLocked) {
		return "", fmt.Errorf("another ADBuddy is creating %s: %w", SecretPath(dataDir), err)
	}
	if err != nil {
		return "", fmt.Errorf("lock %s: %w", secretLockPath(dataDir), err)
	}
	defer lock.Close()
	if s, err := ReadSecret(dataDir); err == nil {
		return s, nil // another process created it meanwhile
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(b[:])
	f, err := os.CreateTemp(dataDir, "ipc.secret.*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // nothing left once renamed
	if _, err := f.WriteString(secret); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if secretWritten != nil {
		secretWritten(tmp)
	}
	if err := os.Rename(tmp, SecretPath(dataDir)); err != nil {
		return "", err
	}
	return secret, nil
}

// A secret that cannot be read whole is read again for a moment: on Windows
// a rename in progress refuses to open it, and a host of an older version
// wrote it in two steps (create, then write). Variables so a test can
// lengthen the wait (a slow CI with -race).
var (
	secretTries = 20
	secretPause = 10 * time.Millisecond
)

// ReadSecret reads the installation secret. No secret at all is answered at
// once (fs.ErrNotExist); any other failure is retried for secretTries.
func ReadSecret(dataDir string) (string, error) {
	var err error
	for i := range secretTries {
		if i > 0 {
			time.Sleep(secretPause)
		}
		var s string
		if s, err = readSecretOnce(dataDir); err == nil || errors.Is(err, fs.ErrNotExist) {
			return s, err
		}
	}
	return "", err
}

func readSecretOnce(dataDir string) (string, error) {
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

// frameChunk is the most ReadFrame reserves before the bytes arrive. Past it
// the buffer doubles only with what has been read, so a header that announces
// MaxFrame (from a client that has not authenticated yet) costs nothing until
// the payload really comes.
const frameChunk = 64 << 10

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	size := binary.BigEndian.Uint32(hdr[1:])
	if size > MaxFrame {
		return 0, nil, fmt.Errorf("frame of %d bytes exceeds %d", size, MaxFrame)
	}
	n := int(size)
	payload := make([]byte, min(n, frameChunk))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	for len(payload) < n {
		start := len(payload)
		more := min(n-start, start)
		payload = slices.Grow(payload, more)[:start+more]
		if _, err := io.ReadFull(r, payload[start:]); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF // part of the frame came
			}
			return 0, nil, err
		}
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
