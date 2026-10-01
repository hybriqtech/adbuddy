package cli

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/pty"
	"github.com/hybriqtech/adbuddy/host/internal/session"
)

// testSession is a session whose agent is never found: no process starts.
func testSession(t *testing.T) *session.Manager {
	m := session.New(session.Options{
		Dir:   t.TempDir(),
		Look:  lookAgent,
		Env:   func(string) []string { return nil },
		Start: func(pty.Options) (pty.PTY, error) { t.Fatal("an agent started"); return nil, nil },
	})
	t.Cleanup(m.Stop)
	return m
}

// The agent cannot attach to, start or stop its own session.
func TestHandleSessionRefusesTheAgent(t *testing.T) {
	sess := testSession(t)
	agentEnv := map[string]string{"ADBUDDY_SOURCE": "agent"}
	for _, req := range []ipc.Request{
		{Kind: ipc.KindAttach, Env: agentEnv},
		{Kind: ipc.KindSession, Argv: []string{"new"}, Env: agentEnv},
		{Kind: ipc.KindSession, Argv: []string{"new", "codex"}, Env: agentEnv},
		{Kind: ipc.KindSession, Argv: []string{"stop"}, Env: agentEnv},
	} {
		e := handleSession(context.Background(), sess, "claude", req, nil)
		if e.Code == 0 || e.ErrorCode != "AGENT_SESSION" {
			t.Errorf("%s %v: exit %+v", req.Kind, req.Argv, e)
		}
	}
}

// new only starts the agents ADBuddy knows, by name: never a program or a
// path the request names.
func TestHandleSessionNewOnlyKnownAgents(t *testing.T) {
	sess := testSession(t)
	for _, name := range []string{"sh", "../x", `..\x`, "/bin/sh", `C:\Windows\System32\cmd.exe`, "cmd", ".."} {
		e := handleSession(context.Background(), sess, "claude", ipc.Request{Kind: ipc.KindSession, Argv: []string{"new", name}}, nil)
		if e.Code != 2 || e.ErrorCode != "UNKNOWN_AGENT" {
			t.Errorf("new %q: exit %+v", name, e)
		}
	}
	// A known one gets as far as looking for it (not installed here).
	e := handleSession(context.Background(), sess, "claude", ipc.Request{Kind: ipc.KindSession, Argv: []string{"new", "codex"}}, nil)
	if e.ErrorCode != "AGENT_NOT_FOUND" {
		t.Fatalf("new codex: exit %+v", e)
	}
}

// adbuddy new and stop refuse the agent's session, as attach does, before
// asking the host.
func TestNewAndStopRefuseTheAgentSession(t *testing.T) {
	isolate(t)
	t.Setenv("ADBUDDY_SOURCE", "agent")
	for _, args := range [][]string{{"new"}, {"new", "codex"}, {"stop"}, {"attach"}} {
		_, errOut, code := run(t, args...)
		if code != ExitFailure || !strings.Contains(errOut, "AGENT_SESSION") {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
}

// new and stop send the caller's ADBUDDY_SOURCE, so the host can refuse the
// agent even if the command did not.
func TestSessionRequestSendsTheSource(t *testing.T) {
	data := isolate(t)
	t.Setenv("ADBUDDY_SOURCE", "pc")
	got := make(chan ipc.Request, 2)
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		got <- req
		return ipc.Exit{}
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	for _, args := range [][]string{{"new"}, {"stop"}} {
		if _, errOut, code := run(t, args...); code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", args, code, errOut)
		}
		if req := <-got; req.Env["ADBUDDY_SOURCE"] != "pc" {
			t.Fatalf("%v: env %v", args, req.Env)
		}
	}
}

// The agent's terminal variables are ADBuddy's: the ones of the terminal
// that started serve go, so there are no duplicates (conpty keeps them all,
// and on Windows the first one wins).
func TestTermEnv(t *testing.T) {
	in := []string{"PATH=/bin", "TERM=dumb", "COLORTERM=no", "TERM_PROGRAM=vscode", "TERM_PROGRAM_VERSION=1", "WT_SESSION=x", "TMUX=/tmp/t", "VTE_VERSION=6", "COLORFGBG=15;0", "TERMINAL=keep", "HOME=/h"}
	if runtime.GOOS == "windows" {
		in = append(in, "Term=dumb", "wt_session=y")
	}
	got := termEnv(in)
	want := []string{"PATH=/bin", "TERMINAL=keep", "HOME=/h", "TERM=xterm-256color", "COLORTERM=truecolor"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("env %q, want %q", got, want)
	}
}

// Without a home folder, or with a relative CLAUDE_CONFIG_DIR, there is no
// configuration file to write: never a relative path.
func TestClaudeConfig(t *testing.T) {
	home := t.TempDir()
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	if got := claudeConfig(get, home); got != filepath.Join(home, ".claude.json") {
		t.Fatalf("with a home: %q", got)
	}
	if got := claudeConfig(get, ""); got != "" {
		t.Fatalf("without a home: %q", got)
	}
	env["CLAUDE_CONFIG_DIR"] = "relative"
	if got := claudeConfig(get, home); got != "" {
		t.Fatalf("relative CLAUDE_CONFIG_DIR: %q", got)
	}
	env["CLAUDE_CONFIG_DIR"] = home
	if got := claudeConfig(get, ""); got != filepath.Join(home, ".claude.json") {
		t.Fatalf("CLAUDE_CONFIG_DIR without a home: %q", got)
	}
}
