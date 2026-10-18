package cli

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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

func TestYoloSetWithNoRidIsLogged(t *testing.T) {
	r, y := yoloRig(t)
	sendApp(t, r.c, bridge.TypeYoloSet, "", bridge.YoloSet{Session: true})
	// A request with no rid is dropped, and so it never turns anything on.
	time.Sleep(200 * time.Millisecond)
	if y.Active() {
		t.Fatal("a yolo.set with no rid turned YOLO on")
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

func TestTheSnapshotAndTheAttachCarryYolo(t *testing.T) {
	r, y := yoloRig(t)
	y.EnableSession()
	r.readYolo(t)
	if s := r.l.snapshot(); !s.Yolo.Active || !s.Yolo.Session || s.Yolo.Until != 0 {
		t.Fatalf("snapshot %+v", s.Yolo)
	}
	r.l.yoloArea().attach()
	if _, st := r.readYolo(t); !st.Active || !st.Session {
		t.Fatalf("attach %+v", st)
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
// (a command, a hook), this test names it. It looks for the names an activation needs
// (`EnableSession`, `YoloShort`, `YoloLong`), not `.Enable(`, which the remote mode has too: a
// call like `y.Enable(15 * time.Minute)` written elsewhere would not be seen (the reviewers
// look for new callers of daemon.Yolo's Enable). Mutation: add
// `daemon.YoloShort` to any other non-test file of host/.
func TestYoloIsOnlyTurnedOnByThePhoneLink(t *testing.T) {
	allowed := map[string]bool{"daemon/yolo.go": true, "cli/phone_yolo.go": true}
	re := regexp.MustCompile(`\bEnableSession\b|\bYoloShort\b|\bYoloLong\b`)
	root := filepath.Join("..", "..") // host/
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join(root, "internal"), path)
		if allowed[filepath.ToSlash(rel)] {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if loc := re.FindIndex(b); loc != nil {
			t.Errorf("%s turns YOLO on (or names its durations): only the phone link may", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
