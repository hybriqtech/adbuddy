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
func newSession(app *App, ws, serial string, onInfo func(session.Info)) *session.Manager {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return session.New(session.Options{
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
				fmt.Fprintf(app.Stderr, "⚠ Could not find Claude Code's configuration to mark %s as trusted: answer its question in the terminal.\n", dir)
				return
			}
			if _, err := agent.TrustFolder(cfg, dir); err != nil {
				fmt.Fprintf(app.Stderr, "⚠ Could not mark %s as trusted for Claude Code (%s): answer its question in the terminal.\n", dir, classify.Printable(err.Error()))
			}
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
// the emulator's TERM and truecolor. The old ones go first, as conpty does
// not drop duplicates and on Windows the first one wins.
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
		fmt.Fprintf(app.Stderr, "⚠ Could not start %s: %s\n  %s\n", name, classify.Printable(err.Error()), agent.InstallHint(name))
		return
	}
	fmt.Fprintf(app.Stderr, "Agent: %s, in this host. Connect a terminal with `adbuddy attach` (Ctrl+] to detach), or chat from the phone.\n", name)
}

var attachIDs atomic.Int64

// sessionRequest answers the IPC requests of the session: attach, new, stop.
// The agent may not use them: it would type into its own terminal.
func handleSession(ctx context.Context, sess *session.Manager, defaultAgent string, req ipc.Request, c *ipc.Conn) ipc.Exit {
	if req.Env["ADBUDDY_SOURCE"] == "agent" {
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
			fmt.Fprintf(c.Stdout(), "New conversation with %s.\n", name)
			return ipc.Exit{}
		case "stop":
			sess.Stop()
			fmt.Fprintln(c.Stdout(), "The agent stopped; adbuddy serve keeps running.")
			return ipc.Exit{}
		}
		return ipc.Exit{Code: 2, ErrorCode: "USAGE", Error: "new or stop"}
	}
	return ipc.Exit{Code: 1, ErrorCode: "UNKNOWN_KIND"}
}
