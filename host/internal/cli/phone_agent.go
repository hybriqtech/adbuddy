package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// The Claude Code adapter (spec M4): hooks tell the daemon what the agent is
// doing and ask it for permissions; the transcript gives the chat its
// messages and tool activity. This file is the hooks, the state and the
// transcript; phone_perm.go is the permissions.
//
// Locking: adapter.mu guards the adapter's fields and is never held while
// calling the bridge server (the server calls snapshot with its lock held and
// snapshot takes adapter.mu), l.mu, l.chat.mu or a Publish.

// Timings, variables for the tests.
var (
	agentPoll    = 300 * time.Millisecond
	startGrace   = 5 * time.Second // a transcript that does not appear this long after SessionStart degrades the chat
	stopDelay    = 500 * time.Millisecond
	stopTries    = 3 // reads of the transcript after a Stop, stopDelay apart, before the fallback
	attentionTTL = 120 * time.Second
	badLines     = 20 // lines in a row that are not JSON before the chat degrades
)

const busyPending = "Answer the pending question first."

// sentText is a chat.send text, to tell a prompt of the phone from one typed
// in the terminal when the transcript shows it.
type sentText struct {
	text string // claudecode.Normalize
	at   time.Time
}

const (
	keepSent = 16 // how many chat.send texts are kept: the oldest leave first
	// sentTTL is how long a chat.send text may wait for its prompt to show in
	// the transcript: Claude Code can be slow to write it (a long turn that
	// queues it), and a prompt that was late came out again as typed in the
	// terminal.
	sentTTL = 10 * time.Minute
)

// agentNow is the clock of sentText, a variable for the tests.
var agentNow = time.Now

// adapter is the state of the Claude Code adapter for the current
// conversation.
type adapter struct {
	mu          sync.Mutex
	conv        string
	active      bool // a SessionStart arrived in this conversation
	degraded    bool
	hookState   string // working, idle or waiting; "" until a hook says
	cancel      context.CancelFunc
	lastText    string          // claudecode.Normalize of the assistant's latest message
	droppedSeen int             // parser.Dropped already logged
	fallbacks   map[string]bool // normalized texts emitted as the Stop fallback
	sent        []sentText
	attention   time.Time // a dialog may be on the terminal's screen until then
	held        bool      // a question was left to Claude Code's dialog on the terminal: until Stop, a prompt or the end (V05)
	perms       map[string]*pendingPerm
	order       []string
	unbound     []unboundTool // tool uses of the transcript no card is tied to (phone_perm.go)
	lastPerm    time.Time
	cfgDir      string // Claude Code's configuration directory; "" = the user's
	// pollMu serializes reads of the transcript (the loop and a Stop's).
	pollMu sync.Mutex
	tail   *claudecode.Tailer
}

// agentArea is the Claude Code adapter (M4, protocol spec §5): the agent's
// messages, its tool activity and its permission requests.
func (l *phoneLink) agentArea() area {
	return area{
		handlers: map[string]func(bridge.Message){bridge.TypePermReply: l.onPermReply},
		snapshot: l.permissionsSnapshot,
	}
}

// adapterState is what the adapter says of the session.state it modifies
// (spec M4 §6.1): the view, chat while there is an adapter that works, and the
// state the hooks last said. Both empty when it has nothing to say.
func (l *phoneLink) adapterState() (view, state string) {
	a := &l.adapter
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.active || a.degraded {
		return "", ""
	}
	return "chat", a.hookState
}

// adapterConversation starts the adapter over when another process of the
// agent begins or this one ends (spec M4 §6.1).
func (l *phoneLink) adapterConversation(conv string, stopped bool) {
	a := &l.adapter
	a.mu.Lock()
	if conv == a.conv && !stopped {
		a.mu.Unlock()
		return
	}
	a.conv = conv
	cancel := a.cancel
	a.cancel, a.tail = nil, nil
	a.active, a.degraded, a.hookState = false, false, ""
	a.lastText, a.fallbacks, a.attention, a.droppedSeen, a.held = "", nil, time.Time{}, 0, false
	a.unbound, a.sent = nil, nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	l.resolveAllPerms("agent")
}

