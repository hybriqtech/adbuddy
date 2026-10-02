package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// Limits of a chat.send (protocol spec §9).
const (
	maxChatText        = 64 << 10
	maxChatAttachments = 10
	// keptChat is how many messages the history keeps (protocol spec §4.3).
	keptChat = 100
	// maxChatSnapshot is the most the chat part of a snapshot may weigh once
	// serialized: the snapshot carries the pending 🔴 too, and the app takes
	// 1 MiB at most.
	maxChatSnapshot = 512 << 10
	// maxSentIDs is how many ids of typed messages are remembered after they
	// leave the history, so a retry is not typed twice (fixed memory, FIFO).
	maxSentIDs = 1024
)

// storedAttachment is an attachment the host keeps and where: Path is
// relative to the environment, "inbox/…" or "outbox/…", with "/".
type storedAttachment struct {
	bridge.Attachment
	Path string
}

// maxRegistered is how many attachments the registry keeps, the oldest
// forgotten first (V22): the history refers to at most keptChat messages of
// maxChatAttachments each, and as many again may wait for their message
// (uploaded or captured and not sent yet, or sent by the agent while the app
// is away). A forgotten one answers not_found to file.get and chat.send; its
// file stays on disk.
const maxRegistered = 2 * keptChat * maxChatAttachments

// attachmentRegistry knows the attachments that exist (task 8 fills it), up
// to maxRegistered.
type attachmentRegistry struct {
	mu    sync.Mutex
	m     map[string]storedAttachment
	order []string // the ids in m, oldest first
}

func (r *attachmentRegistry) put(a bridge.Attachment, path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[string]storedAttachment{}
	}
	if _, ok := r.m[a.ID]; !ok {
		r.order = append(r.order, a.ID)
	}
	r.m[a.ID] = storedAttachment{Attachment: a, Path: path}
	for len(r.order) > maxRegistered {
		delete(r.m, r.order[0])
		r.order = r.order[1:]
	}
}

func (r *attachmentRegistry) get(id string) (storedAttachment, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.m[id]
	return a, ok
}

// chatState is the chat's memory. Its mutex is never held while calling the
// session Manager or the bridge server.
type chatState struct {
	mu       sync.Mutex
	history  []bridge.ChatMessage // the last keptChat of the conversation, oldest first
	sizes    []int                // sizes[i] is the serialized weight of history[i] without thumbnails, comma included
	conv     string               // the conversation the history belongs to
	inflight map[string]bool      // ids of the chat.send being typed

	// sent are the ids already typed, across conversations: the last
	// maxSentIDs, in sentOrder (oldest first).
	sent      map[string]bool
	sentOrder []string

	attachments attachmentRegistry
}

// conversation starts the history over when another process of the agent
// begins.
func (c *chatState) conversation(conv string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resetLocked(conv)
}

func (c *chatState) resetLocked(conv string) {
	if conv != c.conv {
		c.conv = conv
		c.history, c.sizes = nil, nil
	}
}

// markSentLocked remembers that id was typed; the oldest ids leave first.
func (c *chatState) markSentLocked(id string) {
	if c.sent[id] {
		return
	}
	if c.sent == nil {
		c.sent = map[string]bool{}
	}
	c.sent[id] = true
	c.sentOrder = append(c.sentOrder, id)
	if len(c.sentOrder) > maxSentIDs {
		delete(c.sent, c.sentOrder[0])
		c.sentOrder = append([]string(nil), c.sentOrder[1:]...)
	}
}

// chatWeight is what m weighs in a snapshot: serialized, without thumbnails,
// and its comma.
func chatWeight(m bridge.ChatMessage) int {
	atts := make([]bridge.Attachment, len(m.Attachments))
	for j, a := range m.Attachments {
		a.Thumb = ""
		atts[j] = a
	}
	m.Attachments = atts
	raw, _ := json.Marshal(m)
	return len(raw) + 1
}

// upsertLocked stores m, replacing the one with its id.
func (c *chatState) upsertLocked(m bridge.ChatMessage) {
	c.resetLocked(m.Conversation)
	w := chatWeight(m)
	for i := range c.history {
		if c.history[i].ID == m.ID {
			c.history[i], c.sizes[i] = m, w
			return
		}
	}
	c.history = append(c.history, m)
	c.sizes = append(c.sizes, w)
	if len(c.history) > keptChat {
		c.history = append([]bridge.ChatMessage(nil), c.history[len(c.history)-keptChat:]...)
		c.sizes = append([]int(nil), c.sizes[len(c.sizes)-keptChat:]...)
	}
}

func (c *chatState) findLocked(id string) (bridge.ChatMessage, bool) {
	for _, m := range c.history {
		if m.ID == id {
			return m, true
		}
	}
	return bridge.ChatMessage{}, false
}

