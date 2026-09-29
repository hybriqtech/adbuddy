package daemon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// UndoItem is one entry in an undo result.
type UndoItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	UndoID      string `json:"undoId,omitempty"`
	Error       string `json:"error,omitempty"`
}

// UndoResult is the data answered to an undo request.
type UndoResult struct {
	Undone  []UndoItem `json:"undone"`
	Skipped []UndoItem `json:"skipped"` // changes without an inverse
	Failed  []UndoItem `json:"failed"`
}

// UndoError explains why nothing could be selected.
type UndoError struct{ Code, Message string }

func (e *UndoError) Error() string { return e.Code + ": " + e.Message }

// isChange: a 🟡/🔴 command that ran or may have run, and is not undone yet.
func isChange(e journal.Entry) bool {
	switch e.Status {
	case journal.StatusOK, journal.StatusError, journal.StatusRunning:
	default:
		return false
	}
	return e.Source != journal.SourceUndo && e.Level != "green" && e.UndoneBy == nil
}

// undoable: only changes that finished with exit code 0. The inverse of a
// command that failed or never finished (the host stopped) could undo
// something else, so those are reported and left to the person.
func undoable(e journal.Entry) bool {
	return isChange(e) && e.Status == journal.StatusOK && e.Inverse != nil
}

// SelectUndo picks the entries to undo, newest first. target is "last",
// "session" or an entry id; session is the session for "session".
func SelectUndo(entries []journal.Entry, target, session string) (plan, skipped []journal.Entry, err error) {
	switch target {
	case "", "last":
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if undoable(e) {
				return []journal.Entry{e}, skipped, nil
			}
			if isChange(e) {
				skipped = append(skipped, e)
			}
		}
		return nil, skipped, &UndoError{"NOTHING_TO_UNDO", "no change with an inverse left to undo"}
	case "session":
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if e.Session != session {
				continue
			}
			if undoable(e) {
				plan = append(plan, e)
			} else if isChange(e) {
				skipped = append(skipped, e)
			}
		}
		if len(plan) == 0 {
			return nil, skipped, &UndoError{"NOTHING_TO_UNDO", "session " + session + " has no change with an inverse left to undo"}
		}
		return plan, skipped, nil
	}
	for _, e := range entries {
		if e.ID != target {
			continue
		}
		switch {
		case e.UndoneBy != nil:
			return nil, nil, &UndoError{"ALREADY_UNDONE", e.ID + " was already undone by " + *e.UndoneBy}
		case e.Status == journal.StatusError || e.Status == journal.StatusRunning:
			return nil, nil, &UndoError{"UNCERTAIN_STATE", e.ID + " did not finish cleanly (" + e.Status + "): check the phone and undo it by hand if needed"}
		case e.Status != journal.StatusOK:
			return nil, nil, &UndoError{"NOT_APPLIED", e.ID + " never ran (" + e.Status + ")"}
		case e.Inverse == nil:
			return nil, nil, &UndoError{"NO_INVERSE", e.ID + " (" + e.Description + ") has no inverse and cannot be undone"}
		}
		return []journal.Entry{e}, nil, nil
	}
	return nil, nil, &UndoError{"NOT_FOUND", "no journal entry " + target}
}

// undo runs inverses in reverse order without asking again (spec §4.4).
func (d *Daemon) undo(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	entries, _, err := d.o.Journal.Load()
	if err != nil {
		return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNREADABLE", Error: err.Error()}
	}
	session := req.Session
	if session == "" {
		session = d.o.Session
	}
	plan, skipped, err := SelectUndo(entries, req.Target, session)
	var res UndoResult
	for _, e := range skipped {
		res.Skipped = append(res.Skipped, UndoItem{ID: e.ID, Description: e.Description})
	}
	if err != nil {
		ue := err.(*UndoError)
		_ = s.Data(res)
		return ipc.Exit{Code: 1, ErrorCode: ue.Code, Error: ue.Message}
	}
	for _, e := range plan {
		now := d.o.Now().UTC()
		u := journal.Entry{
			ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Source: journal.SourceUndo,
			Argv: e.Inverse.Argv, Level: "yellow", Rule: "undo", Description: undoPrefix(d.o.Lang) + e.Description, Undoes: e.ID,
		}
		running := u
		running.Status = journal.StatusRunning
		if err := d.o.Journal.Append(running); err != nil {
			_ = s.Data(res)
			return ipc.Exit{Code: ExitDenied, ErrorCode: "JOURNAL_UNWRITABLE", Error: "the journal cannot be written: " + err.Error()}
		}
		var errb bytes.Buffer
		code, runErr := d.o.ADB.Run(ctx, "", e.Inverse.Argv, nil, io.Discard, &errb)
		u.ExitCode = &code
		item := UndoItem{ID: e.ID, Description: e.Description, UndoID: u.ID}
		if runErr != nil || code != 0 {
			u.Status = journal.StatusError
			msg := strings.TrimSpace(errb.String())
			if runErr != nil {
				msg = runErr.Error()
			}
			u.Reason, item.Error = msg, msg
			res.Failed = append(res.Failed, item)
			fmt.Fprintf(s.Stderr(), "✗ %s: %s\n", e.Description, msg)
		} else {
			u.Status = journal.StatusOK
			res.Undone = append(res.Undone, item)
			orig := e
			orig.UndoneBy = &u.ID
			if err := d.o.Journal.Append(orig); err != nil {
				d.o.OnWarning("could not mark " + e.ID + " as undone: " + err.Error())
			}
			fmt.Fprintf(s.Stderr(), "✓ %s\n", e.Description)
		}
		d.record(u)
	}
	for _, e := range skipped {
		fmt.Fprintf(s.Stderr(), "– %s: no inverse, skipped\n", e.Description)
	}
	_ = s.Data(res)
	if len(res.Failed) > 0 {
		return ipc.Exit{Code: 1, ErrorCode: "UNDO_FAILED", Error: fmt.Sprintf("%d of %d inverses failed", len(res.Failed), len(plan))}
	}
	return ipc.Exit{}
}

func undoPrefix(lang string) string {
	if lang == "es" {
		return "Deshacer: "
	}
	return "Undo: "
}