// agentBusy says why a chat.send must wait: a question of the agent is
// pending, or a dialog may be on the terminal's screen (spec M4 §6.2): for the
// attention's TTL, or until Stop or a prompt when the host left a question to
// the terminal (held, V05); Manager.Busy() reads the screen too. It does
// not depend on the adapter being active: a PermissionRequest raises its card
// even when the SessionStart could not be followed, and the text typed then
// would land on Claude Code's dialog.
func (l *phoneLink) agentBusy() string {
	a := &l.adapter
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.perms) > 0 || a.held || time.Now().Before(a.attention) {
		return busyPending
	}
	return ""
}

// noteSent remembers the text of a chat.send that was typed.
func (l *phoneLink) noteSent(text string) {
	a := &l.adapter
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = append(a.sent, sentText{claudecode.Normalize(text), agentNow()})
	if len(a.sent) > keepSent {
		a.sent = a.sent[len(a.sent)-keepSent:]
	}
}

// wasSent reports whether a prompt of the transcript is a recent chat.send
// (the prompt may carry the attachments' lines after the text). The text is
// forgotten when it matches: one chat.send is one prompt, so the same words
// typed in the terminal later are shown.
func (l *phoneLink) wasSent(prompt string) bool {
	a := &l.adapter
	a.mu.Lock()
	defer a.mu.Unlock()
	now := agentNow()
	p := claudecode.Normalize(prompt)
	live := a.sent[:0:0]
	found := false
	for _, s := range a.sent {
		if now.Sub(s.at) >= sentTTL {
			continue // too old: it never will show
		}
		if !found && s.text != "" && strings.HasPrefix(p, s.text) {
			found = true // consumed
			continue
		}
		live = append(live, s)
	}
	a.sent = live
	return found
}

// handleHook answers an `adbuddy hook <event>` (spec M4 §3.3): the payload is
// stdin, the decision, if any, stdout. It never fails the hook: whatever is
// wrong is told in the host's log and the exit is 0, which leaves Claude Code
// to its own behavior.
func (l *phoneLink) handleHook(ctx context.Context, ws string, req ipc.Request, c *ipc.Conn) ipc.Exit {
	if len(req.Argv) != 1 {
		return ipc.Exit{}
	}
	raw, err := io.ReadAll(io.LimitReader(c.Stdin(), claudecode.MaxHookInput+1))
	if err != nil {
		l.ignoredHook(req.Argv[0])
		return ipc.Exit{}
	}
	if out := l.hookCall(ctx, ws, req.Argv[0], raw); len(out) > 0 {
		_, _ = c.Stdout().Write(out)
	}
	return ipc.Exit{}
}

// hookCall is handleHook without the socket. It returns what the hook is to
// write on stdout, mostly nothing.
func (l *phoneLink) hookCall(ctx context.Context, ws, event string, raw []byte) []byte {
	in, err := claudecode.ParseHook(event, raw)
	if err != nil {
		fmt.Fprintln(l.log, l.lang.T("phone_agent.claude_code_hook_ignored", err))
		l.ignoredHook(event)
		return nil
	}
	if !sameTree(ws, in.Cwd) {
		fmt.Fprintln(l.log, l.lang.T("phone_agent.claude_code_hook_ignored_it"))
		l.ignoredHook(event)
		return nil
	}
	switch in.Event {
	case claudecode.EventSessionStart:
		l.onSessionStart(in)
	case claudecode.EventUserPromptSubmit:
		l.setHookState("working", true)
	case claudecode.EventStop:
		l.setHookState("idle", true)
		// The delay is read here, not in the goroutine: a test that sets
		// stopDelay restores it when it ends, and the goroutine is not waited for.
		go l.afterStop(in, stopDelay)
	case claudecode.EventNotification:
		switch in.NotificationType {
		case "permission_prompt":
			l.raiseAttention()
		case "idle_prompt":
			l.setHookState("waiting", false)
		}
	case claudecode.EventSessionEnd:
		l.onSessionEnd()
	case claudecode.EventPermissionRequest:
		return l.permissionRequest(ctx, in)
	}
	return nil
}

// ignoredHook holds the chat for a PermissionRequest the host does not take:
// Claude Code opens its dialog on the terminal all the same (V05, round 1).
// It can only keep the chat from typing, which is the safe way to fail.
func (l *phoneLink) ignoredHook(event string) {
	if event == claudecode.EventPermissionRequest {
		l.raiseAttention()
		l.holdTerminal()
	}
}

