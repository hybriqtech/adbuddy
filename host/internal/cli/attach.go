package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// detachKey is Ctrl+]: it lets go of the session and leaves the agent running.
const detachKey = 0x1d

func newAttachCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "attach",
		Short: "Connect this terminal to the agent's session in adbuddy serve (Ctrl+] to detach)",
		Long: `Connect this terminal to the agent that adbuddy serve runs. You see its
screen and type into it; Ctrl+] detaches and leaves the agent running.
While attached, this terminal's size is the agent's.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewError("AGENT_SESSION", "`adbuddy attach` cannot be run from the agent's session", "Run it yourself in a terminal.")
			}
			in, out := os.Stdin, os.Stdout
			if !isTerminal(in) || !isTerminal(out) {
				return UsageError("adbuddy attach needs an interactive terminal")
			}
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			if _, err := ipc.PingDaemon(cmd.Context(), e.dirs.Data); errors.Is(err, ipc.ErrNotRunning) {
				return notRunning
			}
			old, err := term.MakeRaw(int(in.Fd()))
			if err != nil {
				return NewError("TERMINAL_FAILED", err.Error(), "")
			}
			restoreOut := enableVT(out)
			restore := func() { term.Restore(int(in.Fd()), old); restoreOut() }
			defer restore()

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			resizes := watchSize(ctx, out)
			keys := &detachReader{r: in}
			exit, err := ipc.Attach(ctx, e.dirs.Data, ipc.Request{Kind: ipc.KindAttach, Env: map[string]string{"ADBUDDY_SOURCE": app.Getenv("ADBUDDY_SOURCE")}}, keys, out, resizes)
			restore()
			switch {
			case keys.detached():
				fmt.Fprintln(app.Stderr, "\r\nDetached: the agent keeps running in adbuddy serve.")
				return nil
			case err != nil:
				return NewError("ATTACH_FAILED", err.Error(), "Check that `adbuddy serve` is still running.")
			case exit.ErrorCode == "NO_SESSION":
				return NewError("NO_SESSION", exit.Error, "")
			case exit.Code != 0:
				return NewError("ATTACH_FAILED", exit.Error, "")
			}
			fmt.Fprintln(app.Stderr, "\r\nThe session ended.")
			return nil
		},
	}
}

// detachReader passes what is typed until Ctrl+], which ends it.
type detachReader struct {
	r    io.Reader
	mu   sync.Mutex
	done bool
}

func (d *detachReader) Read(p []byte) (int, error) {
	if d.detached() {
		return 0, io.EOF
	}
	n, err := d.r.Read(p)
	if i := bytes.IndexByte(p[:n], detachKey); i >= 0 {
		d.mu.Lock()
		d.done = true
		d.mu.Unlock()
		return i, nil // what came before Ctrl+] still goes; then EOF
	}
	return n, err
}

func (d *detachReader) detached() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.done
}

// watchSize sends the terminal's size now and each time it changes.
func watchSize(ctx context.Context, out *os.File) <-chan [2]int {
	ch := make(chan [2]int, 4)
	send := func() {
		if w, h, err := term.GetSize(int(out.Fd())); err == nil {
			select {
			case ch <- [2]int{w, h}:
			default:
			}
		}
	}
	send()
	go sizeChanges(ctx, out, send)
	return ch
}

func newNewCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "new [agent]",
		Short: "Start a new conversation in adbuddy serve, with another agent if given",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			argv := append([]string{"new"}, args...)
			return sessionRequest(cmd.Context(), app, argv)
		},
	}
}

func newStopCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the agent that adbuddy serve runs (serve keeps running)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sessionRequest(cmd.Context(), app, []string{"stop"})
		},
	}
}

func sessionRequest(ctx context.Context, app *App, argv []string) error {
	e, err := loadEnv(app)
	if err != nil {
		return err
	}
	exit, err := ipc.Call(ctx, e.dirs.Data, ipc.Request{Kind: ipc.KindSession, Argv: argv}, nil, app.Stdout, app.Stderr, nil)
	if errors.Is(err, ipc.ErrNotRunning) {
		return notRunning
	}
	if err != nil {
		return NewError("SESSION_FAILED", err.Error(), "")
	}
	if exit.Code != 0 {
		return NewError(orDefault(exit.ErrorCode, "SESSION_FAILED"), exit.Error, "")
	}
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
