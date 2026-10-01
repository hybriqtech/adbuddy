package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

type permOut struct {
	H struct {
		D struct {
			Behavior string `json:"behavior"`
			Message  string `json:"message"`
			Updated  []struct {
				Destination string `json:"destination"`
				Rules       []struct{ ToolName, RuleContent string }
			} `json:"updatedPermissions"`
		} `json:"decision"`
	} `json:"hookSpecificOutput"`
}

func parsePerm(t *testing.T, b []byte) permOut {
	t.Helper()
	var o permOut
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatalf("hook output %q: %v", b, err)
	}
	return o
}

// ask runs a PermissionRequest hook for a Bash command in the background and
// returns the request the phone gets and a channel for the hook's output.
func (r *agentRig) ask(t *testing.T, cmd string) (bridge.PermissionRequest, <-chan []byte) {
	t.Helper()
	return r.askCtx(t, context.Background(), cmd)
}

func (r *agentRig) askCtx(t *testing.T, ctx context.Context, cmd string) (bridge.PermissionRequest, <-chan []byte) {
	t.Helper()
	out := make(chan []byte, 1)
	p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd, "description": "do it"}})
	go func() { out <- r.l.hookCall(ctx, r.ws, "PermissionRequest", p) }()
	var req bridge.PermissionRequest
	if err := json.Unmarshal(r.readType(t, bridge.TypePermRequest).Payload, &req); err != nil {
		t.Fatal(err)
	}
	return req, out
}

func (r *agentRig) resolved(t *testing.T) bridge.PermissionResolved {
	t.Helper()
	var pr bridge.PermissionResolved
	if err := json.Unmarshal(r.readType(t, bridge.TypePermResolved).Payload, &pr); err != nil {
		t.Fatal(err)
	}
	return pr
}

func (r *agentRig) answer(t *testing.T, id, decision string) {
	t.Helper()
	sendApp(t, r.c, bridge.TypePermReply, "", bridge.PermissionReply{ID: id, Decision: decision})
}

func wait(t *testing.T, out <-chan []byte) []byte {
	t.Helper()
	select {
	case b := <-out:
		return b
	case <-time.After(5 * time.Second):
		t.Fatal("the hook did not answer")
		return nil
	}
}

func TestPermissionAllow(t *testing.T) {
	r := newAgentRig(t)
	req, out := r.ask(t, "touch uno.txt")
	if req.Tool != "Bash" || req.Summary != "do it" || req.Detail != "touch uno.txt" || !req.AllowAlways || !strings.HasPrefix(req.ID, "p_") || !req.ExpiresAt.After(time.Now()) {
		t.Fatalf("request %+v", req)
	}
	if snap := r.l.snapshot(); len(snap.Permissions) != 1 || snap.Permissions[0].ID != req.ID {
		t.Fatalf("the snapshot carries the pending question: %+v", snap.Permissions)
	}
	r.answer(t, req.ID, "allow")
	if pr := r.resolved(t); pr.ID != req.ID || pr.Decision != "allow" || pr.By != "phone" {
		t.Fatalf("resolved %+v", pr)
	}
	o := parsePerm(t, wait(t, out))
	if o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 0 {
		t.Fatalf("%+v", o)
	}
	if snap := r.l.snapshot(); len(snap.Permissions) != 0 {
		t.Fatal("still pending after the answer")
	}
}

func TestPermissionDeny(t *testing.T) {
	r := newAgentRig(t)
	req, out := r.ask(t, "touch uno.txt")
	r.answer(t, req.ID, "deny")
	if pr := r.resolved(t); pr.Decision != "deny" || pr.By != "phone" {
		t.Fatalf("%+v", pr)
	}
	if o := parsePerm(t, wait(t, out)); o.H.D.Behavior != "deny" || o.H.D.Message == "" {
		t.Fatalf("%+v", o)
	}
}