// sameTree reports whether cwd is the environment or lies under it.
func sameTree(ws, cwd string) bool {
	if ws == "" || cwd == "" || !filepath.IsAbs(cwd) {
		return false
	}
	real := func(p string) string {
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	return claudecode.Inside(real(ws), real(cwd))
}

// setHookState records what the hooks say the agent does and shows it. clear
// also lowers the attention (a prompt or an end of turn means the dialog is
// gone).
func (l *phoneLink) setHookState(state string, clear bool) {
	a := &l.adapter
	a.mu.Lock()
	changed := a.hookState != state
	a.hookState = state
	if clear {
		a.attention, a.held = time.Time{}, false
	}
	a.mu.Unlock()
	if changed {
		l.publishState()
	}
}

func (l *phoneLink) raiseAttention() {
	a := &l.adapter
	a.mu.Lock()
	a.attention = time.Now().Add(attentionTTL)
	a.mu.Unlock()
}

// holdTerminal marks a question left to Claude Code's dialog on the terminal:
// nobody may have seen it, and it does not expire with the attention (V05).
func (l *phoneLink) holdTerminal() {
	a := &l.adapter
	a.mu.Lock()
	a.held = true
	a.mu.Unlock()
}

// publishState tells the app the session's state; without an app, nothing.
func (l *phoneLink) publishState() {
	if l.srv != nil {
		_ = l.srv.Publish(bridge.TypeSessionState, l.state())
	}
}

func (l *phoneLink) publishEvent(typ string, payload any) {
	if l.srv != nil {
		_ = l.srv.Publish(typ, payload)
	}
}

func (l *phoneLink) configDir() string {
	a := &l.adapter
	a.mu.Lock()
	d := a.cfgDir
	a.mu.Unlock()
	if d != "" {
		return d
	}
	d, _ = claudecode.ConfigDir(os.Getenv, os.UserHomeDir)
	return d
}

// onSessionStart starts following the transcript (spec M4 §4.1).
func (l *phoneLink) onSessionStart(in claudecode.HookInput) {
	path, err := claudecode.ValidTranscript(in.TranscriptPath, l.configDir())
	if err != nil {
		fmt.Fprintln(l.log, l.lang.T("phone_agent.claude_code_transcript_ignored", err))
		return
	}
	tl := claudecode.NewTailer(path, in.Source == "resume" || in.Source == "compact")
	tl.Parser().SetLang(l.lang)
	ctx, cancel := context.WithCancel(context.Background())
	a := &l.adapter
	a.mu.Lock()
	old := a.cancel
	a.cancel, a.tail = cancel, tl
	a.active, a.degraded = true, false
	a.mu.Unlock()
	if old != nil {
		old()
	}
	l.publishState()
	// agentPoll is read here, before the goroutine, for the same reason as
	// tunnelCheck in startPhoneLink: a test that sets it must not race with a
	// goroutine that has not run yet when the test ends.
	go l.tailLoop(ctx, tl, time.Now(), agentPoll, startGrace)
}

func (l *phoneLink) onSessionEnd() {
	a := &l.adapter
	a.mu.Lock()
	cancel := a.cancel
	a.cancel, a.tail = nil, nil
	a.active, a.hookState, a.attention, a.held = false, "", time.Time{}, false
	a.unbound = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	l.resolveAllPerms("agent")
	l.publishState()
}

func (l *phoneLink) tailLoop(ctx context.Context, tl *claudecode.Tailer, started time.Time, poll, grace time.Duration) {
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if !l.pollTranscript(ctx, tl, started, grace) {
			return
		}
	}
}

// pollTranscript reads what the transcript gained and shows it. false when
// the chat degraded and the reading is over.
func (l *phoneLink) pollTranscript(ctx context.Context, tl *claudecode.Tailer, started time.Time, grace time.Duration) bool {
	a := &l.adapter
	a.pollMu.Lock()
	defer a.pollMu.Unlock()
	if ctx.Err() != nil {
		return false
	}
	events, err := tl.Poll()
	p := tl.Parser()
	switch {
	case err != nil:
		l.degrade(i18n.M("phone_agent.degrade.unreadable"))
		return false
	case !tl.Started() && time.Since(started) > grace:
		l.degrade(i18n.M("phone_agent.degrade.missing"))
		return false
	case p.BadRun() >= badLines:
		l.degrade(i18n.M("phone_agent.degrade.format"))
		return false
	case p.BadVersion():
		l.degrade(i18n.M("phone_agent.degrade.untested_version")) // never the text of the transcript
		return false
	case !claudecode.Supported(p.Version()):
		l.degrade(i18n.M("phone_agent.degrade.untested", p.Version()))
		return false
	}
	// droppedSeen belongs to a.mu like the rest of the state (pollMu is taken
	// before a.mu, never the other way round).
	a.mu.Lock()
	n, fresh := p.Dropped(), false
	if n > a.droppedSeen {
		a.droppedSeen, fresh = n, true
	}
	a.mu.Unlock()
	if fresh {
		fmt.Fprintln(l.log, l.lang.T("phone_agent.claude_code_chat_answer_s", n))
	}
	for _, e := range events {
		l.apply(e)
	}
	return true
}

