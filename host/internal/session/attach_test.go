package session

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// syncBuf is a bytes.Buffer safe for a writer and a reader at once.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// serveIPC starts an IPC server whose attach requests go to m.
func serveIPC(t *testing.T, m *Manager) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "abs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	secret, err := ipc.EnsureSecret(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(dir, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		return m.ServeAttach(ctx, c, "pc")
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return dir
}

// A PC terminal gets the screen, then the live output; what it types reaches
// the agent; its size rules; when its stdin ends, it detaches.
func TestAttach(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	r.fake(0).outW.Write([]byte("before attach"))
	waitUntil(t, "the first output", func() bool { return strings.Contains(r.m.Screen().Text(), "before attach") })
	dir := serveIPC(t, r.m)

	stdinR, stdinW := io.Pipe()
	out := &syncBuf{}
	resizes := make(chan [2]int, 4)
	resizes <- [2]int{90, 20}
	done := make(chan ipc.Exit, 1)
	go func() {
		e, err := ipc.Attach(context.Background(), dir, ipc.Request{Kind: ipc.KindAttach}, stdinR, out, resizes)
		if err != nil {
			t.Error(err)
		}
		done <- e
	}()
	waitUntil(t, "the screen", func() bool { return strings.Contains(out.String(), "before attach") })
	if !r.m.Info().PCAttached {
		t.Fatal("not pcAttached")
	}
	if c, rows := r.m.Screen().Size(); c != 90 || rows != 20 {
		t.Fatalf("size %dx%d", c, rows)
	}

	r.fake(0).outW.Write([]byte("LIVE"))
	waitUntil(t, "the live output", func() bool { return strings.Contains(out.String(), "LIVE") })
	stdinW.Write([]byte("typed"))
	waitUntil(t, "the typing", func() bool { return strings.Contains(r.fake(0).said(), "typed") })
	resizes <- [2]int{70, 18}
	waitUntil(t, "the resize", func() bool { c, _ := r.m.Screen().Size(); return c == 70 })

	stdinW.Close()
	select {
	case e := <-done:
		if e.Code != 0 {
			t.Fatalf("exit %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("attach did not end")
	}
	waitUntil(t, "the detach", func() bool { return !r.m.Info().PCAttached })
}

// Without an agent, attach says so.
func TestAttachWithoutAgent(t *testing.T) {
	r := newRig(t)
	dir := serveIPC(t, r.m)
	resizes := make(chan [2]int, 1)
	resizes <- [2]int{80, 24}
	e, err := ipc.Attach(context.Background(), dir, ipc.Request{Kind: ipc.KindAttach}, strings.NewReader(""), io.Discard, resizes)
	if err != nil || e.ErrorCode != "NO_SESSION" {
		t.Fatalf("exit %+v, %v", e, err)
	}
}

// A new process of the agent while attached: the terminal gets its screen.
func TestAttachFollowsANewProcess(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	dir := serveIPC(t, r.m)
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()
	out := &syncBuf{}
	resizes := make(chan [2]int, 1)
	resizes <- [2]int{80, 24}
	go ipc.Attach(context.Background(), dir, ipc.Request{Kind: ipc.KindAttach}, stdinR, out, resizes)
	waitUntil(t, "attached", func() bool { return r.m.Info().PCAttached })
	r.m.Start("gemini", false)
	r.fake(1).outW.Write([]byte("SECOND"))
	waitUntil(t, "the second screen", func() bool { return strings.Contains(out.String(), "SECOND") })
}
