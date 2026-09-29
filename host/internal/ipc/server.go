package ipc

import (
	"context"
	"crypto/subtle"
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

// Conn is the server side of one request.
type Conn struct {
	stdin *io.PipeReader
	mu    sync.Mutex
	nc    net.Conn
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
// disconnects or the server closes.
type Handler func(ctx context.Context, req Request, c *Conn) Exit

// Server accepts requests on the daemon socket.
type Server struct {
	ln      net.Listener
	secret  string
	handler Handler
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// Listen creates the socket in dataDir. A leftover socket from a dead daemon
// is removed; a live one yields ErrAlreadyRunning.
func Listen(dataDir, secret string, h Handler) (*Server, error) {
	path := SocketPath(dataDir)
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("socket path %s is longer than %d characters; set ADBUDDY_DATA_DIR to a shorter folder", path, maxSocketPath)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, 500*time.Millisecond); err == nil {
			c.Close()
			return nil, ErrAlreadyRunning
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			ln.Close()
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{ln: ln, secret: secret, handler: h, ctx: ctx, cancel: cancel}, nil
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
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(nc)
		}()
	}
}

// Close stops accepting, cancels running requests and waits for them.
func (s *Server) Close() error {
	s.cancel()
	err := s.ln.Close()
	s.wg.Wait()
	return err
}

func (s *Server) handle(nc net.Conn) {
	defer nc.Close()
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	typ, payload, err := ReadFrame(nc)
	if err != nil || typ != FrameRequest {
		return
	}
	var req Request
	if json.Unmarshal(payload, &req) != nil {
		return
	}
	c := &Conn{nc: nc}
	if subtle.ConstantTimeCompare([]byte(req.Secret), []byte(s.secret)) != 1 {
		writeExit(c, Exit{Code: 1, Error: "unauthorized"})
		finish(nc, nil)
		return
	}
	_ = nc.SetReadDeadline(time.Time{})
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	pr, pw := io.Pipe()
	c.stdin = pr
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer cancel() // the client went away
		for {
			typ, payload, err := ReadFrame(nc)
			if err != nil {
				pw.CloseWithError(io.ErrUnexpectedEOF)
				return
			}
			switch typ {
			case FrameStdin:
				_, _ = pw.Write(payload) // fails once the handler stopped reading
			case FrameStdinEOF:
				pw.Close()
			}
		}
	}()
	exit := s.handler(ctx, req, c)
	pr.CloseWithError(errors.New("request finished"))
	writeExit(c, exit)
	finish(nc, readerDone)
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
