package cli

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
)

// writeApp sends raw bytes as a text message, as the app would.
func writeApp(t *testing.T, c *websocket.Conn, data []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

// yoloRig is a link with an app connected and a YOLO state of its own.
func yoloRig(t *testing.T) (*chatRig, *daemon.Yolo) {
	t.Helper()
	r := newChatRig(t)
	y := daemon.NewYolo(nil, nil)
	r.l.useYolo(y)
	return r, y
}

func (r *chatRig) readYolo(t *testing.T) (bridge.Message, bridge.YoloState) {
	t.Helper()
	m := r.read(t, bridge.TypeYoloState)
	var st bridge.YoloState
	if err := json.Unmarshal(m.Payload, &st); err != nil {
		t.Fatal(err)
	}
	return m, st
}

func TestYoloSetOnAnswersWithTheStateAndTheRid(t *testing.T) {
	r, y := yoloRig(t)
	before := time.Now()
	sendApp(t, r.c, bridge.TypeYoloSet, "r_1", bridge.YoloSet{Minutes: 15})
	// The observer tells the change as an event, and the request gets its answer.
	m, st := r.readYolo(t)
	if m.RID != "" || !st.Active || st.Session || st.Until < before.Add(15*time.Minute).UnixMilli() || st.Until > time.Now().Add(15*time.Minute).UnixMilli() {
		t.Fatalf("event %+v %+v", m, st)
	}
	m, st = r.readYolo(t)
	if m.RID != "r_1" || !st.Active {
		t.Fatalf("answer %+v %+v", m, st)
	}
	if !y.Active() {
		t.Fatal("not on")
	}
}

func TestYoloSetSessionAndOff(t *testing.T) {
	r, y := yoloRig(t)
	sendApp(t, r.c, bridge.TypeYoloSet, "r_1", bridge.YoloSet{Session: true})
	r.readYolo(t)
	if _, st := r.readYolo(t); !st.Active || !st.Session || st.Until != 0 {
		t.Fatalf("%+v", st)
	}
	sendApp(t, r.c, bridge.TypeYoloSet, "r_2", bridge.YoloSet{Off: true})
	r.readYolo(t)
	if m, st := r.readYolo(t); m.RID != "r_2" || st.Active || y.Active() {
		t.Fatalf("%+v %+v", m, st)
	}
	// Off while off is answered too.
	sendApp(t, r.c, bridge.TypeYoloSet, "r_3", bridge.YoloSet{Off: true})
	if m, st := r.readYolo(t); m.RID != "r_3" || st.Active {
		t.Fatalf("%+v %+v", m, st)
	}
}

// Only 15, 60 and the session; exactly one thing at a time; garbage is an
// error that answers. Mutation: let any positive Minutes through (map it to a
// Duration without the switch).
func TestYoloSetRefusesWhatIsNotAnAcceptedDuration(t *testing.T) {
	r, y := yoloRig(t)
	for i, payload := range []string{
		`{"minutes":10,"session":false,"off":false}`,
		`{"minutes":120,"session":false,"off":false}`,
		`{"minutes":-15,"session":false,"off":false}`,
		`{"minutes":0,"session":false,"off":false}`,
		`{"minutes":15,"session":true,"off":false}`,
		`{"minutes":15,"session":false,"off":true}`,
		`{"minutes":0,"session":true,"off":true}`,
		`{"minutes":"15"}`,
		`[]`,
		`"on"`,
	} {
		rid := "r_" + string(rune('a'+i))
		data, _ := json.Marshal(bridge.Message{V: bridge.ProtocolVersion, Type: bridge.TypeYoloSet, RID: rid, TS: time.Now(), Payload: json.RawMessage(payload)})
		writeApp(t, r.c, data)
		r.readError(t, rid, bridge.CodeInvalid)
		if y.Active() {
			t.Fatalf("%s turned YOLO on", payload)
		}
	}
}

// yoloLink is a link with an app connected, its log, and what a second
// connection needs (port and token).
type yoloLink struct {
	l     *phoneLink
	c     *websocket.Conn
	log   *syncBuffer
	port  int
	token string
	y     *daemon.Yolo
}

func newYoloLink(t *testing.T) *yoloLink {
	t.Helper()
	e := linkEnv(t)
	f := &appPhone{}
	l, errb, err := startLink(t, e, f, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	tok := f.handedTokens()[0]
	c := openApp(t, e.cfg.ChatPort, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: tok, AppVersion: "0.3.0"})
	if m := readApp(t, c); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	y := daemon.NewYolo(nil, nil)
	l.useYolo(y)
	return &yoloLink{l: l, c: c, log: errb, port: e.cfg.ChatPort, token: tok, y: y}
}

func TestYoloSetWithNoRidIsLogged(t *testing.T) {
	r := newYoloLink(t)
	sendApp(t, r.c, bridge.TypeYoloSet, "", bridge.YoloSet{Session: true})
	// A request with no rid is dropped and logged, and so it never turns anything on.
	want := r.l.lang.T("phone.yolo_set_without_rid")
	waitUntil(t, "the log line", func() bool { return strings.Contains(r.log.String(), want) })
	if r.y.Active() {
		t.Fatal("a yolo.set with no rid turned YOLO on")
	}
}

// The app that connects learns the real state from the snapshot (or from the
// events it replays), and nothing publishes a second, older yolo.state after
// it: attach used to publish from a goroutine out of the observers' order.
// Mutation: publish yolo.state from yoloArea's attach again.
func TestANewConnectionSeesYoloOnceThroughTheSnapshot(t *testing.T) {
	r := newYoloLink(t)
	r.y.EnableSession()
	// A lastSeq the host never reached forces a snapshot.
	c := openApp(t, r.port, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: r.token, AppVersion: "0.3.0", LastSeq: 1 << 40})
	if m := readApp(t, c); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	m := readApp(t, c)
	var snap bridge.Snapshot
	if m.Type != bridge.TypeSnapshot || json.Unmarshal(m.Payload, &snap) != nil || !snap.Yolo.Active || !snap.Yolo.Session {
		t.Fatalf("snapshot %s %s", m.Type, m.Payload)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if _, data, err := c.Read(ctx); err == nil {
		t.Fatalf("a second message after the snapshot: %s", data)
	}
}

func TestYoloSetWhilePausedIsRefused(t *testing.T) {
	r, y := yoloRig(t)
	sendApp(t, r.c, bridge.TypePause, "", bridge.Empty{})
	waitUntil(t, "paused", r.l.Paused)
	sendApp(t, r.c, bridge.TypeYoloSet, "r_1", bridge.YoloSet{Minutes: 15})
	r.readError(t, "r_1", bridge.CodeFailed)
	if y.Active() {
		t.Fatal("YOLO turned on while paused")
	}
	// Off is always accepted.
	sendApp(t, r.c, bridge.TypeYoloSet, "r_2", bridge.YoloSet{Off: true})
	if m, _ := r.readYolo(t); m.RID != "r_2" {
		t.Fatalf("%+v", m)
	}
}

// The pause turns YOLO off and tells the app. Mutation: remove the Off call in
// onPauseMessage.
func TestPauseFromThePhoneTurnsYoloOff(t *testing.T) {
	r, y := yoloRig(t)
	y.EnableSession()
	if _, st := r.readYolo(t); !st.Active {
		t.Fatalf("%+v", st)
	}
	sendApp(t, r.c, bridge.TypePause, "", bridge.Empty{})
	if _, st := r.readYolo(t); st.Active {
		t.Fatalf("still on in the message: %+v", st)
	}
	if y.Active() {
		t.Fatal("YOLO survived the pause")
	}
	sendApp(t, r.c, bridge.TypeResume, "", bridge.Empty{})
	r.read(t, bridge.TypeSessionState)
	if y.Active() {
		t.Fatal("resuming brought YOLO back")
	}
}

func TestYoloExpiryIsTold(t *testing.T) {
	r := newChatRig(t)
	now := time.Now()
	var fire func()
	y := daemon.NewYolo(func() time.Time { return now }, func(d time.Duration, f func()) func() { fire = f; return func() {} })
	r.l.useYolo(y)
	if err := y.Enable(daemon.YoloShort); err != nil {
		t.Fatal(err)
	}
	if _, st := r.readYolo(t); !st.Active {
		t.Fatalf("%+v", st)
	}
	now = now.Add(15 * time.Minute)
	fire()
	if _, st := r.readYolo(t); st.Active || st.Until != 0 {
		t.Fatalf("%+v", st)
	}
}

func TestTheSnapshotCarriesYolo(t *testing.T) {
	r, y := yoloRig(t)
	y.EnableSession()
	r.readYolo(t)
	if s := r.l.snapshot(); !s.Yolo.Active || !s.Yolo.Session || s.Yolo.Until != 0 {
		t.Fatalf("snapshot %+v", s.Yolo)
	}
}

func TestWithNoYoloTheSnapshotSaysOff(t *testing.T) {
	r := newChatRig(t)
	if s := r.l.snapshot(); s.Yolo.Active {
		t.Fatalf("%+v", s.Yolo)
	}
}

// serve gives the phone link the daemon's YOLO state (the daemon is created
// after the link): without it every yolo.set would answer "unavailable" and
// the pause would not turn YOLO off. Looked up in the syntax tree, like
// TestServeWiresRemoteActive. Mutation: remove `link.useYolo(d.Yolo())` from serve.go.
func TestServeGivesTheLinkTheYoloState(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "serve.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	wired := false
	ast.Inspect(file, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || len(c.Args) != 1 {
			return true
		}
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "useYolo" {
			if arg, ok := c.Args[0].(*ast.CallExpr); ok {
				if asel, ok := arg.Fun.(*ast.SelectorExpr); ok && asel.Sel.Name == "Yolo" {
					wired = true
				}
			}
		}
		return true
	})
	if !wired {
		t.Fatal("serve.go does not give the phone link the daemon's YOLO state")
	}
}

