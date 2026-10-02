package ipc

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// shortDir returns a temporary directory with a short path: t.TempDir() on
// macOS can exceed the socket path limit.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "abx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func startServer(t *testing.T, dir string, h Handler) *Server {
	t.Helper()
	secret, err := EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Listen(dir, secret, h)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(func() { s.Close() })
	return s
}

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, FrameStdout, []byte("hola")); err != nil {
		t.Fatal(err)
	}
	typ, p, err := ReadFrame(&buf)
	if err != nil || typ != FrameStdout || string(p) != "hola" {
		t.Fatalf("%c %q %v", typ, p, err)
	}
	if err := WriteFrame(&buf, FrameStdout, make([]byte, MaxFrame+1)); err == nil {
		t.Fatal("oversized frame accepted")
	}
}

func TestSecretFile(t *testing.T) {
	dir := shortDir(t)
	a, err := EnsureSecret(dir)
	if err != nil || len(a) != 64 {
		t.Fatalf("%q %v", a, err)
	}
	b, _ := EnsureSecret(dir)
	if a != b {
		t.Fatal("secret changed")
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(SecretPath(dir))
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}
}

func TestExecStreamsAndStdin(t *testing.T) {
	dir := shortDir(t)
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		in, _ := io.ReadAll(c.Stdin())
		io.WriteString(c.Stdout(), strings.Join(req.Argv, " ")+"|"+string(in))
		io.WriteString(c.Stderr(), "warn")
		return Exit{Code: 7}
	})
	var out, errb bytes.Buffer
	exit, err := Call(context.Background(), dir, Request{Kind: KindExec, Argv: []string{"shell", "ls"}}, strings.NewReader("input"), &out, &errb, nil)
	if err != nil {
		t.Fatal(err)
	}
	if exit.Code != 7 || out.String() != "shell ls|input" || errb.String() != "warn" {
		t.Fatalf("exit %+v out %q err %q", exit, out.String(), errb.String())
	}
}

func TestBinaryOutputLargerThanAFrame(t *testing.T) {
	dir := shortDir(t)
	data := make([]byte, 3*MaxFrame+17)
	rand.Read(data)
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		c.Stdout().Write(data)
		return Exit{}
	})
	var out bytes.Buffer
	if _, err := Call(context.Background(), dir, Request{Kind: KindExec}, nil, &out, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("got %d bytes, want %d", out.Len(), len(data))
	}
}

