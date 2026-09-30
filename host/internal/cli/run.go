package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

func newRunCmd(app *App) *cobra.Command {
	var wsDir string
	cmd := &cobra.Command{
		Use:   "run [agent] [-- agent arguments]",
		Short: "Start a coding agent in the ADBuddy environment",
		Long: `Start the agent (claude, codex, gemini, opencode…) in the ADBuddy environment,
with ADBuddy's adb first in its PATH and the phone selected. ` + "`adbuddy serve`" + ` must
be running in another terminal. The exit code is the agent's.`,
		Example: "  adbuddy run\n  adbuddy run claude\n  adbuddy run codex -- --model o4",
		Args: func(cmd *cobra.Command, args []string) error {
			before := args
			if at := cmd.ArgsLenAtDash(); at >= 0 {
				before = args[:at]
			}
			if len(before) > 1 {
				return UsageError("only one agent can be given; put its arguments after --")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			before, rest := args, []string(nil)
			if at := cmd.ArgsLenAtDash(); at >= 0 {
				before, rest = args[:at], args[at:]
			}
			name := e.cfg.Agent
			if len(before) == 1 {
				name = before[0]
			}
			if name == "" {
				name = agent.Known[0]
			}
			ping, err := ipc.PingDaemon(cmd.Context(), e.dirs.Data)
			if errors.Is(err, ipc.ErrNotRunning) {
				return notRunning
			}
			if err != nil {
				return NewError("DAEMON_UNREACHABLE", err.Error(), "Restart `adbuddy serve`.")
			}
			// The agent must work where serve looks: its journal, and the only
			// place where adb pull may write without asking.
			serveWS := serveWorkspace(ping)
			ws := serveWS
			if wsDir != "" || serveWS == "" {
				if ws, err = filepath.Abs(e.workspace(wsDir)); err != nil {
					return NewError("WORKSPACE_FAILED", err.Error(), "")
				}
			}
			if serveWS != "" && ws != serveWS {
				if !sameDir(ws, serveWS) {
					return NewError("WORKSPACE_MISMATCH", "`adbuddy serve` is watching "+serveWS+", not "+ws,
						"Run `adbuddy run` without --workspace to use "+serveWS+", or stop serve and start it with `adbuddy serve --workspace "+ws+"`.")
				}
				ws = serveWS
			}
			exe, err := os.Executable()
			if err != nil {
				return NewError("EXECUTABLE_UNKNOWN", err.Error(), "")
			}
			if _, err := workspace.Init(workspace.Options{Dir: ws, Exe: exe}); err != nil {
				return NewError("WORKSPACE_FAILED", err.Error(), "Close other agents running in "+ws+" and try again.")
			}
			fmt.Fprintf(app.Stderr, "Starting %s in %s (phone %s)…\n", name, ws, ping.Phone())
			code, err := agent.Run(cmd.Context(), agent.Options{
				Agent: name, Args: rest, Dir: ws, BinDir: workspace.BinDir(ws), Serial: ping.Phone(), // the name the journal knows it by
				Environ: os.Environ(), Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
			})
			if errors.Is(err, agent.ErrNotFound) {
				return NewError("AGENT_NOT_FOUND", name+" is not installed or not in PATH", agent.InstallHint(name))
			}
			if err != nil {
				return NewError("AGENT_FAILED", err.Error(), "")
			}
			return ExitWith(code)
		},
	}
	cmd.Flags().StringVar(&wsDir, "workspace", "", "agent environment; must be the one `adbuddy serve` is watching (default: that one)")
	return cmd
}
