package daemon

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// Settling: after an act, or on request, the screen is read again until two
// reads in a row are the same, so the agent sees what its tap did without
// guessing a sleep. Variables for the tests (settle copies them on entry).
var (
	settleFirst = 150 * time.Millisecond // after an act, before the first read: the effect starts
	settleGap   = 120 * time.Millisecond // between two reads
	settleMin   = 400 * time.Millisecond // two equal reads count as settled only this long after the act or the request: a slow transition shows the old screen twice
	settleMax   = 2 * time.Second        // all in all, counted from the act or from the request
	settleReads = 8                      // reads at most
)

// ScreenSettled is dump's data when the client asked to wait for the screen
// to settle. Settled is false when the screen was still changing at the end
// of the wait (an animation, a video).
type ScreenSettled struct {
	bridge.UIDump
	Settled bool `json:"settled"`
}

// ClickedScreen is click's data with the screen after it. Screen is nil, and
// Note says why, when it could not be read (the guard blocked it: the tap may
// have opened a protected app).
type ClickedScreen struct {
	bridge.UIClicked
	Screen *ScreenSettled `json:"screen,omitempty"`
	Note   string         `json:"screenNote,omitempty"`
}

// TextSetScreen is setText's data with the screen after it.
type TextSetScreen struct {
	bridge.UITextSet
	Screen *ScreenSettled `json:"screen,omitempty"`
	Note   string         `json:"screenNote,omitempty"`
}

// asDump returns the dump an app or adb answered.
func asDump(v any) (bridge.UIDump, bool) {
	switch d := v.(type) {
	case *bridge.UIDump:
		if d == nil {
			return bridge.UIDump{}, false
		}
		return *d, true
	case bridge.UIDump:
		return d, true
	}
	return bridge.UIDump{}, false
}

// sleepCtx waits d, or until ctx ends (false).
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// settle reads the screen until two reads in a row are equal and settleMin
// has passed since start, at most settleReads reads and until start+settleMax.
// first is a read already made (nil after an act: the first read waits
// settleFirst). Every read goes through the doors and the guard like any other
// read of the screen: paused, a 🔴 question waiting, and guardForeground (a
// protected app, ADBuddy itself, the shade, a notification...). If one of them
// says no, there is no screen: why says it. why and f are both empty when the
// context ended (the agent or serve went away). It fails closed: nothing it
// could not read is returned. via says how the last read was made.
func (d *Daemon) settle(ctx context.Context, args json.RawMessage, first *bridge.UIDump, start time.Time) (screen *ScreenSettled, via, why string, f *uiFailure) {
	firstWait, gap, min, max, maxReads := settleFirst, settleGap, settleMin, settleMax, settleReads
	prev := first
	deadline := start.Add(max)
	reads := 0
	for {
		wait := gap
		if prev == nil {
			wait = firstWait
		}
		if !sleepCtx(ctx, wait) {
			return nil, "", "", nil
		}
		if d.o.Paused() {
			return nil, "", PausedMessage, nil
		}
		release, other := d.admit("Read the screen", false)
		if other != "" {
			return nil, "", other, nil
		}
		release()
		if why := d.guardForeground(ctx, nil); why != "" {
			if ctx.Err() != nil {
				return nil, "", "", nil
			}
			return nil, "", why, nil
		}
		if d.o.Paused() { // paused while the guard looked
			return nil, "", PausedMessage, nil
		}
		v, how, uf := d.runUI(ctx, bridge.UIOpDump, args, nil)
		if uf != nil {
			return nil, "", "", uf
		}
		cur, ok := asDump(v)
		if !ok {
			return nil, "", "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
		}
		via = how
		reads++
		now := time.Now()
		if prev != nil && reflect.DeepEqual(*prev, cur) && now.Sub(start) >= min {
			return &ScreenSettled{UIDump: cur, Settled: true}, via, "", nil
		}
		prev = &cur
		if reads >= maxReads || !now.Before(deadline) {
			return &ScreenSettled{UIDump: cur, Settled: false}, via, "", nil
		}
	}
}

// notDone goes first in every note about a screen that could not be read
// after an act: the act happened, and repeating it would do it twice.
const notDone = "The action was done; do not repeat it. The screen could not be read after it: "

// withScreen adds the screen after an act (click, setText) to its result and
// journals the read as the entry of an `adbuddy screen`: a green one, or a
// blocked one when the guard said no. The act itself already happened and is
// reported as such: a block only means there is no screen to show.
func (d *Daemon) withScreen(ctx context.Context, req ipc.Request, data any) any {
	scr, via, why, f := d.settle(ctx, json.RawMessage("{}"), nil, time.Now())
	now := d.o.Now().UTC()
	h := helpers[bridge.UIOpDump]
	e := journal.Entry{
		ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Serial: d.identity(), Source: journal.SourceUser,
		Argv: helperArgv(bridge.UIOpDump, nil), Level: h.level.String(), Rule: h.rule, Description: h.title.In(d.o.Lang),
	}
	if req.Env["ADBUDDY_SOURCE"] == journal.SourceAgent {
		e.Source = journal.SourceAgent
	}
	note := ""
	code := 0
	switch {
	case f != nil && f.exit == ExitDenied: // the app's own check (a protected screen, FLAG_SECURE, a notification)
		e.Status, e.Reason, code = journal.StatusBlocked, f.msg, ExitDenied
		note = notDone + "Blocked by ADBuddy: " + classify.Printable(f.msg)
	case f != nil:
		e.Status, e.Reason, code = journal.StatusError, f.msg, f.exit
		note = notDone + classify.Printable(f.msg)
	case why != "":
		e.Status, e.Reason, code = journal.StatusBlocked, why, ExitDenied
		note = notDone + "Blocked by ADBuddy: " + classify.Printable(why)
	case scr == nil: // the agent or serve went away
		e.Status, e.Reason, code = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it ran", "abandoned: ADBuddy stopped before it ran"), ExitDenied
		note = notDone + "stopped"
	default:
		e.Status, e.Reason = journal.StatusOK, "after "+req.UIOp+", via "+via+", settled="+strconv.FormatBool(scr.Settled)
	}
	e.ExitCode = &code
	d.record(e)
	switch v := data.(type) {
	case *bridge.UIClicked:
		return ClickedScreen{UIClicked: *v, Screen: scr, Note: note}
	case bridge.UIClicked:
		return ClickedScreen{UIClicked: v, Screen: scr, Note: note}
	case *bridge.UITextSet:
		return TextSetScreen{UITextSet: *v, Screen: scr, Note: note}
	case bridge.UITextSet:
		return TextSetScreen{UITextSet: v, Screen: scr, Note: note}
	}
	return data
}
