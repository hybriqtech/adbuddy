package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// agentRig is a chat rig whose agent is Claude Code: an environment, a
// configuration directory of Claude Code with a projects/ folder, and fast
// timings.
type agentRig struct {
	*chatRig
	ws, cfg, transcript string
}

func setTiming[T any](t *testing.T, v *T, to T) {
	t.Helper()
	old := *v
	*v = to
	t.Cleanup(func() { *v = old })
}

func newAgentRig(t *testing.T) *agentRig {
	t.Helper()
	setTiming(t, &agentPoll, 15*time.Millisecond)
	setTiming(t, &stopDelay, 15*time.Millisecond)
	setTiming(t, &permGap, 0)
	r := &agentRig{chatRig: newChatRig(t), ws: t.TempDir(), cfg: t.TempDir()}
	proj := filepath.Join(r.cfg, "projects", "p1")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	r.transcript = filepath.Join(proj, "s.jsonl")
	r.l.useWorkspace(r.ws)
	r.l.adapter.cfgDir = r.cfg
	r.start(t, "claude")
	return r
}

const sid = "3ee4bd10-5ae3-4f83-b02f-1c26acf6fa35"

func (r *agentRig) payload(event string, extra map[string]any) []byte {
	m := map[string]any{"hook_event_name": event, "session_id": sid, "transcript_path": r.transcript, "cwd": r.ws}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}

func (r *agentRig) hook(t *testing.T, event string, extra map[string]any) []byte {
	t.Helper()
	return r.l.hookCall(context.Background(), r.ws, event, r.payload(event, extra))
}

// readType returns the next message of the type, skipping the others.
func (r *agentRig) readType(t *testing.T, typ string) bridge.Message {
	t.Helper()
	for range 40 {
		if m := readApp(t, r.c); m.Type == typ {
			return m
		}
	}
	t.Fatalf("no %s", typ)
	return bridge.Message{}
}

func (r *agentRig) viewIs(t *testing.T, want string) {
	t.Helper()
	waitUntil(t, "view "+want, func() bool { return r.l.state().View == want })
}

