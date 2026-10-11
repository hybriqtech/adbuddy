package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/pty"
	"github.com/hybriqtech/adbuddy/host/internal/session"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// startPTY is pty.Start; a variable so tests can run a fake agent.
var startPTY = pty.Start

// lookAgent finds the agent's program; a variable for the same reason.
var lookAgent = exec.LookPath

// newSession makes the manager of the agent's session in serve (spec M3 §3):
// the agent runs in a PTY with the environment of `adbuddy run`, and Claude
// Code finds the environment already trusted.
func newSession(app *App, ws, serial string, onInfo func(session.Info), model, effort string) *session.Manager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return session.New(session.Options{
		Lang: app.Lang,
		Dir:  ws,
		Look: lookAgent,
		Env: func(string) []string {
			return termEnv(agent.Env(os.Environ(), workspace.BinDir(ws), serial))
		},
		Start: startPTY,
		PreTrust: func(name, dir string) {
			if name != "claude" {
				return
			}
			cfg := claudeConfig(app.Getenv, home)
			if cfg == "" {
				fmt.Fprintln(app.Stderr, app.T("serve_session.could_not_find_claude_code", dir))
				return
			}
			if _, err := agent.TrustFolder(cfg, dir); err != nil {
				fmt.Fprintln(app.Stderr, app.T("serve_session.could_not_mark_as_trusted", dir, classify.Printable(err.Error())))
			}
		},
		Args: func(name string) []string {
			m, e := agent.Effective(name, model, effort)
			a, _ := agent.Args(name, m, e)
			return a
		},
		OnInfo: onInfo,
	})
}

// claudeConfig is Claude Code's configuration file, or "" when it has no
// absolute path (no home folder, a relative CLAUDE_CONFIG_DIR): a relative
// one would be written in serve's working folder.
func claudeConfig(getenv func(string) string, home string) string {
	cfg := agent.ClaudeConfigFile(getenv, home)
	if !filepath.IsAbs(cfg) {
		return ""
	}
	return cfg
}

// termVars are the variables of the terminal that started serve, which say
// nothing true about the agent's terminal.
var termVars = []string{"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "WT_SESSION", "TMUX", "VTE_VERSION", "COLORFGBG"}

// termEnv replaces the terminal variables of env with the agent's terminal:
// the emulator's TERM and truecolor. The old ones go first: the environment
// block that internal/pty hands to CreateProcess keeps duplicates, and on
// Windows the first one wins.
func termEnv(env []string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, v := range termVars {
			if key == v || (runtime.GOOS == "windows" && strings.EqualFold(key, v)) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return append(out, "TERM=xterm-256color", "COLORTERM=truecolor")
}

// startAgent starts the agent when serve starts: Claude Code continues its
// last conversation. A missing agent leaves serve without a session.
func startAgent(app *App, sess *session.Manager, name string) {
	if err := sess.Start(name, true); err != nil {
		fmt.Fprintln(app.Stderr, app.T("serve_session.could_not_start", name, classify.Printable(err.Error()), agent.InstallHint(name)))
		return
	}
	fmt.Fprintln(app.Stderr, app.T("serve_session.agent_in_this_host_connect", name))
}

var attachIDs atomic.Int64

// sessionRequest answers the IPC requests of the session: attach, new, stop.
// The agent may not use them: it would type into its own terminal. Who is
// the agent is fromAgent's to say (V07).
func handleSession(ctx context.Context, sess *session.Manager, defaultAgent string, req ipc.Request, c *ipc.Conn) ipc.Exit {
	return handleSessionIn(i18n.EN, ctx, sess, defaultAgent, req, c)
}

// handleSessionIn is handleSession with what it tells the person written in lang.
func handleSessionIn(lang i18n.Lang, ctx context.Context, sess *session.Manager, defaultAgent string, req ipc.Request, c *ipc.Conn) ipc.Exit {
	if fromAgent(sess, req, c) {
		return ipc.Exit{Code: 1, ErrorCode: "AGENT_SESSION", Error: "the agent cannot control its own session"}
	}
	switch req.Kind {
	case ipc.KindAttach:
		return sess.ServeAttach(ctx, c, "pc-"+strconv.FormatInt(attachIDs.Add(1), 10))
	case ipc.KindSession:
		if len(req.Argv) == 0 {
			return ipc.Exit{Code: 2, ErrorCode: "USAGE", Error: "new or stop"}
		}
		switch req.Argv[0] {
		case "new":
			name := defaultAgent
			if i := sess.Info(); i.Agent != "" {
				name = i.Agent
			}
			if len(req.Argv) > 1 {
				// Only the agents ADBuddy knows, never a program or a path
				// the request names. The default comes from the user's own
				// configuration.
				if !agent.IsKnown(req.Argv[1]) {
					return ipc.Exit{Code: 2, ErrorCode: "UNKNOWN_AGENT", Error: fmt.Sprintf("unknown agent %q: use one of %s", classify.Printable(req.Argv[1]), strings.Join(agent.Known, ", "))}
				}
				name = req.Argv[1]
			}
			if err := sess.Start(name, false); err != nil {
				return ipc.Exit{Code: 1, ErrorCode: "AGENT_NOT_FOUND", Error: classify.Printable(err.Error()) + ". " + agent.InstallHint(name)}
			}
			tell(langWriter{Writer: c.Stdout(), Lang: lang}, "serve_session.new_conversation_with", name)
			return ipc.Exit{}
		case "stop":
			sess.Stop()
			tell(langWriter{Writer: c.Stdout(), Lang: lang}, "serve_session.the_agent_stopped_adbuddy_serve")
			return ipc.Exit{}
		}
		return ipc.Exit{Code: 2, ErrorCode: "USAGE", Error: "new or stop"}
	}
	return ipc.Exit{Code: 1, ErrorCode: "UNKNOWN_KIND"}
}
