package cli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// linkedAppIn is linkedApp (phone_areas_test.go) with serve speaking lang:
// the App of the link carries it from the start, as mainContext's does.
func linkedAppIn(t *testing.T, lang i18n.Lang) (*phoneLink, *syncBuffer, func(typ, rid string) bridge.Message) {
	t.Helper()
	e := linkEnv(t)
	f := &appPhone{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var errb syncBuffer
	app := &App{Stderr: &errb, Stdout: io.Discard, Getenv: func(string) string { return "" }, Lang: lang}
	l, err := startPhoneLink(ctx, app, e, linkStore(e), nil, f, "s_1", "claude", confirm.Confirmer(&recording{}))
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	c := openApp(t, e.cfg.ChatPort, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: f.handedTokens()[0], AppVersion: "0.3.0"})
	if m := readApp(t, c); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	send := func(typ, rid string) bridge.Message {
		sendApp(t, c, typ, rid, bridge.Empty{})
		return readApp(t, c)
	}
	return l, &errb, send
}

// M6c §3 and §4.1: what the host tells the phone in a notice is in the
// language of the PC; the text of an error is English by protocol (the app
// shows its own by code).
func TestNoticeInTheLanguageOfThePCAndErrorInEnglish(t *testing.T) {
	_, _, send := linkedAppIn(t, i18n.ES)
	m := send("made.up.request", "r_1")
	var er bridge.ErrorReply
	if err := json.Unmarshal(m.Payload, &er); err != nil || m.Type != bridge.TypeError || er.Code != bridge.CodeUnsupported ||
		er.Text != "This version of the ADBuddy host does not handle made.up.request yet." {
		t.Fatalf("the error must stay English: %s %s", m.Type, m.Payload)
	}
	m = send("made.up.notice", "")
	if m.Type != bridge.TypeNotice || !strings.Contains(string(m.Payload), "Esta versión de ADBuddy en el PC todavía no gestiona made.up.notice.") {
		t.Fatalf("the notice must be in Spanish: %s %s", m.Type, m.Payload)
	}
}

func TestNoticeIsEnglishWithoutALanguage(t *testing.T) {
	_, _, send := linkedAppIn(t, "")
	m := send("made.up.notice", "")
	if !strings.Contains(string(m.Payload), "This version of the ADBuddy host does not handle made.up.notice yet.") {
		t.Fatalf("got %s", m.Payload)
	}
}

// The text of a reply to a question that is no longer pending (the confirmer's
// and the permission card's) is in the language of the PC; in English it is
// still bridge.StaleReplyText.
func TestStaleReplyUsesTheLanguage(t *testing.T) {
	l, _, _ := linkedAppIn(t, i18n.ES)
	if l.conf.StaleText != "Esa pregunta ya se respondió o pasó a la terminal del PC." {
		t.Fatalf("the confirmer's stale text: %q", l.conf.StaleText)
	}
	if got := (&phoneLink{}).lang.T("notice.stale_reply"); got != bridge.StaleReplyText {
		t.Fatalf("English must stay StaleReplyText: %q", got)
	}
}

// The permission card names the tool in the language of the PC (the verb of
// ToolSummaryIn), and the host closes the card when the transcript shows the
// end of the tool that asked by comparing that summary with the one the
// transcript's parser writes (resolvePermsOf). Both are written with the same
// language (l.lang), so the card still closes with the PC in Spanish; if one
// side kept English the card would stay open and the test would fail.
func TestPermissionCardSummaryAndTranscriptAgreeInTheLanguage(t *testing.T) {
	for _, c := range []struct {
		lang    i18n.Lang
		summary string
	}{{"", "Read b.go"}, {i18n.ES, "Leer b.go"}} {
		t.Run("lang="+string(c.lang), func(t *testing.T) {
			r := newAgentRig(t)
			r.l.lang = c.lang // before any hook: nothing reads it until the session starts
			r.hook(t, "SessionStart", map[string]any{"source": "startup"})
			in := map[string]any{"file_path": "/a/b.go"}
			r.append(t, atool("u1", "msg_1", "toolu_1", "Read", in))
			out := make(chan []byte, 1)
			p := r.payload("PermissionRequest", map[string]any{"tool_name": "Read", "tool_input": in})
			go func() { out <- r.l.hookCall(context.Background(), r.ws, "PermissionRequest", p) }()
			var req bridge.PermissionRequest
			if err := json.Unmarshal(r.readType(t, bridge.TypePermRequest).Payload, &req); err != nil {
				t.Fatal(err)
			}
			if req.Summary != c.summary || req.Detail != "/a/b.go" {
				t.Fatalf("the card says %q / %q, want %q / the whole path", req.Summary, req.Detail, c.summary)
			}
			// The end of the tool in the transcript closes the card: as answered on the terminal.
			r.append(t, aresult("u2", "toolu_1", false))
			if pr := r.resolved(t); pr.ID != req.ID || pr.By != "terminal" {
				t.Fatalf("the card was not closed by the end of its tool: %+v", pr)
			}
			if b := wait(t, out); b != nil {
				t.Fatalf("%s", b)
			}
		})
	}
}
