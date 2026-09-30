package daemon

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// gatedADB holds the commands that contain gate in adb until open is
// closed: a change that takes its time on the phone.
type gatedADB struct {
	*fakeADB
	gate    string
	started chan string
	open    chan struct{}
}

func newGated(f *fakeADB, gate string) *gatedADB {
	return &gatedADB{fakeADB: f, gate: gate, started: make(chan string, 4), open: make(chan struct{})}
}

func (g *gatedADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if joined := strings.Join(args, " "); strings.Contains(joined, g.gate) {
		g.started <- joined
		select {
		case <-g.open:
		case <-ctx.Done():
		}
	}
	return g.fakeADB.Run(ctx, dir, args, stdin, stdout, stderr)
}

func (g *gatedADB) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-g.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the gated command never reached adb")
	}
}

// notices records OnNotice.
type notices struct {
	mu sync.Mutex
	v  []string
}

func (n *notices) add(s string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.v = append(n.v, s)
}

func (n *notices) count(sub string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := 0
	for _, s := range n.v {
		if strings.Contains(s, sub) {
			c++
		}
	}
	return c
}

// waitFor waits for a notice containing sub. It may run outside the test's
// goroutine: it reports false instead of failing.
func (n *notices) waitFor(sub string) bool {
	deadline := time.Now().Add(5 * time.Second)
	for n.count(sub) == 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true
}

func notAsked(t *testing.T, held *heldConfirmer, why string) {
	t.Helper()
	select {
	case r := <-held.asked:
		t.Fatalf("%q asked while %s", r.Title, why)
	case <-time.After(300 * time.Millisecond):
	}
}

func asked(t *testing.T, held *heldConfirmer) confirm.Request {
	t.Helper()
	select {
	case r := <-held.asked:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("the question was never asked")
	}
	return confirm.Request{}
}

// Ruling on doubt 1: a 🟡 already running (`sleep; input tap` in the
// foreground) could tap Allow once the question shows up: the 🔴 waits for
// it to finish before asking, and says so once. New commands are refused
// meanwhile.
func TestQuestionWaitsForAChangeInFlight(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	g := newGated(e.adb, "input tap")
	e.d.o.ADB = g
	held := newHeld()
	e.d.o.Confirmer = held
	n := &notices{}
	e.d.o.OnNotice = n.add
	yellow := e.execAsync("shell", "sleep 1; input tap 1 1")
	g.waitStarted(t)
	red := e.execAsync("shell", "reboot")
	if !n.waitFor(`Waiting for "Interface action: tap 1 1" to finish before asking "Restart the phone"`) {
		t.Fatalf("not said: %q", n.v)
	}
	notAsked(t, held, "a change was running")
	r := waitResult(t, e.execAsync("shell", "getprop", "ro.product.model"))
	if r.exit.Code != ExitDenied || !strings.Contains(r.s.errOut.String(), `waiting for your answer to "Restart the phone"`) {
		t.Fatalf("new command while the 🔴 waits: %+v %q", r.exit, r.s.errOut.String())
	}
	close(g.open)
	if r := waitResult(t, yellow); r.exit.Code != 0 {
		t.Fatalf("the change in flight: %+v %q", r.exit, r.s.errOut.String())
	}
	asked(t, held)
	held.answer <- confirm.Deny
	waitResult(t, red)
	if c := n.count("Waiting for"); c != 1 {
		t.Fatalf("said %d times", c)
	}
}

// A 🟢 in flight (logcat) cannot inject touches: it does not delay the
// question.
func TestAGreenInFlightDoesNotDelayTheQuestion(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	g := newGated(e.adb, "logcat")
	e.d.o.ADB = g
	held := newHeld()
	e.d.o.Confirmer = held
	logcat := e.execAsync("logcat")
	g.waitStarted(t)
	red := e.execAsync("shell", "reboot")
	asked(t, held)
	held.answer <- confirm.Deny
	waitResult(t, red)
	close(g.open)
	waitResult(t, logcat)
}