// chatSnapshot fills the chat of a snapshot: a copy of the history without
// thumbnails, and the oldest messages left out until it weighs less than
// maxChatSnapshot serialized.
func (l *phoneLink) chatSnapshot(s *bridge.Snapshot) {
	l.chat.mu.Lock()
	sizesCopy := append([]int(nil), l.chat.sizes...)
	out := make([]bridge.ChatMessage, len(l.chat.history))
	for i, m := range l.chat.history {
		atts := make([]bridge.Attachment, len(m.Attachments))
		for j, a := range m.Attachments {
			a.Thumb = ""
			atts[j] = a
		}
		m.Attachments = atts
		out[i] = m
	}
	l.chat.mu.Unlock()
	sizes := sizesCopy
	total := 2 // the brackets
	for _, n := range sizes {
		total += n
	}
	drop := 0
	for drop < len(out) && total >= maxChatSnapshot {
		total -= sizes[drop]
		drop++
	}
	s.Chat = out[drop:]
}

// addMessage keeps a message in the history and tells the app (no rid: it
// answers no request). Without a server it is only kept.
func (l *phoneLink) addMessage(m bridge.ChatMessage) {
	if m.Attachments == nil {
		m.Attachments = []bridge.Attachment{}
	}
	l.chat.mu.Lock()
	l.chat.upsertLocked(m)
	l.chat.mu.Unlock()
	if l.srv != nil {
		_ = l.srv.PublishRID(bridge.TypeChatMessage, "", m)
	}
}

func (l *phoneLink) session() *session.Manager {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.manager
}

// stateOf is the session.state of info, the Manager's own: l.state() follows
// it late (sessionInfo), and a reply must tell what the request did.
func (l *phoneLink) stateOf(info session.Info) bridge.SessionState {
	st := l.state()
	if info.Agent != "" {
		st.Agent, st.State, st.Conversation, st.PCAttached = info.Agent, info.State, info.Conversation, info.PCAttached
	}
	if st.State == session.StateStopped {
		st.Activity = ""
	}
	return st
}

// fail answers a request with an error.
func (l *phoneLink) fail(m bridge.Message, code, text string) {
	_ = l.srv.Reply(m, bridge.TypeError, bridge.ErrorReply{Code: code, Text: text})
}

// chatArea is the chat (M3, protocol spec §5-§7): session, chat.send,
// attachments, files, captures and the terminal.
func (l *phoneLink) chatArea() area {
	a := area{
		handlers: map[string]func(bridge.Message){
			bridge.TypeChatSend:    l.onChatSend,
			bridge.TypeSessionNew:  l.onSessionNew,
			bridge.TypeSessionStop: l.onSessionStop,
			bridge.TypeInterrupt:   l.onInterrupt,

			bridge.TypeTerminalWatch:   l.onTermWatch,
			bridge.TypeTerminalUnwatch: l.onTermUnwatch,
			bridge.TypeTerminalInput:   l.onTermInput,
			bridge.TypeTerminalResize:  l.onTermResize,
		},
		snapshot: l.chatSnapshot,
		attach:   func() { l.stopTerm(); l.resetFiles() },
		gone:     func() { l.stopTerm(); l.resetFiles() },
		frame:    l.onUploadFrame,
	}
	for typ, h := range l.filesHandlers() {
		a.handlers[typ] = h
	}
	return a
}