func (r *agentRig) append(t *testing.T, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(r.transcript, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

func tline(typ, uuid string, message any) string {
	b, _ := json.Marshal(map[string]any{"type": typ, "uuid": uuid, "message": message, "version": "2.1.286"})
	return string(b)
}

func atext(uuid, id, text string) string {
	return tline("assistant", uuid, map[string]any{"id": id, "role": "assistant", "content": []map[string]any{{"type": "text", "text": text}}})
}

func atool(uuid, id, toolID, name string, input map[string]any) string {
	return tline("assistant", uuid, map[string]any{"id": id, "role": "assistant", "content": []map[string]any{{"type": "tool_use", "id": toolID, "name": name, "input": input}}})
}

func aresult(uuid, toolID string, isErr bool) string {
	return tline("user", uuid, map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": toolID, "content": "SECRET", "is_error": isErr}}})
}

func TestAdapterIsOffUntilSessionStart(t *testing.T) {
	r := newAgentRig(t)
	if v := r.l.state().View; v != "terminal" {
		t.Fatalf("view %q before any hook", v)
	}
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.viewIs(t, "chat")
	r.hook(t, "UserPromptSubmit", map[string]any{"prompt": "hola"})
	waitUntil(t, "working", func() bool { return r.l.state().State == "working" })
	r.hook(t, "Stop", map[string]any{"last_assistant_message": ""})
	waitUntil(t, "idle", func() bool { return r.l.state().State == "idle" })
	r.hook(t, "SessionEnd", map[string]any{"reason": "other"})
	r.viewIs(t, "terminal")
}

func TestAdapterIgnoresHooksOfOtherPlaces(t *testing.T) {
	r := newAgentRig(t)
	other := t.TempDir()
	p := r.payload("SessionStart", map[string]any{"cwd": other})
	if out := r.l.hookCall(context.Background(), r.ws, "SessionStart", p); out != nil || r.l.state().View != "terminal" {
		t.Fatal("a hook from another folder was taken")
	}
	// A transcript outside Claude Code's projects folder is not followed.
	evil := filepath.Join(t.TempDir(), "x.jsonl")
	r.hook(t, "SessionStart", map[string]any{"transcript_path": evil})
	if r.l.state().View != "terminal" {
		t.Fatal("a transcript outside of the projects folder was followed")
	}
	// Garbage and unknown events do nothing and answer nothing.
	for _, b := range []string{"", "{", `{"session_id":"x"}`} {
		if out := r.l.hookCall(context.Background(), r.ws, "Stop", []byte(b)); out != nil {
			t.Errorf("%q answered", b)
		}
	}
	if out := r.l.hookCall(context.Background(), r.ws, "Evil", r.payload("Evil", nil)); out != nil {
		t.Error("unknown event answered")
	}
}

func TestAdapterShowsTheTranscript(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t,
		atext("u1", "msg_1", "Voy a mirar."),
		atool("u2", "msg_2", "toolu_1", "Bash", map[string]any{"command": "ls", "description": "List files"}),
		aresult("u3", "toolu_1", false),
		atext("u4", "msg_3", "Listo."),
	)
	m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage))
	if m.ID != "a_msg_1" || m.Role != "assistant" || m.Text != "Voy a mirar." || m.Conversation == "" {
		t.Fatalf("first message %+v", m)
	}
	var ta []bridge.ToolActivity
	for len(ta) < 2 {
		var a bridge.ToolActivity
		msg := r.readType(t, bridge.TypeToolActivity)
		if err := json.Unmarshal(msg.Payload, &a); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(msg.Payload), "SECRET") {
			t.Fatal("the content of a tool result went to the phone")
		}
		ta = append(ta, a)
	}
	if ta[0] != (bridge.ToolActivity{ID: "toolu_1", MessageID: "a_msg_2", Summary: "List files", Status: "running"}) ||
		ta[1] != (bridge.ToolActivity{ID: "toolu_1", MessageID: "a_msg_2", Summary: "List files", Status: "done"}) {
		t.Fatalf("activity %+v", ta)
	}
	if m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage)); m.ID != "a_msg_3" || m.Text != "Listo." {
		t.Fatalf("last message %+v", m)
	}
	// One message over several lines is one message, upserted.
	r.append(t, atext("u5", "msg_4", "uno"), atext("u6", "msg_4", "dos"))
	a := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage))
	b := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage))
	if a.ID != "a_msg_4" || b.ID != "a_msg_4" || b.Text != "uno\n\ndos" {
		t.Fatalf("%+v %+v", a, b)
	}
	if snap := r.l.snapshot(); len(snap.Chat) != 3 || snap.Chat[2].Text != "uno\n\ndos" {
		t.Fatalf("the snapshot keeps the last version: %+v", snap.Chat)
	}
}

func TestAdapterPromptsOfThePhoneAreNotRepeated(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.l.noteSent("arregla   el test")
	r.append(t,
		tline("user", "p1", map[string]any{"role": "user", "content": "arregla el test\n\nAdjuntos:\n- inbox/a.png"}),
		tline("user", "p2", map[string]any{"role": "user", "content": "esto lo escribí en el PC"}),
		tline("user", "p3", map[string]any{"role": "user", "content": "<command-name>/clear</command-name>"}),
	)
	m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage))
	if m.Role != "user" || m.Text != "esto lo escribí en el PC" || m.ID != "t_p2" {
		t.Fatalf("%+v", m)
	}
}

func TestAdapterStopFallsBackToLastAssistantMessage(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.hook(t, "Stop", map[string]any{"prompt_id": "pr1", "last_assistant_message": "Respuesta final"})
	m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage))
	if m.ID != "last_pr1" || m.Text != "Respuesta final" || m.Role != "assistant" {
		t.Fatalf("fallback %+v", m)
	}
	// The transcript catching up later does not say it twice.
	r.append(t, atext("u1", "msg_9", "Respuesta  final"), atext("u2", "msg_10", "otra cosa"))
	if m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage)); m.ID != "a_msg_10" {
		t.Fatalf("a repeat of the fallback was shown: %+v", m)
	}
}

