package session

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/pty"
)

// fakePTY is an agent in memory: the test writes its output and reads what
// was typed.
type fakePTY struct {
	out   *io.PipeReader
	outW  *io.PipeWriter
	mu    sync.Mutex
	typed strings.Builder
	sizes [][2]int
	code  int
	done  chan struct{}
	once  sync.Once
}

func newFake() *fakePTY {
	r, w := io.Pipe()
	return &fakePTY{out: r, outW: w, done: make(chan struct{})}
}

func (f *fakePTY) Read(b []byte) (int, error) { return f.out.Read(b) }
func (f *fakePTY) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typed.Write(b)
	return len(b), nil
}
func (f *fakePTY) Resize(c, r int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, [2]int{c, r})
	return nil
}
func (f *fakePTY) Pid() int { return 1 }
func (f *fakePTY) Wait(ctx context.Context) (int, error) {
	select {
	case <-f.done:
		return f.code, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (f *fakePTY) Kill() error { f.exit(137); return nil }
func (f *fakePTY) exit(code int) {
	f.once.Do(func() { f.code = code; f.outW.Close(); close(f.done) })
}
func (f *fakePTY) said() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.typed.String()
}

// rig is a Manager with fake agents; each Start makes a new one.
type rig struct {
	m      *Manager
	mu     sync.Mutex
	fakes  []*fakePTY
	starts []pty.Options
	infos  []Info
	output strings.Builder
}

func newRig(t *testing.T) *rig {
	r := &rig{}
	r.m = New(Options{
		Dir:   t.TempDir(),
		Look:  func(name string) (string, error) { return "/bin/" + name, nil },
		Env:   func(string) []string { return []string{"A=1"} },
		Quiet: 50 * time.Millisecond,
		Retry: time.Second,
		Start: func(o pty.Options) (pty.PTY, error) {
			f := newFake()
			r.mu.Lock()
			r.fakes = append(r.fakes, f)
			r.starts = append(r.starts, o)
			r.mu.Unlock()
			return f, nil
		},
		OnInfo: func(i Info) { r.mu.Lock(); r.infos = append(r.infos, i); r.mu.Unlock() },
	})
	r.m.Subscribe(func(o Output) { r.mu.Lock(); r.output.Write(o.Data); r.mu.Unlock() })
	t.Cleanup(r.m.Stop)
	return r
}

func (r *rig) fake(i int) *fakePTY {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fakes[i]
}

func (r *rig) waitState(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for r.m.Info().State != want {
		if time.Now().After(deadline) {
			t.Fatalf("state %q, want %q", r.m.Info().State, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The state follows the output: idle, working while it writes, waiting after
// Quiet without output, stopped when it ends.
func TestStates(t *testing.T) {
	r := newRig(t)
	if err := r.m.Start("codex", false); err != nil {
		t.Fatal(err)
	}
	i := r.m.Info()
	if i.State != StateIdle || i.Agent != "codex" || !strings.HasPrefix(i.Epoch, "e_") || !strings.HasPrefix(i.Conversation, "c_") {
		t.Fatalf("info %+v", i)
	}
	if o := r.starts[0]; o.Cols != DefaultCols || o.Rows != DefaultRows || len(o.Args) != 0 {
		t.Fatalf("start %+v", o)
	}
	r.fake(0).outW.Write([]byte("hello"))
	r.waitState(t, StateWorking)
	r.waitState(t, StateWaiting)
	if r.m.Screen().Offset() != 5 {
		t.Fatalf("offset %d", r.m.Screen().Offset())
	}
	r.mu.Lock()
	got := r.output.String()
	r.mu.Unlock()
	if got != "hello" {
		t.Fatalf("subscriber got %q", got)
	}
	r.fake(0).exit(0)
	r.waitState(t, StateStopped)
	if err := r.m.Input([]byte("x")); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("Input without agent: %v", err)
	}
	if r.m.Screen() != nil {
		t.Fatal("a screen without agent")
	}
}

// A resumed claude that ends at once with an error starts again without
// --continue (nothing to continue).
func TestResumeFallsBackToANewConversation(t *testing.T) {
	r := newRig(t)
	if err := r.m.Start("claude", true); err != nil {
		t.Fatal(err)
	}
	if args := r.starts[0].Args; len(args) != 1 || args[0] != "--continue" {
		t.Fatalf("args %v", args)
	}
	r.fake(0).exit(1)
	deadline := time.Now().Add(3 * time.Second)
	for {
		r.mu.Lock()
		n := len(r.starts)
		r.mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not started again")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if args := r.starts[1].Args; len(args) != 0 {
		t.Fatalf("second start args %v", args)
	}
	if r.m.Info().State != StateIdle {
		t.Fatalf("state %q", r.m.Info().State)
	}
}

// Paste sends one message: bracketed paste when the agent turned it on, one
// line when not; control characters never get through.
func TestPaste(t *testing.T) {
	pasteGap = time.Millisecond
	r := newRig(t)
	r.m.Start("claude", false)
	if err := r.m.Paste("one\ntwo"); err != nil {
		t.Fatal(err)
	}
	if got := r.fake(0).said(); got != "one two\r" {
		t.Fatalf("without bracketed paste: %q", got)
	}
	r.fake(0).outW.Write([]byte("\x1b[?2004h"))
	r.waitState(t, StateWorking)
	r.m.Paste("a\x1b[201~\x03b\r\nc ñ")
	if got := r.fake(0).said(); !strings.HasSuffix(got, "\x1b[200~a[201~b\nc ñ\x1b[201~\r") {
		t.Fatalf("with bracketed paste: %q", got)
	}
}

// Busy reports Claude Code's trust dialog on the screen.
func TestBusyWithTheTrustDialog(t *testing.T) {
	r := newRig(t)
	r.m.Start("claude", false)
	if r.m.Busy() != "" {
		t.Fatal("busy at start")
	}
	r.fake(0).outW.Write([]byte("Quick safety check: Is this a project you created or one you trust?"))
	r.waitState(t, StateWorking)
	if r.m.Busy() == "" {
		t.Fatal("the dialog was not seen")
	}
}

// The last PC terminal rules the size, then the phone, then 120×40.
func TestSizeRules(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	f := r.fake(0)
	r.m.ResizePhone(50, 20)
	r.m.AttachPC("a", 100, 30)
	r.m.AttachPC("b", 90, 25)
	if !r.m.Info().PCAttached {
		t.Fatal("pcAttached")
	}
	r.m.ResizePhone(40, 10) // a PC rules: no change
	r.m.DetachPC("b")
	r.m.DetachPC("a")
	if r.m.Info().PCAttached {
		t.Fatal("still pcAttached")
	}
	want := [][2]int{{50, 20}, {100, 30}, {90, 25}, {100, 30}, {40, 10}}
	f.mu.Lock()
	got := f.sizes
	f.mu.Unlock()
	if len(got) != len(want) {
		t.Fatalf("sizes %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sizes %v, want %v", got, want)
		}
	}
	if c, rr := r.m.Screen().Size(); c != 40 || rr != 10 {
		t.Fatalf("screen %dx%d", c, rr)
	}
}

// Start replaces the process: the old one is killed and a new epoch begins.
func TestStartReplaces(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	first := r.m.Info().Epoch
	r.m.Start("gemini", false)
	select {
	case <-r.fake(0).done:
	case <-time.After(3 * time.Second):
		t.Fatal("the first process is still running")
	}
	if i := r.m.Info(); i.Epoch == first || i.Agent != "gemini" || i.State != StateIdle {
		t.Fatalf("info %+v", i)
	}
}