func TestPermissionAllowAlwaysIsTheExactCommand(t *testing.T) {
	r := newAgentRig(t)
	req, out := r.ask(t, "touch uno.txt")
	r.answer(t, req.ID, "allow_always")
	if pr := r.resolved(t); pr.Decision != "allow_always" {
		t.Fatalf("%+v", pr)
	}
	o := parsePerm(t, wait(t, out))
	u := o.H.D.Updated
	if o.H.D.Behavior != "allow" || len(u) != 1 || u[0].Destination != "localSettings" || len(u[0].Rules) != 1 ||
		u[0].Rules[0].ToolName != "Bash" || u[0].Rules[0].RuleContent != "touch uno.txt" {
		t.Fatalf("%+v", o)
	}
}

// Nothing that could touch adb or the phone is allowed for good, and an app
// that asks for "always" anyway only gets "yes" (spec M4 §5.3).
func TestPermissionAllowAlwaysIsNeverOfferedForAdb(t *testing.T) {
	r := newAgentRig(t)
	for _, cmd := range []string{"adb shell rm -rf /sdcard/DCIM", "touch a; adb devices"} {
		req, out := r.ask(t, cmd)
		if req.AllowAlways {
			t.Fatalf("%q offered for good", cmd)
		}
		r.answer(t, req.ID, "allow_always")
		if pr := r.resolved(t); pr.Decision != "allow" {
			t.Fatalf("%q: %+v", cmd, pr)
		}
		if o := parsePerm(t, wait(t, out)); o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 0 {
			t.Fatalf("%q: a rule was added: %+v", cmd, o)
		}
	}
}

func TestPermissionTimesOutWithNoDecision(t *testing.T) {
	setTiming(t, &permTimeout, 80*time.Millisecond)
	r := newAgentRig(t)
	req, out := r.ask(t, "touch x")
	if pr := r.resolved(t); pr.ID != req.ID || pr.By != "timeout" {
		t.Fatalf("%+v", pr)
	}
	if b := wait(t, out); b != nil {
		t.Fatalf("a timeout must leave Claude Code's dialog: %s", b)
	}
	r.answer(t, req.ID, "allow") // too late
	if n := r.readType(t, bridge.TypeNotice); !strings.Contains(string(n.Payload), "already answered") {
		t.Fatalf("%s", n.Payload)
	}
}

func TestPermissionAnsweredOnTheTerminalWhenTheHookIsClosed(t *testing.T) {
	r := newAgentRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	req, out := r.askCtx(t, ctx, "touch x")
	cancel()
	if pr := r.resolved(t); pr.ID != req.ID || pr.By != "terminal" {
		t.Fatalf("%+v", pr)
	}
	if b := wait(t, out); b != nil {
		t.Fatalf("%s", b)
	}
}

func TestPermissionAnsweredOnTheTerminalShowsInTheTranscript(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	in := map[string]any{"command": "touch x", "description": "do it"}
	r.append(t, atool("u1", "msg_1", "toolu_1", "Bash", in), atool("u2", "msg_2", "toolu_2", "Read", map[string]any{"file_path": "/a/b.go"}))
	req, out := r.ask(t, "touch x")
	// Another tool ending does not take the question away...
	r.append(t, aresult("u3", "toolu_2", false))
	select {
	case <-out:
		t.Fatal("an unrelated tool's end resolved the question")
	case <-time.After(150 * time.Millisecond):
	}
	// ... the end of the tool that asked does.
	r.append(t, aresult("u4", "toolu_1", false))
	if pr := r.resolved(t); pr.ID != req.ID || pr.By != "terminal" {
		t.Fatalf("%+v", pr)
	}
	if b := wait(t, out); b != nil {
		t.Fatalf("%s", b)
	}
}

func TestPermissionEndsWithTheSession(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	req, out := r.ask(t, "touch x")
	r.hook(t, "SessionEnd", map[string]any{"reason": "other"})
	if pr := r.resolved(t); pr.ID != req.ID || pr.By != "agent" {
		t.Fatalf("%+v", pr)
	}
	if b := wait(t, out); b != nil {
		t.Fatalf("%s", b)
	}
}