func TestWrongSecretIsRejected(t *testing.T) {
	dir := shortDir(t)
	var called atomic.Bool
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		called.Store(true)
		return Exit{}
	})
	if err := os.WriteFile(SecretPath(dir), []byte(strings.Repeat("0", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	exit, err := Call(context.Background(), dir, Request{Kind: KindPing}, nil, io.Discard, io.Discard, nil)
	if err != nil || exit.Code != 1 || exit.Error != "unauthorized" || called.Load() {
		t.Fatalf("exit %+v err %v called %v", exit, err, called.Load())
	}
}

func TestNotRunning(t *testing.T) {
	dir := shortDir(t)
	if _, err := Call(context.Background(), dir, Request{Kind: KindPing}, nil, io.Discard, io.Discard, nil); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("no secret: %v", err)
	}
	if _, err := EnsureSecret(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Call(context.Background(), dir, Request{Kind: KindPing}, nil, io.Discard, io.Discard, nil); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("no socket: %v", err)
	}
}

func TestSecondServerRefusedAndStaleSocketReplaced(t *testing.T) {
	dir := shortDir(t)
	s := startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	secret, err := ReadSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(dir, secret, nil); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second server: %v", err)
	}
	s.Close()
	// Leave a stale socket file behind, as a crashed daemon would.
	if err := os.WriteFile(SocketPath(dir), nil, 0o600); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	s2, err := Listen(dir, secret, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	if err != nil {
		t.Fatalf("stale socket: %v", err)
	}
	s2.Close()
}

func TestClientDisconnectCancelsHandler(t *testing.T) {
	dir := shortDir(t)
	cancelled := make(chan struct{})
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		select {
		case <-ctx.Done():
			close(cancelled)
		case <-time.After(5 * time.Second):
		}
		return Exit{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	if _, err := Call(ctx, dir, Request{Kind: KindExec}, nil, io.Discard, io.Discard, nil); err == nil {
		t.Fatal("call should fail when cancelled")
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("handler context was not cancelled")
	}
}

func TestPingDaemon(t *testing.T) {
	dir := shortDir(t)
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		if req.Kind != KindPing {
			return Exit{Code: 1}
		}
		c.Data(Ping{Version: "v", Session: "s_1", Serial: "abc"})
		return Exit{}
	})
	p, err := PingDaemon(context.Background(), dir)
	if err != nil || p.Session != "s_1" || p.Serial != "abc" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestSocketPathTooLong(t *testing.T) {
	if _, err := Listen(strings.Repeat("d", 120), "x", nil); err == nil || !strings.Contains(err.Error(), "ADBUDDY_DATA_DIR") {
		t.Fatalf("%v", err)
	}
}

func TestListenRejectsInvalidSecrets(t *testing.T) {
	for _, secret := range []string{"", "x", strings.Repeat("g", 64)} {
		dir := shortDir(t)
		s, err := Listen(dir, secret, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
		if err == nil {
			s.Close()
			t.Errorf("secret %q accepted", secret)
		}
	}
}

func TestDisconnectCancelsHandlerThatIgnoresStdin(t *testing.T) {
	dir := shortDir(t)
	cancelled := make(chan struct{})
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		select {
		case <-ctx.Done():
			close(cancelled)
		case <-time.After(5 * time.Second):
		}
		return Exit{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	if _, err := Call(ctx, dir, Request{Kind: KindExec}, strings.NewReader("some input"), io.Discard, io.Discard, nil); err == nil {
		t.Fatal("call should fail when cancelled")
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("handler context was not cancelled")
	}
}

func TestStdinTooLargeCancels(t *testing.T) {
	dir := shortDir(t)
	cancelled := make(chan struct{})
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		select {
		case <-ctx.Done():
			close(cancelled)
		case <-time.After(5 * time.Second):
		}
		return Exit{}
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		Call(context.Background(), dir, Request{Kind: KindExec}, bytes.NewReader(make([]byte, 17<<20)), io.Discard, io.Discard, nil)
	}()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Error("handler context was not cancelled")
	}
	<-done
}

func TestHandlerPanicIsReported(t *testing.T) {
	dir := shortDir(t)
	var calls atomic.Int32
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		if calls.Add(1) == 1 {
			panic("boom")
		}
		return Exit{Code: 3}
	})
	exit, err := Call(context.Background(), dir, Request{Kind: KindExec}, nil, io.Discard, io.Discard, nil)
	if err != nil || exit.Code != 1 || exit.ErrorCode != "INTERNAL" || !strings.Contains(exit.Error, "boom") {
		t.Fatalf("exit %+v err %v", exit, err)
	}
	exit, err = Call(context.Background(), dir, Request{Kind: KindExec}, nil, io.Discard, io.Discard, nil)
	if err != nil || exit.Code != 3 {
		t.Fatalf("second request: exit %+v err %v", exit, err)
	}
}

// T5 re-review, M2: a handler can tell the host stopping (serve's Ctrl+C)
// from the client going away.
func TestCloseCancelsHandlersWithErrClosed(t *testing.T) {
	dir := shortDir(t)
	causes := make(chan error, 1)
	started := make(chan struct{})
	s := startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		close(started)
		<-ctx.Done()
		causes <- context.Cause(ctx)
		return Exit{}
	})
	go Call(context.Background(), dir, Request{Kind: KindExec}, nil, io.Discard, io.Discard, nil)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("the handler never ran")
	}
	s.Close()
	if err := <-causes; !errors.Is(err, ErrClosed) {
		t.Fatalf("cause %v, want ErrClosed", err)
	}
}

