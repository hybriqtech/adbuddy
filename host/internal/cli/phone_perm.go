package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
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
	req    bridge.PermissionRequest
	rule   string          // what "yes, always" would add; "" when it is not offered
	key    string          // claudecode.ToolKey of what asks: how the transcript's tool use is tied to this card
	toolID string          // the tool use tied to it ("" until one is); only its end retires the card
	done   chan permResult // receives the outcome once; buffered
}

type permResult struct {
	d    claudecode.Decision
	rule string
}

// permissionRequest asks the phone and waits for the answer. It returns what
// the hook writes: nothing leaves Claude Code's own dialog on the terminal
// (spec M4 §5.1).
func (l *phoneLink) permissionRequest(ctx context.Context, in claudecode.HookInput) []byte {
	if y := l.yoloOf(); y != nil && y.Active() {
		return l.yoloPermission(in)
	}
	if l.srv == nil || !l.srv.Connected() {
		l.holdTerminal() // the person is at the PC; the dialog is still there if the app comes
		return nil
	}
	if !claudecode.CardSafe(in.ToolName, in.ToolInput) {
		// The card could not show what a "yes" would approve: the terminal
		// asks, and its dialog must not get the chat's text (spec §6.3).
		l.raiseAttention()
		l.holdTerminal()
		return nil
	}
	rule, always := claudecode.AlwaysRule(in.ToolName, in.ToolInput)
	now := time.Now()
	timeout := permTimeout // read once: tests restore it when they end
	req := bridge.PermissionRequest{
		ID:          newID("p_"),
		Tool:        in.ToolName,
		Summary:     claudecode.ToolSummaryIn(l.lang, in.ToolName, in.ToolInput),
		Detail:      claudecode.ToolDetail(in.ToolName, in.ToolInput),
		ExpiresAt:   now.Add(timeout).UTC(),
		AllowAlways: always,
	}
	p := &pendingPerm{req: req, rule: rule, key: claudecode.ToolKey(in.ToolName, in.ToolInput), done: make(chan permResult, 1)}
	a := &l.adapter
	a.mu.Lock()
	if len(a.perms) >= maxPerms || now.Sub(a.lastPerm) < permGap {
		a.attention = now.Add(attentionTTL) // Claude Code's dialog is on the terminal
		a.held = true
		a.mu.Unlock()
		return nil
	}
	if a.perms == nil {
		a.perms = map[string]*pendingPerm{}
	}
	a.perms[req.ID] = p
	a.order = append(a.order, req.ID)
	a.tieCardLocked(p) // the tool use may have reached the transcript first
	a.lastPerm = now
	a.attention = now.Add(attentionTTL)
	a.mu.Unlock()
	l.publishEvent(bridge.TypePermRequest, req)
	// YOLO may have begun between the check above and the card being pending:
	// the observer did not see it. Nothing waits for a card under YOLO.
	if y := l.yoloOf(); y != nil && y.Active() {
		l.resolvePerm(req.ID, claudecode.Allow, "yolo")
	}

	timer := time.NewTimer(timeout)
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
// When YOLO approved it (by "yolo") the app is told the escaped summary and
// there is no rule, whatever the card offered.
func (l *phoneLink) resolvePerm(id string, d claudecode.Decision, by string) bool {
	summary := ""
	a := &l.adapter
	a.mu.Lock()
	p := a.perms[id]
	if p == nil {
		a.mu.Unlock()
		return false
	}
	delete(a.perms, id)
	rule := p.rule
	if by == "yolo" {
		rule = "" // YOLO is temporary: never a permanent rule in localSettings
		summary = classify.Visible(p.req.Summary)
	}
	if by == "timeout" {
		a.held = true // nobody answered: Claude Code's dialog stays on the terminal (V05)
	}
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
	p.done <- permResult{d, rule}
	l.publishEvent(bridge.TypePermResolved, bridge.PermissionResolved{ID: id, Decision: decision, By: by, Summary: summary})
	return true
}

// maxUnbound bounds the tool uses of the transcript waiting for a card.
const maxUnbound = 64

// unboundTool is a tool use of the transcript that no card is tied to (yet).
type unboundTool struct{ id, key string }

// The PermissionRequest hook does not say which tool use asks (spec M4 §5.1),
// so a card and a tool use are tied by their key (name and canonical input,
// claudecode.ToolKey), in either order: the tool use is written before the tool
// runs and the request comes a moment later, so it is usually there first. The
// oldest of each is tied first. Only the end of the tied tool use retires the
// card (spec M4 §5.2); with no tie, nothing does and the card stays until the
// hook closes or it expires. All of it is under adapter.mu.

// tieCardLocked ties a new card to the oldest tool use with its key.
func (a *adapter) tieCardLocked(p *pendingPerm) {
	if p.key == "" {
		return
	}
	for i, u := range a.unbound {
		if u.key == p.key {
			p.toolID = u.id
			a.unbound = append(a.unbound[:i:i], a.unbound[i+1:]...)
			return
		}
	}
}

// toolStarted ties a tool use that begins to the oldest card without one that
// has its key, or keeps it for the card that may come.
func (l *phoneLink) toolStarted(id, key string) {
	if key == "" {
		return
	}
	a := &l.adapter
	a.mu.Lock()
	defer a.mu.Unlock()
	// The same tool use again (a re-read of the transcript): it is already
	// tied to a card, and kept for no other.
	for _, p := range a.perms {
		if p != nil && p.toolID == id {
			return
		}
	}
	for _, oid := range a.order {
		if p := a.perms[oid]; p != nil && p.toolID == "" && p.key == key {
			p.toolID = id
			return
		}
	}
	for _, u := range a.unbound {
		if u.id == id {
			return
		}
	}
	a.unbound = append(a.unbound, unboundTool{id, key})
	if len(a.unbound) > maxUnbound {
		a.unbound = append([]unboundTool(nil), a.unbound[len(a.unbound)-maxUnbound:]...)
	}
}

// toolEnded ends, as answered on the terminal, the pending permissions tied to
// a tool use that has ended (spec M4 §5.2). Another tool's end, even of the
// same name and summary, does not take a question away.
func (l *phoneLink) toolEnded(id string) {
	a := &l.adapter
	a.mu.Lock()
	var ids []string
	for _, oid := range a.order {
		if p := a.perms[oid]; p != nil && p.toolID == id {
			ids = append(ids, oid)
		}
	}
	for i, u := range a.unbound {
		if u.id == id {
			a.unbound = append(a.unbound[:i:i], a.unbound[i+1:]...)
			break
		}
	}
	a.mu.Unlock()
	for _, oid := range ids {
		l.resolvePerm(oid, claudecode.NoDecision, "terminal")
	}
}

// resolveAllPermsAs ends every pending permission with d.
func (l *phoneLink) resolveAllPermsAs(d claudecode.Decision, by string) {
	a := &l.adapter
	a.mu.Lock()
	ids := append([]string(nil), a.order...)
	a.mu.Unlock()
	for _, id := range ids {
		l.resolvePerm(id, d, by)
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
	// While YOLO is on, "yes, always" is only a "yes": a tap injected on a card
	// that was pending when YOLO began must not leave a permanent rule. Read
	// before adapter.mu is taken (Yolo.mu is a leaf; the order stays simple).
	yoloOn := false
	if y := l.yoloOf(); y != nil {
		yoloOn = y.Active()
	}
	a := &l.adapter
	a.mu.Lock()
	p := a.perms[rep.ID]
	if p != nil && d == claudecode.AllowAlways && (!p.req.AllowAlways || p.rule == "" || yoloOn) {
		d = claudecode.Allow
	}
	a.mu.Unlock()
	if p == nil || !l.resolvePerm(rep.ID, d, "phone") {
		l.publishEvent(bridge.TypeNotice, bridge.Notice{Level: "info", Text: l.lang.T("notice.stale_reply")})
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

// yoloResolvePerms approves, as "allow" by yolo, the cards that were pending
// when YOLO was turned on (a YOLO observer: adapter.mu is taken one card at a
// time, and nothing here turns YOLO on or off). Left pending, a tap injected on
// one could answer "always" and leave a rule that outlives YOLO.
func (l *phoneLink) yoloResolvePerms() {
	l.resolveAllPermsAs(claudecode.Allow, "yolo")
}

// yoloPermission answers a permission question of the agent with "allow" while
// YOLO is on: nothing is asked of the phone (it may not even be connected) and
// the card is not needed. It is "allow", never "allow_always": that would leave
// a permanent rule in Claude Code's localSettings, and YOLO is temporary. The
// app is told what was approved (permission.resolved, by yolo), so the chat can
// show it, and serve's log says it too.
func (l *phoneLink) yoloPermission(in claudecode.HookInput) []byte {
	// The summary is the agent's words: escaped, as for a card (control, bidi and invisible characters).
	summary := classify.Visible(claudecode.ToolSummaryIn(l.lang, in.ToolName, in.ToolInput))
	if l.log != nil {
		fmt.Fprintln(l.log, l.lang.T("phone.yolo_approved", summary))
	}
	l.publishEvent(bridge.TypePermResolved, bridge.PermissionResolved{ID: newID("p_"), Decision: "allow", By: "yolo", Summary: summary})
	return claudecode.PermissionOutput(claudecode.Allow, "")
}
