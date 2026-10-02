package session

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests of the writer (V08). Each one reads the package variables it changes
// (writeWait, pasteGap, stopWait) before any goroutine it starts, and waits
// for those goroutines before it restores them.

// waitErr is the error from ch, failing the test after d.
func waitErr(t *testing.T, ch <-chan error, d time.Duration, what string) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(d):
		t.Fatalf("%s: no result after %v", what, d)
		return nil
	}
}

// waitSignal waits for a signal on ch, failing the test after 3 s.
func waitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: not after 3 s", what)
	}
}

// waitParked waits until exactly n goroutines are parked in waitRoom's select
// (waiting for room in a queue), as the runtime reports them: an explicit
// sign that a Wait is in line, with no sleep.
func waitParked(t *testing.T, n int) {
	t.Helper()
	parked := func() int {
		buf := make([]byte, 1<<20)
		buf = buf[:runtime.Stack(buf, true)]
		count := 0
		for _, g := range strings.Split(string(buf), "\n\n") {
			if strings.Contains(g, " [select") && strings.Contains(g, "session.(*proc).waitRoom") {
				count++
			}
		}
		return count
	}
	deadline := time.Now().Add(3 * time.Second)
	for parked() != n {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines waiting for room, want %d", parked(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

func drainsOf(pr *proc) uint64 {
	pr.qmu.Lock()
	defer pr.qmu.Unlock()
	return pr.drains
}

// stuck makes the writer of the current process wait in the Write of first
// and fills the queue behind it with ty: tokens <1>..<16>. It returns the
// fake's release and open (fakePTY.holdWrites) and the writing signal.
func (r *rig) stuck(t *testing.T, i int, ty *Typer, first string) (writing <-chan struct{}, release, open func()) {
	t.Helper()
	writing, release, open = r.fake(i).holdWrites()
	t.Cleanup(open)
	if err := ty.Input([]byte(first)); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, writing, "the first Write")
	for n := 1; n <= writeQueue; n++ {
		if err := ty.Input([]byte(fmt.Sprintf("<%d>", n))); err != nil {
			t.Fatalf("queueing <%d>: %v", n, err)
		}
	}
	return writing, release, open
}

func queued() string {
	var b strings.Builder
	for n := 1; n <= writeQueue; n++ {
		fmt.Fprintf(&b, "<%d>", n)
	}
	return b.String()
}

// 1. V08: with the agent's terminal stuck in a Write, Input and Paste return
// at once (Paste within writeWait): whoever types (the phone channel's
// reader) is never held.
func TestTypingDoesNotWaitForAStuckTerminal(t *testing.T) {
	defer func(w time.Duration) { writeWait = w }(writeWait)
	writeWait = 300 * time.Millisecond
	r := newRig(t)
	r.m.Start("codex", false)
	ty := r.m.NewTyper()
	start := time.Now()
	r.stuck(t, 0, ty, "a")
	for i := 0; i < 5; i++ {
		if err := ty.Input([]byte("b")); !errors.Is(err, ErrBusy) {
			t.Fatalf("Input into a full queue: %v", err)
		}
	}
	if err := r.m.Paste("hello", nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("Paste into a full queue: %v", err)
	}
	if d := time.Since(start); d > writeWait {
		t.Fatalf("typing took %v with the terminal stuck", d)
	}
}

// 2. A full queue drops a whole Input, never part of it; what was queued
// goes out in order once the terminal reads, and what was dropped never.
func TestAFullQueueDropsWholeInputs(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	ty := r.m.NewTyper()
	_, _, open := r.stuck(t, 0, ty, "<0>")
	if err := ty.Input([]byte("<dropped>")); !errors.Is(err, ErrDropped) {
		t.Fatalf("Input into a full queue: %v, want ErrDropped", err)
	}
	open()
	want := "<0>" + queued()
	waitFor(t, func() bool { return r.fake(0).said() == want })
	pr := r.m.current()
	waitFor(t, func() bool { return drainsOf(pr) > 0 })
	if got := r.fake(0).said(); got != want {
		t.Fatalf("typed %q, want %q", got, want)
	}
}

// 3. After a drop, the same Typer drops everything until the writer empties
// the queue: ESC dropped, then "[A" and "\r" are not typed on their own. The
// first drop is ErrDropped, the next ones ErrBusy. Another Typer goes on.
func TestADropHoldsItsTyperUntilTheQueueEmpties(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	other, ty := r.m.NewTyper(), r.m.NewTyper()
	writing, release, open := r.stuck(t, 0, other, "a")
	if err := ty.Input([]byte("\x1b")); !errors.Is(err, ErrDropped) {
		t.Fatalf("ESC into a full queue: %v, want ErrDropped", err)
	}
	release() // "a" goes; the writer waits in the Write of <1>: room for one
	waitSignal(t, writing, "the Write of <1>")
	if err := ty.Input([]byte("[A")); !errors.Is(err, ErrBusy) || errors.Is(err, ErrDropped) {
		t.Fatalf("the rest of the sequence: %v, want a plain ErrBusy", err)
	}
	if err := other.Input([]byte("c")); err != nil {
		t.Fatalf("another Typer: %v", err)
	}
	if err := ty.Input([]byte("\r")); !errors.Is(err, ErrBusy) || errors.Is(err, ErrDropped) {
		t.Fatalf("a lone Enter: %v, want a plain ErrBusy", err)
	}
	pr := r.m.current()
	before := drainsOf(pr)
	open()
	waitFor(t, func() bool { return drainsOf(pr) > before })
	if err := ty.Input([]byte("z")); err != nil {
		t.Fatalf("after the queue emptied: %v", err)
	}
	want := "a" + queued() + "cz"
	waitFor(t, func() bool { return r.fake(0).said() == want })
}

// 3. A new process of the agent lifts a Typer's drop: its first Input goes in.
func TestANewProcessLiftsTheDrop(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	ty := r.m.NewTyper()
	r.stuck(t, 0, ty, "a")
	if err := ty.Input([]byte("x")); !errors.Is(err, ErrDropped) {
		t.Fatalf("%v, want ErrDropped", err)
	}
	if err := ty.Input([]byte("y")); !errors.Is(err, ErrBusy) || errors.Is(err, ErrDropped) {
		t.Fatalf("%v, want a plain ErrBusy", err)
	}
	r.m.Stop()
	if err := r.m.Start("codex", false); err != nil {
		t.Fatal(err)
	}
	if err := ty.Input([]byte("first")); err != nil {
		t.Fatalf("the first Input into the new process: %v", err)
	}
	waitFor(t, func() bool { return r.fake(1).said() == "first" })
}

// 4. Nothing typed meanwhile gets between a pasted text and its Enter.
func TestPasteIsOneJob(t *testing.T) {
	defer func(g time.Duration) { pasteGap = g }(pasteGap)
	pasteGap = 30 * time.Millisecond
	r := newRig(t)
	r.m.Start("codex", false)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, k := range []string{"X", "Y"} {
		ty := r.m.NewTyper()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = ty.Input([]byte(k))
				time.Sleep(time.Millisecond)
			}
		}()
	}
	waitFor(t, func() bool { s := r.fake(0).said(); return strings.Contains(s, "X") && strings.Contains(s, "Y") })
	err := r.m.Paste("hello world", nil)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	got := r.fake(0).said()
	i := strings.Index(got, "hello world")
	if i < 0 || !strings.HasPrefix(got[i:], "hello world\r") {
		t.Fatalf("typed %q: something between the text and its Enter", got)
	}
}

