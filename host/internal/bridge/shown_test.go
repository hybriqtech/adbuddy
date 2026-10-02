package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

func hostileReq() confirm.Request {
	return confirm.Request{ID: "j_9", Title: "מחק א", Detail: "שורה א\nسطر ٣", RawCommand: "adb shell mv א ب",
		ExpiresAt: time.Now().Add(time.Minute), Key: "k"}
}

func rtlIn(s string) bool { return strings.IndexFunc(s, classify.RTL) >= 0 }

func TestAppQuestionShowsNoRTL(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Deny))
	c.RTLNote = "NOTE: right to left"
	asked := make(chan ConfirmRequest, 1)
	app.onPublish = func(r ConfirmRequest) { asked <- r }
	done := make(chan confirm.Decision, 1)
	go func() { done <- c.Confirm(context.Background(), hostileReq()) }()
	got := <-asked

	for name, s := range map[string]string{"title": got.Title, "detail": got.Detail, "raw": got.RawCommand} {
		if rtlIn(s) {
			t.Errorf("published %s has RTL: %q", name, s)
		}
	}
	if lines := strings.Split(got.Detail, "\n"); len(lines) != 3 || lines[2] != "NOTE: right to left" {
		t.Errorf("Detail must keep its real line break and end with the note: %q", got.Detail)
	}
	wire, _ := json.Marshal(got)
	if rtlIn(string(wire)) {
		t.Errorf("RTL on the wire: %s", wire)
	}
	pend := c.Pending()
	if len(pend) != 1 {
		t.Fatalf("pending: %+v", pend)
	}
	wire, _ = json.Marshal(pend)
	if rtlIn(string(wire)) {
		t.Errorf("RTL in Pending: %s", wire)
	}
	reply(c, "j_9", "deny")
	<-done
}

func TestAppQuestionWithoutRTLIsUntouched(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Deny))
	asked := make(chan ConfirmRequest, 1)
	app.onPublish = func(r ConfirmRequest) { asked <- r }
	done := make(chan confirm.Decision, 1)
	r := confirm.Request{ID: "j_1", Title: "Restart", Detail: "a\nb", RawCommand: "adb shell reboot", ExpiresAt: time.Now().Add(time.Minute)}
	go func() { done <- c.Confirm(context.Background(), r) }()
	got := <-asked
	if got.Title != "Restart" || got.Detail != "a\nb" || got.RawCommand != "adb shell reboot" {
		t.Errorf("changed: %+v", got)
	}
	reply(c, "j_1", "deny")
	<-done
}

func TestAppNoteDefaultsToEnglish(t *testing.T) {
	app := &fakeApp{connected: true}
	c := NewConfirmer(app, fixed(confirm.Deny))
	asked := make(chan ConfirmRequest, 1)
	app.onPublish = func(r ConfirmRequest) { asked <- r }
	done := make(chan confirm.Decision, 1)
	go func() { done <- c.Confirm(context.Background(), hostileReq()) }()
	got := <-asked
	if !strings.Contains(got.Detail, "Some characters read right to left") {
		t.Errorf("no English note: %q", got.Detail)
	}
	reply(c, "j_9", "deny")
	<-done
}

// The app leaves while the question waits: the terminal (Fallback) gets the
// original and escapes it itself; what Notify says carries no RTL.
func TestMovedQuestionKeepsTheOriginalForTheFallback(t *testing.T) {
	app := &fakeApp{connected: true}
	fb := &recorderReq{}
	c := NewConfirmer(app, fb)
	n := &notes{}
	c.Notify = n.add
	app.onPublish = func(ConfirmRequest) {
		app.mu.Lock()
		app.connected = false
		app.mu.Unlock()
		c.AppGone()
	}
	confirmWithin(t, c, hostileReq())
	if fb.last.Title != "מחק א" || fb.last.RawCommand != "adb shell mv א ب" {
		t.Errorf("the fallback did not get the original: %+v", fb.last)
	}
	for _, s := range n.get() {
		if rtlIn(s) {
			t.Errorf("Notify title has RTL: %q", s)
		}
	}
	if len(n.get()) == 0 {
		t.Error("no notice")
	}
}

type recorderReq struct{ last confirm.Request }

func (r *recorderReq) Confirm(_ context.Context, q confirm.Request) confirm.Decision {
	r.last = q
	return confirm.Deny
}

// With no app connected the question is the terminal's, and it shows no RTL.
func TestQuestionWithoutAppIsShownByTheTerminalWithoutRTL(t *testing.T) {
	var out bytes.Buffer
	c := NewConfirmer(&fakeApp{connected: false}, confirm.NewTerminal(strings.NewReader(""), &out))
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	c.Confirm(ctx, hostileReq())
	if out.Len() == 0 || rtlIn(out.String()) {
		t.Fatalf("terminal output: %q", out.String())
	}
}
