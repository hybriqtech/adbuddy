package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

func TestServeRefusesTheAgentSession(t *testing.T) {
	t.Setenv("ADBUDDY_SOURCE", "agent")
	_, errOut, code := run(t, "serve", "--adb", "adb", "--confirm", "terminal")
	if code != ExitFailure || !strings.Contains(errOut, "AGENT_SESSION") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestServeNeedsAnAdb(t *testing.T) {
	t.Setenv("ADBUDDY_SOURCE", "")
	t.Setenv("ADBUDDY_CONFIG_DIR", t.TempDir())
	t.Setenv("ADBUDDY_DATA_DIR", t.TempDir())
	_, errOut, code := run(t, "serve")
	if code != ExitConfig || !strings.Contains(errOut, "ADB_NOT_CONFIGURED") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestUndoAndRunWithoutHost(t *testing.T) {
	t.Setenv("ADBUDDY_CONFIG_DIR", t.TempDir())
	t.Setenv("ADBUDDY_DATA_DIR", t.TempDir())
	for _, args := range [][]string{{"undo"}, {"run", "claude"}} {
		_, errOut, code := run(t, args...)
		if code != ExitFailure || !strings.Contains(errOut, "DAEMON_NOT_RUNNING") || !strings.Contains(errOut, "adbuddy serve") {
			t.Fatalf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
}

func TestJournalEmptyJSON(t *testing.T) {
	t.Setenv("ADBUDDY_CONFIG_DIR", t.TempDir())
	t.Setenv("ADBUDDY_DATA_DIR", t.TempDir())
	out, _, code := run(t, "journal", "--json", "--workspace", t.TempDir())
	if code != 0 || strings.TrimSpace(out) != `{"ok":true,"data":{"entries":[],"skippedLines":0}}` {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
}

func TestUndoneLogPath(t *testing.T) {
	data := t.TempDir()
	a := undoneLogPath(data, filepath.Join(t.TempDir(), "journal"))
	b := undoneLogPath(data, filepath.Join(t.TempDir(), "journal"))
	if filepath.Dir(a) != data || !strings.HasPrefix(filepath.Base(a), "undone-") || !strings.HasSuffix(a, ".log") || len(filepath.Base(a)) != len("undone-")+16+len(".log") {
		t.Fatalf("undone log %q", a)
	}
	if a == b {
		t.Fatalf("two journals share %q", a)
	}
}

// A forged line cannot hide its own warning with a carriage return or an
// escape sequence in the id or the status.
func TestJournalEscapesForgedIDAndStatus(t *testing.T) {
	data := isolate(t)
	if _, err := ipc.EnsureSecret(data); err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	appendEntries(t, ws, nil, journal.Entry{
		ID:     "j_X\r\x1b[2K12:00:00 ok  Disable Facebook  [j_01GENUINE] adb shell pm disable-user com.facebook.katana",
		Status: "ok\x1b[2K\rFAKE", Level: "yellow", Description: "Disable Facebook", Argv: []string{"shell", "pm", "disable-user", "com.facebook.katana"},
	})
	out, errOut, code := run(t, "journal", "--workspace", ws)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if strings.ContainsAny(out, "\r\x1b") || !strings.Contains(out, "not written by ADBuddy") {
		t.Fatalf("stdout %q", out)
	}
}

// journalVerified returns the "verified" field of each entry of journal --json, by id.
func journalVerified(t *testing.T, out string) map[string]any {
	t.Helper()
	var env struct {
		OK   bool
		Data struct{ Entries []map[string]any }
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || !env.OK {
		t.Fatalf("stdout %q: %v", out, err)
	}
	verified := map[string]any{}
	for _, e := range env.Data.Entries {
		verified[e["id"].(string)] = e["verified"]
	}
	return verified
}

// --json says which entries carry a good signature, and a missing secret is
// reported instead of silently skipping the check.
func TestJournalVerified(t *testing.T) {
	data := isolate(t)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	appendEntries(t, ws, []byte(secret), journal.Entry{ID: "j_good", Status: "ok", Level: "green", Argv: []string{"devices"}})
	appendEntries(t, ws, nil, journal.Entry{ID: "j_forged", Status: "ok", Level: "green", Argv: []string{"devices"}})

	out, errOut, code := run(t, "journal", "--json", "--workspace", ws)
	got := journalVerified(t, out)
	if code != 0 || got["j_good"] != true || got["j_forged"] != false || strings.Contains(errOut, "could not be verified") {
		t.Fatalf("exit %d, verified %v, stderr %q", code, got, errOut)
	}

	if err := os.Remove(ipc.SecretPath(data)); err != nil {
		t.Fatal(err)
	}
	const warning = "⚠ the journal signatures could not be verified (no ipc.secret)"
	out, errOut, code = run(t, "journal", "--json", "--workspace", ws)
	got = journalVerified(t, out)
	if code != 0 || got["j_good"] != false || got["j_forged"] != false || !strings.Contains(errOut, warning) {
		t.Fatalf("no secret, json: exit %d, verified %v, stderr %q", code, got, errOut)
	}
	if _, errOut, code := run(t, "journal", "--workspace", ws); code != 0 || !strings.Contains(errOut, warning) {
		t.Fatalf("no secret, human: exit %d, stderr %q", code, errOut)
	}
	if _, errOut, _ := run(t, "journal", "--workspace", t.TempDir()); strings.Contains(errOut, warning) {
		t.Fatalf("warning with an empty journal: %q", errOut)
	}
}

func TestJournalSession(t *testing.T) {
	isolate(t)
	ws := t.TempDir()
	appendEntries(t, ws, nil,
		journal.Entry{ID: "j_1", Session: "s_X", Status: "ok", Level: "green"},
		journal.Entry{ID: "j_2", Session: "s_Y", Status: "ok", Level: "green"},
		journal.Entry{ID: "j_3", Session: "s_X", Status: "ok", Level: "green"},
	)
	out, errOut, code := run(t, "journal", "--session", "s_X", "--json", "--workspace", ws)
	got := journalVerified(t, out)
	if _, ok1 := got["j_1"]; code != 0 || len(got) != 2 || !ok1 {
		t.Fatalf("exit %d, entries %v, stderr %q", code, got, errOut)
	}
	if _, ok3 := got["j_3"]; !ok3 {
		t.Fatalf("entries %v", got)
	}
	if _, errOut, code := run(t, "journal", "--session", "current", "--workspace", ws); code != ExitFailure || !strings.Contains(errOut, "DAEMON_NOT_RUNNING") {
		t.Fatalf("--session current without host: exit %d, stderr %q", code, errOut)
	}
	if _, errOut, code := run(t, "journal", "--workspace", ws, "--session"); code != ExitUsage {
		t.Fatalf("--session without a value: exit %d, stderr %q", code, errOut)
	}
}

func TestJournalNegativeLimit(t *testing.T) {
	isolate(t)
	if _, errOut, code := run(t, "journal", "--limit", "-1", "--workspace", t.TempDir()); code != ExitUsage || !strings.Contains(errOut, "USAGE") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

// ALREADY_RUNNING is found before touching the workspace or waiting for the phone.
func TestServeAlreadyRunning(t *testing.T) {
	data := isolate(t)
	fakeDaemon(t, data, nil, ipc.Exit{})
	ws := filepath.Join(t.TempDir(), "ws")
	_, errOut, code := run(t, "serve", "--adb", filepath.Join(t.TempDir(), "no-adb"), "--workspace", ws)
	if code != ExitFailure || !strings.Contains(errOut, "ALREADY_RUNNING") || exists(ws) {
		t.Fatalf("exit %d, workspace created %v, stderr %q", code, exists(ws), errOut)
	}
}

// Ctrl+C while waiting for the phone is not an adb failure.
func TestServeInterruptedWhileWaitingForThePhone(t *testing.T) {
	isolate(t)
	t.Setenv("FAKEADB_SCRIPT", "") // no rules: `adb devices` lists nothing
	t.Setenv("FAKEADB_LOG", "")
	adb := fakeADB(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	_, errOut, code := runCtx(t, ctx, "serve", "--adb", adb, "--workspace", filepath.Join(t.TempDir(), "ws"), "--confirm", "deny")
	if code != 130 || !strings.Contains(errOut, "INTERRUPTED") || strings.Contains(errOut, "ADB_FAILED") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

// serveUntil returns only once the requests in flight have finished, even
// when a handler needs a while after the stop to write its journal entry.
func TestServeUntilWaitsForHandlers(t *testing.T) {
	data := shortDir(t)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var finished atomic.Bool
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		close(started)
		<-ctx.Done()
		time.Sleep(300 * time.Millisecond) // e.g. journaling the denial
		finished.Store(true)
		return ipc.Exit{Code: 126}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	served := make(chan error, 1)
	go func() { served <- serveUntil(ctx, stop, srv) }()
	called := make(chan ipc.Exit, 1)
	go func() {
		exit, _ := ipc.Call(context.Background(), data, ipc.Request{Kind: ipc.KindExec}, nil, io.Discard, io.Discard, nil)
		called <- exit
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the request never reached the handler")
	}
	stop()
	if err := <-served; err != nil || !finished.Load() {
		t.Fatalf("serveUntil returned (%v) before the handler finished", err)
	}
	if exit := <-called; exit.Code != 126 {
		t.Fatalf("client got %+v", exit)
	}
}

// Stopping serve lets the requests in flight finish: a pending 🔴 is
// journaled as denied and the shim gets its answer.
func TestServeWaitsForRequestsInFlight(t *testing.T) {
	data := isolate(t)
	tmp := t.TempDir()
	script, err := testutil.WriteScript(tmp, testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\ndev1\tdevice\n"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKEADB_SCRIPT", script)
	t.Setenv("FAKEADB_LOG", filepath.Join(tmp, "adb.log"))
	adb := fakeADB(t)
	ws := filepath.Join(tmp, "ws")
	stdin, answers := io.Pipe() // nobody answers the confirmation
	t.Cleanup(func() { answers.Close() })
	serveErr := &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan int, 1)
	go func() {
		served <- mainContext(ctx, []string{"serve", "--adb", adb, "--serial", "dev1", "--workspace", ws, "--confirm", "terminal"}, stdin, io.Discard, serveErr)
	}()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := ipc.PingDaemon(context.Background(), data); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("serve did not start:\n%s", serveErr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	type answer struct {
		exit ipc.Exit
		err  error
	}
	called := make(chan answer, 1)
	go func() {
		req := ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "rm", "-rf", "/sdcard/DCIM"}, Cwd: tmp, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}
		exit, err := ipc.Call(context.Background(), data, req, nil, io.Discard, io.Discard, nil)
		called <- answer{exit, err}
	}()
	for !strings.Contains(serveErr.String(), "Code: ") {
		if time.Now().After(deadline) {
			t.Fatalf("no confirmation:\n%s", serveErr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case code := <-served:
		if code != 0 || !strings.Contains(serveErr.String(), "ADBuddy stopped.") {
			t.Fatalf("serve exit %d:\n%s", code, serveErr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
	}
	// Checked as soon as serve returns: nothing may still be in flight.
	entries, _, err := journal.Open(workspace.JournalDir(ws)).Load()
	if err != nil || len(entries) != 1 || entries[0].Status != journal.StatusDenied {
		t.Fatalf("journal when serve returned: %+v %v", entries, err)
	}
	select {
	case a := <-called:
		if a.err != nil || a.exit.Code != 126 {
			t.Fatalf("shim got %+v %v", a.exit, a.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the request got no answer")
	}
}