// 5. A Paste still queued at writeWait is never typed (ErrBusy); one the
// writer started returns nil at writeWait without waiting for it. The writer
// then ends that one and goes on (its result's channel has room).
func TestPasteGivesUpAtWriteWait(t *testing.T) {
	defer func(w time.Duration) { writeWait = w }(writeWait)
	writeWait = 300 * time.Millisecond
	r := newRig(t)
	r.m.Start("codex", false)
	writing, _, open := r.fake(0).holdWrites()
	t.Cleanup(open)
	slow := make(chan error, 1)
	go func() { slow <- r.m.Paste("slow", nil) }()
	waitSignal(t, writing, "the Write of the first paste")
	start := time.Now()
	err := r.m.Paste("never", nil)
	if d := time.Since(start); d < writeWait-50*time.Millisecond || d > writeWait+time.Second {
		t.Fatalf("the queued Paste returned after %v, want about %v", d, writeWait)
	}
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("the queued Paste: %v, want ErrBusy", err)
	}
	if err := waitErr(t, slow, writeWait+time.Second, "the started Paste"); err != nil {
		t.Fatalf("the started Paste: %v, want nil", err)
	}
	open()
	if err := r.m.NewTyper().Input([]byte("z")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.fake(0).said() == "slow\rz" })
	time.Sleep(50 * time.Millisecond)
	if got := r.fake(0).said(); got != "slow\rz" {
		t.Fatalf("typed %q, want %q", got, "slow\rz")
	}
}

