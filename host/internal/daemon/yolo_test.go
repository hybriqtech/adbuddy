package daemon

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// yoloClock is the clock and the timers of a Yolo under test: time only moves
// when the test says so.
type yoloClock struct {
	mu     sync.Mutex
	t      time.Time
	timers []*yoloTimer
	// lateStop makes the stop function a no-op: the Stop of a real AfterFunc can lose the race with the timer
	// firing, so a replaced timer may still run its function. Then only the generation check (and the clock)
	// protects the new state.
	lateStop bool
}

type yoloTimer struct {
	at      time.Time
	f       func()
	stopped bool
	fired   bool
}

func newYoloClock() *yoloClock {
	return &yoloClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
}

func (c *yoloClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *yoloClock) after(d time.Duration, f func()) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &yoloTimer{at: c.t.Add(d), f: f}
	c.timers = append(c.timers, t)
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if !c.lateStop {
			t.stopped = true
		}
	}
}

// advance moves the clock and runs, oldest first, the timers that are due.
func (c *yoloClock) advance(d time.Duration) {
	c.jump(d)
	c.fireDue()
}

// jump moves the clock without running any timer: the PC slept through them.
func (c *yoloClock) jump(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func (c *yoloClock) fireDue() {
	for {
		c.mu.Lock()
		var next *yoloTimer
		for _, t := range c.timers {
			if !t.stopped && !t.fired && !t.at.After(c.t) && (next == nil || t.at.Before(next.at)) {
				next = t
			}
		}
		if next != nil {
			next.fired = true
		}
		c.mu.Unlock()
		if next == nil {
			return
		}
		next.f()
	}
}

// yoloSeen records what the observers are told.
type yoloSeen struct {
	mu  sync.Mutex
	got []YoloChange
}

func (s *yoloSeen) add(c YoloChange) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, c)
}

func (s *yoloSeen) whys() []YoloWhy {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []YoloWhy
	for _, c := range s.got {
		out = append(out, c.Why)
	}
	return out
}

func newTestYolo() (*Yolo, *yoloClock, *yoloSeen) {
	c := newYoloClock()
	y := NewYolo(c.now, c.after)
	s := &yoloSeen{}
	y.Subscribe(s.add)
	return y, c, s
}

func TestYoloLastsExactlyTheTimeItWasGiven(t *testing.T) {
	for _, d := range []time.Duration{YoloShort, YoloLong} {
		y, c, seen := newTestYolo()
		start := c.now()
		if err := y.Enable(d); err != nil {
			t.Fatal(err)
		}
		if st := y.State(); !st.Active || st.Session || !st.Until.Equal(start.Add(d)) {
			t.Fatalf("%v: state %+v", d, st)
		}
		c.advance(d - time.Second)
		if !y.Active() {
			t.Fatalf("%v: off a second early", d)
		}
		c.advance(time.Second)
		if y.Active() {
			t.Fatalf("%v: still on at the expiry", d)
		}
		if got := seen.whys(); !slices.Equal(got, []YoloWhy{YoloOn, YoloOffExpired}) {
			t.Fatalf("%v: told %v", d, got)
		}
	}
}

// Mutation: remove the check of the duration in Enable.
func TestYoloOnlyAcceptsTheTwoDurations(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Minute, time.Minute, 14 * time.Minute, 16 * time.Minute, 2 * time.Hour, 24 * time.Hour} {
		y, _, seen := newTestYolo()
		if err := y.Enable(d); err != ErrYoloDuration {
			t.Errorf("%v: err %v", d, err)
		}
		if y.Active() || len(seen.whys()) != 0 {
			t.Errorf("%v: turned on, or told, by a refused duration", d)
		}
	}
}

func TestYoloSessionDoesNotExpire(t *testing.T) {
	y, c, seen := newTestYolo()
	y.EnableSession()
	c.advance(1000 * time.Hour)
	if st := y.State(); !st.Active || !st.Session || !st.Until.IsZero() {
		t.Fatalf("state %+v", st)
	}
	if got := seen.whys(); !slices.Equal(got, []YoloWhy{YoloOn}) {
		t.Fatalf("told %v", got)
	}
}

func TestYoloOffTellsOnlyWhenItWasOn(t *testing.T) {
	y, _, seen := newTestYolo()
	y.Off(YoloOffCLI)
	if len(seen.whys()) != 0 {
		t.Fatalf("off while off was told: %v", seen.whys())
	}
	y.EnableSession()
	y.Off(YoloOffApp)
	y.Off(YoloOffApp)
	if got := seen.whys(); !slices.Equal(got, []YoloWhy{YoloOn, YoloOffApp}) {
		t.Fatalf("told %v", got)
	}
	if y.Active() {
		t.Fatal("still on")
	}
}

// Enabling again replaces the first expiry: the old timer (whose Stop lost the
// race, lateStop) must not switch the new one off. Mutation: in expire, remove
// `g != y.gen ||` and the `if now.Before(y.until) {…}` block (the two things
// that protect the new state from the old timer).
func TestYoloEnablingAgainDoesNotInheritTheOldTimer(t *testing.T) {
	y, c, seen := newTestYolo()
	c.lateStop = true
	if err := y.Enable(YoloShort); err != nil {
		t.Fatal(err)
	}
	c.advance(10 * time.Minute)
	if err := y.Enable(YoloShort); err != nil {
		t.Fatal(err)
	}
	c.advance(10 * time.Minute) // the first one would have expired 5 minutes ago
	if !y.Active() {
		t.Fatal("the old timer switched the new YOLO off")
	}
	c.advance(5 * time.Minute)
	if y.Active() {
		t.Fatal("still on after its 15 minutes")
	}
	if got := seen.whys(); !slices.Equal(got, []YoloWhy{YoloOn, YoloOn, YoloOffExpired}) {
		t.Fatalf("told %v", got)
	}
}

