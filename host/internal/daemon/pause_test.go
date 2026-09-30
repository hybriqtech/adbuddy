package daemon

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// undoEntries counts the undo records in the journal.
func undoEntries(t *testing.T, e *env) int {
	t.Helper()
	all, _, err := e.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, j := range all {
		if j.Source == journal.SourceUndo {
			n++
		}
	}
	return n
}

// runningLines counts the "running" records written to the journal files.
func runningLines(t *testing.T, e *env) int {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(e.store.Dir(), "*.jsonl"))
	n := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		n += strings.Count(string(data), `"status":"running"`)
	}
	return n
}

func TestPausedRefusesEverything(t *testing.T) {
	e := newEnv(t)
	// A change with an inverse, made before pausing: undo must not touch it.
	e.adb.shell["settings get global a"] = "1\n"
	if _, exit := e.exec(t, "shell", "settings", "put", "global", "a", "2"); exit.Code != 0 {
		t.Fatalf("setup: %+v", exit)
	}
	runs := len(e.adb.ran())

	paused := true
	e.d.o.Paused = func() bool { return paused }
	s, exit := e.exec(t, "shell", "getprop", "ro.product.model")
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "paused from the phone") {
		t.Fatalf("green while paused: %+v %q", exit, s.errOut.String())
	}
	if len(e.adb.ran()) != runs {
		t.Fatalf("adb ran while paused: %v", e.adb.ran())
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || !strings.Contains(j.Reason, "paused") {
		t.Fatalf("journal %+v", j)
	}
	u := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	if u.Code != ExitDenied || u.ErrorCode != "PAUSED" {
		t.Fatalf("undo while paused: %+v", u)
	}
	if len(e.adb.ran()) != runs || undoEntries(t, e) != 0 {
		t.Fatalf("undo ran while paused: runs %q, undo entries %d", e.adb.ran(), undoEntries(t, e))
	}
	// A 🔴 while paused is not even asked.
	if _, exit := e.exec(t, "shell", "rm", "/sdcard/x"); exit.Code != ExitDenied || len(e.conf.asked) != 0 {
		t.Fatalf("red while paused: %+v asked %d", exit, len(e.conf.asked))
	}
	if len(e.adb.ran()) != runs {
		t.Fatalf("adb ran while paused: %v", e.adb.ran())
	}
	paused = false
	e.adb.out["shell getprop ro.product.model"] = "Pixel\n"
	if _, exit := e.exec(t, "shell", "getprop", "ro.product.model"); exit.Code != 0 {
		t.Fatalf("after resume: %+v", exit)
	}
}

// M1: the blocked record says what was attempted, at its real level.
func TestPausedEntrySaysWhatWasAttempted(t *testing.T) {
	e := newEnv(t)
	e.d.o.Paused = func() bool { return true }
	e.exec(t, "shell", "getprop", "ro.product.model")
	j := e.last(t)
	if j.Status != journal.StatusBlocked || j.Level != "green" || j.Rule == "" || j.Description == "" {
		t.Fatalf("paused green journaled as %+v", j)
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("adb ran while paused: %v", e.adb.ran())
	}
}

// pausingConfirmer pauses the agent while the question is on the phone, then
// the person taps Allow (or an Allow already queued arrives).
type pausingConfirmer struct {
	pause func()
	asked int
}

func (c *pausingConfirmer) Confirm(context.Context, confirm.Request) confirm.Decision {
	c.asked++
	c.pause()
	return confirm.Allow
}

