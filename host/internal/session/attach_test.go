package session

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"

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
	var ids atomic.Int64
	srv, err := ipc.Listen(dir, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		return m.ServeAttach(ctx, c, fmt.Sprintf("pc-%d", ids.Add(1)))
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

// term is a PC terminal attached by IPC: what it types, what it shows.
type pcTerm struct {
	stdin *io.PipeWriter
	out   *syncBuf
	size  chan [2]int
	done  chan ipc.Exit
}

func attachPC(t *testing.T, dir string, cols, rows int, out io.Writer) *pcTerm {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	p := &pcTerm{stdin: stdinW, out: &syncBuf{}, size: make(chan [2]int, 4), done: make(chan ipc.Exit, 1)}
	if out == nil {
		out = p.out
	}
	p.size <- [2]int{cols, rows}
	go func() {
		e, err := ipc.Attach(context.Background(), dir, ipc.Request{Kind: ipc.KindAttach}, stdinR, out, p.size)
		if err != nil {
			t.Error(err)
		}
		p.done <- e
	}()
	t.Cleanup(func() { stdinW.Close() })
	return p
}

// When the agent ends, the attached terminal says so and stays attached;
// what it types goes nowhere; a new process draws its screen at once.
func TestAttachWhenTheAgentEnds(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	r.fake(0).outW.Write([]byte("FIRST"))
	dir := serveIPC(t, r.m)
	pc := attachPC(t, dir, 80, 24, nil)
	waitUntil(t, "the screen", func() bool { return strings.Contains(pc.out.String(), "FIRST") })

	r.fake(0).exit(0)
	waitUntil(t, "the notice", func() bool { return strings.Contains(pc.out.String(), AgentEnded) })
	if _, err := pc.stdin.Write([]byte("typed into nothing")); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-pc.done:
		t.Fatalf("attach ended with the agent: %+v", e)
	case <-time.After(100 * time.Millisecond):
	}

	// A new process: its screen replaces the notice, before any output.
	before := strings.Count(pc.out.String(), "\x1b[2J")
	r.m.Start("gemini", false)
	waitUntil(t, "the new screen", func() bool { return strings.Count(pc.out.String(), "\x1b[2J") > before })
	r.fake(1).outW.Write([]byte("SECOND"))
	waitUntil(t, "the new output", func() bool { return strings.Contains(pc.out.String(), "SECOND") })
	if got := r.fake(1).said(); got != "" {
		t.Fatalf("the new agent got %q typed without an agent", got)
	}

	// Stop says so too, once.
	r.m.Stop()
	waitUntil(t, "the second notice", func() bool { return strings.Count(pc.out.String(), AgentEnded) == 2 })
	pc.stdin.Close()
	if e := <-pc.done; e.Code != 0 {
		t.Fatalf("exit %+v", e)
	}
}

// gate is a terminal that stops reading while closed.
type gate struct {
	mu   sync.Mutex
	open *sync.Cond
	shut bool
	buf  syncBuf
}

func newGate() *gate { g := &gate{}; g.open = sync.NewCond(&g.mu); return g }

func (g *gate) Write(p []byte) (int, error) {
	g.mu.Lock()
	for g.shut {
		g.open.Wait()
	}
	g.mu.Unlock()
	return g.buf.Write(p)
}

func (g *gate) set(shut bool) { g.mu.Lock(); g.shut = shut; g.open.Broadcast(); g.mu.Unlock() }

// A PC terminal too slow for the output gets the whole screen when it
// catches up: what it shows is the agent's screen.
func TestAttachLaggingGetsTheWholeScreen(t *testing.T) {
	old := attachBacklog
	attachBacklog = 4
	defer func() { attachBacklog = old }()
	r := newRig(t)
	r.m.Start("codex", false)
	dir := serveIPC(t, r.m)
	g := newGate()
	pc := attachPC(t, dir, 80, 24, g)
	r.fake(0).outW.Write([]byte("FIRST"))
	waitUntil(t, "the screen", func() bool { return strings.Contains(g.buf.String(), "FIRST") })

	g.set(true) // the terminal stops reading
	line := strings.Repeat("x", 90)
	for i := 0; i < 2000; i++ {
		r.fake(0).outW.Write([]byte(fmt.Sprintf("\x1b[%dm%05d %s\x1b[0m\r\n", 31+i%7, i, line)))
	}
	r.fake(0).outW.Write([]byte("END-MARK"))
	waitUntil(t, "the screen to get it all", func() bool { return strings.Contains(r.m.Screen().Text(), "END-MARK") })
	g.set(false)

	want := r.m.Screen().Text()
	waitUntil(t, "the terminal to show the screen", func() bool {
		em := vt.NewEmulator(80, 24)
		defer em.Close()
		go io.Copy(io.Discard, em)
		em.Write([]byte(g.buf.String()))
		return em.String() == want
	})
	if n := strings.Count(g.buf.String(), "\x1b[2J"); n < 2 {
		t.Fatalf("%d screens sent: the terminal never lagged", n)
	}
	pc.stdin.Close()
	<-pc.done
}

// Two PC terminals: the last attached rules the size; when it detaches, the
// one before.
func TestTwoPCTerminals(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	dir := serveIPC(t, r.m)
	size := func() [2]int { c, rows := r.m.Screen().Size(); return [2]int{c, rows} }
	a := attachPC(t, dir, 100, 30, nil)
	waitUntil(t, "the first size", func() bool { return size() == [2]int{100, 30} })
	b := attachPC(t, dir, 90, 25, nil)
	waitUntil(t, "the second size", func() bool { return size() == [2]int{90, 25} })
	b.stdin.Close()
	<-b.done
	waitUntil(t, "the first size again", func() bool { return size() == [2]int{100, 30} })
	if !r.m.Info().PCAttached {
		t.Fatal("not pcAttached with a terminal left")
	}
	a.stdin.Close()
	<-a.done
	waitUntil(t, "no terminal", func() bool { return !r.m.Info().PCAttached })
}
