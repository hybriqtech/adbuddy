package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"

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

	mu      sync.Mutex
	pending map[string]*pendingReq
	session map[string]bool // keys allowed for the rest of the session
}

type pendingReq struct {
	req   ConfirmRequest
	reply chan confirm.Decision
	key   string
	gone  chan struct{} // closed by AppGone; replaced when the question stays on the phone
}

// NewConfirmer returns a Confirmer that asks on app, or fallback.
func NewConfirmer(app Publisher, fallback confirm.Confirmer) *Confirmer {
	return &Confirmer{App: app, Fallback: fallback, pending: map[string]*pendingReq{}, session: map[string]bool{}}
}

// Confirm implements confirm.Confirmer.
// A request without an ID, or with the ID of one still waiting, is denied
// without asking: its answer could be taken for another question's.
func (c *Confirmer) Confirm(ctx context.Context, r confirm.Request) confirm.Decision {
	if r.ID == "" {
		return confirm.Deny
	}
	// "Allow during the session" covers only the same rule with the same
	// arguments, and never a command that may cut the connection or one
	// whose effect may come from stdin, which the key does not cover (A4-1).
	allowSession := r.Key != "" && !r.Special && !r.Stdin
	c.mu.Lock()
	if allowSession && c.session[r.Key] {
		c.mu.Unlock()
		return confirm.Allow
	}
	c.mu.Unlock()
	if !c.App.Connected() {
		return c.Fallback.Confirm(ctx, r)
	}
	// What the app shows is what Shown makes visible; the Fallback (the
	// terminal) gets the original and does the same.
	note := c.RTLNote
	if note == "" {
		note = i18n.EN.T("confirm.rtl_escaped")
	}
	shown := r.Shown(note)
	req := ConfirmRequest{ID: r.ID, Level: "red", Title: shown.Title, Detail: shown.Detail, RawCommand: shown.RawCommand,
		ExpiresAt: r.ExpiresAt.UTC(), Special: r.Special, AllowSession: allowSession}
	p := &pendingReq{req: req, reply: make(chan confirm.Decision, 1), key: r.Key, gone: make(chan struct{})}
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
			return d
		case <-gone:
			// Never under c.mu: the server answers Connected under its
			// lock, and holds that lock when a snapshot calls Pending.
			back := c.App.Connected()
			c.mu.Lock()
			_, waiting := c.pending[r.ID]
			if !waiting { // answered just now
				c.mu.Unlock()
				return <-p.reply
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
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return confirm.Timeout
			}
			return confirm.Deny
		}
	}
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

// StaleReplyText tells the app its answer came too late.
const StaleReplyText = "That question was already answered or moved to the PC terminal."

// Reply handles a confirm.reply from the app. A reply for an id that is not
// waiting (answered, withdrawn or moved to the terminal) does nothing but
// tell the app so.
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
	switch rep.Decision {
	case "allow":
		d = confirm.Allow
	case "allow_session":
		d = confirm.Allow
		if p.req.AllowSession {
			c.session[p.key] = true
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