func TestPauseDuringConfirmationWinsOverAllow(t *testing.T) {
	e := newEnv(t)
	paused := false
	e.d.o.Paused = func() bool { return paused }
	c := &pausingConfirmer{pause: func() { paused = true }}
	e.d.o.Confirmer = c
	s, exit := e.exec(t, "shell", "rm", "/sdcard/x")
	if c.asked != 1 {
		t.Fatalf("asked %d", c.asked)
	}
	if exit.Code != ExitDenied || !strings.Contains(s.errOut.String(), "paused from the phone") {
		t.Fatalf("allowed after pausing: %+v %q", exit, s.errOut.String())
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("adb ran after pausing: %v", e.adb.ran())
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || j.Level != "red" {
		t.Fatalf("journal %+v", j)
	}
	if n := runningLines(t, e); n != 0 {
		t.Fatalf("%d running records before the block", n)
	}
}

// pausedAfter is a Paused that turns on after n checks: it stands for a
// pause that arrives while the command waits (for the lock, for example).
func pausedAfter(n int) func() bool {
	calls := 0
	return func() bool {
		calls++
		return calls > n
	}
}

func TestPauseWhileWaitingForTheLockBlocksTheChange(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.d.o.Paused = pausedAfter(1) // free at the door, paused once it has the lock
	_, exit := e.exec(t, "shell", "settings", "put", "global", "a", "2")
	if exit.Code != ExitDenied {
		t.Fatalf("exit %+v", exit)
	}
	if len(e.adb.ran()) != 0 {
		t.Fatalf("adb (or a precheck) ran after pausing: %v", e.adb.ran())
	}
	if j := e.last(t); j.Status != journal.StatusBlocked || j.Level != "yellow" {
		t.Fatalf("journal %+v", j)
	}
	if n := runningLines(t, e); n != 0 {
		t.Fatalf("%d running records before the block", n)
	}
}

func TestUndoPausedWhileWaitingForTheLock(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	runs := len(e.adb.ran())
	e.d.o.Paused = pausedAfter(1)
	u := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	if u.ErrorCode != "PAUSED" || len(e.adb.ran()) != runs || undoEntries(t, e) != 0 {
		t.Fatalf("%+v runs %q undo entries %d", u, e.adb.ran(), undoEntries(t, e))
	}
}

// pausingADB pauses the agent right after running a command.
type pausingADB struct {
	*fakeADB
	pause func()
}

func (p *pausingADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	code, err := p.fakeADB.Run(ctx, dir, args, stdin, stdout, stderr)
	p.pause()
	return code, err
}

func TestUndoStopsWhenPausedBetweenInverses(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.adb.shell["settings get global b"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	e.exec(t, "shell", "settings", "put", "global", "b", "2")
	runs := len(e.adb.ran())
	paused := false
	e.d.o.Paused = func() bool { return paused }
	e.d.o.ADB = &pausingADB{fakeADB: e.adb, pause: func() { paused = true }}

	s := &stream{}
	u := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "session"}, s)
	if u.Code != ExitDenied || u.ErrorCode != "PAUSED" {
		t.Fatalf("%+v %q", u, s.errOut.String())
	}
	if got := e.adb.ran()[runs:]; len(got) != 1 {
		t.Fatalf("inverses run: %q", got)
	}
	if len(s.data) != 1 {
		t.Fatalf("result not sent: %d", len(s.data))
	}
	var res UndoResult
	if err := json.Unmarshal(s.data[0], &res); err != nil || len(res.Undone) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if n := undoEntries(t, e); n != 1 {
		t.Fatalf("undo entries %d", n)
	}
}

func TestConfirmationKeyIsRuleFolderAndExactArguments(t *testing.T) {
	e := newEnv(t)
	here, there := t.TempDir(), t.TempDir()
	e.execIn(t, here, "shell", "reboot")
	e.execIn(t, here, "shell", "reboot")
	e.execIn(t, here, "shell", "reboot", "bootloader")
	e.execIn(t, there, "shell", "reboot")
	if len(e.conf.asked) != 4 {
		t.Fatalf("asked %d", len(e.conf.asked))
	}
	k := func(i int) string { return e.conf.asked[i].Key }
	if k(0) == "" || !strings.HasPrefix(k(0), e.last(t).Rule+"\x00") {
		t.Fatalf("key %q", k(0))
	}
	if k(0) != k(1) {
		t.Fatalf("same command, different keys: %q %q", k(0), k(1))
	}
	if k(0) == k(2) {
		t.Fatalf("other arguments, same key: %q", k(0))
	}
	if k(0) == k(3) {
		t.Fatalf("other folder, same key: %q", k(0))
	}
}

// A 🔴 that depends on the state of the phone or of the PC may be fine the
// next time and not the time after: it is never allowed for the session.
func TestNoSessionKeyWhenTheStateMadeItRed(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Deny
	e.adb.shell["pm list packages -s com.user"] = ""
	e.exec(t, "shell", "pm", "uninstall", "--user", "0", "com.user")
	e.execIn(t, t.TempDir(), "pull", "/sdcard/Download/x", "run.txt")
	if len(e.conf.asked) != 2 {
		t.Fatalf("asked %d", len(e.conf.asked))
	}
	for i, r := range e.conf.asked {
		if r.Key != "" {
			t.Errorf("%d (%s): key %q", i, r.RawCommand, r.Key)
		}
	}
}
