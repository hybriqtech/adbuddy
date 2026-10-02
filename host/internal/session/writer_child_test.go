package session

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/pty"
)

// TestMain lets the test binary be an agent that never reads its input: with
// ADBUDDY_DEAF_AGENT=1 it only sleeps.
func TestMain(m *testing.M) {
	if os.Getenv("ADBUDDY_DEAF_AGENT") == "1" {
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// 7. With a real agent in a real terminal (ConPTY, a Unix pty) that does not
// read its input, the writer gets stuck in a Write; Stop still ends, and the
// Kill frees the writer. The test first proves the Write got stuck (the
// queue stays full with nothing typed): a terminal that takes everything
// (conhost may read its input whatever the program does) is skipped, as
// there is no stuck Write to free.
func TestStopEndsWithARealAgentThatDoesNotRead(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a real agent")
	}
	defer func(d time.Duration) { stopWait = d }(stopWait)
	stopWait = 200 * time.Millisecond
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := New(Options{
		Dir:   t.TempDir(),
		Look:  func(string) (string, error) { return exe, nil },
		Env:   func(string) []string { return append(os.Environ(), "ADBUDDY_DEAF_AGENT=1") },
		Start: pty.Start,
	})
	t.Cleanup(m.Stop)
	if err := m.Start("codex", false); err != nil {
		t.Fatal(err)
	}
	pr := m.current()
	// Lines, so that a Unix terminal in canonical mode queues them for the
	// program instead of dropping what no line fits.
	line := []byte(strings.Repeat("x", 1023) + "\r")
	// Bytes typed, or time, before deciding that the terminal takes it all.
	const most = 4 << 20
	until := time.Now().Add(20 * time.Second)
	ty, sent, stuck := m.NewTyper(), 0, false
	for sent < most && time.Now().Before(until) && !stuck {
		err := ty.Input(line)
		if err == nil {
			sent += len(line)
			continue
		}
		if !errors.Is(err, ErrBusy) {
			t.Fatal(err)
		}
		// Full: stuck, or only behind? A stuck writer takes nothing more
		// from the queue.
		stuck = true
		for wait := time.Now().Add(time.Second); time.Now().Before(wait); time.Sleep(10 * time.Millisecond) {
			if len(pr.q) < cap(pr.q) {
				stuck = false
				break
			}
		}
		ty = m.NewTyper() // the drop's episode does not matter here
	}
	if !stuck {
		t.Skipf("the terminal took %d bytes with no Write stuck (on %s): nothing to free; see the plan's manual validations", sent, runtime.GOOS)
	}
	t.Logf("the writer got stuck after about %d bytes", sent)

	stopped := make(chan struct{})
	go func() { m.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(stopWait + 20*time.Second):
		t.Fatal("Stop hangs on the stuck Write")
	}
	select {
	case <-pr.wdone:
	case <-time.After(10 * time.Second):
		t.Fatal("the Kill did not free the writer's Write")
	}
}