func TestClientDisconnectIsNotErrClosed(t *testing.T) {
	dir := shortDir(t)
	causes := make(chan error, 1)
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		<-ctx.Done()
		causes <- context.Cause(ctx)
		return Exit{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	Call(ctx, dir, Request{Kind: KindExec}, nil, io.Discard, io.Discard, nil)
	select {
	case err := <-causes:
		if errors.Is(err, ErrClosed) {
			t.Fatal("a client that went away looks like the host stopping")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("handler context was not cancelled")
	}
}

// A resize payload keeps the size; out of 1..1000 or of the wrong length it
// is refused.
func TestResizePayload(t *testing.T) {
	for _, c := range []struct {
		cols, rows int
		ok         bool
	}{{80, 24, true}, {1, 1, true}, {1000, 1000, true}, {0, 24, false}, {80, 0, false}, {1001, 24, false}, {80, 1001, false}} {
		cols, rows, ok := DecodeResize(EncodeResize(c.cols, c.rows))
		if ok != c.ok || (ok && (cols != c.cols || rows != c.rows)) {
			t.Errorf("%dx%d: got %dx%d %v", c.cols, c.rows, cols, rows, ok)
		}
	}
	for _, b := range [][]byte{nil, {0, 80, 0}, {0, 80, 0, 24, 0}} {
		if _, _, ok := DecodeResize(b); ok {
			t.Errorf("%v accepted", b)
		}
	}
}

// The sizes an attach sends reach the handler's Resizes; refused ones do not.
func TestAttachResizesReachTheHandler(t *testing.T) {
	dir := shortDir(t)
	startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit {
		for i := 0; i < 2; i++ {
			select {
			case size := <-c.Resizes():
				fmt.Fprintf(c.Stdout(), "%dx%d;", size[0], size[1])
			case <-time.After(5 * time.Second):
				return Exit{Code: 1}
			}
		}
		return Exit{}
	})
	resizes := make(chan [2]int, 4)
	resizes <- [2]int{0, 24}
	resizes <- [2]int{1001, 24}
	resizes <- [2]int{80, 24}
	resizes <- [2]int{100, 30}
	var out bytes.Buffer
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	exit, err := Attach(context.Background(), dir, Request{Kind: KindAttach}, stdinR, &out, resizes)
	if err != nil || exit.Code != 0 || out.String() != "80x24;100x30;" {
		t.Fatalf("exit %+v, %v, got %q", exit, err, out.String())
	}
}

// V29: the daemon holds a lock for as long as it serves, so a second Listen
// in the same directory fails even when the first one's socket is gone: two
// serve that start at once could otherwise both find a dead socket, and the
// second remove the first one's live socket (Stat, Dial and Remove are not
// one step).
func TestSecondServerRefusedEvenWithoutTheSocket(t *testing.T) {
	dir := shortDir(t)
	s := startServer(t, dir, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	secret, err := ReadSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(SocketPath(dir)); err != nil {
		t.Fatal(err)
	}
	s2, err := Listen(dir, secret, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	if !errors.Is(err, ErrAlreadyRunning) {
		if s2 != nil {
			s2.Close()
		}
		t.Fatalf("second server: %v", err)
	}
	s.Close()
	// The lock goes with the first server.
	s3, err := Listen(dir, secret, func(ctx context.Context, req Request, c *Conn) Exit { return Exit{} })
	if err != nil {
		t.Fatalf("after close: %v", err)
	}
	s3.Close()
}

// V29: many creators at once agree on one secret, and a reader never sees a
// corrupt one meanwhile.
func TestSecretCreatedOnceUnderARace(t *testing.T) {
	dir := shortDir(t)
	setTries(t, 500) // a slow CI must not turn a rename in progress into an error
	const n = 16
	got := make(chan string, n)
	stop := make(chan struct{})
	stopped := false
	defer func() { // a Fatalf must not leave the reader running
		if !stopped {
			close(stop)
		}
	}()
	readerErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				readerErr <- nil
				return
			default:
			}
			if _, err := ReadSecret(dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
				readerErr <- err
				return
			}
		}
	}()
	for range n {
		go func() {
			s, err := EnsureSecret(dir)
			if err != nil {
				t.Error(err)
			}
			got <- s
		}()
	}
	// Collect every creator and the reader before judging, so that a failure
	// leaves no goroutine running after the test (t.Error, secretTries).
	results := make([]string, 0, n)
	for range n {
		results = append(results, <-got)
	}
	close(stop)
	stopped = true
	rerr := <-readerErr
	for _, s := range results[1:] {
		if s != results[0] {
			t.Fatalf("two secrets: %q and %q", results[0], s)
		}
	}
	if rerr != nil {
		t.Fatalf("a reader saw %v", rerr)
	}
}

