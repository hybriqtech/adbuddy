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
	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
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
		"private use":   {"echo a\ue000b", "do it"}, // V18
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

// Round 1, N1: a PermissionRequest the host leaves to the terminal (the card
// cannot show it, too many pending, too fast) still opens Claude Code's dialog
// there: the chat must be busy, so Enter does not land on it.
func TestChatSendIsBusyWhenThePermissionIsLeftToTheTerminal(t *testing.T) {
	typed := func(r *agentRig, id string) {
		sendApp(t, r.c, bridge.TypeChatSend, id, bridge.ChatSend{ID: "c" + id, Text: "hola"})
		r.readError(t, id, bridge.CodeBusy)
		if said := r.pty(0).said(); said != "" {
			t.Fatalf("something was typed into the dialog: %q", said)
		}
	}
	ask := func(r *agentRig, cmd string) []byte {
		p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd, "description": "do it"}})
		return r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p)
	}
	t.Run("card cannot show it", func(t *testing.T) {
		r := newAgentRig(t)
		if out := ask(r, "echo "+string(rune(0x202e))+"evil"); out != nil {
			t.Fatalf("decided: %s", out)
		}
		typed(r, "r_1")
	})
	t.Run("rate", func(t *testing.T) {
		r := newAgentRig(t)
		setTiming(t, &permGap, time.Hour)
		_, out := r.ask(t, "touch a")
		r.l.resolveAllPerms("test")
		_ = wait(t, out)
		r.resolved(t)
		r.l.adapter.mu.Lock()
		r.l.adapter.attention = time.Time{}
		r.l.adapter.mu.Unlock()
		if out := ask(r, "touch b"); out != nil {
			t.Fatalf("decided: %s", out)
		}
		typed(r, "r_2")
	})
	t.Run("limit", func(t *testing.T) {
		r := newAgentRig(t)
		var outs []<-chan []byte
		for i := 0; i < maxPerms; i++ {
			_, out := r.ask(t, "touch a"+string(rune('a'+i)))
			outs = append(outs, out)
		}
		r.l.adapter.mu.Lock()
		r.l.adapter.attention = time.Time{}
		r.l.adapter.mu.Unlock()
		if out := ask(r, "touch z"); out != nil {
			t.Fatalf("decided: %s", out)
		}
		r.l.resolveAllPerms("test")
		for _, o := range outs {
			_ = wait(t, o)
			r.resolved(t)
		}
		// pending ones are gone, attention was raised by the refused one
		typed(r, "r_3")
	})
}