func TestAdapterStopWithTheTextInTheTranscriptDoesNotRepeat(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t, atext("u1", "msg_1", "Hecho"))
	if m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage)); m.ID != "a_msg_1" {
		t.Fatal(m)
	}
	r.hook(t, "Stop", map[string]any{"prompt_id": "pr1", "last_assistant_message": "Hecho"})
	time.Sleep(time.Duration(stopTries+2) * stopDelay)
	if snap := r.l.snapshot(); len(snap.Chat) != 1 {
		t.Fatalf("a fallback was added: %+v", snap.Chat)
	}
}

func TestAdapterDegradesOnAnUntestedVersion(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.viewIs(t, "chat")
	r.append(t, strings.Replace(atext("u1", "msg_1", "x"), `"2.1.286"`, `"9.0.1"`, 1))
	n := r.readType(t, bridge.TypeNotice)
	if !strings.Contains(string(n.Payload), "9.0.1") {
		t.Fatalf("notice %s", n.Payload)
	}
	r.viewIs(t, "terminal")
	// From then on the answers come from Stop.
	r.hook(t, "Stop", map[string]any{"prompt_id": "pr2", "last_assistant_message": "Sigo aquí"})
	if m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage)); m.ID != "last_pr2" {
		t.Fatalf("%+v", m)
	}
}

func TestAdapterDegradesWhenTheTranscriptDoesNotAppear(t *testing.T) {
	setTiming(t, &startGrace, 50*time.Millisecond)
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.readType(t, bridge.TypeNotice)
	r.viewIs(t, "terminal")
}

func TestAdapterDegradesOnGarbage(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	lines := make([]string, badLines)
	for i := range lines {
		lines[i] = "{not json"
	}
	r.append(t, lines...)
	r.readType(t, bridge.TypeNotice)
	r.viewIs(t, "terminal")
}

func TestAdapterNewConversationStartsOver(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.viewIs(t, "chat")
	if err := r.m.Start("claude", false); err != nil { // a new process: a new conversation
		t.Fatal(err)
	}
	r.viewIs(t, "terminal")
}

func TestAgentBusyWhileAQuestionIsPending(t *testing.T) {
	r := newAgentRig(t)
	if r.l.agentBusy() != "" {
		t.Fatal("busy before there is an adapter")
	}
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	if r.l.agentBusy() != "" {
		t.Fatal("busy with nothing pending")
	}
	r.hook(t, "Notification", map[string]any{"notification_type": "permission_prompt"})
	if r.l.agentBusy() == "" {
		t.Fatal("a permission prompt on the screen did not make the chat busy")
	}
	r.hook(t, "UserPromptSubmit", map[string]any{"prompt": "x"})
	if r.l.agentBusy() != "" {
		t.Fatal("still busy after a prompt")
	}
	setTiming(t, &attentionTTL, 30*time.Millisecond)
	r.hook(t, "Notification", map[string]any{"notification_type": "permission_prompt"})
	time.Sleep(60 * time.Millisecond)
	if r.l.agentBusy() != "" {
		t.Fatal("attention does not expire")
	}
}

func TestChatSendIsBusyWhileAPermissionIsPending(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	done := make(chan []byte, 1)
	go func() {
		done <- r.hook(t, "PermissionRequest", map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "touch x"}})
	}()
	r.readType(t, bridge.TypePermRequest)
	sendApp(t, r.c, bridge.TypeChatSend, "r_1", bridge.ChatSend{ID: "c1", Text: "hola"})
	r.readError(t, "r_1", bridge.CodeBusy)
	if said := r.pty(0).said(); said != "" {
		t.Fatalf("something was typed into the dialog: %q", said)
	}
	r.l.resolveAllPerms("agent")
	<-done
}