// degrade puts the chat on its fallback (spec M4 §4.4): the terminal view
// and the last answer of each turn. It says so once.
func (l *phoneLink) degrade(why i18n.Msg) {
	a := &l.adapter
	a.mu.Lock()
	already := a.degraded
	a.degraded = true
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if already {
		return
	}
	fmt.Fprintln(l.log, l.lang.T("phone_agent.claude_code_chat_view_off", why))
	l.publishEvent(bridge.TypeNotice, bridge.Notice{Level: "info", Text: l.lang.T("notice.chat_off", why)})
	l.publishState()
}

// apply shows one event of the transcript.
func (l *phoneLink) apply(e claudecode.Event) {
	a := &l.adapter
	conv := l.currentConversation()
	switch e.Kind {
	case claudecode.KindMessage:
		norm := claudecode.Normalize(e.Text)
		if e.Role == "user" {
			if l.wasSent(e.Text) {
				return
			}
			id := session.NewID("t_")
			if e.MessageID != "" {
				id = "t_" + e.MessageID // the same on every read of the transcript (spec M4 §4.2)
			}
			l.addMessage(bridge.ChatMessage{ID: id, Role: "user", Text: e.Text, Conversation: conv})
			return
		}
		a.mu.Lock()
		if a.fallbacks[norm] { // the Stop fallback already said it
			delete(a.fallbacks, norm)
			a.mu.Unlock()
			return
		}
		a.lastText = norm
		a.mu.Unlock()
		l.addMessage(bridge.ChatMessage{ID: "a_" + e.MessageID, Role: "assistant", Text: e.Text, Conversation: conv})
	case claudecode.KindTool:
		of := ""
		if e.MessageOf != "" {
			of = "a_" + e.MessageOf // the same namespace as the message it hangs from
		}
		l.publishEvent(bridge.TypeToolActivity, bridge.ToolActivity{ID: e.ToolID, MessageID: of, Summary: e.Summary, Status: e.Status})
		if e.Status == "running" {
			l.toolStarted(e.ToolID, e.ToolKey)
		} else {
			// The tool that asked ended: it was answered on the terminal
			// (spec M4 §5.2).
			l.toolEnded(e.ToolID)
		}
	}
}

func (l *phoneLink) currentConversation() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sess.Conversation
}

// afterStop makes sure the last answer of the turn is in the chat (spec M4
// §4.4): it rereads the transcript a few times and, if the text of Stop is
// still not there, says it itself.
func (l *phoneLink) afterStop(in claudecode.HookInput, delay time.Duration) {
	want := claudecode.Normalize(in.LastAssistantMessage)
	if want == "" {
		return
	}
	a := &l.adapter
	for range stopTries {
		a.mu.Lock()
		live, tl := a.active && !a.degraded, a.tail
		a.mu.Unlock()
		if !live || tl == nil {
			break
		}
		time.Sleep(delay)
		a.pollMu.Lock()
		events, _ := tl.Poll()
		for _, e := range events {
			l.apply(e)
		}
		a.pollMu.Unlock()
		a.mu.Lock()
		got := strings.HasSuffix(a.lastText, want)
		a.mu.Unlock()
		if got {
			return
		}
	}
	a.mu.Lock()
	if a.fallbacks == nil || len(a.fallbacks) >= 16 {
		a.fallbacks = map[string]bool{}
	}
	a.fallbacks[want] = true
	a.mu.Unlock()
	id := "last_" + in.PromptID
	if !claudecode.ValidID(in.PromptID) {
		id = session.NewID("last_")
	}
	l.addMessage(bridge.ChatMessage{ID: id, Role: "assistant", Text: in.LastAssistantMessage, Conversation: l.currentConversation()})
}
