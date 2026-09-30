package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
)

// Publisher is the part of Server a Confirmer uses.
type Publisher interface {
	Connected() bool
	Publish(typ string, payload any) error
}

// Confirmer asks on the phone (spec §4.5). When no app is connected at the
// moment of asking it uses Fallback (the terminal, or deny).
type Confirmer struct {
	App      Publisher
	Fallback confirm.Confirmer

	mu      sync.Mutex
	pending map[string]pendingReq
	session map[string]bool // keys allowed for the rest of the session
}

type pendingReq struct {
	req   ConfirmRequest
	reply chan confirm.Decision
	key   string
}

// NewConfirmer returns a Confirmer that asks on app, or fallback.
func NewConfirmer(app Publisher, fallback confirm.Confirmer) *Confirmer {
	return &Confirmer{App: app, Fallback: fallback, pending: map[string]pendingReq{}, session: map[string]bool{}}
}

// Confirm implements confirm.Confirmer.
// A request without an ID, or with the ID of one still waiting, is denied
// without asking: its answer could be taken for another question's.
func (c *Confirmer) Confirm(ctx context.Context, r confirm.Request) confirm.Decision {
	if r.ID == "" {
		return confirm.Deny
	}
	// "Allow during the session" covers only the same rule with the same
	// arguments, and never a command that may cut the connection.
	allowSession := r.Key != "" && !r.Special
	c.mu.Lock()
	if allowSession && c.session[r.Key] {
		c.mu.Unlock()
		return confirm.Allow
	}
	c.mu.Unlock()
	if !c.App.Connected() {
		return c.Fallback.Confirm(ctx, r)
	}
	req := ConfirmRequest{ID: r.ID, Level: "red", Title: r.Title, Detail: r.Detail, RawCommand: r.RawCommand,
		ExpiresAt: r.ExpiresAt.UTC(), Special: r.Special, AllowSession: allowSession}
	p := pendingReq{req: req, reply: make(chan confirm.Decision, 1), key: r.Key}
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
	select {
	case d := <-p.reply:
		return d
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return confirm.Timeout
		}
		return confirm.Deny
	}
}

// Reply handles a confirm.reply from the app. Replies for unknown or already
// answered ids are ignored.
func (c *Confirmer) Reply(raw json.RawMessage) {
	var rep ConfirmReply
	if json.Unmarshal(raw, &rep) != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pending[rep.ID]
	if !ok {
		return
	}
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
