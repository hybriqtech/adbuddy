package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
)

// hookBash runs a PermissionRequest hook for a Bash command and returns its output.
func (r *agentRig) hookBash(t *testing.T, cmd, desc string) []byte {
	t.Helper()
	p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd, "description": desc}})
	done := make(chan []byte, 1)
	go func() { done <- r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p) }()
	return wait(t, done)
}

// Under YOLO the agent's own permission questions are answered `allow` at the
// host, never allow_always (that would leave a permanent rule in localSettings),
// and the app is told what was approved. Mutation: remove the YOLO branch of
// permissionRequest.
func TestYoloAllowsThePermissionsOfTheAgent(t *testing.T) {
	r := newAgentRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.useYolo(y)
	y.EnableSession()
	r.readType(t, bridge.TypeYoloState)
	out := r.hookBash(t, "touch uno.txt", "do it")
	o := parsePerm(t, out)
	if o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 0 {
		t.Fatalf("%s", out) // "touch uno.txt" would offer allow_always to the phone; YOLO never gives it
	}
	pr := r.resolved(t)
	if pr.By != "yolo" || pr.Decision != "allow" || pr.Summary != "do it" || !strings.HasPrefix(pr.ID, "p_") {
		t.Fatalf("resolved %+v", pr)
	}
	if snap := r.l.snapshot(); len(snap.Permissions) != 0 {
		t.Fatalf("a card was raised: %+v", snap.Permissions)
	}
}

// YOLO outlives the app: with no app connected the hook still answers.
func TestYoloAllowsThePermissionsWithNoAppConnected(t *testing.T) {
	r := newAgentRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.useYolo(y)
	r.c.CloseNow()
	waitUntil(t, "the app gone", func() bool { return !r.l.srv.Connected() })
	y.EnableSession()
	if o := parsePerm(t, r.hookBash(t, "touch x", "")); o.H.D.Behavior != "allow" {
		t.Fatalf("%+v", o)
	}
}

// A command the card could not show whole is approved too: with YOLO nobody
// reads cards.
func TestYoloAllowsWhatTheCardCouldNotShow(t *testing.T) {
	r := newAgentRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.useYolo(y)
	y.EnableSession()
	r.readType(t, bridge.TypeYoloState)
	for _, cmd := range []string{strings.Repeat("a", 2000) + "; rm -rf ~", "echo ‮evil"} {
		if o := parsePerm(t, r.hookBash(t, cmd, "do it")); o.H.D.Behavior != "allow" {
			t.Fatalf("%q: %+v", cmd, o)
		}
	}
}

// What the chat shows of an approval is the agent's text: nothing that hides
// or rewrites it gets to the phone. Mutation: publish the summary without
// classify.Visible.
func TestYoloApprovalSummaryIsEscaped(t *testing.T) {
	r := newAgentRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.useYolo(y)
	y.EnableSession()
	r.readType(t, bridge.TypeYoloState)
	r.hookBash(t, "echo a", "harmless ‮\x1b[2J")
	pr := r.resolved(t)
	if strings.ContainsAny(pr.Summary, "‮\x1b") || !strings.Contains(pr.Summary, "harmless") {
		t.Fatalf("summary %q", pr.Summary)
	}
}

// Turned off, the cards are back.
func TestYoloOffBringsTheCardsBack(t *testing.T) {
	r := newAgentRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.useYolo(y)
	y.EnableSession()
	r.readType(t, bridge.TypeYoloState)
	y.Off(daemon.YoloOffApp)
	r.readType(t, bridge.TypeYoloState)
	req, out := r.ask(t, "touch uno.txt")
	r.answer(t, req.ID, "deny")
	_ = wait(t, out)
}
