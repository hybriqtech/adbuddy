package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/pty"
	"github.com/hybriqtech/adbuddy/host/internal/session"
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
	if code != 0 || strings.TrimSpace(out) != `{"ok":true,"data":{"entries":[],"skippedLines":0,"total":0,"truncated":false}}` {
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

// A socket that answers but not with a ping is not reported as another serve
// running: the error says what failed and how to clear the socket.
func TestServeDaemonUnreachable(t *testing.T) {
	data := isolate(t)
	fakeDaemon(t, data, nil, ipc.Exit{Code: 1, ErrorCode: "UNAUTHORIZED", Error: "unauthorized"})
	ws := filepath.Join(t.TempDir(), "ws")
	_, errOut, code := run(t, "serve", "--adb", filepath.Join(t.TempDir(), "no-adb"), "--workspace", ws)
	if code != ExitFailure || !strings.Contains(errOut, "DAEMON_UNREACHABLE") || !strings.Contains(errOut, "unauthorized") ||
		!strings.Contains(errOut, ipc.SocketPath(data)) || strings.Contains(errOut, "ALREADY_RUNNING") || exists(ws) {
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
		served <- mainContext(ctx, []string{"serve", "--adb", adb, "--serial", "dev1", "--workspace", ws, "--confirm", "terminal", "--no-agent"}, stdin, io.Discard, serveErr)
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
	// The lock went with the server's Close: the next serve can start.
	if lock, err := ipc.Lock(data); err != nil {
		t.Fatalf("the lock is still held after serve stopped: %v", err)
	} else {
		lock.Close()
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

// Final review of M1b, N3: Ctrl+C before serve waits for the phone (here,
// while it asks a socket that never answers whether another serve runs)
// exits with 130 too, and prepares nothing.
func TestServeInterruptedWhileStarting(t *testing.T) {
	data := isolate(t)
	if _, err := ipc.EnsureSecret(data); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", ipc.SocketPath(data))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() // never answers
		}
	}()
	ws := filepath.Join(t.TempDir(), "ws")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	start := time.Now()
	_, errOut, code := runCtx(t, ctx, "serve", "--adb", filepath.Join(t.TempDir(), "no-adb"), "--workspace", ws, "--confirm", "deny")
	if code != ExitInterrupt || !strings.Contains(errOut, "INTERRUPTED") || strings.Contains(errOut, "DAEMON_UNREACHABLE") || exists(ws) {
		t.Fatalf("exit %d, workspace created %v, stderr %q", code, exists(ws), errOut)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("took %v", d)
	}
}

// M5b: serve gives the daemon the remote mode's state, so the default
// capture is JPEG while it is on (Closing counts as on).
func TestServeWiresRemoteActive(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "41234")
	f := remoteActive(g.ctl)
	if f() {
		t.Fatal("remote mode reported on before enabling it")
	}
	if _, err := g.ctl.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !f() {
		t.Fatal("remote mode on but RemoteActive says off")
	}
	// Looked up in the syntax tree, not as text: formatting or neighbouring
	// fields can change without breaking it.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "serve.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	wired := false
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "RemoteActive" {
			if c, ok := kv.Value.(*ast.CallExpr); ok {
				if fn, ok := c.Fun.(*ast.Ident); ok && fn.Name == "remoteActive" && len(c.Args) == 1 {
					if a, ok := c.Args[0].(*ast.Ident); ok && a.Name == "ctl" {
						wired = true
					}
				}
			}
		}
		return true
	})
	if !wired {
		t.Fatal("serve.go does not give daemon.Options the remote mode's state")
	}
}

// One serve per data folder: a second one that finds the lock taken (a serve
// that has not reached its socket yet) stops with ALREADY_RUNNING before it
// touches the workspace, asks adb anything or starts its watchers.
func TestServeFindsTheLockBeforeStarting(t *testing.T) {
	data := isolate(t)
	held, err := ipc.Lock(data)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ws := filepath.Join(t.TempDir(), "ws")
	_, errOut, code := run(t, "serve", "--adb", filepath.Join(t.TempDir(), "no-adb"), "--workspace", ws, "--confirm", "deny")
	if code != ExitFailure || !strings.Contains(errOut, "ALREADY_RUNNING") || exists(ws) {
		t.Fatalf("exit %d, workspace created %v, stderr %q", code, exists(ws), errOut)
	}
}

// serve releases the lock however it ends: here adb cannot even run.
func TestServeReleasesTheLockWhenItFails(t *testing.T) {
	data := isolate(t)
	ws := filepath.Join(t.TempDir(), "ws")
	_, errOut, code := run(t, "serve", "--adb", filepath.Join(t.TempDir(), "no-adb"), "--workspace", ws, "--confirm", "deny")
	if code == 0 || strings.Contains(errOut, "ALREADY_RUNNING") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	lock, err := ipc.Lock(data)
	if err != nil {
		t.Fatalf("the lock is still held after serve ended: %v", err)
	}
	lock.Close()
}

