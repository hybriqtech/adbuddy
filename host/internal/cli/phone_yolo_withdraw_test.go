package cli

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
)

// Task 7b: the permits of a session and the cards of Claude Code do not
// outlive YOLO.

// askRed asks a 🔴 with a key (so the app may offer "allow during the
// session") through the link's Confirmer, in the background.
func askRed(r *agentRig, id string) <-chan confirm.Decision {
	out := make(chan confirm.Decision, 1)
	go func() {
		out <- r.l.Confirmer().Confirm(context.Background(), confirm.Request{
			ID: id, Title: "Delete", Key: "rm\x00/work\x00shell\x00rm\x00/sdcard/x", ExpiresAt: time.Now().Add(time.Minute),
		})
	}()
	return out
}

func (r *agentRig) confirmRequest(t *testing.T) bridge.ConfirmRequest {
	t.Helper()
	var q bridge.ConfirmRequest
	if err := json.Unmarshal(r.readType(t, bridge.TypeConfirmReq).Payload, &q); err != nil {
		t.Fatal(err)
	}
	return q
}

func decisionOf(t *testing.T, ch <-chan confirm.Decision) confirm.Decision {
	t.Helper()
	select {
	case d := <-ch:
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("the question did not end")
	}
	return confirm.Deny
}

// serve gives the Confirmer the YOLO state (useYolo): an allow_session during
// YOLO is a single allow. Without YOLO it is kept, as always.
// Mutation: remove `l.conf.SetYoloActive(y.Active)` from useYolo (the permit is
// then kept while YOLO is on, and asked for again only after it ends: so the
// second question is made with YOLO still on).
func TestYoloMakesAllowSessionASingleAllow(t *testing.T) {
	for _, yolo := range []bool{false, true} {
		r := newAgentRig(t)
		y := daemon.NewYolo(nil, nil)
		r.l.useYolo(y)
		if yolo {
			y.EnableSession()
		}
		first := askRed(r, "j_1")
		q := r.confirmRequest(t)
		if !q.AllowSession {
			t.Fatal("allow_session was not offered")
		}
		sendApp(t, r.c, bridge.TypeConfirmReply, "", bridge.ConfirmReply{ID: q.ID, Decision: "allow_session"})
		if d := decisionOf(t, first); d != confirm.Allow {
			t.Fatalf("yolo=%v: %v", yolo, d)
		}
		// Still on if yolo: the Confirmer itself must not have kept the permit.
		second := askRed(r, "j_2")
		if !yolo { // kept: no question
			if d := decisionOf(t, second); d != confirm.Allow {
				t.Fatalf("without YOLO the permit was not kept: %v", d)
			}
			continue
		}
		q = r.confirmRequest(t) // asked again: nothing was kept
		sendApp(t, r.c, bridge.TypeConfirmReply, "", bridge.ConfirmReply{ID: q.ID, Decision: "deny"})
		if d := decisionOf(t, second); d != confirm.Deny {
			t.Fatalf("%v", d)
		}
	}
}

