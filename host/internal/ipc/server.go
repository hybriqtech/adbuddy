package ipc

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
)

// maxStdin bounds the stdin a request may send.
const maxStdin = 16 << 20

var errStdinTooLarge = fmt.Errorf("stdin exceeds %d bytes", maxStdin)

// stdinBuffer holds the stdin the client sends. The connection reader never
// blocks on it, so it always sees the client go away even when the handler
// is not reading stdin (for example while it waits for a confirmation).
type stdinBuffer struct {
	mu   sync.Mutex
	cond *sync.Cond
	buf  bytes.Buffer
	eof  bool
	err  error
}

func newStdinBuffer() *stdinBuffer {
	b := &stdinBuffer{}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// write appends p. It returns false when the request exceeds maxStdin; the
// handler then gets an error on its next Read. Data after close is dropped.
func (b *stdinBuffer) write(p []byte) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.eof || b.err != nil {
		return true
	}
	if b.buf.Len()+len(p) > maxStdin {
		b.err = errStdinTooLarge
		b.cond.Broadcast()
		return false
	}
	b.buf.Write(p)
	b.cond.Broadcast()
	return true
}

// close ends the stream: nil means a clean EOF; otherwise the first error
// wins.
func (b *stdinBuffer) close(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		b.eof = true
	} else if b.err == nil {
		b.err = err
	}
	b.cond.Broadcast()
}

// Read waits for data, EOF or an error. Buffered data comes first, then the
// error, then io.EOF.
func (b *stdinBuffer) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for b.buf.Len() == 0 && !b.eof && b.err == nil {
		b.cond.Wait()
	}
	if b.buf.Len() > 0 {
		return b.buf.Read(p)
	}
	if b.err != nil {
		return 0, b.err
	}
	return 0, io.EOF
}

// Conn is the server side of one request.
type Conn struct {
	stdin  *stdinBuffer
	resize chan [2]int
	mu     sync.Mutex
	nc     net.Conn
}

// Resizes delivers the sizes a terminal sends with FrameResize (KindAttach).
// When they come faster than they are taken, only the newest count.
func (c *Conn) Resizes() <-chan [2]int { return c.resize }

func (c *Conn) pushResize(cols, rows int) {
	for {
		select {
		case c.resize <- [2]int{cols, rows}:
			return
		default:
			select { // full: drop the oldest
			case <-c.resize:
			default:
			}
		}
	}
}

// Stdin streams what the client sends as stdin.
func (c *Conn) Stdin() io.Reader { return c.stdin }

// Stdout sends bytes to the client's stdout.
func (c *Conn) Stdout() io.Writer { return frameWriter{c, FrameStdout} }

// Stderr sends bytes to the client's stderr.
func (c *Conn) Stderr() io.Writer { return frameWriter{c, FrameStderr} }

// Data sends a JSON value (results of undo and ping).
func (c *Conn) Data(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.write(FrameData, b)
}

func (c *Conn) write(typ byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return WriteFrame(c.nc, typ, payload)
}

type frameWriter struct {
	c   *Conn
	typ byte
}

func (w frameWriter) Write(p []byte) (int, error) {
	for off := 0; off < len(p); {
		n := len(p) - off
		if n > 64*1024 {
			n = 64 * 1024
		}
		if err := w.c.write(w.typ, p[off:off+n]); err != nil {
			return off, err
		}
		off += n
	}
	return len(p), nil
}

// Handler serves one authenticated request. ctx is cancelled when the client
// disconnects or the server closes; in the second case context.Cause(ctx) is
// ErrClosed.
type Handler func(ctx context.Context, req Request, c *Conn) Exit

// maxPreauth bounds the connections that have not authenticated yet: each
// may keep the server reading a request (up to MaxFrame) for 5 seconds. One
// over it is closed at once.
const maxPreauth = 32

// Server accepts requests on the daemon socket.
type Server struct {
	ln      net.Listener
	lock    *os.File      // held until Close (lockPath)
	preauth chan struct{} // a slot per connection that has not authenticated yet
	secret  string
	handler Handler
	ctx     context.Context
	cancel  context.CancelCauseFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	closed  bool
}