// Nothing but the phone link turns YOLO on: the day another part of serve does
// (a command, a hook), this test names it. It parses every non-test file of
// host/ (comments do not count) and flags the names an activation needs
// (`EnableSession`, `YoloShort`, `YoloLong`) and any call to a method named
// `Enable`/`EnableSession`, which catches `y.Enable(15*time.Minute)` and
// `y.Enable(time.Hour)` too. Allowed: daemon/yolo.go, the phone link, and the
// remote mode's `Enable(ctx)` in phone_remote.go. Mutation: a real call
// `y.Enable(time.Hour)` in any other non-test file of host/.
func TestYoloIsOnlyTurnedOnByThePhoneLink(t *testing.T) {
	names := map[string]bool{"EnableSession": true, "YoloShort": true, "YoloLong": true}
	allowed := map[string]bool{"daemon/yolo.go": true, "cli/phone_yolo.go": true}
	enableOK := map[string]bool{"cli/phone_remote.go": true} // remote mode's Enable(ctx)
	root := filepath.Join("..", "..")                        // host/
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join(root, "internal"), path)
		rel = filepath.ToSlash(rel)
		if allowed[rel] {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0) // no comments
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if names[n.Name] {
					t.Errorf("%s names %s: only the phone link may turn YOLO on", rel, n.Name)
				}
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "EnableSession" || sel.Sel.Name == "Enable" && !enableOK[rel]) {
					t.Errorf("%s calls .%s(): only the phone link may turn YOLO on", rel, sel.Sel.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