// 6. The guard and the screen are asked by the writer: busy while the paste
// waits in the queue, nothing is typed; busy between the text and its
// Enter, the Enter is not typed.
func TestPasteGuardIsAskedWhenTyping(t *testing.T) {
	defer func(w, g time.Duration) { writeWait, pasteGap = w, g }(writeWait, pasteGap)
	writeWait, pasteGap = 2*time.Second, 10*time.Millisecond

	t.Run("busy while queued", func(t *testing.T) {
		r := newRig(t)
		r.m.Start("codex", false)
		ty := r.m.NewTyper()
		writing, release, open := r.fake(0).holdWrites()
		t.Cleanup(open)
		if err := ty.Input([]byte("a")); err != nil {
			t.Fatal(err)
		}
		waitSignal(t, writing, "the Write of a")
		var busy atomic.Bool
		res := make(chan error, 1)
		go func() {
			res <- r.m.Paste("text", func() string {
				if busy.Load() {
					return "busy now"
				}
				return ""
			})
		}()
		pr := r.m.current()
		waitFor(t, func() bool { return len(pr.q) == 1 })
		busy.Store(true)
		release()
		err := waitErr(t, res, 3*time.Second, "the Paste")
		var be *BusyError
		if !errors.As(err, &be) || be.Why != "busy now" || !errors.Is(err, ErrBusy) {
			t.Fatalf("Paste: %v, want the guard's BusyError", err)
		}
		open()
		if err := ty.Input([]byte("z")); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return r.fake(0).said() == "az" })
	})

	t.Run("busy before the Enter", func(t *testing.T) {
		r := newRig(t)
		r.m.Start("codex", false)
		var calls atomic.Int32
		err := r.m.Paste("text", func() string {
			if calls.Add(1) >= 2 {
				return "a dialog"
			}
			return ""
		})
		var be *BusyError
		if !errors.As(err, &be) || be.Why != "a dialog" {
			t.Fatalf("Paste: %v, want the guard's BusyError", err)
		}
		if got := r.fake(0).said(); got != "text" {
			t.Fatalf("typed %q, want the text without its Enter", got)
		}
	})

	t.Run("a question on the screen while queued", func(t *testing.T) {
		r := newRig(t)
		r.m.Start("claude", false)
		ty := r.m.NewTyper()
		writing, release, open := r.fake(0).holdWrites()
		t.Cleanup(open)
		if err := ty.Input([]byte("a")); err != nil {
			t.Fatal(err)
		}
		waitSignal(t, writing, "the Write of a")
		res := make(chan error, 1)
		go func() { res <- r.m.Paste("text", nil) }()
		pr := r.m.current()
		waitFor(t, func() bool { return len(pr.q) == 1 })
		r.fake(0).outW.Write([]byte("Do you want to proceed?\r\n❯ 1. Yes"))
		waitFor(t, func() bool { return r.m.Busy() != "" })
		release()
		err := waitErr(t, res, 3*time.Second, "the Paste")
		var be *BusyError
		if !errors.As(err, &be) || be.Why != r.m.Busy() {
			t.Fatalf("Paste: %v, want the screen's BusyError", err)
		}
		open()
		if err := ty.Input([]byte("z")); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { return r.fake(0).said() == "az" })
	})
}

// 7. Stop ends with the terminal stuck in a Write that only the Kill frees:
// it does not wait for the writer. What was queued is dropped (ErrNoAgent);
// the stopped process takes nothing more and does not panic (its queue is
// never closed); nothing of it reaches the next process.
func TestStopEndsWithAStuckTerminal(t *testing.T) {
	defer func(d time.Duration) { stopWait = d }(stopWait)
	stopWait = 100 * time.Millisecond
	r := newRig(t)
	r.m.Start("codex", false)
	f := r.fake(0)
	f.mu.Lock()
	f.deaf = true
	f.mu.Unlock()
	writing, _, _ := f.holdWrites() // only the Kill frees it
	ty := r.m.NewTyper()
	if err := ty.Input([]byte("a")); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, writing, "the Write of a")
	old := r.m.current()
	res := make(chan error, 1)
	go func() { res <- r.m.Paste("queued", nil) }()
	waitFor(t, func() bool { return len(old.q) == 1 })
	if err := ty.Input([]byte("b")); err != nil {
		t.Fatal(err)
	}

	stopped := make(chan struct{})
	go func() { r.m.Stop(); close(stopped) }()
	waitSignal(t, stopped, "Stop with the terminal stuck")
	if err := waitErr(t, res, 3*time.Second, "the queued Paste"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("the queued Paste: %v, want ErrNoAgent", err)
	}
	waitSignal(t, old.wdone, "the writer's end")
	if _, err := old.enqueue(&writeJob{gen: old.gen, data: []byte("late")}); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("queueing into the stopped process: %v", err)
	}
	if err := ty.Input([]byte("late")); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("Input after Stop: %v", err)
	}
	if err := r.m.Paste("late", nil); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("Paste after Stop: %v", err)
	}

	r.m.Start("gemini", false)
	if err := ty.Input([]byte("new")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.fake(1).said() == "new" })
	time.Sleep(50 * time.Millisecond)
	if got := r.fake(1).said(); got != "new" {
		t.Fatalf("the new process got %q", got)
	}
	if got := f.said(); got != "" {
		t.Fatalf("the stopped process got %q", got)
	}
}