// An undo running its inverses counts as a change in flight.
func TestQuestionWaitsForAnUndoInFlight(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	e.adb.shell["settings get global a"] = "1\n"
	if _, exit := e.exec(t, "shell", "settings", "put", "global", "a", "2"); exit.Code != 0 {
		t.Fatalf("setup: %+v", exit)
	}
	g := newGated(e.adb, "settings put global a 1")
	e.d.o.ADB = g
	held := newHeld()
	e.d.o.Confirmer = held
	undo := make(chan ipc.Exit, 1)
	go func() {
		undo <- e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	}()
	g.waitStarted(t)
	red := e.execAsync("shell", "reboot")
	notAsked(t, held, "an undo was running")
	close(g.open)
	if u := <-undo; u.Code != 0 {
		t.Fatalf("undo: %+v", u)
	}
	asked(t, held)
	held.answer <- confirm.Deny
	waitResult(t, red)
}

// While it waits for the changes in flight, a pause or the agent leaving
// ends the 🔴 without asking, and leaves no question behind.
func TestWaitingForChangesInFlightEnds(t *testing.T) {
	for _, how := range []string{"pause", "agent gone"} {
		t.Run(how, func(t *testing.T) {
			e := newEnv(t)
			e.d.o.OnEntry = func(journal.Entry) {}
			var paused atomic.Bool
			e.d.o.Paused = paused.Load
			g := newGated(e.adb, "input tap")
			e.d.o.ADB = g
			held := newHeld()
			e.d.o.Confirmer = held
			n := &notices{}
			e.d.o.OnNotice = n.add
			yellow := e.execAsync("shell", "input", "tap", "1", "1")
			g.waitStarted(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			red := make(chan result, 1)
			go func() {
				s := &stream{}
				red <- result{s, e.d.Handle(ctx, ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "reboot"}, Cwd: e.ws}, s)}
			}()
			if !n.waitFor("Waiting for") {
				t.Fatal("not said")
			}
			want := "Blocked by ADBuddy: " + PausedMessage
			if how == "pause" {
				paused.Store(true)
				e.d.CancelConfirmations()
			} else {
				cancel()
				want = ""
			}
			r := waitResult(t, red)
			if r.exit.Code != ExitDenied || strings.TrimSpace(r.s.errOut.String()) != want {
				t.Fatalf("%+v %q", r.exit, r.s.errOut.String())
			}
			if len(held.asked) != 0 {
				t.Fatal("asked")
			}
			j := e.last(t)
			if how == "pause" && (j.Status != journal.StatusBlocked || j.Reason != PausedMessage) ||
				how != "pause" && (j.Status != journal.StatusDenied || !strings.Contains(j.Reason, "abandoned")) {
				t.Fatalf("journal %+v", j)
			}
			close(g.open)
			waitResult(t, yellow)
			// The next 🔴 is not refused by a question left behind.
			paused.Store(false)
			next := e.execAsync("shell", "reboot")
			asked(t, held)
			held.answer <- confirm.Deny
			waitResult(t, next)
		})
	}
}

// Minor 3: a 🔴 allowed by its question runs, even when another 🔴 comes in
// between the answer and adb: that one waits for it instead.
func TestAnAllowedRedRunsWhenAnotherComesInBetween(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	held := newHeld()
	e.d.o.Confirmer = held
	n := &notices{}
	e.d.o.OnNotice = n.add
	var second chan result
	e.d.beforeRun = func(argv []string) {
		if len(argv) == 2 && argv[1] == "reboot" && second == nil {
			second = e.execAsync("shell", "reboot", "bootloader")
			n.waitFor("Waiting for")
		}
	}
	first := e.execAsync("shell", "reboot")
	asked(t, held)
	held.answer <- confirm.Allow
	if r := waitResult(t, first); r.exit.Code != 0 {
		t.Fatalf("allowed 🔴: %+v %q", r.exit, r.s.errOut.String())
	}
	if n.count("Waiting for") != 1 {
		t.Fatalf("the second 🔴 did not wait: %q", n.v)
	}
	asked(t, held)
	held.answer <- confirm.Deny
	waitResult(t, second)
	if ran := e.adb.ran(); strings.Join(ran, "|") != "shell reboot" {
		t.Fatalf("adb ran %q", ran)
	}
}
