package bridge

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

const yoloKey = "rm\x00/work\x00shell\x00rm\x00/sdcard/x"

// askSession asks one 🔴 on the phone and answers allow_session; it reports
// the decision.
func askSession(t *testing.T, c *Confirmer, app *fakeApp, id string) confirm.Decision {
	t.Helper()
	app.mu.Lock()
	app.onPublish = func(r ConfirmRequest) { reply(c, r.ID, "allow_session") }
	app.mu.Unlock()
	req := confirm.Request{ID: id, Title: "Delete", Key: yoloKey, ExpiresAt: time.Now().Add(time.Minute)}
	return confirmWithin(t, c, req)
}

func sessionKeys(c *Confirmer) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.session)
}

// While YOLO is on, an allow_session is a single allow and keeps nothing: a
// tap injected on a question that was waiting when YOLO began would otherwise
// leave a permit that outlives YOLO. Without YOLO it is kept (M2's behaviour).
// Mutation: remove `&& !(c.yoloActive != nil && c.yoloActive())` from Reply.
func TestAllowSessionWhileYoloIsActiveKeepsNothing(t *testing.T) {
	for _, yolo := range []bool{false, true} {
		app := &fakeApp{connected: true}
		c := NewConfirmer(app, fixed(confirm.Deny))
		var on atomic.Bool
		c.SetYoloActive(on.Load)
		on.Store(yolo)
		if got := askSession(t, c, app, "j_1"); got != confirm.Allow {
			t.Fatalf("yolo=%v: got %v, want a single allow", yolo, got)
		}
		if n := sessionKeys(c); n != map[bool]int{false: 1, true: 0}[yolo] {
			t.Fatalf("yolo=%v: %d session permits", yolo, n)
		}
		on.Store(false)
		// The same question asked again: with YOLO off it asks unless a permit was kept.
		app.mu.Lock()
		before := len(app.published)
		app.mu.Unlock()
		got := askSession(t, c, app, "j_2")
		app.mu.Lock()
		asked := len(app.published) > before
		app.mu.Unlock()
		if got != confirm.Allow || asked != yolo {
			t.Fatalf("yolo=%v: second question asked=%v got %v", yolo, asked, got)
		}
	}
}

// grant makes a session permit the way Reply does and returns its key.
func grant(t *testing.T, c *Confirmer, app *fakeApp, id string, key string) {
	t.Helper()
	app.mu.Lock()
	app.onPublish = func(r ConfirmRequest) { reply(c, r.ID, "allow_session") }
	app.mu.Unlock()
	req := confirm.Request{ID: id, Title: "t", Key: key, ExpiresAt: time.Now().Add(time.Minute)}
	if got := confirmWithin(t, c, req); got != confirm.Allow {
		t.Fatalf("%s: %v", id, got)
	}
}

func hasPermit(c *Confirmer, key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.session[key]
	return ok
}

// What was granted since YOLO began dies with it; what was granted before
// stays. The observer lags the state (Reply sees YOLO off, the mark is already
// put), which is when a grant can still be made: it is numbered above the mark.
// Mutations: (1) remove the deletion in YoloEnded; (2) move the mark in every
// YoloStarted (drop the `if !c.yoloOn`).
func TestYoloEndedDropsWhatWasGrantedSinceItBegan(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Deny))
	grant(t, c, app, "j_1", "before")
	c.YoloStarted()
	grant(t, c, app, "j_2", "during-1")
	c.YoloStarted() // 15 min renewed to 1 h: the mark stays
	grant(t, c, app, "j_3", "during-2")
	c.YoloEnded()
	if !hasPermit(c, "before") {
		t.Error("a permit from before YOLO was dropped")
	}
	if hasPermit(c, "during-1") || hasPermit(c, "during-2") {
		t.Errorf("permits made during YOLO survive it: %v", c.session)
	}
	// A second run starts from a new mark and spares what is left.
	c.YoloStarted()
	grant(t, c, app, "j_4", "run-2")
	c.YoloEnded()
	if !hasPermit(c, "before") || hasPermit(c, "run-2") {
		t.Errorf("second run: %v", c.session)
	}
	c.YoloEnded() // no run open: nothing
	if !hasPermit(c, "before") {
		t.Error("an end with no start dropped a permit")
	}
}

// The permit that outlived YOLO is the one that matters: after YOLO a 🔴 whose
// permit was made while it was on asks again.
func TestAPermitMadeDuringYoloIsGoneForTheNextQuestion(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Deny))
	c.YoloStarted()
	grant(t, c, app, "j_1", yoloKey)
	c.YoloEnded()
	app.mu.Lock()
	before := len(app.published)
	app.mu.Unlock()
	app.mu.Lock()
	app.onPublish = func(r ConfirmRequest) { reply(c, r.ID, "deny") }
	app.mu.Unlock()
	req := confirm.Request{ID: "j_2", Key: yoloKey, ExpiresAt: time.Now().Add(time.Minute)}
	if got := confirmWithin(t, c, req); got != confirm.Deny {
		t.Fatalf("got %v", got)
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if len(app.published) != before+1 {
		t.Fatal("the question was not asked: the permit survived YOLO")
	}
}