// Listen creates the socket in dataDir. A leftover socket from a dead daemon
// is removed; a live one yields ErrAlreadyRunning. secret must be the
// 64-hex-character installation secret.
func Listen(dataDir, secret string, h Handler) (*Server, error) {
	path := SocketPath(dataDir)
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("socket path %s is longer than %d characters; set ADBUDDY_DATA_DIR to a shorter folder", path, maxSocketPath)
	}
	if _, err := hex.DecodeString(secret); err != nil || len(secret) != 64 {
		return nil, errors.New("the IPC secret must be 64 hexadecimal characters")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	// One daemon per data directory, for as long as it serves: two serve that
	// start at once could otherwise both find a dead socket and the second
	// remove the first one's live socket (Stat, Dial and Remove are not one
	// step).
	lock, err := tryLock(lockPath(dataDir))
	if errors.Is(err, errLocked) {
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Server, error) {
		lock.Close()
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		// A daemon of an older version holds no lock, but it answers.
		if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
			c.Close()
			return fail(ErrAlreadyRunning)
		}
		if err := os.Remove(path); err != nil {
			return fail(err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fail(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			ln.Close()
			return fail(err)
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &Server{ln: ln, lock: lock, preauth: make(chan struct{}, maxPreauth), secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
}

// Serve accepts connections until Close.
func (s *Server) Serve() error {
	for {
		nc, err := s.ln.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case s.preauth <- struct{}{}: // handle gives it back once authenticated or gone
		default:
			nc.Close() // too many connections waiting to authenticate
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			<-s.preauth
			nc.Close()
			return nil
		}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			s.handle(nc)
		}()
	}
}

// Close stops accepting, cancels running requests and waits for them.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel(ErrClosed)
	err := s.ln.Close()
	s.wg.Wait()
	// Released after the socket is gone: the next daemon finds none alive.
	_ = s.lock.Close()
	return err
}

func (s *Server) handle(nc net.Conn) {
	defer nc.Close()
	release := sync.OnceFunc(func() { <-s.preauth }) // the slot Serve took for it
	defer release()
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	typ, payload, err := ReadFrame(nc)
	if err != nil || typ != FrameRequest {
		return
	}
	var req Request
	if json.Unmarshal(payload, &req) != nil {
		return
	}
	c := &Conn{nc: nc, resize: make(chan [2]int, 4)}
	if subtle.ConstantTimeCompare([]byte(req.Secret), []byte(s.secret)) != 1 {
		// Refused: it keeps its slot (the deferred release) while finish
		// waits up to 2 s for the client to close, so refusals stay bounded.
		writeExit(c, Exit{Code: 1, Error: "unauthorized"})
		finish(nc, nil)
		return
	}
	release() // authenticated: it no longer waits
	_ = nc.SetReadDeadline(time.Time{})
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	// Once the request is cancelled, writes to a client that stopped reading
	// must not block Close forever.
	context.AfterFunc(ctx, func() { _ = nc.SetWriteDeadline(time.Now().Add(2 * time.Second)) })
	stdin := newStdinBuffer()
	c.stdin = stdin
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			typ, payload, err := ReadFrame(nc)
			if err != nil {
				// The client went away: cancel first, so a child process
				// never sees a clean end of stdin before the cancellation.
				cancel()
				stdin.close(io.ErrUnexpectedEOF)
				return
			}
			switch typ {
			case FrameStdin:
				if !stdin.write(payload) {
					cancel() // request too large
				}
			case FrameStdinEOF:
				stdin.close(nil)
			case FrameResize:
				if cols, rows, ok := DecodeResize(payload); ok {
					c.pushResize(cols, rows)
				}
			}
		}
	}()
	exit := s.run(ctx, req, c)
	stdin.close(errors.New("request finished"))
	writeExit(c, exit)
	finish(nc, readerDone)
}

// run calls the handler and turns a panic into an INTERNAL error, so one
// failing request does not bring the daemon down.
func (s *Server) run(ctx context.Context, req Request, c *Conn) (exit Exit) {
	defer func() {
		if r := recover(); r != nil {
			exit = Exit{Code: 1, ErrorCode: "INTERNAL", Error: fmt.Sprint(r)}
		}
	}()
	return s.handler(ctx, req, c)
}

// finish half-closes the connection and waits for the client to close its
// side. Closing with unread input makes Windows reset the connection, and the
// client could lose the Exit frame.
func finish(nc net.Conn, readerDone <-chan struct{}) {
	if cw, ok := nc.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
	if readerDone == nil {
		_ = nc.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _ = io.Copy(io.Discard, nc)
		return
	}
	select {
	case <-readerDone:
	case <-time.After(5 * time.Second):
	}
}

func writeExit(c *Conn, e Exit) {
	b, _ := json.Marshal(e)
	_ = c.write(FrameExit, b)
}