func TestAdapterUsesRandomIdsWhenTheTranscriptIdIsNotPlain(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t, tline("user", "bad id\n‮", map[string]any{"role": "user", "content": "desde el PC"}))
	m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage))
	if !regexp.MustCompile(`^t_[0-9a-f]{16}$`).MatchString(m.ID) || m.Text != "desde el PC" {
		t.Fatalf("%+v", m)
	}
}

func TestAdapterStopIgnoresAPromptIDThatIsNotPlain(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.hook(t, "Stop", map[string]any{"prompt_id": "bad id\n", "last_assistant_message": "Respuesta final"})
	m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage))
	if !regexp.MustCompile(`^last_[0-9a-f]{16}$`).MatchString(m.ID) {
		t.Fatalf("%+v", m)
	}
}

func TestAdapterAssistantIdsHaveTheirOwnNamespace(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.append(t, atool("u1", "msg_1", "toolu_1", "Bash", map[string]any{"command": "ls", "description": "List"}))
	var a bridge.ToolActivity
	msg := r.readType(t, bridge.TypeToolActivity)
	if err := json.Unmarshal(msg.Payload, &a); err != nil {
		t.Fatal(err)
	}
	if a.MessageID != "a_msg_1" {
		t.Fatalf("%+v", a)
	}
	r.append(t, atext("u2", "msg_1", "Hola"))
	if m := chatMessageOf(t, r.readType(t, bridge.TypeChatMessage)); m.ID != "a_msg_1" {
		t.Fatalf("%+v", m)
	}
}

func TestAdapterDegradesOnAGarbageVersionWithoutCopyingIt(t *testing.T) {
	r := newAgentRig(t)
	r.hook(t, "SessionStart", map[string]any{"source": "startup"})
	r.viewIs(t, "chat")
	r.append(t, strings.Replace(atext("u1", "msg_1", "x"), `"2.1.286"`, `"9 <script>‮"`, 1))
	n := r.readType(t, bridge.TypeNotice)
	if strings.Contains(string(n.Payload), "script") || strings.Contains(string(n.Payload), "202e") || !strings.Contains(string(n.Payload), "not a tested version") {
		t.Fatalf("notice %s", n.Payload)
	}
	r.viewIs(t, "terminal")
}

func TestWasSentExpiresInTenMinutesIsBoundedAndIsConsumed(t *testing.T) {
	r := newAgentRig(t)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	setTiming(t, &agentNow, func() time.Time { return now })
	l := r.l

	l.noteSent("arregla el test")
	now = now.Add(5 * time.Minute)
	if !l.wasSent("arregla  el test\n\nAdjuntos:\n- inbox/a.png") {
		t.Fatal("a prompt of 5 minutes ago was not known")
	}
	if l.wasSent("arregla el test") {
		t.Fatal("one chat.send hid two prompts: it was not consumed on matching")
	}

	l.noteSent("otra cosa")
	now = now.Add(sentTTL + time.Minute)
	if l.wasSent("otra cosa") {
		t.Fatal("a prompt older than sentTTL was taken for the phone's")
	}
	if sentTTL != 10*time.Minute {
		t.Fatalf("sentTTL %v", sentTTL)
	}

	for i := 0; i < keepSent+10; i++ {
		l.noteSent(fmt.Sprintf("texto %d", i))
	}
	a := &l.adapter
	a.mu.Lock()
	n := len(a.sent)
	a.mu.Unlock()
	if n != keepSent {
		t.Fatalf("%d kept, want %d", n, keepSent)
	}
	if l.wasSent("texto 0") || !l.wasSent(fmt.Sprintf("texto %d", keepSent+9)) {
		t.Fatal("the oldest ones are the ones that leave")
	}

	l.noteSent("pendiente")
	l.adapterConversation("another", false)
	if l.wasSent("pendiente") {
		t.Fatal("the sent texts survived the change of conversation")
	}
}