func TestPermissionLimitsAndGarbage(t *testing.T) {
	r := newAgentRig(t)
	var outs []<-chan []byte
	for i := 0; i < maxPerms; i++ {
		_, out := r.ask(t, "touch "+string(rune('a'+i)))
		outs = append(outs, out)
	}
	// The ninth is not put to the phone: Claude Code asks on the terminal.
	p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "touch z"}})
	if out := r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p); out != nil {
		t.Fatalf("answered the ninth: %s", out)
	}
	if n := len(r.l.snapshot().Permissions); n != maxPerms {
		t.Fatalf("%d pending", n)
	}
	// Replies that make no sense do nothing.
	r.answer(t, "p_unknown", "allow")
	r.readType(t, bridge.TypeNotice)
	sendApp(t, r.c, bridge.TypePermReply, "", map[string]any{"id": 5})
	sendApp(t, r.c, bridge.TypePermReply, "", bridge.PermissionReply{ID: "x", Decision: "maybe"})
	r.l.resolveAllPerms("agent")
	for _, o := range outs {
		wait(t, o)
	}
}

func TestPermissionRateLimit(t *testing.T) {
	r := newAgentRig(t)
	setTiming(t, &permGap, time.Hour)
	_, out := r.ask(t, "touch a")
	p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "touch b"}})
	if b := r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p); b != nil {
		t.Fatalf("a second request at once: %s", b)
	}
	r.l.resolveAllPerms("agent")
	wait(t, out)
}

func TestPermissionOfAnotherToolHasNoAlways(t *testing.T) {
	r := newAgentRig(t)
	out := make(chan []byte, 1)
	p := r.payload("PermissionRequest", map[string]any{"tool_name": "Edit", "tool_input": map[string]any{"file_path": "C:\\w\\env\\main.go", "old_string": "a"}})
	go func() { out <- r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p) }()
	var req bridge.PermissionRequest
	if err := json.Unmarshal(r.readType(t, bridge.TypePermRequest).Payload, &req); err != nil {
		t.Fatal(err)
	}
	if req.AllowAlways || req.Summary != "Edit main.go" || req.Detail != "C:\\w\\env\\main.go" {
		t.Fatalf("%+v", req)
	}
	r.answer(t, req.ID, "allow")
	r.resolved(t)
	wait(t, out)
}

func TestPermissionWithNoAppConnectedIsLeftToTheTerminal(t *testing.T) {
	r := newAgentRig(t)
	r.c.CloseNow()
	waitUntil(t, "the app gone", func() bool { return !r.l.srv.Connected() })
	p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "touch x"}})
	done := make(chan []byte, 1)
	go func() { done <- r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p) }()
	if b := wait(t, done); b != nil {
		t.Fatalf("%s", b)
	}
}

// Review I3: the card must show what a "yes" approves. What does not fit whole
// or holds controls, bidi or invisible characters is left to the terminal: no
// card, nothing pending.
func TestPermissionIsNotAskedWhenTheCardCannotShowIt(t *testing.T) {
	r := newAgentRig(t)
	for name, c := range map[string]struct{ cmd, desc string }{
		"does not fit":  {strings.Repeat("a", 2000) + "; rm -rf ~", "do it"},
		"bidi":          {"echo \u202eevil", "do it"},
		"escape":        {"echo \x1b[2J", "do it"},
		"zero width":    {"echo a\u200bb", "do it"},
		"bidi in title": {"ls", "harmless \u202e"},
	} {
		p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": c.cmd, "description": c.desc}})
		got := make(chan []byte, 1)
		go func() { got <- r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p) }()
		select {
		case out := <-got:
			if out != nil {
				t.Errorf("%s: the hook decided: %s", name, out)
			}
		case <-time.After(500 * time.Millisecond):
			r.l.resolveAllPerms("test")
			t.Fatalf("%s: the hook waits for the phone", name)
		}
		if snap := r.l.snapshot(); len(snap.Permissions) != 0 {
			t.Fatalf("%s: a card was raised: %+v", name, snap.Permissions)
		}
	}
	// What does fit and is clean still asks.
	req, out := r.ask(t, "echo a\n\tb")
	r.answer(t, req.ID, "deny")
	_ = wait(t, out)
}

