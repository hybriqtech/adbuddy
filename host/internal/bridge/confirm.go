package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"unicode/utf16"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// Publisher is the part of Server a Confirmer uses.
type Publisher interface {
	Connected() bool
	Publish(typ string, payload any) error
}

// Confirmer asks on the phone (spec §4.5). When no app is connected at the
// moment of asking it uses Fallback (the terminal, or deny), and so does a
// question already on the phone when the app disconnects (AppGone).
type Confirmer struct {
	App      Publisher
	Fallback confirm.Confirmer
	// Notify, if set, tells the person what happened to a question when the
	// app disconnected while it waited: it moved to Fallback (moved), or it
	// stays on the phone because the app is already back. Called from the
	// goroutine that waits for the answer; title comes from the rules and the
	// agent's arguments: escape it before printing.
	Notify func(title string, moved bool)
	// StaleText is what the app is told when its answer comes too late, in the
	// language of the PC; StaleReplyText (English) if empty.
	StaleText string
	// RTLNote is the line a question gets when right-to-left characters in it
	// were escaped (see confirm.Request.Shown), in the language of the PC;
	// English if empty.
	RTLNote string

	// Lock order (YOLO, spec 2026-10-18): daemon.Yolo calls its observers under
	// its notifyMu, and they call YoloStarted/YoloEnded (mu) and the daemon's
	// withdrawal (askMu). So notifyMu comes before mu, and mu comes before
	// the yolo.Active callback's own lock: nothing here is held while calling
	// Enable, EnableSession or Off, which would take notifyMu the other way.
	mu      sync.Mutex
	pending map[string]*pendingReq
	session map[string]uint64 // keys allowed for the rest of the session -> number of the grant (seq)

	seq        uint64      // grants so far, under mu
	yoloActive func() bool // nil: YOLO is off; under mu
	yoloOn     bool        // between YoloStarted and YoloEnded
	yoloMark   uint64      // seq when the run of YOLO began: the grants above it die with it
}

type pendingReq struct {
	req     ConfirmRequest
	reply   chan confirm.Decision
	key     string
	gone    chan struct{} // closed by AppGone; replaced when the question stays on the phone
	granted bool          // the answer added key to the session (under c.mu)
	unread  bool          // longer than the app's dialog shows: an allow is a deny (MaxDialogCommand)
}

// NewConfirmer returns a Confirmer that asks on app, or fallback.
func NewConfirmer(app Publisher, fallback confirm.Confirmer) *Confirmer {
	return &Confirmer{App: app, Fallback: fallback, pending: map[string]*pendingReq{}, session: map[string]uint64{}}
}

// SetYoloActive gives the Confirmer the question "is YOLO on?" (nil: it is
// not). While it is on, an allow_session is only an allow: a tap injected on a
// question that was waiting when YOLO began must not leave a permit that
// outlives it. f is called with c.mu held, so it must not call back into c.
func (c *Confirmer) SetYoloActive(f func() bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.yoloActive = f
}

// YoloStarted notes where YOLO began, so YoloEnded can drop what was granted
// since. Called again while it is already on (the time was renewed) it keeps
// the first mark: a renewal must not let the grants made in between stay.
func (c *Confirmer) YoloStarted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.yoloOn {
		c.yoloOn, c.yoloMark = true, c.seq
	}
}

// YoloEnded drops the session grants made since YOLO began, however it ended
// (app, CLI, expiry, pause, end of session). What was granted before it began
// stays. Called with no YOLO run open it does nothing.
func (c *Confirmer) YoloEnded() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.yoloOn {
		return
	}
	for k, n := range c.session {
		if n > c.yoloMark {
			delete(c.session, k)
		}
	}
	c.yoloOn = false
}

