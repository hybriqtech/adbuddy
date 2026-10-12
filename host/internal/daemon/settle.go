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

// Settling: after an act, or on request, the screen is read again until it
// has changed from the screen it started from and stayed the same for a
// while, so the agent sees what its tap did without guessing a sleep.
// Variables for the tests (settle copies them on entry).
var (
	settleFirst     = 150 * time.Millisecond // after an act, before the first read: the effect starts
	settleGap       = 120 * time.Millisecond // between two attempts (a read or a block)
	settleQuiet     = 600 * time.Millisecond // after a change, the screen the same at least this long counts as settled: above the longest stable gap measured inside a real transition (max 495-512 ms in the emulator, median 344 ms), so a half-drawn screen is not taken as settled
	settleIdle      = 2 * time.Second        // with no change at all (or the start unknown), the wait ends after this: screen --settle on a still screen takes about this
	settleBlockWait = 2 * time.Second        // a block of the guard is tried again until this (a transition shows nothing for a moment)
	settleMax       = 3 * time.Second        // all in all, counted from the act or from the request
	settleReads     = 40                     // attempts at most: reads or blocks
	baselineWait    = time.Second            // the read of the start, before the act, waits at most this: tap --screen takes up to this plus about settleMax
	baselineSlack   = 150 * time.Millisecond // if the start took longer, the guard is asked again before the act
)