// A lock that cannot be opened is reported as that, not as another ADBuddy
// creating the secret.
func TestEnsureSecretLockFailureIsNotBlamedOnAnotherHost(t *testing.T) {
	dir := shortDir(t)
	if err := os.Mkdir(secretLockPath(dir), 0o700); err != nil { // a folder cannot be opened as the lock
		t.Fatal(err)
	}
	_, err := EnsureSecret(dir)
	if err == nil || strings.Contains(err.Error(), "another ADBuddy") {
		t.Fatalf("got %v", err)
	}
}

// setTries lengthens (or shortens) ReadSecret's retries for one test.
func setTries(t *testing.T, n int) {
	t.Helper()
	old := secretTries
	secretTries = n
	t.Cleanup(func() { secretTries = old })
}

// V29: a secret that is not whole yet (an older host writes it in two steps)
// is read again for a moment instead of failing as corrupt.
func TestReadSecretWaitsForAPartialOne(t *testing.T) {
	dir := shortDir(t)
	setTries(t, 500) // up to 5 s for a write that comes after 50 ms, on any CI
	full := strings.Repeat("ab", 32)
	if err := os.WriteFile(SecretPath(dir), []byte(full[:10]), 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		done <- os.WriteFile(SecretPath(dir), []byte(full), 0o600)
	}()
	s, err := ReadSecret(dir)
	if werr := <-done; werr != nil {
		t.Fatal(werr)
	}
	if err != nil || s != full {
		t.Fatalf("%q %v", s, err)
	}
	// A corrupt one stays corrupt, after the retries.
	secretTries = 3
	if err := os.WriteFile(SecretPath(dir), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSecret(dir); err == nil || !strings.Contains(err.Error(), "is corrupt") {
		t.Fatalf("corrupt: %v", err)
	}
}

// V29: the secret is written whole under a temporary name and then renamed,
// so nobody reads a part of it; and a second creator waits for the first and
// takes its secret.
func TestSecretIsNeverSeenHalfWritten(t *testing.T) {
	dir := shortDir(t)
	second := make(chan string, 1)
	calls := 0
	secretWritten = func(tmp string) {
		calls++
		if calls > 1 {
			t.Error("the secret was created twice")
			return
		}
		if _, err := ReadSecret(dir); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("before the rename a reader got %v, want no secret", err)
		}
		if b, err := os.ReadFile(tmp); err != nil || len(b) != 64 {
			t.Errorf("temporary file %q: %d bytes, %v", tmp, len(b), err)
		}
		go func() {
			s, err := EnsureSecret(dir) // waits for this one to finish
			if err != nil {
				t.Error(err)
			}
			second <- s
		}()
		time.Sleep(50 * time.Millisecond) // the second one is waiting by now
	}
	t.Cleanup(func() { secretWritten = nil })
	first, err := EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("secretWritten called %d times", calls)
	}
	select {
	case s := <-second:
		if s != first {
			t.Fatalf("second creator got %q, want %q", s, first)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the second creator never returned")
	}
	if s, err := ReadSecret(dir); err != nil || s != first {
		t.Fatalf("file %q %v", s, err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "ipc.secret.*.tmp")); len(left) != 0 {
		t.Fatalf("temporary files left: %q", left)
	}
}