// --model and --effort are checked before serve touches anything: a bad value
// is a usage error, and one in config.toml a configuration error.
func TestServeModelAndEffortAreChecked(t *testing.T) {
	t.Run("flags", func(t *testing.T) {
		isolate(t)
		for _, args := range [][]string{{"--model", "--x"}, {"--effort", "ultra"}, {"--effort", "INHERIT"}} {
			_, errOut, code := run(t, append([]string{"serve", "--adb", "adb"}, args...)...)
			want := "--model"
			if args[0] == "--effort" {
				want = "--effort"
			}
			if code != ExitUsage || !strings.Contains(errOut, want+" (or agent_") {
				t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
			}
		}
	})
	t.Run("config.toml", func(t *testing.T) {
		isolate(t)
		for key, data := range map[string]string{
			"agent_model":  "agent_model = \"--x\"\n",
			"agent_effort": "agent_effort = \"ultra\"\n",
		} {
			dir := t.TempDir()
			t.Setenv("ADBUDDY_CONFIG_DIR", dir)
			if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, errOut, code := run(t, "serve", "--adb", "adb")
			// config.Load already refuses it: a configuration error, naming the key.
			if code != ExitConfig || !strings.Contains(errOut, "CONFIG_INVALID") || !strings.Contains(errOut, key) {
				t.Errorf("%s: exit %d, stderr %q", key, code, errOut)
			}
		}
	})
}

// What model and effort say reaches the arguments Claude Code starts with, and
// no other agent gets them.
func TestNewSessionPassesModelAndEffort(t *testing.T) {
	origPTY, origLook := startPTY, lookAgent
	t.Cleanup(func() { startPTY, lookAgent = origPTY, origLook })
	var got []string
	lookAgent = func(name string) (string, error) { return "/bin/" + name, nil }
	startPTY = func(o pty.Options) (pty.PTY, error) {
		got = o.Args
		return nil, errors.New("not started")
	}
	app := &App{Stdout: io.Discard, Stderr: io.Discard, Getenv: func(string) string { return "" }}
	for _, c := range []struct{ agent, model, effort, want string }{
		{"claude", "opus", "low", "--model opus --effort low"},
		{"claude", "", "", "--model claude-haiku-5-5 --effort low"},
		{"claude", "inherit", "inherit", ""},
		{"claude", "inherit", "high", "--effort high"},
		{"codex", "", "", ""},
		{"codex", "opus", "", ""},
	} {
		m := newSession(app, t.TempDir(), "dev1", func(session.Info) {}, c.model, c.effort)
		got = nil
		err := m.Start(c.agent, false)
		m.Stop()
		if err == nil {
			t.Fatalf("%s: the fake PTY should have failed", c.agent)
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%s (%q, %q) started with %q, want %q", c.agent, c.model, c.effort, got, c.want)
		}
	}
}

// With --no-agent there is no agent to give them to: serve says so.
func TestServeModelWithNoAgentSaysItIsNotApplied(t *testing.T) {
	out := serveNoAgent(t, "--model", "opus")
	if !strings.Contains(out, "--model and --effort do not apply with --no-agent") {
		t.Fatalf("no notice:\n%s", out)
	}
	// Nothing set: the default model is not a setting, so no notice.
	if out := serveNoAgent(t); strings.Contains(out, "do not apply with --no-agent") {
		t.Fatalf("notice without flags:\n%s", out)
	}
	// Only inherit: it passes validation and there is nothing to say.
	if out := serveNoAgent(t, "--model", "inherit", "--effort", "inherit"); strings.Contains(out, "do not apply with --no-agent") {
		t.Fatalf("notice for inherit:\n%s", out)
	}
}

