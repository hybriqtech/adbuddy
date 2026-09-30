package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/spf13/cobra"
)

var undoFixes = map[string]string{
	"NOTHING_TO_UNDO":    "See `adbuddy journal` for what was done.",
	"NOT_FOUND":          "Check the id with `adbuddy journal`.",
	"NO_INVERSE":         "This change has to be reverted by hand.",
	"ALREADY_UNDONE":     "Nothing to do.",
	"NOT_APPLIED":        "Nothing to do: the command never ran.",
	"UNTRUSTED_ENTRY":    "The entry was not written by ADBuddy: check the phone and undo it by hand if needed.",
	"JOURNAL_UNWRITABLE": "Check that the journal folder is writable; the changes listed as undone did happen.",
	"UNCERTAIN_STATE":    "Check the phone: the command failed, the host stopped while it ran, or its place on the phone is taken again, so its inverse is not run automatically.",
	"OTHER_DEVICE":       "Connect the phone the change was made on (see `adbuddy journal`) and undo it there, or revert it by hand.",
	"UNDO_FAILED":        "Check the phone and the journal; failed inverses can be retried with `adbuddy undo <id>`.",
}

func newUndoCmd(app *App) *cobra.Command {
	var session string
	cmd := &cobra.Command{
		Use:   "undo [last|session|<id>]",
		Short: "Undo journaled changes by running their inverses",
		Long: `Undo the last change (default), one journal entry, or every change of a session,
newest first. Inverses run without asking again; changes without an inverse
are reported and skipped. Needs ` + "`adbuddy serve`" + ` running.`,
		Example: "  adbuddy undo\n  adbuddy undo j_01J9Z…\n  adbuddy undo session\n  adbuddy undo session --session s_01J9Y…",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			target := "last"
			if len(args) == 1 {
				target = args[0]
			}
			var res daemon.UndoResult
			exit, err := ipc.Call(cmd.Context(), e.dirs.Data, ipc.Request{Kind: ipc.KindUndo, Target: target, Session: session}, nil, app.Stderr, app.Stderr,
				func(b []byte) error { return json.Unmarshal(b, &res) })
			if errors.Is(err, ipc.ErrNotRunning) {
				return notRunning
			}
			if err != nil {
				return NewError("DAEMON_UNREACHABLE", err.Error(), "")
			}
			code := exit.ErrorCode
			if code == "" {
				code = "DAEMON_ERROR" // e.g. "unauthorized" after the secret changed
			}
			failure := NewError(code, exit.Error, undoFixes[code])
			partial := code == "UNDO_FAILED" || code == "JOURNAL_UNWRITABLE"
			if exit.Code != 0 && !partial {
				if code == "NOTHING_TO_UNDO" {
					// Tell why the last changes were not undone.
					if app.JSON {
						return writeFailureData(app.Stdout, failure, res)
					}
					printUndoItems(app.Stderr, nil, nil, res.Skipped)
				}
				return failure
			}
			if app.JSON {
				if exit.Code != 0 {
					return writeFailureData(app.Stdout, failure, res)
				}
				return WriteData(app.Stdout, res)
			}
			printUndoItems(app.Stdout, res.Undone, res.Failed, res.Skipped)
			if exit.Code != 0 {
				return failure
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "session for `undo session` (default: the current one)")
	return cmd
}

// printUndoItems lists what undo did. Descriptions and errors come from the
// journal, which the agent can write: they are escaped.
func printUndoItems(w io.Writer, undone, failed, skipped []daemon.UndoItem) {
	for _, it := range undone {
		fmt.Fprintf(w, "undone   %s  %s\n", classify.Printable(it.ID), classify.Printable(it.Description))
	}
	for _, it := range failed {
		fmt.Fprintf(w, "failed   %s  %s: %s\n", classify.Printable(it.ID), classify.Printable(it.Description), classify.Printable(it.Error))
	}
	for _, it := range skipped {
		fmt.Fprintf(w, "skipped  %s  %s (%s)\n", classify.Printable(it.ID), classify.Printable(it.Description), classify.Printable(it.Error))
	}
}