// V05: a question the host leaves to Claude Code's dialog on the terminal
// keeps the chat busy past the attention's TTL (it was 120 s and then a
// chat.send typed Enter on a dialog nobody had seen), until Stop, a prompt or
// the end of the session. attentionTTL is 0: the TTL is over at once, no sleep.
func TestChatSendIsBusyWhileTheTerminalHoldsAQuestionPastTheAttention(t *testing.T) {
	typed := func(t *testing.T, r *agentRig, id string) {
		t.Helper()
		sendApp(t, r.c, bridge.TypeChatSend, id, bridge.ChatSend{ID: "c" + id, Text: "hola"})
		r.readError(t, id, bridge.CodeBusy)
		if said := r.pty(0).said(); said != "" {
			t.Fatalf("something was typed into the dialog: %q", said)
		}
	}
	left := func(t *testing.T, r *agentRig, input map[string]any) {
		t.Helper()
		p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": input})
		got := make(chan []byte, 1)
		go func() { got <- r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p) }()
		select {
		case out := <-got:
			if out != nil {
				t.Fatalf("decided: %s", out)
			}
		case <-time.After(2 * time.Second):
			r.l.resolveAllPerms("test")
			<-got
			t.Fatal("a card was raised: the hook waits for the phone")
		}
	}
	t.Run("card cannot show it", func(t *testing.T) {
		setTiming(t, &attentionTTL, 0)
		r := newAgentRig(t)
		left(t, r, map[string]any{"command": "echo " + string(rune(0x202e)) + "evil", "description": "do it"})
		typed(t, r, "r_1")
	})
	t.Run("out of the sandbox", func(t *testing.T) {
		setTiming(t, &attentionTTL, 0)
		r := newAgentRig(t)
		left(t, r, map[string]any{"command": "ls", "description": "do it", "dangerouslyDisableSandbox": true})
		typed(t, r, "r_1")
	})
	t.Run("in the background", func(t *testing.T) {
		setTiming(t, &attentionTTL, 0)
		r := newAgentRig(t)
		left(t, r, map[string]any{"command": "ls", "description": "do it", "run_in_background": true})
		typed(t, r, "r_1")
	})
	t.Run("rate", func(t *testing.T) {
		setTiming(t, &attentionTTL, 0)
		r := newAgentRig(t)
		setTiming(t, &permGap, time.Hour) // newAgentRig sets it to 0
		_, out := r.ask(t, "touch a")
		r.l.resolveAllPerms("test")
		_ = wait(t, out)
		r.resolved(t)
		left(t, r, map[string]any{"command": "touch b", "description": "do it"})
		typed(t, r, "r_1")
	})
	t.Run("the card timed out", func(t *testing.T) {
		setTiming(t, &attentionTTL, 0)
		setTiming(t, &permTimeout, 80*time.Millisecond)
		r := newAgentRig(t)
		req, out := r.ask(t, "touch x")
		if pr := r.resolved(t); pr.ID != req.ID || pr.By != "timeout" {
			t.Fatalf("%+v", pr)
		}
		if b := wait(t, out); b != nil {
			t.Fatalf("decided: %s", b)
		}
		typed(t, r, "r_1")
	})
	t.Run("no app", func(t *testing.T) {
		setTiming(t, &attentionTTL, 0)
		r := newAgentRig(t)
		r.c.CloseNow()
		waitUntil(t, "the app gone", func() bool { return !r.l.srv.Connected() })
		left(t, r, map[string]any{"command": "touch x", "description": "do it"})
		if r.l.agentBusy() == "" {
			t.Fatal("not busy with the dialog on the terminal")
		}
	})
}

// V05: what ends the dialog the host left on the terminal frees the chat; the
// end of a tool does not (it may be another tool's with the same summary).
func TestTheTerminalsQuestionEndsWithStopPromptOrSession(t *testing.T) {
	for name, end := range map[string]func(t *testing.T, r *agentRig){
		"stop":    func(t *testing.T, r *agentRig) { r.hook(t, "Stop", map[string]any{}) },
		"prompt":  func(t *testing.T, r *agentRig) { r.hook(t, "UserPromptSubmit", map[string]any{"prompt": "x"}) },
		"session": func(t *testing.T, r *agentRig) { r.hook(t, "SessionEnd", map[string]any{}) },
		"conversation": func(t *testing.T, r *agentRig) {
			if err := r.m.Start("claude", false); err != nil { // a new process: a new conversation
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			setTiming(t, &attentionTTL, 0)
			r := newAgentRig(t)
			p := r.payload("PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "echo " + string(rune(0x202e)) + "evil"}})
			if out := r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p); out != nil {
				t.Fatalf("decided: %s", out)
			}
			if r.l.agentBusy() == "" {
				t.Fatal("not busy")
			}
			end(t, r)
			waitUntil(t, "the chat free", func() bool { return r.l.agentBusy() == "" })
		})
	}
}

// V05: past the attention's TTL, a dialog Busy() reads on the screen still
// keeps the chat from typing.
func TestChatSendIsBusyWithADialogOnTheScreenPastTheAttention(t *testing.T) {
	setTiming(t, &attentionTTL, 0)
	r := newAgentRig(t)
	r.hook(t, "Notification", map[string]any{"notification_type": "permission_prompt"})
	r.pty(0).outW.Write([]byte("Do you want to proceed?"))
	waitUntil(t, "the question on the screen", func() bool { return r.m.Busy() != "" })
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "c1", Text: "hola"})
	r.readError(t, "r_1", bridge.CodeBusy)
	if said := r.pty(0).said(); said != "" {
		t.Fatalf("something was typed into the dialog: %q", said)
	}
}