// Confirm implements confirm.Confirmer.
// A request without an ID, or with the ID of one still waiting, is denied
// without asking: its answer could be taken for another question's.
func (c *Confirmer) Confirm(ctx context.Context, r confirm.Request) confirm.Decision {
	if r.ID == "" {
		return confirm.Deny
	}
	// What the app shows is what Shown makes visible; the Fallback (the
	// terminal) gets the original and does the same.
	note := c.RTLNote
	if note == "" {
		note = i18n.EN.T("confirm.rtl_escaped")
	}
	shown := r.Shown(note)
	units := len(utf16.Encode([]rune(shown.RawCommand)))
	// "Allow during the session" covers only the same rule with the same
	// arguments, and never a command that may cut the connection, one whose
	// effect may come from stdin, which the key does not cover (A4-1), or one
	// the notification does not show whole (M1: whatever the app offers).
	allowSession := r.Key != "" && !r.Special && !r.Stdin && units <= MaxSessionCommand
	c.mu.Lock()
	if _, granted := c.session[r.Key]; allowSession && granted {
		c.mu.Unlock()
		return confirm.Allow
	}
	c.mu.Unlock()
	if !c.App.Connected() {
		return c.Fallback.Confirm(ctx, r)
	}
	req := ConfirmRequest{ID: r.ID, Level: "red", Title: shown.Title, Detail: shown.Detail, RawCommand: shown.RawCommand,
		ExpiresAt: r.ExpiresAt.UTC(), Special: r.Special, AllowSession: allowSession}
	p := &pendingReq{req: req, reply: make(chan confirm.Decision, 1), key: r.Key, gone: make(chan struct{}),
		unread: units > MaxDialogCommand}
	c.mu.Lock()
	if _, dup := c.pending[r.ID]; dup {
		c.mu.Unlock()
		return confirm.Deny
	}
	c.pending[r.ID] = p
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, r.ID)
		c.mu.Unlock()
	}()
	if err := c.App.Publish(TypeConfirmReq, req); err != nil {
		return confirm.Deny
	}
	// The app may have left between the check above and the question being
	// pending: then AppGone already ran and did not see it.
	if !c.App.Connected() {
		c.AppGone()
	}
	for {
		c.mu.Lock()
		gone := p.gone
		c.mu.Unlock()
		select {
		case d := <-p.reply:
			return c.answer(ctx, p, d)
		case <-gone:
			// Never under c.mu: the server answers Connected under its
			// lock, and holds that lock when a snapshot calls Pending.
			back := c.App.Connected()
			c.mu.Lock()
			_, waiting := c.pending[r.ID]
			if !waiting { // answered just now
				c.mu.Unlock()
				return c.answer(ctx, p, <-p.reply)
			}
			if back {
				// Back already: it has the question (resent, or in a
				// snapshot), so it keeps waiting there.
				p.gone = make(chan struct{})
				c.mu.Unlock()
				c.notify(shown.Title, false)
				// If it left again before the new channel was in place,
				// AppGone closed the old one only.
				if !c.App.Connected() {
					c.AppGone()
				}
				continue
			}
			delete(c.pending, r.ID)
			c.mu.Unlock()
			c.notify(shown.Title, true)
			return c.Fallback.Confirm(ctx, r)
		case <-ctx.Done():
			// An answer may have come at the same moment: no longer
			// pending from here on, so none can, and what it brought is
			// undone (answer).
			c.mu.Lock()
			delete(c.pending, r.ID)
			c.mu.Unlock()
			return c.answer(ctx, p, confirm.Deny)
		}
	}
}

// answer is the app's decision d, unless the question expired or was
// abandoned meanwhile: a select picks at random when the answer and the
// expiry come together, and an answer after the expiry never allows (A4-5).
// Then an allow_session it brought is not kept either.
func (c *Confirmer) answer(ctx context.Context, p *pendingReq, d confirm.Decision) confirm.Decision {
	if ctx.Err() == nil {
		return d
	}
	c.mu.Lock()
	if p.granted {
		delete(c.session, p.key)
	}
	c.mu.Unlock()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return confirm.Timeout
	}
	return confirm.Deny
}

// AppGone moves the questions waiting on the phone to Fallback: the app
// disconnected and nobody would answer them there. Call it when the app is
// gone (the server's OnConnect(false)). A question whose app is back by the
// time it looks keeps waiting on the phone.
func (c *Confirmer) AppGone() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.pending {
		select {
		case <-p.gone: // already told
		default:
			close(p.gone)
		}
	}
}

func (c *Confirmer) notify(title string, moved bool) {
	if c.Notify != nil {
		c.Notify(title, moved)
	}
}

// MaxSessionCommand is the longest raw command, in UTF-16 units (what the app
// measures), that the notification shows whole; a longer one is never
// allowed for the session (A4-2, M1). The app's ConfirmUi.MAX_COMMAND.
const MaxSessionCommand = 300

// MaxDialogCommand is the longest raw command, in UTF-16 units, that the
// app's dialog shows; a longer one cannot have been read to its end, so it is
// never allowed (H3, M1). The app's ConfirmUi.DIALOG_MAX_COMMAND.
const MaxDialogCommand = 20_000

// StaleReplyText tells the app its answer came too late.
const StaleReplyText = "That question was already answered or moved to the PC terminal."

// Reply handles a confirm.reply from the app. A reply for an id that is not
// waiting (answered, withdrawn or moved to the terminal) does nothing but
// tell the app so. The host's policy holds whatever the app offered (M1): an
// allow_session the question did not offer is a single allow, and an allow
// of a command longer than the dialog shows is a deny.
func (c *Confirmer) Reply(raw json.RawMessage) {
	var rep ConfirmReply
	if json.Unmarshal(raw, &rep) != nil {
		return
	}
	c.mu.Lock()
	p, ok := c.pending[rep.ID]
	if !ok {
		c.mu.Unlock()
		// Publish takes the server's lock: never under c.mu.
		text := c.StaleText
		if text == "" {
			text = StaleReplyText
		}
		_ = c.App.Publish(TypeNotice, Notice{Level: "info", Text: text})
		return
	}
	defer c.mu.Unlock()
	d := confirm.Deny
	switch {
	case p.unread:
	case rep.Decision == "allow":
		d = confirm.Allow
	case rep.Decision == "allow_session":
		d = confirm.Allow
		// While YOLO is on, nothing is kept: a question that was waiting when
		// it began could be answered by a tap the agent injected, and the
		// permit would outlive YOLO. Read under c.mu, so a grant is either
		// refused here or numbered above YoloStarted's mark and dropped by
		// YoloEnded: there is no gap between the state and the observers.
		if _, had := c.session[p.key]; p.req.AllowSession && !had && !(c.yoloActive != nil && c.yoloActive()) {
			c.seq++
			c.session[p.key] = c.seq
			p.granted = true
		}
	}
	delete(c.pending, rep.ID)
	p.reply <- d
}

// Pending lists the questions waiting for an answer, oldest first, for a snapshot.
func (c *Confirmer) Pending() []ConfirmRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ConfirmRequest, 0, len(c.pending))
	for _, p := range c.pending {
		out = append(out, p.req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
