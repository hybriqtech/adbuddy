package cli

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// recordingExec is the daemon's adb: it records every call (Shell for the
// guard, Run for the command) and runs nothing.
type recordingExec struct {
	mu    sync.Mutex
	calls []string
}

func (x *recordingExec) note(s string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.calls = append(x.calls, s)
}

func (x *recordingExec) called() []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	return append([]string(nil), x.calls...)
}

func (x *recordingExec) Shell(_ context.Context, command string) (string, int, error) {
	x.note("shell " + command)
	return "", 1, io.ErrUnexpectedEOF
}

func (x *recordingExec) Pull(context.Context, string, string) error {
	x.note("pull")
	return io.ErrUnexpectedEOF
}

func (x *recordingExec) Run(_ context.Context, _ string, args []string, _ io.Reader, _, _ io.Writer) (int, error) {
	x.note("run " + strings.Join(args, " "))
	return 0, nil
}

// remoteDaemon is serve's daemon for the link l: it asks with l's confirmer
// (read at each question, so a test may swap it) and is paused with l.
func remoteDaemon(t *testing.T, l *phoneLink, x daemon.Executor) *daemon.Daemon {
	t.Helper()
	c, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if x == nil {
		x = &recordingExec{}
	}
	return daemon.New(daemon.Options{
		Classifier: c, ADB: x, Journal: journal.Open(filepath.Join(t.TempDir(), "journal")),
		Confirmer: confirmerFunc(func(ctx context.Context, r confirm.Request) confirm.Decision {
			return l.Confirmer().Confirm(ctx, r)
		}),
		Paused: l.Paused, Serial: "dev1", Session: "s_1", Lang: "en", Workspace: t.TempDir(),
	})
}

// Final review C1: while the question of `remote on` is open, the agent's
// commands are refused like during any 🔴 question: an `input tap` could
// press Allow on the phone's notification itself.
func TestRemoteOnQuestionBlocksTheAgentsCommands(t *testing.T) {
	data := isolate(t)
	g := newRemoteRig(t, withTailscale(), "41234")
	entered, answer := make(chan struct{}, 4), make(chan confirm.Decision)
	l := &phoneLink{remote: g.r, log: g.log}
	l.conf = bridge.NewConfirmer(noApp{}, confirmerFunc(func(ctx context.Context, _ confirm.Request) confirm.Decision {
		entered <- struct{}{}
		select {
		case d := <-answer:
			return d
		case <-ctx.Done():
			return confirm.Deny
		}
	}))
	x := &recordingExec{}
	d := remoteDaemon(t, l, x)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, serveHandler(l, withTailscale(), func(string) string { return "" }, t.TempDir(), "claude", nil, d))
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	agent := func(argv ...string) (ipc.Exit, string) {
		t.Helper()
		var errb strings.Builder
		exit, err := ipc.Call(context.Background(), data, ipc.Request{Kind: ipc.KindExec, Argv: argv, Cwd: t.TempDir(), Env: map[string]string{"ADBUDDY_SOURCE": "agent"}},
			nil, nopWriter{}, &errb, nil)
		if err != nil {
			t.Fatal(err)
		}
		return exit, errb.String()
	}
	on := func() chan ipc.Exit {
		ch := make(chan ipc.Exit, 1)
		go func() {
			exit, err := ipc.Call(context.Background(), data, ipc.Request{Kind: ipc.KindRemote, Argv: []string{"on"}}, nil, nopWriter{}, nopWriter{}, nil)
			if err != nil {
				t.Error(err)
			}
			ch <- exit
		}()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("remote on did not ask")
		}
		return ch
	}
	wantBlocked := `Blocked by ADBuddy: waiting for your answer to "Open adb's network port 5555 for 12 hours"`

	for _, answerWith := range []confirm.Decision{confirm.Deny, confirm.Allow} {
		done := on()
		for _, argv := range [][]string{{"shell", "input", "tap", "1", "1"}, {"shell", "reboot"}} {
			exit, stderr := agent(argv...)
			if exit.Code != daemon.ExitDenied || strings.TrimSpace(stderr) != wantBlocked {
				t.Fatalf("%v while remote on asks: %+v %q", argv, exit, stderr)
			}
		}
		if calls := x.called(); len(calls) != 0 {
			t.Fatalf("the daemon's adb was used while remote on asked: %q", calls)
		}
		answer <- answerWith
		exit := <-done
		if (answerWith == confirm.Allow) != (exit.Code == 0) {
			t.Fatalf("answered %v: %+v", answerWith, exit)
		}
		// Answered: the agent's commands get past the door again (the
		// guard then asks this fake adb, and refuses on its own).
		if _, stderr := agent("shell", "input", "tap", "1", "1"); strings.Contains(stderr, "waiting for your answer") || len(x.called()) == 0 {
			t.Fatalf("after %v: %q, calls %q", answerWith, stderr, x.called())
		}
		x.mu.Lock()
		x.calls = nil
		x.mu.Unlock()
	}
}

// A 🔴 question of the agent and the question of `remote on` are one at a
// time: `on` is busy while the 🔴 waits.
func TestRemoteOnIsBusyWhileARedQuestionWaits(t *testing.T) {
	data := isolate(t)
	g := newRemoteRig(t, withTailscale(), "41234")
	entered, release := make(chan confirm.Request, 4), make(chan struct{})
	l := &phoneLink{remote: g.r, log: g.log}
	l.conf = bridge.NewConfirmer(noApp{}, confirmerFunc(func(ctx context.Context, r confirm.Request) confirm.Decision {
		entered <- r
		select {
		case <-release:
		case <-ctx.Done():
		}
		return confirm.Deny
	}))
	d := remoteDaemon(t, l, nil)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, serveHandler(l, withTailscale(), func(string) string { return "" }, t.TempDir(), "claude", nil, d))
	if err != nil {
		t.Skipf("no socket: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	red := make(chan ipc.Exit, 1)
	go func() {
		exit, _ := ipc.Call(context.Background(), data, ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "reboot"}, Cwd: t.TempDir(), Env: map[string]string{"ADBUDDY_SOURCE": "agent"}},
			nil, nopWriter{}, nopWriter{}, nil)
		red <- exit
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the 🔴 was not asked")
	}
	exit, err := ipc.Call(context.Background(), data, ipc.Request{Kind: ipc.KindRemote, Argv: []string{"on"}}, nil, nopWriter{}, nopWriter{}, nil)
	if err != nil || exit.Code != 1 || exit.ErrorCode != "REMOTE_BUSY" || !strings.Contains(exit.Error, "Restart the phone") {
		t.Fatalf("%v %+v", err, exit)
	}
	if len(entered) != 0 || len(g.phone.called()) != 0 {
		t.Fatalf("asked again or touched the phone: %v", g.phone.called())
	}
	close(release)
	<-red
}
