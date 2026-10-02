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

// undoFixKeys are the catalog keys of the fix of each code the daemon can
// answer `adbuddy undo` with; a code that is not here has no fix.
var undoFixKeys = map[string]string{
	"NOTHING_TO_UNDO":    "err.undo.fix.nothing_to_undo",
	"NOT_FOUND":          "err.undo.fix.not_found",
	"NO_INVERSE":         "err.undo.fix.no_inverse",
	"ALREADY_UNDONE":     "err.undo.fix.already_undone",
	"NOT_APPLIED":        "err.undo.fix.not_applied",
	"UNTRUSTED_ENTRY":    "err.undo.fix.untrusted_entry",
	"JOURNAL_UNWRITABLE": "err.undo.fix.journal_unwritable",
	"UNCERTAIN_STATE":    "err.undo.fix.uncertain_state",
	"OTHER_DEVICE":       "err.undo.fix.other_device",
	"UNDO_FAILED":        "err.undo.fix.undo_failed",
	"WAITING_FOR_ANSWER": "err.undo.fix.waiting_for_answer",
	"PAUSED":             "err.undo.fix.paused",
}

// undoTransient are the failures that go away by themselves: the pause ends,
// or the question is answered. They exit with 75.
var undoTransient = map[string]bool{"PAUSED": true, "WAITING_FOR_ANSWER": true}

func newUndoCmd(app *App) *cobra.Command {
	var session string
	cmd := &cobra.Command{
		Use:     "undo [last|session|<id>]",
		Short:   app.T("help.undo.short"),
		Long:    app.T("help.undo.long"),
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
			failure := NewErrorT(code, "error.cause", undoFixKeys[code], exit.Error)
			if undoTransient[code] {
				failure = TransientErrorT(code, "error.cause", undoFixKeys[code], exit.Error)
			}
			// A pause in the middle stops undo after some changes: say which.
			paused := code == "PAUSED" && len(res.Undone)+len(res.Failed)+len(res.Skipped) > 0
			partial := code == "UNDO_FAILED" || code == "JOURNAL_UNWRITABLE" || paused
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
	cmd.Flags().StringVar(&session, "session", "", app.T("help.undo.flag.session"))
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