// serveNoAgent runs serve --no-agent with extra flags until it is up, stops it
// and returns what it said.
func serveNoAgent(t *testing.T, extra ...string) string {
	t.Helper()
	data := isolate(t)
	tmp := t.TempDir()
	script, err := testutil.WriteScript(tmp, testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\ndev1\tdevice\n"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKEADB_SCRIPT", script)
	t.Setenv("FAKEADB_LOG", filepath.Join(tmp, "adb.log"))
	adb := fakeADB(t)
	serveErr := &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan int, 1)
	go func() {
		args := []string{"serve", "--adb", adb, "--serial", "dev1", "--workspace", filepath.Join(tmp, "ws"), "--confirm", "terminal", "--no-agent"}
		args = append(args, extra...)
		served <- mainContext(ctx, args, strings.NewReader(""), io.Discard, serveErr)
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
	cancel()
	select {
	case code := <-served:
		if code != 0 {
			t.Fatalf("serve exit %d:\n%s", code, serveErr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
	}
	return serveErr.String()
}

func journalOf(t *testing.T, n int) string {
	t.Helper()
	isolate(t)
	ws := t.TempDir()
	var es []journal.Entry
	for i := 0; i < n; i++ {
		es = append(es, journal.Entry{ID: fmt.Sprintf("j_%03d", i), Level: "green", Status: "ok",
			Description: "d", Argv: []string{"shell", "true"}})
	}
	appendEntries(t, ws, nil, es...)
	return ws
}

func TestJournalSaysWhenItTruncates(t *testing.T) {
	ws := journalOf(t, 60)
	out, errOut, code := run(t, "journal", "--limit", "50", "--workspace", ws)
	if code != 0 || strings.Count(out, "\n") != 50 || !strings.Contains(errOut, "Showing the last 50 of 60 entries (use --limit 0 for all).") {
		t.Fatalf("exit %d, lines %d, stderr %q", code, strings.Count(out, "\n"), errOut)
	}
	out, errOut, code = run(t, "journal", "--limit", "0", "--workspace", ws)
	if code != 0 || strings.Count(out, "\n") != 60 || strings.Contains(errOut, "Showing the last") {
		t.Fatalf("exit %d, lines %d, stderr %q", code, strings.Count(out, "\n"), errOut)
	}
	out, _, _ = run(t, "journal", "--limit", "50", "--workspace", ws, "--json")
	if !strings.Contains(out, `"total":60`) || !strings.Contains(out, `"truncated":true`) {
		t.Fatalf("json %q", tail80(out))
	}
	out, _, _ = run(t, "journal", "--limit", "0", "--workspace", ws, "--json")
	if !strings.Contains(out, `"total":60`) || !strings.Contains(out, `"truncated":false`) {
		t.Fatalf("json %q", tail80(out))
	}
	t.Setenv("ADBUDDY_LANG", "es")
	_, errOut, _ = run(t, "journal", "--limit", "50", "--workspace", ws)
	if !strings.Contains(errOut, "Mostrando las últimas 50 de 60 entradas (usa --limit 0 para verlas todas).") {
		t.Fatalf("stderr %q", errOut)
	}
}

func TestServeWaitingTextIsAboutThePhone(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.EN, i18n.ES} {
		app := &App{Lang: l, Getenv: func(string) string { return "" }}
		got := app.T("serve.waiting_for_the_phone_on", 45)
		if strings.Contains(got, "adb server") || strings.Contains(got, "servidor adb") || strings.Contains(got, "5038") || !strings.Contains(got, "45 s") {
			t.Errorf("%v: %q", l, got)
		}
	}
	// Looked up in the syntax tree, not as text: the call must pass the
	// seconds of deviceWait.
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "serve.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	passes := false
	ast.Inspect(file, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || len(c.Args) != 2 {
			return true
		}
		if lit, ok := c.Args[0].(*ast.BasicLit); !ok || lit.Value != `"serve.waiting_for_the_phone_on"` {
			return true
		}
		ast.Inspect(c.Args[1], func(m ast.Node) bool {
			if id, ok := m.(*ast.Ident); ok && id.Name == "deviceWait" {
				passes = true
			}
			return true
		})
		return true
	})
	if !passes {
		t.Fatal("serve must pass the seconds of deviceWait")
	}
}

func TestSetupCodeWithoutTerminalHasItsOwnFix(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	_, errOut, code := runWithInput(t, "", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != ExitUsage || !strings.Contains(errOut, "Error: USAGE — no answer on stdin") ||
		!strings.Contains(errOut, "Run it in a terminal, or give the answers on stdin, one per line.") || strings.Contains(errOut, "--help") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

// tail80 is the last 80 bytes of s (all of it if shorter), for failure messages.
func tail80(s string) string {
	if len(s) > 80 {
		return s[len(s)-80:]
	}
	return s
}

// What serve says about the model and effort the agent starts with.
func TestStartNote(t *testing.T) {
	for _, c := range []struct {
		agent, model, effort string
		key                  string
		arg                  string
	}{
		{"claude", "", "", "serve.model_default", "model claude-haiku-5-5, effort low"},
		{"claude", "opus", "", "serve.model_default", "model opus, effort low"},
		{"claude", "", "high", "serve.model_default", "model claude-haiku-5-5, effort high"},
		{"claude", "opus", "high", "serve.model_applied", "model opus, effort high"},
		{"claude", "inherit", "inherit", "serve.model_inherited", ""},
		{"claude", "inherit", "", "serve.model_default", "effort low"},
		{"claude", "", "inherit", "serve.model_default", "model claude-haiku-5-5"},
		{"claude", "inherit", "high", "serve.model_applied", "effort high"},
		{"claude", "opus", "inherit", "serve.model_applied", "model opus"},
		{"codex", "", "", "", ""},
		{"codex", "inherit", "inherit", "", ""},
		{"codex", "opus", "", "serve.model_not_applied", "codex"},
		{"codex", "", "high", "serve.model_not_applied", "codex"},
	} {
		note := startNote(c.agent, c.model, c.effort)
		key, args := note.Key, note.Args
		arg := ""
		if len(args) > 0 {
			arg = args[0].(string)
		}
		if key != c.key || arg != c.arg {
			t.Errorf("startNote(%q, %q, %q) = %q, %q; want %q, %q", c.agent, c.model, c.effort, key, arg, c.key, c.arg)
		}
	}
}