// 8. Wait waits for room in the queue, in order, and gives up with its ctx
// or when the process stops; without an agent it does not wait.
func TestTyperWait(t *testing.T) {
	r := newRig(t)
	if err := r.m.NewTyper().Wait(context.Background(), []byte("x")); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("without an agent: %v", err)
	}
	r.m.Start("codex", false)
	ty := r.m.NewTyper()
	writing, _, open := r.fake(0).holdWrites()
	t.Cleanup(open)
	ctx := context.Background()
	if err := ty.Wait(ctx, []byte("<0>")); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, writing, "the Write of <0>")
	for n := 1; n <= writeQueue; n++ {
		if err := ty.Wait(ctx, []byte(fmt.Sprintf("<%d>", n))); err != nil {
			t.Fatal(err)
		}
	}

	cctx, cancel := context.WithCancel(ctx)
	res := make(chan error, 1)
	go func() { res <- ty.Wait(cctx, []byte("<cancelled>")) }()
	select {
	case err := <-res:
		t.Fatalf("Wait into a full queue did not wait: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	if err := waitErr(t, res, 3*time.Second, "the cancelled Wait"); !errors.Is(err, context.Canceled) {
		t.Fatalf("the cancelled Wait: %v", err)
	}

	// FIFO: q starts once p is parked in its send, waiting for room.
	waitParked(t, 0)
	p, q := make(chan error, 1), make(chan error, 1)
	go func() { p <- ty.Wait(ctx, []byte("<p>")) }()
	waitParked(t, 1)
	go func() { q <- r.m.NewTyper().Wait(ctx, []byte("<q>")) }()
	waitParked(t, 2)
	open()
	for _, ch := range []chan error{p, q} {
		if err := waitErr(t, ch, 3*time.Second, "a waiting Wait"); err != nil {
			t.Fatal(err)
		}
	}
	want := "<0>" + queued() + "<p><q>"
	waitFor(t, func() bool { return r.fake(0).said() == want })
}

// 8. A Wait for room gives up with ErrNoAgent when the process stops.
func TestTyperWaitEndsWithTheProcess(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	ty := r.m.NewTyper()
	r.stuck(t, 0, ty, "a")
	res := make(chan error, 1)
	go func() { res <- ty.Wait(context.Background(), []byte("waiting")) }()
	select {
	case err := <-res:
		t.Fatalf("Wait into a full queue did not wait: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	r.m.Stop()
	if err := waitErr(t, res, 3*time.Second, "the Wait"); !errors.Is(err, ErrNoAgent) {
		t.Fatalf("Wait when the process stopped: %v", err)
	}
}

// 8. A Wait into a stopped process with room in its queue never reports
// typing that will not happen: ErrNoAgent every time (its select may choose
// the send).
func TestWaitIntoAStoppedProcessIsNoAgent(t *testing.T) {
	r := newRig(t)
	r.m.Start("codex", false)
	pr := r.m.current()
	r.m.Stop()
	waitSignal(t, pr.wdone, "the writer's end")
	for i := 0; i < writeQueue; i++ { // each try may fill one slot
		if err := pr.waitRoom(context.Background(), &writeJob{gen: pr.gen, data: []byte("x")}); !errors.Is(err, ErrNoAgent) {
			t.Fatalf("try %d: %v, want ErrNoAgent", i, err)
		}
	}
}

// 2. Bracketed paste or one line is chosen when the paste is typed, not when
// it was queued: the agent turns bracketed paste on while the paste waits.
func TestPasteChoosesBracketsWhenTyped(t *testing.T) {
	r := newRig(t)
	r.m.Start("claude", false)
	ty := r.m.NewTyper()
	writing, _, open := r.fake(0).holdWrites()
	t.Cleanup(open)
	if err := ty.Input([]byte("a")); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, writing, "the Write of a")
	res := make(chan error, 1)
	go func() { res <- r.m.Paste("one\ntwo", nil) }()
	pr := r.m.current()
	waitFor(t, func() bool { return len(pr.q) == 1 })
	r.fake(0).outW.Write([]byte("\x1b[?2004h"))
	waitFor(t, func() bool { return pr.screen.BracketedPaste() })
	open()
	if err := waitErr(t, res, 5*time.Second, "the Paste"); err != nil {
		t.Fatal(err)
	}
	if got, want := r.fake(0).said(), "a\x1b[200~one\ntwo\x1b[201~\r"; got != want {
		t.Fatalf("typed %q, want %q", got, want)
	}
}