// V05, round 1: a PermissionRequest the host cannot take (not JSON, too large,
// not from the environment) still opens Claude Code's dialog on the terminal:
// the chat is held as when the host leaves it there. Other events are not.
func TestAPermissionRequestTheHostIgnoresHoldsTheChat(t *testing.T) {
	setTiming(t, &attentionTTL, 0)
	r := newAgentRig(t)
	elsewhere := map[string]any{"hook_event_name": "PermissionRequest", "session_id": sid, "cwd": t.TempDir(), "tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}}
	far, _ := json.Marshal(elsewhere)
	for name, raw := range map[string][]byte{
		"not json":      []byte("not json"),
		"empty":         []byte("{}"),
		"too large":     []byte(`{"session_id":"` + sid + `","x":"` + strings.Repeat("a", claudecode.MaxHookInput) + `"}`),
		"another place": far,
	} {
		for _, ev := range []string{"Stop", "Notification"} {
			r.l.hookCall(context.Background(), r.ws, ev, raw)
			if why := r.l.agentBusy(); why != "" {
				t.Fatalf("%s: %s held the chat", name, ev)
			}
		}
		if out := r.l.hookCall(context.Background(), r.ws, "PermissionRequest", raw); out != nil {
			t.Fatalf("%s: decided: %s", name, out)
		}
		if r.l.agentBusy() == "" {
			t.Errorf("%s: not busy with the dialog on the terminal", name)
		}
		r.hook(t, "UserPromptSubmit", map[string]any{"prompt": "x"})
	}
}

// bashIn is the input of the Bash calls of these tests: same description, so
// that only the command tells them apart.
func bashIn(cmd string) map[string]any {
	return map[string]any{"command": cmd, "description": "do it"}
}

// pending lists the ids of the questions the phone has.
func (r *agentRig) pending(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, p := range r.l.snapshot().Permissions {
		ids = append(ids, p.ID)
	}
	return ids
}

func (r *agentRig) stillPending(t *testing.T, want ...string) {
	t.Helper()
	time.Sleep(150 * time.Millisecond) // the poll's interval is 15 ms
	got := r.pending(t)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("pending %v, want %v", got, want)
	}
}

func TestPermissionOfTwoToolsWithTheSameDescriptionOnlyEndsWithItsOwn(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t, atool("u1", "msg_1", "toolu_a", "Bash", bashIn("touch a")), atool("u2", "msg_2", "toolu_b", "Bash", bashIn("touch b")))
	reqA, outA := r.ask(t, "touch a")
	reqB, outB := r.ask(t, "touch b")
	r.append(t, aresult("u3", "toolu_b", false))
	if pr := r.resolved(t); pr.ID != reqB.ID || pr.By != "terminal" {
		t.Fatalf("%+v", pr)
	}
	wait(t, outB)
	r.stillPending(t, reqA.ID) // the end of the other command does not take A's question away
	r.append(t, aresult("u4", "toolu_a", false))
	if pr := r.resolved(t); pr.ID != reqA.ID {
		t.Fatalf("%+v", pr)
	}
	wait(t, outA)
}

func TestPermissionOfTwoIdenticalToolsEndsOneByOneInOrder(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t, atool("u1", "msg_1", "toolu_a", "Bash", bashIn("touch x")), atool("u2", "msg_2", "toolu_b", "Bash", bashIn("touch x")))
	reqA, outA := r.ask(t, "touch x")
	reqB, outB := r.ask(t, "touch x")
	r.append(t, aresult("u3", "toolu_a", false))
	if pr := r.resolved(t); pr.ID != reqA.ID {
		t.Fatalf("the first card is the first tool's: %+v", pr)
	}
	wait(t, outA)
	r.stillPending(t, reqB.ID)
	r.append(t, aresult("u4", "toolu_b", true))
	if pr := r.resolved(t); pr.ID != reqB.ID {
		t.Fatalf("%+v", pr)
	}
	wait(t, outB)
}