// Review I2: with the adapter not active (a SessionStart it could not follow)
// a pending question still stops chat.send: Enter must not land on the dialog.
func TestChatSendIsBusyWithAPendingPermissionAndNoActiveAdapter(t *testing.T) {
	r := newAgentRig(t)
	req, out := r.ask(t, "touch x") // no SessionStart: the adapter is not active
	if r.l.agentBusy() == "" {
		t.Fatal("not busy with a permission pending")
	}
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "c1", Text: "hola"})
	r.readError(t, "r_1", bridge.CodeBusy)
	if said := r.pty(0).said(); said != "" {
		t.Fatalf("something was typed into the dialog: %q", said)
	}
	r.answer(t, req.ID, "deny")
	_ = wait(t, out)
}

func TestChatSendIsBusyWithAnAttentionAndNoActiveAdapter(t *testing.T) {
	r := newAgentRig(t)
	r.l.raiseAttention()
	if r.l.agentBusy() == "" {
		t.Fatal("not busy with a dialog on the screen")
	}
}

// recordingADB is an adb that records what ran; nothing here may reach it.
type recordingADB struct {
	mu   sync.Mutex
	runs []string
}

func (a *recordingADB) Shell(context.Context, string) (string, int, error) { return "", 1, nil }
func (a *recordingADB) Pull(context.Context, string, string) error         { return errors.New("no pull") }
func (a *recordingADB) Run(_ context.Context, _ string, args []string, _ io.Reader, _, _ io.Writer) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.runs = append(a.runs, strings.Join(args, " "))
	return 0, nil
}

// countConfirmer counts the 🔴 confirmations asked and denies them.
type countConfirmer struct {
	mu    sync.Mutex
	asked []confirm.Request
}

func (c *countConfirmer) Confirm(_ context.Context, r confirm.Request) confirm.Decision {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked = append(c.asked, r)
	return confirm.Deny
}

// Review M6 (spec M4 §5.3 and §10): a "yes" (and a "yes, always") of the phone
// to the agent's own question about `adb shell rm …` does not skip the shim:
// the same command that reaches the daemon is still a 🔴 that asks.
func TestPermissionYesDoesNotSkipTheShim(t *testing.T) {
	data := shortDir(t)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	adb, conf := &recordingADB{}, &countConfirmer{}
	ws := t.TempDir()
	d := daemon.New(daemon.Options{
		Classifier: cls, ADB: adb, Journal: journal.OpenSigned(filepath.Join(ws, "journal"), []byte("test key")), Confirmer: conf,
		Serial: "dev1", Session: "s_1", Lang: "en", Workspace: ws,
	})
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit { return d.Handle(ctx, req, c) })
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	r := newAgentRig(t)
	const cmd = "adb shell rm -rf /sdcard/DCIM"
	for _, decision := range []string{"allow", "allow_always"} {
		req, out := r.ask(t, cmd)
		if req.AllowAlways {
			t.Fatal("offered for good")
		}
		r.answer(t, req.ID, decision)
		if o := parsePerm(t, wait(t, out)); o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 0 {
			t.Fatalf("%s: %+v", decision, o)
		}
		before := len(conf.asked)
		call := ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "rm", "-rf", "/sdcard/DCIM"}, Cwd: ws, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}
		exit, err := ipc.Call(context.Background(), data, call, nil, io.Discard, io.Discard, nil)
		if err != nil || exit.Code == 0 {
			t.Fatalf("%s: the shim ran it after a yes: %+v %v", decision, exit, err)
		}
		conf.mu.Lock()
		asked := len(conf.asked)
		conf.mu.Unlock()
		if asked != before+1 {
			t.Fatalf("%s: the 🔴 was not asked (%d -> %d)", decision, before, asked)
		}
	}
	adb.mu.Lock()
	defer adb.mu.Unlock()
	for _, run := range adb.runs {
		if strings.Contains(run, "rm") {
			t.Fatalf("adb ran it: %q", run)
		}
	}
}