// A switched-off YOLO is not brought back by an old timer either (same mutation).
func TestYoloOffCancelsTheTimer(t *testing.T) {
	y, c, seen := newTestYolo()
	c.lateStop = true
	_ = y.Enable(YoloShort)
	y.Off(YoloOffApp)
	_ = y.Enable(YoloLong)
	c.advance(YoloShort)
	if !y.Active() {
		t.Fatal("the first timer switched the second YOLO off")
	}
	if got := seen.whys(); !slices.Equal(got, []YoloWhy{YoloOn, YoloOffApp, YoloOn}) {
		t.Fatalf("told %v", got)
	}
}

// The expiry is the clock's, not the timer's: the PC slept through the timer
// (or it ran late) and the clock is past the expiry. Mutation: make Active
// return true while until is set, whatever the clock says; or (second part)
// remove the `if !y.late` guard of Active.
func TestYoloExpiryDoesNotNeedTheTimer(t *testing.T) {
	y, c, seen := newTestYolo()
	_ = y.Enable(YoloShort)
	c.jump(20 * time.Minute) // no timer runs
	if st := y.State(); st.Active || !st.Until.IsZero() {
		t.Fatalf("state %+v after its expiry", st)
	}
	if y.Active() {
		t.Fatal("active after its expiry because the timer did not run")
	}
	// Many doors ask in a row: the late telling is scheduled once (the original timer and one more).
	y.Active()
	y.Active()
	if n := len(c.timers); n != 2 {
		t.Fatalf("%d timers: every Active scheduled its own late telling", n)
	}
	c.fireDue() // Active asked for the late expiry to be told
	if got := seen.whys(); !slices.Equal(got, []YoloWhy{YoloOn, YoloOffExpired}) {
		t.Fatalf("told %v", got)
	}
	c.fireDue()
	if len(seen.whys()) != 2 {
		t.Fatalf("told twice: %v", seen.whys())
	}
}

// A timer that runs a moment early (the wall clock moved) waits for the rest.
func TestYoloAnEarlyTimerWaitsForTheRest(t *testing.T) {
	y, c, seen := newTestYolo()
	_ = y.Enable(YoloShort)
	c.mu.Lock()
	c.timers[0].at = c.t.Add(14 * time.Minute) // fires a minute early
	c.mu.Unlock()
	c.advance(14 * time.Minute)
	if !y.Active() || len(seen.whys()) != 1 {
		t.Fatalf("switched off early: %v", seen.whys())
	}
	c.advance(time.Minute)
	if y.Active() || !slices.Equal(seen.whys(), []YoloWhy{YoloOn, YoloOffExpired}) {
		t.Fatalf("not switched off at the expiry: %v", seen.whys())
	}
}

// An off over an expiry nobody has been told yet tells that expiry.
func TestYoloOffOverALateExpiryTellsTheExpiry(t *testing.T) {
	y, c, seen := newTestYolo()
	_ = y.Enable(YoloShort)
	c.jump(20 * time.Minute)
	y.Off(YoloOffPause)
	if got := seen.whys(); !slices.Equal(got, []YoloWhy{YoloOn, YoloOffExpired}) {
		t.Fatalf("told %v", got)
	}
}

func TestYoloObserversSeeTheStateOfTheChange(t *testing.T) {
	y, _, seen := newTestYolo()
	y.Subscribe(func(c YoloChange) {
		if got := y.State(); got != c.State { // observers may read the state
			t.Errorf("state %+v, change %+v", got, c.State)
		}
	})
	_ = y.Enable(YoloLong)
	y.Off(YoloOffApp)
	if len(seen.whys()) != 2 {
		t.Fatal(seen.whys())
	}
}

// The pause is asked through d.paused: it switches YOLO off for good (resuming
// does not bring it back). Mutation: remove the Off call in paused.
func TestPauseSwitchesYoloOff(t *testing.T) {
	e := newEnv(t)
	if err := e.d.Yolo().Enable(YoloShort); err != nil {
		t.Fatal(err)
	}
	paused := true
	e.d.o.Paused = func() bool { return paused }
	s, exit := e.exec(t, "shell", "getprop", "ro.product.model")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "paused from the phone") {
		t.Fatalf("exit %+v %q", exit, s.errOut.String())
	}
	if e.d.Yolo().Active() {
		t.Fatal("YOLO survived a pause")
	}
	paused = false
	if e.d.Yolo().Active() {
		t.Fatal("resuming brought YOLO back")
	}
}

func TestYoloChangesAreInTheJournal(t *testing.T) {
	e := newEnv(t)
	y := e.d.Yolo()
	if err := y.Enable(YoloShort); err != nil {
		t.Fatal(err)
	}
	j := e.last(t)
	if j.Rule != "yolo.on" || j.Level != "green" || j.Status != journal.StatusOK || !strings.Contains(j.Description, "YOLO mode on until") || j.Source != journal.SourceUser {
		t.Fatalf("entry %+v", j)
	}
	y.Off(YoloOffApp)
	if j := e.last(t); j.Rule != "yolo.off" || !strings.Contains(j.Description, "turned off from the phone") {
		t.Fatalf("entry %+v", j)
	}
	if len(e.entries) != 2 {
		t.Fatalf("the live view got %d entries", len(e.entries))
	}
	// They are events, not changes: there is nothing to undo.
	if _, exit, _ := e.undoLast(t); exit.Code == 0 {
		t.Fatalf("undo found something to undo in YOLO's events: %+v", exit)
	}
}