// The same tool use seen again as running (a re-read) while it is tied to a
// card is not kept for the next card with its key: that card is left with no
// tie, and the tool's end takes away only the first.
func TestPermissionTiedToolUseSeenAgainIsNotKeptForAnotherCard(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t, atool("u1", "msg_1", "toolu_a", "Bash", bashIn("touch x")))
	reqA, outA := r.ask(t, "touch x")
	a := &r.l.adapter
	var key string
	waitUntil(t, "the card is tied to the tool use", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, p := range a.perms {
			if p.toolID == "toolu_a" {
				key = p.key
			}
		}
		return key != ""
	})
	r.l.toolStarted("toolu_a", key)
	a.mu.Lock()
	n := len(a.unbound)
	a.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d tool uses kept, want none: the id is tied already", n)
	}
	reqB, outB := r.ask(t, "touch x")
	a.mu.Lock()
	tied := a.perms[reqB.ID].toolID
	a.mu.Unlock()
	if tied != "" {
		t.Fatalf("the second card was tied to %q", tied)
	}
	r.append(t, aresult("u2", "toolu_a", false))
	if pr := r.resolved(t); pr.ID != reqA.ID {
		t.Fatalf("%+v", pr)
	}
	wait(t, outA)
	r.stillPending(t, reqB.ID)
	r.l.resolveAllPerms("agent")
	wait(t, outB)
}

func TestPermissionCardBeforeTheToolUseIsTiedToo(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	reqA, outA := r.ask(t, "touch a")
	reqB, outB := r.ask(t, "touch b")
	// The tool uses reach the transcript after the cards, in the other order.
	r.append(t, atool("u1", "msg_1", "toolu_b", "Bash", bashIn("touch b")), atool("u2", "msg_2", "toolu_a", "Bash", bashIn("touch a")),
		atool("u3", "msg_3", "toolu_r", "Read", map[string]any{"file_path": "/a/b.go"}), aresult("u4", "toolu_r", false))
	r.stillPending(t, reqA.ID, reqB.ID)
	r.append(t, aresult("u5", "toolu_a", false))
	if pr := r.resolved(t); pr.ID != reqA.ID {
		t.Fatalf("%+v", pr)
	}
	wait(t, outA)
	r.stillPending(t, reqB.ID)
	r.append(t, aresult("u6", "toolu_b", false))
	if pr := r.resolved(t); pr.ID != reqB.ID {
		t.Fatalf("%+v", pr)
	}
	wait(t, outB)
}

func TestPermissionWithNoToolUseToTieIsNeverTakenAwayByATranscript(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	req, out := r.ask(t, "touch a")
	// A tool whose use was never seen ends, and another with another input.
	r.append(t, aresult("u1", "toolu_unseen", false), atool("u2", "msg_1", "toolu_o", "Bash", bashIn("touch other")), aresult("u3", "toolu_o", false))
	r.stillPending(t, req.ID)
	r.l.resolveAllPerms("agent")
	wait(t, out)
}

func TestPermissionTiesAreForgottenWithTheConversation(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t, atool("u1", "msg_1", "toolu_a", "Bash", bashIn("touch a")))
	waitUntil(t, "the tool use is kept", func() bool {
		a := &r.l.adapter
		a.mu.Lock()
		defer a.mu.Unlock()
		return len(a.unbound) == 1
	})
	r.l.adapterConversation("another", false)
	a := &r.l.adapter
	a.mu.Lock()
	n := len(a.unbound)
	a.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d tool uses kept after the conversation changed", n)
	}
}

func TestPermissionUnboundToolUsesAreBounded(t *testing.T) {
	r := newAgentRig(t)
	for i := 0; i < maxUnbound+20; i++ {
		r.l.apply(claudecode.Event{Kind: claudecode.KindTool, ToolID: "toolu_" + string(rune('a'+i%26)) + strings.Repeat("x", i/26), Status: "running", ToolKey: "k"})
	}
	a := &r.l.adapter
	a.mu.Lock()
	n := len(a.unbound)
	a.mu.Unlock()
	if n != maxUnbound {
		t.Fatalf("%d kept, want %d", n, maxUnbound)
	}
}
