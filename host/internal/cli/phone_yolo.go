package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
)

// YOLO mode's part of the phone link (spec 2026-10-18-adbuddy-modo-yolo-design.md):
// yolo.set from the app and yolo.state to it. The state lives in daemon.Yolo;
// the daemon is created after the link, so serve gives it with useYolo.
//
// This is the only place that turns YOLO on, and only for a yolo.set that came
// by the app's authenticated channel (TestYoloIsOnlyTurnedOnByThePhoneLink).

// useYolo gives the link the daemon's YOLO state and makes the link tell the
// person (serve's log) and the app of every change.
func (l *phoneLink) useYolo(y *daemon.Yolo) {
	l.mu.Lock()
	l.yolo = y
	l.mu.Unlock()
	if l.conf != nil {
		l.conf.SetYoloActive(y.Active)
	}
	y.Subscribe(l.yoloChanged)
}

// yoloOf is the YOLO state, nil until serve gives it.
func (l *phoneLink) yoloOf() *daemon.Yolo {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.yolo
}

// toBridgeYolo is the state as the app sees it.
func toBridgeYolo(s daemon.YoloState) bridge.YoloState {
	out := bridge.YoloState{Active: s.Active, Session: s.Active && s.Session}
	if s.Active && !s.Session {
		out.Until = s.Until.UnixMilli()
	}
	return out
}

// yoloChanged tells serve's log and the app that YOLO changed (an event). It
// runs under the observers' lock of daemon.Yolo: it never turns YOLO on or off. l.srv, l.log and l.lang are set
// by startPhoneLink before useYolo and never change after, so they are read without l.mu.
func (l *phoneLink) yoloChanged(c daemon.YoloChange) {
	if l.log != nil {
		fmt.Fprintln(l.log, "⚠ "+daemon.YoloText(l.lang, c))
	}
	if l.srv != nil {
		_ = l.srv.Publish(bridge.TypeYoloState, toBridgeYolo(c.State))
	}
	// Turning YOLO on: what was granted from here on is dropped when it ends, and
	// the cards pending are approved by YOLO itself (phone_perm.go). The daemon's
	// own observer (daemon.New, subscribed before this one) has already withdrawn the
	// pending 🔴 question.
	// Lock order: all under Yolo.notifyMu; Confirmer.mu and adapter.mu are taken
	// one at a time and never while calling Enable, EnableSession or Off.
	if c.State.Active {
		if l.conf != nil {
			l.conf.YoloStarted()
		}
		l.yoloResolvePerms()
	} else if l.conf != nil {
		l.conf.YoloEnded()
	}
}

// yoloArea is YOLO's part of the phone link (phone_areas.go).
func (l *phoneLink) yoloArea() area {
	return area{
		handlers: map[string]func(bridge.Message){bridge.TypeYoloSet: l.handleYoloSet},
		snapshot: func(s *bridge.Snapshot) {
			if y := l.yoloOf(); y != nil {
				s.Yolo = toBridgeYolo(y.State())
			}
		},
	}
}

// handleYoloSet turns YOLO on or off at the app's request. It comes from the
// current authenticated connection only (the bridge's token), and every
// request is answered: with yolo.state carrying its rid, or an error.
func (l *phoneLink) handleYoloSet(m bridge.Message) {
	srv := l.srv
	if srv == nil || !srv.Current(m) { // no app, or a connection already replaced
		return
	}
	if m.RID == "" {
		// A request by protocol: with no rid there is nobody to answer.
		fmt.Fprintln(l.log, l.lang.T("phone.yolo_set_without_rid"))
		return
	}
	fail := func(code, text string) {
		_ = srv.Reply(m, bridge.TypeError, bridge.ErrorReply{Code: code, Text: text})
	}
	y := l.yoloOf()
	if y == nil {
		fail(bridge.CodeUnavailable, "YOLO mode is not available on this host.")
		return
	}
	var req bridge.YoloSet
	if err := json.Unmarshal(m.Payload, &req); err != nil || !exactlyOne(req) {
		fail(bridge.CodeInvalid, `yolo.set needs {"minutes": 15 or 60}, {"session": true} or {"off": true}, only one of them.`)
		return
	}
	switch {
	case req.Off:
		y.Off(daemon.YoloOffApp)
	case l.Paused():
		// The pause would turn it off at the next command: say so instead of a mode that is off already.
		fail(bridge.CodeFailed, "The agent is paused: resume it before turning YOLO on.")
		return
	case req.Session:
		y.EnableSession()
	default:
		var d time.Duration
		switch req.Minutes {
		case 15:
			d = daemon.YoloShort
		case 60:
			d = daemon.YoloLong
		}
		if err := y.Enable(d); err != nil { // any other number
			fail(bridge.CodeInvalid, err.Error())
			return
		}
	}
	_ = srv.PublishRID(bridge.TypeYoloState, m.RID, toBridgeYolo(y.State()))
}

// exactlyOne reports whether req asks for one thing: a number of minutes, the
// session, or off.
func exactlyOne(req bridge.YoloSet) bool {
	n := 0
	if req.Minutes != 0 {
		n++
	}
	if req.Session {
		n++
	}
	if req.Off {
		n++
	}
	return n == 1
}