// ScreenSettled is dump's data when the client asked to wait for the screen
// to settle. Settled is false when the screen was still changing at the end
// of the wait (an animation, a video), or when the screen it started from had
// an application window and all that it showed was the system's own windows
// (see bareApp). Changed says whether the screen that comes out is not the
// one it started from: false with Settled means the screen is the start
// again (nothing happened on screen, or it changed and came back); nil means it is not known (the start could not be read, or
// this read came another way than the start: the app and uiautomator differ
// in format, so they are not compared).
type ScreenSettled struct {
	bridge.UIDump
	Settled bool  `json:"settled"`
	Changed *bool `json:"changed,omitempty"`
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

// sameScreen reports whether two reads show the same screen: everything but
// which element has the focus, with no elements and none being the same. A
// change of focus is not the transition a tap causes; a switch that turned
// (Checked) is.
func sameScreen(a, b bridge.UIDump) bool {
	a.Elements, b.Elements = noFocus(a.Elements), noFocus(b.Elements)
	return reflect.DeepEqual(a, b)
}

// noFocus is a copy of es without the focus flag; nil and empty are the same.
func noFocus(es []bridge.UIElement) []bridge.UIElement {
	out := make([]bridge.UIElement, len(es))
	for i, e := range es {
		e.Focused = false
		out[i] = e
	}
	return out
}

// bareApp reports whether d, which the app read (how), shows nothing of any
// application window while the phone is unlocked: only the system's own
// windows, or no element at all. A transition does that, while the window the
// tap opens (or the one it leaves) has no content to read yet; the longer the
// transition, the longer it lasts, and it can outlast settleQuiet. It is not
// a screen the agent can use, so when the start had an application window
// (settle checks that, not bareApp) it never counts as settled: the wait goes
// on until the cap, which gives it back with Settled false. Only the app's
// reads say which window each element is in (a read made by uiautomator marks
// every element "app"): a read made another way is never taken as bare.
func bareApp(d bridge.UIDump, how string) bool {
	if how != viaApp || d.Locked {
		return false
	}
	for _, e := range d.Elements {
		if e.Window == "app" || e.Window == "dialog" {
			return false
		}
	}
	return true
}

// settleIn is what settle starts from.
type settleIn struct {
	args    json.RawMessage
	base    *bridge.UIDump // the screen the wait starts from; nil: unknown
	baseVia string         // how base was read: a read made another way is not compared with it
	first   *bridge.UIDump // a read already made (and guarded); nil: none
	via     string         // how first was read
	start   time.Time
	adopt   bool // base is nil: the first valid read becomes it
}

// settle reads the screen until it has changed from in.base and stayed the
// same for settleQuiet, or, if it never changes, until settleIdle has passed
// since in.start; at most until in.start+settleMax and settleReads attempts.
// Without a base (the start could not be read) it is not known whether the
// screen changed: only settleIdle ends the wait, and Changed is nil, never
// false. A read made another way than the base (uiautomator after an app
// that stopped answering) is not compared with it either: it does not count
// as a change, so it can only end the wait at settleIdle, and if it comes out
// its Changed is nil. With in.adopt the first valid read is the base (read
// the way it was) and does not come out. The first read of screen --settle
// (in.first, the base too) does not come out either, unless the cap comes
// with no newer read: then it does, with Settled false (it passed the guard
// before it was read). Changed, when known, is about the read that comes out
// (a screen that changed and came back says false); the wait itself ends
// early only after some read differed from the base.
//
// Every attempt goes through the doors and the guard like any other read of
// the screen: paused, a 🔴 question waiting, and guardForeground (a protected
// app, ADBuddy itself, the shade, a notification...). A pause or a question
// ends the wait at once: why says it. A block of the guard, or of the app's
// own check (ExitDenied: a transition shows a window with no content, which
// the app calls protected), is tried again after settleGap until
// settleBlockWait since in.start; then why says it. Only a read made after
// the guard said yes comes out, and if the last attempt was a block nothing
// read before comes out. why and f are both empty when the context ended (the
// agent or serve went away). It fails closed: nothing it could not read is
// returned. A read of the app that shows no application window (bareApp: a
// transition that has not drawn the window yet), when the start had one, is
// never quiet, so it does not settle. via says how the read that comes out
// was made. No goroutines.
func (d *Daemon) settle(ctx context.Context, in settleIn) (screen *ScreenSettled, via, why string, f *uiFailure) {
	firstWait, gap, quiet, idle, blockWait, max, maxTries := settleFirst, settleGap, settleQuiet, settleIdle, settleBlockWait, settleMax, settleReads
	base, baseVia, cur, via := in.base, in.baseVia, in.first, in.via
	deadline := in.start.Add(max)
	// A bare read only holds the wait back when the start was not bare: a tap that
	// leaves a screen with an application window for one without (a transition
	// that has not drawn its window). From a start that was bare already (a game,
	// the camera) or unknown, it is the screen there is, as before.
	protectBare := in.base != nil && !bareApp(*in.base, in.baseVia)
	changed := false     // some read made the way base was differed from it
	lastMove := in.start // when the screen last moved (a read differed from the one before, or first from base)
	blocked := ""
	// vsBase is Changed for r, read via how: nil when there is no base or r
	// was read another way than base (a difference of format is no change).
	vsBase := func(r bridge.UIDump, how string) *bool {
		if base == nil || how != baseVia {
			return nil
		}
		c := !sameScreen(*base, r)
		return &c
	}
	if in.first != nil {
		if c := vsBase(*in.first, in.via); c != nil && *c {
			changed = true
		}
	}
	for tries := 0; ; tries++ {
		wait := gap
		if cur == nil && tries == 0 {
			wait = firstWait
		}
		// The cap: no new attempt starts past the deadline or the tries. If the
		// last attempt was a block nothing read before is returned; else what the
		// last read said (it passed the guard) is what comes out. With nothing
		// read and nothing blocked (a deadline shorter than the first wait), read once.
		if tries >= maxTries || time.Now().Add(wait).After(deadline) {
			switch {
			case blocked != "":
				return nil, "", blocked, nil
			case cur != nil:
				return &ScreenSettled{UIDump: *cur, Settled: false, Changed: vsBase(*cur, via)}, via, "", nil
			}
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
			blocked = why
			if time.Since(in.start) >= blockWait {
				return nil, "", blocked, nil // the block lasts: no screen
			}
			continue // maybe a transition: try again
		}
		blocked = ""
		if d.o.Paused() { // paused while the guard looked
			return nil, "", PausedMessage, nil
		}
		v, how, uf := d.runUI(ctx, bridge.UIOpDump, in.args, nil, time.Time{})
		if uf != nil {
			if ctx.Err() != nil { // the agent or serve went away during the read
				return nil, "", "", nil
			}
			if uf.exit == ExitDenied { // the app's own check: a transition or a protected screen
				blocked = uf.msg
				if time.Since(in.start) >= blockWait {
					return nil, "", blocked, nil
				}
				continue
			}
			return nil, "", "", uf
		}
		read, ok := asDump(v)
		if !ok {
			return nil, "", "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
		}
		via = how
		now := time.Now()
		if base == nil && in.adopt {
			base, baseVia = &read, how // the first valid read is where it started: it does not come out
			cur = &read
			lastMove = now
			continue
		}
		if cur != nil && !sameScreen(*cur, read) {
			lastMove = now
		}
		if c := vsBase(read, how); c != nil && *c {
			if !changed {
				lastMove = now // the first change counts as a move
			}
			changed = true
		}
		cur = &read
		still := now.Sub(lastMove) >= quiet && !(protectBare && bareApp(read, how))
		switch {
		case changed && still: // it moved and has been quiet
			return &ScreenSettled{UIDump: read, Settled: true, Changed: vsBase(read, how)}, via, "", nil
		case still && now.Sub(in.start) >= idle: // it never moved (or the start is unknown): the idle wait is over
			return &ScreenSettled{UIDump: read, Settled: true, Changed: vsBase(read, how)}, via, "", nil
		}
	}
}

// baselineScreen reads the screen the wait of --screen starts from, the one
// before the act: only from the app, with a short limit (baselineWait). The
// read is only compared: it is never returned, written or journaled. nil when
// it cannot be read (no app, no answer in time, an error); then settle waits
// settleIdle and says nothing about a change. Without the app it does not
// fall back to uiautomator: that takes about 2 s, more than the limit, and
// would leave a dump running on the phone next to settle's. denied is set
// when the app itself says something protected is in front: then the act
// must not happen.
func (d *Daemon) baselineScreen(ctx context.Context) (*bridge.UIDump, *uiFailure) {
	bctx, cancel := context.WithTimeout(ctx, baselineWait)
	defer cancel()
	res, err := d.callApp(bctx, bridge.UIOpDump, json.RawMessage("{}")) // read viaApp
	if err != nil {
		if uf := appFailure(bctx, err); uf.exit == ExitDenied {
			return nil, uf
		}
		return nil, nil
	}
	v, err := checkResult(bridge.UIOpDump, res)
	if err != nil {
		return nil, nil
	}
	if dump, ok := asDump(v); ok {
		return &dump, nil
	}
	return nil, nil
}

// viaApp is how runUI says a read came from the app (the start of --screen
// always does).
const viaApp = "the app"

// changedWord is Changed for the journal.
func changedWord(c *bool) string {
	if c == nil {
		return "unknown"
	}
	return strconv.FormatBool(*c)
}

// notDone goes first in every note about a screen that could not be read
// after an act: the act happened, and repeating it would do it twice.
const notDone = "The action was done; do not repeat it. The screen could not be read after it: "

// withScreen adds the screen after an act (click, setText) to its result and
// journals the read as the entry of an `adbuddy screen`: a green one (with
// settled= and changed=), or a blocked one when the guard said no. base is
// the screen before the act (nil: unknown). The act itself already happened and is
// reported as such: a block only means there is no screen to show.
func (d *Daemon) withScreen(ctx context.Context, req ipc.Request, data any, base *bridge.UIDump) any {
	scr, via, why, f := d.settle(ctx, settleIn{args: json.RawMessage("{}"), base: base, baseVia: viaApp, start: time.Now()})
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
		e.Status, e.Reason = journal.StatusOK, "after "+req.UIOp+", via "+via+", settled="+strconv.FormatBool(scr.Settled)+", changed="+changedWord(scr.Changed)
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
