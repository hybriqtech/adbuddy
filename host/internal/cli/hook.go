package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/spf13/cobra"
)

// hookWait is the longest a hook waits for the daemon: a PermissionRequest
// waits for the phone up to 600 s (permTimeout), and Claude Code's own limit
// for the hook is 610 s (workspace.HookTimeout).
var hookWait = 620 * time.Second

// newHookCmd is `adbuddy hook <event>`: the command Claude Code runs for the
// hooks ADBuddy installs in the agent's environment (spec M4 §3.3). It passes
// the event's payload (stdin) to the daemon and writes its decision, if any,
// to stdout. It fails open: whatever goes wrong it writes nothing and exits 0,
// which leaves Claude Code to its own behavior. An exit code 2 would BLOCK the
// agent, so there is no error path out of here, not even for bad usage.
func newHookCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:                "hook <event>",
		Short:              app.T("help.hook.short"),
		Hidden:             true,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 {
				runHook(cmd.Context(), app, args[0])
			}
		},
	}
}

func runHook(ctx context.Context, app *App, event string) {
	// A panic would exit with 2, which blocks in Claude Code: fail open.
	defer func() { _ = recover() }()
	if !claudecode.IsEvent(event) || app.Stdin == nil {
		return
	}
	payload, err := io.ReadAll(io.LimitReader(app.Stdin, claudecode.MaxHookInput+1))
	if err != nil || len(payload) > claudecode.MaxHookInput {
		if event != claudecode.EventPermissionRequest {
			return
		}
		// Claude Code's dialog opens all the same: the daemon is told, with
		// an empty payload it ignores, so that it holds the chat (V05).
		payload = []byte("{}")
	}
	e, err := loadEnv(app)
	if err != nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, hookWait)
	defer cancel()
	var out bytes.Buffer
	exit, err := ipc.Call(ctx, e.dirs.Data, ipc.Request{Kind: ipc.KindHook, Argv: []string{event}}, bytes.NewReader(payload), &out, io.Discard, nil)
	if err != nil || exit.Code != 0 || out.Len() == 0 || out.Len() > claudecode.MaxHookInput || !json.Valid(out.Bytes()) {
		return
	}
	_, _ = app.Stdout.Write(out.Bytes())
}
