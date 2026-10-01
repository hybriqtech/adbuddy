package cli

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
)

// The agent's own permission questions (spec M4 §5): Claude Code's
// PermissionRequest hook waits here for the phone's answer. They are not the
// 🔴 confirmations of ADBuddy, which keep their own channel (M2).

// Limits, variables for the tests.
var (
	permTimeout = 600 * time.Second
	permGap     = time.Second // at least this between two requests
)

const maxPerms = 8

type pendingPerm struct {
	req           bridge.PermissionRequest
	rule          string          // what "yes, always" would add; "" when it is not offered
	tool, summary string          // how the transcript names the tool use that asks, to tell its end
	done          chan permResult // receives the outcome once; buffered
}

type permResult struct {
	d    claudecode.Decision
	rule string
}

// permissionRequest asks the phone and waits for the answer. It returns what
// the hook writes: nothing leaves Claude Code's own dialog on the terminal
// (spec M4 §5.1).
func (l *phoneLink) permissionRequest(ctx context.Context, in claudecode.HookInput) []byte {
	if l.srv == nil || !l.srv.Connected() {
		return nil // the person is at the PC
	}
	if !claudecode.CardSafe(in.ToolName, in.ToolInput) {
		return nil // the card could not show what a "yes" would approve: the terminal asks
	}
	rule, always := claudecode.AlwaysRule(in.ToolName, in.ToolInput)
	now := time.Now()
	req := bridge.PermissionRequest{
		ID:          newID("p_"),
		Tool:        in.ToolName,
		Summary:     claudecode.ToolSummary(in.ToolName, in.ToolInput),
		Detail:      claudecode.ToolDetail(in.ToolName, in.ToolInput),
		ExpiresAt:   now.Add(permTimeout).UTC(),
		AllowAlways: always,
	}
	p := &pendingPerm{req: req, rule: rule, tool: in.ToolName, summary: req.Summary, done: make(chan permResult, 1)}
	a := &l.adapter
	a.mu.Lock()
	if len(a.perms) >= maxPerms || now.Sub(a.lastPerm) < permGap {
		a.mu.Unlock()
		return nil
	}
	if a.perms == nil {
		a.perms = map[string]*pendingPerm{}
	}
	a.perms[req.ID] = p
	a.order = append(a.order, req.ID)
	a.lastPerm = now
	a.attention = now.Add(attentionTTL)
	a.mu.Unlock()
	l.publishEvent(bridge.TypePermRequest, req)

	timer := time.NewTimer(permTimeout)
	defer timer.Stop()
	select {
	case r := <-p.done:
		return claudecode.PermissionOutput(r.d, r.rule)
	case <-ctx.Done(): // Claude Code closed the hook: it was answered on the terminal
		l.resolvePerm(req.ID, claudecode.NoDecision, "terminal")
	case <-timer.C:
		l.resolvePerm(req.ID, claudecode.NoDecision, "timeout")
	}
	r := <-p.done
	return claudecode.PermissionOutput(r.d, r.rule)
}

// resolvePerm ends a pending permission with the decision and tells the app.
// It reports whether it was still pending: of two answers at once, one wins.
func (l *phoneLink) resolvePerm(id string, d claudecode.Decision, by string) bool {
	a := &l.adapter
	a.mu.Lock()
	p := a.perms[id]
	if p == nil {
		a.mu.Unlock()
		return false
	}
	delete(a.perms, id)
	for i, o := range a.order {
		if o == id {
			a.order = append(a.order[:i:i], a.order[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	decision := "deny" // what the app is told when nobody decided
	switch d {
	case claudecode.Allow:
		decision = "allow"
	case claudecode.AllowAlways:
		decision = "allow_always"
	}
	p.done <- permResult{d, p.rule}
	l.publishEvent(bridge.TypePermResolved, bridge.PermissionResolved{ID: id, Decision: decision, By: by})
	return true
}

// resolvePermsOf ends, as answered on the terminal, the pending permissions of
// a tool use that has ended (spec M4 §5.2). It matches by the tool and its
// summary, so that another tool's end does not take a question away.
func (l *phoneLink) resolvePermsOf(tool, summary string) {
	a := &l.adapter
	a.mu.Lock()
	var ids []string
	for _, id := range a.order {
		if p := a.perms[id]; p != nil && p.tool == tool && p.summary == summary {
			ids = append(ids, id)
		}
	}
	a.mu.Unlock()
	for _, id := range ids {
		l.resolvePerm(id, claudecode.NoDecision, "terminal")
	}
}

// resolveAllPerms ends every pending permission with no decision.
func (l *phoneLink) resolveAllPerms(by string) {
	a := &l.adapter
	a.mu.Lock()
	ids := append([]string(nil), a.order...)
	a.mu.Unlock()
	for _, id := range ids {
		l.resolvePerm(id, claudecode.NoDecision, by)
	}
}

// onPermReply handles the phone's answer. "Yes, always" that the host did
// not offer is only a "yes": the app is not trusted with it (spec M4 §5.3).
func (l *phoneLink) onPermReply(m bridge.Message) {
	var rep bridge.PermissionReply
	if json.Unmarshal(m.Payload, &rep) != nil {
		return
	}
	var d claudecode.Decision
	switch rep.Decision {
	case "allow":
		d = claudecode.Allow
	case "allow_always":
		d = claudecode.AllowAlways
	case "deny":
		d = claudecode.Deny
	default:
		return
	}
	a := &l.adapter
	a.mu.Lock()
	p := a.perms[rep.ID]
	if p != nil && d == claudecode.AllowAlways && (!p.req.AllowAlways || p.rule == "") {
		d = claudecode.Allow
	}
	a.mu.Unlock()
	if p == nil || !l.resolvePerm(rep.ID, d, "phone") {
		l.publishEvent(bridge.TypeNotice, bridge.Notice{Level: "info", Text: bridge.StaleReplyText})
	}
}

// permissionsSnapshot fills the permissions of a snapshot. It is called with
// the server's lock held: it only takes the adapter's.
func (l *phoneLink) permissionsSnapshot(s *bridge.Snapshot) {
	a := &l.adapter
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]bridge.PermissionRequest, 0, len(a.order))
	for _, id := range a.order {
		if p := a.perms[id]; p != nil {
			out = append(out, p.req)
		}
	}
	s.Permissions = out
}