// Every way YOLO ends drops the permits made since it began (app, CLI,
// expiry, pause). The permit is made while the Confirmer still thinks YOLO is
// off (SetYoloActive false: the window between the state and the observer),
// which is when one can be made at all. Mutation: remove `l.conf.YoloEnded()`
// from yoloChanged.
func TestEveryWayYoloEndsDropsThePermitsMadeSinceItBegan(t *testing.T) {
	type way struct {
		name string
		off  func(t *testing.T, r *agentRig, y *daemon.Yolo, expire func())
	}
	ways := []way{
		{"app", func(t *testing.T, r *agentRig, y *daemon.Yolo, _ func()) {
			sendApp(t, r.c, bridge.TypeYoloSet, "r_off", bridge.YoloSet{Off: true})
			for range 40 { // the answer comes after the observers ran
				if m := readApp(t, r.c); m.Type == bridge.TypeYoloState && m.RID == "r_off" {
					return
				}
			}
			t.Fatal("no answer")
		}},
		{"cli", func(t *testing.T, r *agentRig, y *daemon.Yolo, _ func()) { y.Off(daemon.YoloOffCLI) }},
		{"expiry", func(t *testing.T, r *agentRig, y *daemon.Yolo, expire func()) { expire() }},
		{"pause", func(t *testing.T, r *agentRig, y *daemon.Yolo, _ func()) {
			sendApp(t, r.c, bridge.TypePause, "", bridge.Empty{})
			r.readType(t, bridge.TypeSessionState)
		}},
	}
	for _, w := range ways {
		t.Run(w.name, func(t *testing.T) {
			r := newAgentRig(t)
			now := time.Now()
			var fire func()
			y := daemon.NewYolo(func() time.Time { return now }, func(d time.Duration, f func()) func() { fire = f; return func() {} })
			r.l.useYolo(y)
			r.l.conf.SetYoloActive(func() bool { return false })
			if err := y.Enable(daemon.YoloShort); err != nil {
				t.Fatal(err)
			}
			first := askRed(r, "j_1")
			q := r.confirmRequest(t)
			sendApp(t, r.c, bridge.TypeConfirmReply, "", bridge.ConfirmReply{ID: q.ID, Decision: "allow_session"})
			if d := decisionOf(t, first); d != confirm.Allow {
				t.Fatalf("%v", d)
			}
			w.off(t, r, y, func() { now = now.Add(15 * time.Minute); fire() })
			if y.Active() {
				t.Fatal("still on")
			}
			second := askRed(r, "j_2")
			q = r.confirmRequest(t) // asked again: the permit died with YOLO
			sendApp(t, r.c, bridge.TypeConfirmReply, "", bridge.ConfirmReply{ID: q.ID, Decision: "deny"})
			if d := decisionOf(t, second); d != confirm.Deny {
				t.Fatalf("%v", d)
			}
		})
	}
}

// A card of Claude Code that was pending when YOLO began is approved at once,
// "allow" with no rule, by yolo, and is gone: a tap injected on it could not
// leave a permanent rule. Without YOLO the same card offers and takes
// "always". Mutation: remove `l.yoloResolvePerms()` from yoloChanged.
func TestYoloApprovesThePendingCardsWithNoRule(t *testing.T) {
	r := newAgentRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.useYolo(y)
	// Without YOLO: "always" adds the rule.
	req, out := r.ask(t, "touch uno.txt")
	if !req.AllowAlways {
		t.Fatal("always was not offered")
	}
	r.answer(t, req.ID, "allow_always")
	if o := parsePerm(t, wait(t, out)); o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 1 {
		t.Fatalf("without YOLO: %+v", o)
	}
	// With YOLO turned on while it waits.
	req, out = r.ask(t, "touch dos.txt")
	y.EnableSession()
	pr := r.resolved(t)
	if pr.ID != req.ID || pr.By != "yolo" || pr.Decision != "allow" || pr.Summary != "do it" {
		t.Fatalf("resolved %+v", pr)
	}
	if o := parsePerm(t, wait(t, out)); o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 0 {
		t.Fatalf("with YOLO: %+v", o)
	}
	if s := r.l.snapshot(); len(s.Permissions) != 0 {
		t.Fatalf("a card is still pending: %+v", s.Permissions)
	}
	// The late answer, from the app or injected, is stale.
	r.answer(t, req.ID, "allow_always")
	if n := r.readType(t, bridge.TypeNotice); n.Type != bridge.TypeNotice {
		t.Fatalf("%+v", n)
	}
}

// "Always" that comes while YOLO is on (a race: the card was still pending)
// is a plain "allow". Here the link has YOLO's state but no observer, so the
// card stays pending. Mutation: remove `|| yoloOn` from onPermReply.
func TestAlwaysDuringYoloIsAPlainAllow(t *testing.T) {
	r := newAgentRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.mu.Lock()
	r.l.yolo = y // no Subscribe: nothing resolves the card
	r.l.mu.Unlock()
	req, out := r.ask(t, "touch uno.txt")
	y.EnableSession()
	r.answer(t, req.ID, "allow_always")
	pr := r.resolved(t)
	if pr.Decision != "allow" || pr.By != "phone" {
		t.Fatalf("resolved %+v", pr)
	}
	if o := parsePerm(t, wait(t, out)); o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 0 {
		t.Fatalf("%+v", o)
	}
}
