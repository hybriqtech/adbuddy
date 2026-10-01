package cli

import (
	"fmt"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// area is what one milestone adds to the phone link (protocol spec §10). Each
// one builds its own in its own file (phone_chat.go, phone_agent.go,
// phone_ui.go), so lines of work in parallel do not edit the same code. A nil
// function does nothing.
type area struct {
	// handlers handle the app's message types; no two areas share one.
	handlers map[string]func(bridge.Message)
	// snapshot fills the area's part of a snapshot. It is called with the
	// server's lock held: no Publish, Send, Reply nor Connected.
	snapshot func(*bridge.Snapshot)
	// attach: an app authenticated, new or replacing another. What the area
	// keeps per connection (a terminal subscription, uploads) starts over.
	// A message of the replaced connection may still reach a handler after
	// this: a handler that keeps state per connection checks
	// l.srv.Current(m) first.
	attach func()
	// gone: no app is connected any more.
	gone func()
	// frame receives a binary frame from the app (uploads, M3).
	frame func(id bridge.FrameID, offset int64, data []byte)
}

// setAreas keeps the areas and merges their handlers. A type in two areas is
// a programming error.
func (l *phoneLink) setAreas(areas ...area) {
	l.areas = areas
	l.handlers = map[string]func(bridge.Message){}
	for _, a := range areas {
		for typ, h := range a.handlers {
			if _, dup := l.handlers[typ]; dup {
				panic(fmt.Sprintf("phone link: %s is handled by two areas", typ))
			}
			l.handlers[typ] = h
		}
	}
}

// onFrame hands a binary frame from the app to the areas.
func (l *phoneLink) onFrame(id bridge.FrameID, offset int64, data []byte) {
	for _, a := range l.areas {
		if a.frame != nil {
			a.frame(id, offset, data)
		}
	}
}

// onAttach starts the state kept per connection over.
func (l *phoneLink) onAttach() {
	l.mu.Lock()
	l.noticed = nil
	l.mu.Unlock()
	for _, a := range l.areas {
		if a.attach != nil {
			a.attach()
		}
	}
}

// onGone tells the areas that no app is connected.
func (l *phoneLink) onGone() {
	for _, a := range l.areas {
		if a.gone != nil {
			a.gone()
		}
	}
}

// confirmArea is M2's: 🔴 confirmations and the pause.
func (l *phoneLink) confirmArea() area {
	return area{handlers: map[string]func(bridge.Message){
		bridge.TypeConfirmReply: func(m bridge.Message) { l.conf.Reply(m.Payload) },
		bridge.TypePause:        l.onPauseMessage,
		bridge.TypeResume:       l.onPauseMessage,
	}}
}
