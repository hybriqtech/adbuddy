package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

func rtlIn(s string) bool { return strings.IndexFunc(s, classify.RTL) >= 0 }

// bidiSurfaces is a Confirmer that keeps the request the daemon built (the
// original) and shows it on every surface a person decides on: the question
// of the terminal, the one published to the app and the one a snapshot lists.
type bidiSurfaces struct {
	asked    []confirm.Request
	terminal string
	app      []bridge.ConfirmRequest
	pending  []bridge.ConfirmRequest
	decision confirm.Decision
}

type pubFunc struct{ f func(typ string, payload any) }

func (p pubFunc) Connected() bool                       { return true }
func (p pubFunc) Publish(typ string, payload any) error { p.f(typ, payload); return nil }

func (b *bidiSurfaces) Confirm(_ context.Context, r confirm.Request) confirm.Decision {
	b.asked = append(b.asked, r)
	short := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 250*time.Millisecond)
	}
	var out bytes.Buffer
	ctx, cancel := short()
	confirm.NewTerminal(strings.NewReader(""), &out).WithLang(i18n.ES).Confirm(ctx, r)
	cancel()
	b.terminal += out.String()

	var c *bridge.Confirmer
	c = bridge.NewConfirmer(pubFunc{func(typ string, payload any) {
		if q, ok := payload.(bridge.ConfirmRequest); ok {
			b.app = append(b.app, q)
			b.pending = append(b.pending, c.Pending()...)
		}
	}}, confirm.DenyAll{})
	ctx, cancel = short()
	c.Confirm(ctx, r)
	cancel()
	return b.decision
}

func (b *bidiSurfaces) noRTL(t *testing.T) {
	t.Helper()
	if len(b.asked) != 1 || len(b.app) != 1 || len(b.pending) != 1 || b.terminal == "" {
		t.Fatalf("surfaces not all reached: asked %d app %d pending %d terminal %q", len(b.asked), len(b.app), len(b.pending), b.terminal)
	}
	for name, v := range map[string]any{"app": b.app, "pending": b.pending} {
		wire, _ := json.Marshal(v)
		if rtlIn(string(wire)) {
			t.Errorf("%s shows RTL: %s", name, wire)
		}
	}
	if rtlIn(b.terminal) {
		t.Errorf("terminal shows RTL:\n%s", b.terminal)
	}
}

func TestRedCommandWithRTLShowsNoRTLAnywhere(t *testing.T) {
	e := newEnv(t)
	b := &bidiSurfaces{}
	e.d.o.Confirmer = b
	e.exec(t, "shell", "settings", "delete", "global", "א\nب")
	b.noRTL(t)
	// The daemon does not escape: what it asked keeps the original title, and
	// the journal keeps the description as the rules wrote it.
	if got := e.last(t).Description; got != b.asked[0].Title {
		t.Errorf("journal %q, question %q", got, b.asked[0].Title)
	}
	if rtlIn(b.asked[0].RawCommand) {
		t.Errorf("RawCommand from DisplayCommand has RTL: %q", b.asked[0].RawCommand)
	}
}

// An undo question is built by classify and shown through the same Shown.
func TestUndoQuestionShowsNoRTLAnywhere(t *testing.T) {
	e := newEnv(t)
	b := &bidiSurfaces{}
	e.d.o.Confirmer = b
	e.forge(t, "j_f1", putArgv, "shell", "settings", "delete", "global", "a")
	e.undoLast(t)
	b.noRTL(t)
	if !strings.Contains(b.asked[0].RawCommand, "settings delete global a") {
		t.Errorf("the undo question does not show its command: %q", b.asked[0].RawCommand)
	}
}

// precheck writes the inverse of a push as `rm -f` with the path quoted by
// shq.Quote, and validInverse accepts any byte inside single quotes: an undo
// of a file with a Hebrew name is a real 🔴 question with RTL letters in its
// command. It is asked, and it shows none on any surface.
func TestUndoOfAFileWithRTLNameShowsNoRTLAnywhere(t *testing.T) {
	e := newEnv(t)
	b := &bidiSurfaces{}
	e.d.o.Confirmer = b
	remote := "/sdcard/Download/א"
	e.forge(t, "j_f1", []string{"push", "x", remote}, "shell", "rm", "-f", shq.Quote(remote))
	e.undoLast(t)
	b.noRTL(t)
	if !strings.Contains(b.asked[0].RawCommand, string(rune(92))+"u05d0") {
		t.Errorf("the escaped command is not shown: %q", b.asked[0].RawCommand)
	}
}

// The notice that a question waits for a change in flight names that question.
func TestWaitingNoticeShowsNoRTL(t *testing.T) {
	e := newEnv(t)
	e.d.o.OnEntry = func(journal.Entry) {}
	g := newGated(e.adb, "input tap")
	e.d.o.ADB = g
	held := newHeld()
	e.d.o.Confirmer = held
	n := &notices{}
	e.d.o.OnNotice = n.add
	yellow := e.execAsync("shell", "input tap 1 1")
	g.waitStarted(t)
	red := e.execAsync("shell", "settings", "delete", "global", "א")
	if !n.waitFor("Waiting for") {
		t.Fatalf("not said: %q", n.get())
	}
	for _, s := range n.get() {
		if rtlIn(s) {
			t.Errorf("notice shows RTL: %q", s)
		}
	}
	close(g.open)
	waitResult(t, yellow)
	asked(t, held)
	held.answer <- confirm.Deny
	waitResult(t, red)
}

// DisplayCommand escapes, but the text next to it (Title, Detail) only gets
// escaped by Request.Shown in each Confirmer. A new use of DisplayCommand must
// be reviewed for that: it fails here until it is added to the list.
func TestDisplayCommandIsUsedOnlyWhereTheQuestionIsBuilt(t *testing.T) {
	allowed := map[string]int{"internal/daemon/daemon.go": 1, "internal/daemon/undo.go": 1}
	got := map[string]int{}
	err := filepath.Walk("../..", func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if n := strings.Count(string(src), "DisplayCommand("); n > 0 {
			rel, _ := filepath.Rel("../..", p)
			got[filepath.ToSlash(rel)] = n
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	delete(got, "internal/classify/display.go") // its definition
	if len(got) != len(allowed) {
		t.Fatalf("uses of DisplayCommand: %v, allowed %v: its result must reach a person only through confirm.Request.Shown", got, allowed)
	}
	for f, n := range allowed {
		if got[f] != n {
			t.Fatalf("uses of DisplayCommand: %v, allowed %v", got, allowed)
		}
	}
}
