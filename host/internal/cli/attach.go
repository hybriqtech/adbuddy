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
		Short: app.T("help.attach.short"),
		Long:  app.T("help.attach.long"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewErrorT("AGENT_SESSION", "err.attach.agent_session", "err.attach.agent_session.fix")
			}
			in, out := os.Stdin, os.Stdout
			if !isTerminal(in) || !isTerminal(out) {
				return UsageErrorT("err.attach.usage_adbuddy_attach_needs_an_interactive")
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
			restore := leaveTerminal(out, func() { term.Restore(int(in.Fd()), old) }, restoreOut)
			defer restore()

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			resizes := watchSize(ctx, out)
			keys := &detachReader{r: in}
			exit, err := ipc.Attach(ctx, e.dirs.Data, ipc.Request{Kind: ipc.KindAttach, Env: map[string]string{"ADBUDDY_SOURCE": app.Getenv("ADBUDDY_SOURCE")}}, keys, out, resizes)
			restore()
			switch {
			case keys.detached():
				// Whether the agent still runs, serve said on the screen.
				fmt.Fprintln(app.Stderr, app.T("attach.detached_adbuddy_attach_connects_again"))
				return nil
			case err != nil:
				return NewErrorT("ATTACH_FAILED", "error.cause", "err.attach.attach_failed.fix", err.Error())
			case exit.ErrorCode == "NO_SESSION":
				return NewError("NO_SESSION", exit.Error, "")
			case exit.Code != 0:
				return NewError("ATTACH_FAILED", exit.Error, "")
			}
			fmt.Fprintln(app.Stderr, app.T("attach.adbuddy_serve_closed_the_connection"))
			return nil
		},
	}
}

// termReset turns off what the agent may have left on in the terminal:
// attributes, hidden cursor, bracketed paste, focus and mouse reports, the
// alternate screen.
const termReset = "\x1b[0m\x1b[?25h\x1b[?2004l\x1b[?1004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l"

// leaveTerminal returns the function that gives the terminal back, by any
// way out of attach and only once: termReset first, while escape sequences
// still work, then each restore in order.
func leaveTerminal(out io.Writer, restores ...func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			_, _ = io.WriteString(out, termReset)
			for _, r := range restores {
				r()
			}
		})
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
		Short: app.T("help.new.short"),
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
		Short: app.T("help.stop.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return sessionRequest(cmd.Context(), app, []string{"stop"})
		},
	}
}

// sessionRequest asks serve to start or stop the agent. The agent may not:
// it would control its own session (spec M3 §3.4); serve checks it too.
func sessionRequest(ctx context.Context, app *App, argv []string) error {
	if app.Getenv("ADBUDDY_SOURCE") == "agent" {
		return NewErrorT("AGENT_SESSION", "err.attach.agent_session_2", "err.attach.agent_session_2.fix", argv[0])
	}
	e, err := loadEnv(app)
	if err != nil {
		return err
	}
	req := ipc.Request{Kind: ipc.KindSession, Argv: argv, Env: map[string]string{"ADBUDDY_SOURCE": app.Getenv("ADBUDDY_SOURCE")}}
	exit, err := ipc.Call(ctx, e.dirs.Data, req, nil, app.Stdout, app.Stderr, nil)
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