// onChatSend types a message of the person into the agent (spec M3 §5.1).
// Neither the text nor the paths go to any log or error.
func (l *phoneLink) onChatSend(m bridge.Message) {
	var cs bridge.ChatSend
	if err := json.Unmarshal(m.Payload, &cs); err != nil || !bridge.ValidRID(cs.ID) {
		l.fail(m, bridge.CodeInvalid, "The message has no valid id.")
		return
	}
	if len(cs.Text) > maxChatText || len(cs.Attachments) > maxChatAttachments {
		l.fail(m, bridge.CodeTooLarge, "The message is too large: at most 64 KiB of text and 10 attachments.")
		return
	}
	if strings.TrimSpace(session.Clean(cs.Text)) == "" && len(cs.Attachments) == 0 {
		l.fail(m, bridge.CodeInvalid, "The message is empty.")
		return
	}

	// A retry of a message already typed confirms the existing one; one
	// that is being typed now is not typed twice.
	l.chat.mu.Lock()
	if old, ok := l.chat.findLocked(cs.ID); ok {
		l.chat.mu.Unlock()
		_ = l.srv.PublishRID(bridge.TypeChatMessage, m.RID, old)
		return
	}
	if l.chat.sent[cs.ID] {
		// Typed already, but no longer in the history (another conversation,
		// or more than keptChat messages later): typing it again would
		// repeat what the person said, and its text is gone. Fail closed:
		// an error the protocol already has, and nothing enters the history.
		l.chat.mu.Unlock()
		l.fail(m, bridge.CodeInvalid, "The message has no valid id.")
		return
	}
	if l.chat.inflight[cs.ID] {
		l.chat.mu.Unlock()
		l.fail(m, bridge.CodeBusy, "This message is already being sent.")
		return
	}
	if l.chat.inflight == nil {
		l.chat.inflight = map[string]bool{}
	}
	l.chat.inflight[cs.ID] = true
	l.chat.mu.Unlock()
	release := func() {
		l.chat.mu.Lock()
		delete(l.chat.inflight, cs.ID)
		l.chat.mu.Unlock()
	}

	mgr := l.session()
	if mgr == nil {
		release()
		l.fail(m, bridge.CodeNoAgent, "The agent is not running. Start a new conversation.")
		return
	}
	atts := make([]bridge.Attachment, 0, len(cs.Attachments))
	text := cs.Text
	if len(cs.Attachments) > 0 {
		var b strings.Builder
		b.WriteString(cs.Text)
		b.WriteString("\n\nAdjuntos:")
		for _, id := range cs.Attachments {
			a, ok := l.chat.attachments.get(id)
			if !ok {
				release()
				l.fail(m, bridge.CodeNotFound, "An attachment of the message does not exist.")
				return
			}
			atts = append(atts, a.Attachment)
			b.WriteString("\n- " + a.Path)
		}
		text = b.String()
	}
	if why := l.agentBusy(); why != "" {
		release()
		l.fail(m, bridge.CodeBusy, why)
		return
	}
	if why := mgr.Busy(); why != "" {
		release()
		l.fail(m, bridge.CodeBusy, why)
		return
	}
	// The writer asks agentBusy again, and the screen, right before the
	// text and before its Enter: a question may come while it waits.
	if err := mgr.Paste(text, l.agentBusy); err != nil {
		release()
		var be *session.BusyError
		switch {
		case errors.As(err, &be):
			l.fail(m, bridge.CodeBusy, be.Why)
		case errors.Is(err, session.ErrBusy):
			l.fail(m, bridge.CodeBusy, "The agent is not reading its input. Try again in a moment.")
		case errors.Is(err, session.ErrNoAgent):
			l.fail(m, bridge.CodeNoAgent, "The agent is not running. Start a new conversation.")
		default:
			l.fail(m, bridge.CodeFailed, "Could not type the message into the agent.")
		}
		return
	}
	// From the Manager, not from l.state(), which follows it late.
	conv := mgr.Info().Conversation
	l.noteSent(cs.Text)
	msg := bridge.ChatMessage{ID: cs.ID, Role: "user", Text: cs.Text, Attachments: atts, Conversation: conv}
	l.chat.mu.Lock()
	l.chat.upsertLocked(msg)
	l.chat.markSentLocked(cs.ID)
	delete(l.chat.inflight, cs.ID)
	l.chat.mu.Unlock()
	_ = l.srv.PublishRID(bridge.TypeChatMessage, m.RID, msg)
}

// onSessionNew starts a new conversation, with another agent if asked.
func (l *phoneLink) onSessionNew(m bridge.Message) {
	var sn bridge.SessionNew
	if err := json.Unmarshal(m.Payload, &sn); err != nil {
		l.fail(m, bridge.CodeInvalid, "The request is not valid.")
		return
	}
	if sn.Agent != "" && !agent.IsKnown(sn.Agent) {
		l.fail(m, bridge.CodeInvalid, "Unknown agent.")
		return
	}
	mgr := l.session()
	if mgr == nil {
		l.fail(m, bridge.CodeFailed, "The agent's session is not available in this host.")
		return
	}
	name := sn.Agent
	if name == "" {
		name = mgr.Info().Agent
	}
	if name == "" {
		name = l.agent
	}
	if err := mgr.Start(name, false); err != nil {
		l.fail(m, bridge.CodeFailed, agent.InstallHint(name))
		return
	}
	_ = l.srv.PublishRID(bridge.TypeSessionState, m.RID, l.stateOf(mgr.Info()))
}

// onSessionStop stops the agent; the host keeps running.
func (l *phoneLink) onSessionStop(m bridge.Message) {
	st := l.state()
	if mgr := l.session(); mgr != nil {
		mgr.Stop()
		st = l.stateOf(mgr.Info())
	}
	_ = l.srv.PublishRID(bridge.TypeSessionState, m.RID, st)
}

// onInterrupt types ESC, which interrupts the agent.
func (l *phoneLink) onInterrupt(bridge.Message) {
	l.typeKeys([]byte{0x1b})
}

// typeKeys types what the phone sends (terminal.input, the interrupt's ESC)
// without waiting for the agent's terminal (V08): when the agent is not
// reading, the keys are dropped and the person is told once for each run of
// drops (session.Typer).
func (l *phoneLink) typeKeys(p []byte) {
	l.mu.Lock()
	ty := l.typer
	l.mu.Unlock()
	if ty == nil {
		return
	}
	if err := ty.Input(p); errors.Is(err, session.ErrDropped) {
		l.publishEvent(bridge.TypeNotice, bridge.Notice{Level: "info", Text: l.lang.T("notice.typing_dropped")})
	}
}
