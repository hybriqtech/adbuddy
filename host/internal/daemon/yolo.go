package daemon

import (
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// YOLO mode (spec 2026-10-18-adbuddy-modo-yolo-design.md): for a time the
// person chose on the phone, the agent works directly. Only the phone turns it
// on (the app's authenticated channel; nothing the agent can reach), and it
// lives in memory: it does not survive serve. The pause, `adbuddy yolo off`,
// the app and the expiry turn it off.

// The durations Enable accepts.
const (
	YoloShort = 15 * time.Minute
	YoloLong  = time.Hour
)

// ErrYoloDuration is Enable's error for any other duration.
var ErrYoloDuration = errors.New("YOLO lasts 15 minutes, 1 hour or until the session ends")

// YoloWhy says what changed YOLO.
type YoloWhy string

const (
	YoloOn         YoloWhy = "on"
	YoloOffApp     YoloWhy = "app"     // yolo.set off from the phone
	YoloOffCLI     YoloWhy = "cli"     // adbuddy yolo off
	YoloOffExpired YoloWhy = "expired" // its time is up
	YoloOffPause   YoloWhy = "pause"   // the agent was paused from the phone
)

// YoloState is YOLO now.
type YoloState struct {
	Active  bool
	Until   time.Time // zero when off or until the session ends
	Session bool      // until serve stops
}

// YoloChange is what an observer is told: the state after the change and why.
type YoloChange struct {
	State YoloState
	Why   YoloWhy
}

// Yolo is the state of the mode. It is safe for concurrent use. now and after
// are the clock and the timer, injectable for the tests.
//
// Lock order: notifyMu, then mu. A change and the calls to its observers
// happen under notifyMu, so the observers see the changes in order; they run
// without mu, and may call State and Active but never Enable, EnableSession
// or Off.
type Yolo struct {
	now   func() time.Time
	after func(d time.Duration, f func()) (stop func())

	notifyMu  sync.Mutex
	mu        sync.Mutex
	until     time.Time
	session   bool
	gen       uint64 // a timer only acts for the generation it was armed in
	stop      func()
	late      bool // Active found the expiry passed and has scheduled telling it: once
	observers []func(YoloChange)
}

// NewYolo returns YOLO, off. nil now is time.Now and nil after is time.AfterFunc.
func NewYolo(now func() time.Time, after func(d time.Duration, f func()) (stop func())) *Yolo {
	if now == nil {
		now = time.Now
	}
	if after == nil {
		after = func(d time.Duration, f func()) func() {
			t := time.AfterFunc(d, f)
			return func() { t.Stop() }
		}
	}
	return &Yolo{now: now, after: after}
}

// Subscribe registers f to be told of every change (including the expiry).
func (y *Yolo) Subscribe(f func(YoloChange)) {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.observers = append(y.observers, f)
}

// State is YOLO now. An expiry that has passed counts as off even if its
// timer has not run yet.
func (y *Yolo) State() YoloState {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.stateLocked()
}

func (y *Yolo) stateLocked() YoloState {
	switch {
	case y.session:
		return YoloState{Active: true, Session: true}
	case !y.until.IsZero() && y.now().Before(y.until):
		return YoloState{Active: true, Until: y.until}
	}
	return YoloState{}
}

// Active reports whether YOLO is on. It asks the clock, not the timer: if the
// expiry has passed and nobody has been told (the PC slept), the telling is
// scheduled.
func (y *Yolo) Active() bool {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.session {
		return true
	}
	if y.until.IsZero() {
		return false
	}
	if y.now().Before(y.until) {
		return true
	}
	if !y.late {
		y.late = true
		g := y.gen
		y.after(0, func() { y.expire(g) })
	}
	return false
}

// Enable turns YOLO on for d, which must be YoloShort or YoloLong. Turning it
// on again replaces the time.
func (y *Yolo) Enable(d time.Duration) error {
	if d != YoloShort && d != YoloLong {
		return ErrYoloDuration
	}
	y.change(func() (YoloChange, bool) {
		y.disarmLocked()
		y.until = y.now().Add(d).Round(0) // wall clock only: the PC may sleep
		g := y.gen
		y.stop = y.after(d, func() { y.expire(g) })
		return YoloChange{State: y.stateLocked(), Why: YoloOn}, true
	})
	return nil
}

// EnableSession turns YOLO on until serve stops.
func (y *Yolo) EnableSession() {
	y.change(func() (YoloChange, bool) {
		y.disarmLocked()
		y.session = true
		return YoloChange{State: y.stateLocked(), Why: YoloOn}, true
	})
}

// Off turns YOLO off, for why. Off while off tells nobody; off over an expiry
// nobody has been told yet tells that expiry.
func (y *Yolo) Off(why YoloWhy) {
	y.change(func() (YoloChange, bool) {
		set := y.session || !y.until.IsZero()
		was := y.stateLocked().Active
		y.disarmLocked()
		switch {
		case was:
			return YoloChange{Why: why}, true
		case set:
			return YoloChange{Why: YoloOffExpired}, true
		}
		return YoloChange{}, false
	})
}

// expire is the timer's: it switches YOLO off if g is still the generation it
// was armed for and the expiry has come.
func (y *Yolo) expire(g uint64) {
	y.change(func() (YoloChange, bool) {
		if g != y.gen || y.session || y.until.IsZero() {
			return YoloChange{}, false
		}
		if now := y.now(); now.Before(y.until) { // it ran early (the wall clock moved): wait for the rest
			y.stop = y.after(y.until.Sub(now), func() { y.expire(g) })
			return YoloChange{}, false
		}
		y.disarmLocked()
		return YoloChange{Why: YoloOffExpired}, true
	})
}

// disarmLocked clears the state and the timer. mu must be held.
func (y *Yolo) disarmLocked() {
	if y.stop != nil {
		y.stop()
		y.stop = nil
	}
	y.gen++
	y.until, y.session, y.late = time.Time{}, false, false
}

// change runs f under both locks and tells the observers if it changed
// something.
func (y *Yolo) change(f func() (YoloChange, bool)) {
	y.notifyMu.Lock()
	defer y.notifyMu.Unlock()
	y.mu.Lock()
	c, ok := f()
	obs := slices.Clone(y.observers)
	y.mu.Unlock()
	if !ok {
		return
	}
	for _, o := range obs {
		o(c)
	}
}

// YoloText is what the person reads when YOLO changes, in lang: the journal's
// description and serve's log line.
func YoloText(lang i18n.Lang, c YoloChange) string {
	switch c.Why {
	case YoloOn:
		if c.State.Session {
			return lang.T("yolo.event.on_session")
		}
		return lang.T("yolo.event.on_until", c.State.Until.Local().Format("15:04"))
	case YoloOffApp:
		return lang.T("yolo.event.off_app")
	case YoloOffCLI:
		return lang.T("yolo.event.off_cli")
	case YoloOffPause:
		return lang.T("yolo.event.off_pause")
	}
	return lang.T("yolo.event.off_expired")
}

// yoloEvent journals a change of YOLO. It is an event, not a change of the
// phone: green, nothing to undo.
func (d *Daemon) yoloEvent(c YoloChange) {
	now := d.o.Now().UTC()
	rule, code := "yolo.off", 0
	if c.Why == YoloOn {
		rule = "yolo.on"
	}
	d.record(journal.Entry{
		ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Serial: d.identity(), Source: journal.SourceUser,
		Argv: []string{"yolo", string(c.Why)}, Level: classify.Green.String(), Rule: rule,
		Description: YoloText(d.lang(), c), Status: journal.StatusOK, ExitCode: &code,
	})
}
