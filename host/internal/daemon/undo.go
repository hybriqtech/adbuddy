package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// UndoItem is one entry in an undo result.
type UndoItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	UndoID      string `json:"undoId,omitempty"`
	Error       string `json:"error,omitempty"` // why it failed or was skipped
}

// UndoResult is the data answered to an undo request.
type UndoResult struct {
	Undone  []UndoItem `json:"undone"`
	Skipped []UndoItem `json:"skipped"` // changes that are not undone automatically, with the reason
	Failed  []UndoItem `json:"failed"`
}

func newUndoResult() UndoResult {
	return UndoResult{Undone: []UndoItem{}, Skipped: []UndoItem{}, Failed: []UndoItem{}}
}

// UndoError explains why nothing could be selected.
type UndoError struct{ Code, Message string }

func (e *UndoError) Error() string { return e.Code + ": " + e.Message }

// Reasons for skipping a change.
const (
	skipNoInverse = "no inverse"
	skipUncertain = "did not finish cleanly: check the phone and undo it by hand if needed"
	skipUntrusted = "not written by this ADBuddy (signature does not match): undo it by hand if needed"
)

// isChange: a 🟡/🔴 command that ran or may have run, and is not undone yet.
func isChange(e journal.Entry) bool {
	switch e.Status {
	case journal.StatusOK, journal.StatusError, journal.StatusRunning:
	default:
		return false
	}
	return e.Source != journal.SourceUndo && e.Level != "green" && e.UndoneBy == nil
}

// undoState indexes the undo entries by the change they undo: an undo that
// finished counts as done even if marking the change failed; one that never
// finished leaves the change uncertain.
func undoState(entries []journal.Entry) map[string]string {
	state := map[string]string{}
	for _, e := range entries {
		if e.Source != journal.SourceUndo || e.Undoes == "" || e.Tampered {
			continue
		}
		switch {
		case e.Status == journal.StatusOK:
			state[e.Undoes] = journal.StatusOK
		case e.Status == journal.StatusRunning && state[e.Undoes] != journal.StatusOK:
			state[e.Undoes] = journal.StatusRunning
		}
	}
	return state
}

// skipReason is "" when e can be undone automatically: a change signed by
// this ADBuddy, finished with exit code 0, with an inverse, not undone yet.
func skipReason(e journal.Entry, state map[string]string) string {
	switch {
	case e.Tampered:
		return skipUntrusted
	case e.Status != journal.StatusOK || state[e.ID] == journal.StatusRunning:
		return skipUncertain
	case e.Inverse == nil:
		return skipNoInverse
	}
	return ""
}

// SelectUndo picks the entries to undo, newest first. target is "last",
// "session" or an entry id; session is the session for "session".
func SelectUndo(entries []journal.Entry, target, session string) (plan []journal.Entry, skipped []UndoItem, err error) {
	state := undoState(entries)
	pending := func(e journal.Entry) bool { return isChange(e) && state[e.ID] != journal.StatusOK }
	skip := func(e journal.Entry, why string) {
		skipped = append(skipped, UndoItem{ID: e.ID, Description: e.Description, Error: why})
	}
	switch target {
	case "", "last":
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if !pending(e) {
				continue
			}
			if why := skipReason(e, state); why != "" {
				skip(e, why)
				continue
			}
			return []journal.Entry{e}, skipped, nil
		}
		return nil, skipped, &UndoError{"NOTHING_TO_UNDO", "no change with an inverse left to undo"}
	case "session":
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			if e.Session != session || !pending(e) {
				continue
			}
			if why := skipReason(e, state); why != "" {
				skip(e, why)
				continue
			}
			plan = append(plan, e)
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
		case e.Tampered:
			return nil, nil, &UndoError{"UNTRUSTED_ENTRY", e.ID + " was " + skipUntrusted}
		case e.Source == journal.SourceUndo:
			return nil, nil, &UndoError{"NO_INVERSE", e.ID + " is itself an undo; undo the original change again only by hand"}
		case e.UndoneBy != nil || state[e.ID] == journal.StatusOK:
			return nil, nil, &UndoError{"ALREADY_UNDONE", e.ID + " was already undone"}
		case e.Status == journal.StatusError || e.Status == journal.StatusRunning || state[e.ID] == journal.StatusRunning:
			return nil, nil, &UndoError{"UNCERTAIN_STATE", e.ID + " " + skipUncertain}
		case e.Status != journal.StatusOK:
			return nil, nil, &UndoError{"NOT_APPLIED", e.ID + " never ran (" + e.Status + ")"}
		case e.Inverse == nil:
			return nil, nil, &UndoError{"NO_INVERSE", e.ID + " (" + e.Description + ") has no inverse and cannot be undone"}
		}
		return []journal.Entry{e}, nil, nil
	}
	return nil, nil, &UndoError{"NOT_FOUND", "no journal entry " + target}
}

// undo runs inverses in reverse order without asking again (spec §4.4). It
// holds the changes lock, so two undos (or an undo and a change) never race.
func (d *Daemon) undo(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	d.changes.Lock()
	defer d.changes.Unlock()
	entries, _, err := d.o.Journal.Load()
	if err != nil {
		return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNREADABLE", Error: err.Error()}
	}
	session := req.Session
	if session == "" {
		session = d.o.Session
	}
	done, err := readUndoneLog(d.o.UndoneLog)
	if err != nil {
		return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNREADABLE", Error: err.Error()}
	}
	for i := range entries {
		if done[entries[i].ID] && entries[i].UndoneBy == nil {
			id := "(recorded outside the journal)"
			entries[i].UndoneBy = &id
		}
	}
	plan, skipped, err := SelectUndo(entries, req.Target, session)
	res := newUndoResult()
	res.Skipped = append(res.Skipped, skipped...)
	if err != nil {
		ue := err.(*UndoError)
		_ = s.Data(res)
		return ipc.Exit{Code: 1, ErrorCode: ue.Code, Error: ue.Message}
	}
	for _, e := range plan {
		if ctx.Err() != nil {
			break // the client left: stop before touching the phone again
		}
		item := UndoItem{ID: e.ID, Description: e.Description}
		if err := validInverse(e.Inverse.Argv, d.o.Journal.BackupDir(e.ID)); err != nil {
			item.Error = "refused: " + err.Error()
			res.Failed = append(res.Failed, item)
			fmt.Fprintf(s.Stderr(), "✗ %s: %s\n", classify.Printable(e.Description), classify.Printable(item.Error))
			continue
		}
		now := d.o.Now().UTC()
		u := journal.Entry{
			ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Source: journal.SourceUndo,
			Argv: e.Inverse.Argv, Level: "yellow", Rule: "undo", Description: undoPrefix(d.o.Lang) + e.Description, Undoes: e.ID,
		}
		item.UndoID = u.ID
		running := u
		running.Status = journal.StatusRunning
		if err := d.o.Journal.Append(running); err != nil {
			_ = s.Data(res)
			return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNWRITABLE", Error: fmt.Sprintf("the journal cannot be written (%v); undid %d change(s) before stopping", err, len(res.Undone))}
		}
		var errb bytes.Buffer
		code, runErr := d.o.ADB.Run(ctx, "", e.Inverse.Argv, nil, io.Discard, &errb)
		if runErr != nil && code == 0 {
			code = 1
		}
		u.ExitCode = &code
		if runErr != nil || code != 0 {
			u.Status = journal.StatusError
			msg := strings.TrimSpace(errb.String())
			if runErr != nil {
				msg = runErr.Error()
			}
			u.Reason, item.Error = msg, msg
			res.Failed = append(res.Failed, item)
			d.record(u)
			fmt.Fprintf(s.Stderr(), "✗ %s: %s\n", classify.Printable(e.Description), classify.Printable(msg))
			continue
		}
		u.Status = journal.StatusOK
		d.record(u)
		res.Undone = append(res.Undone, item)
		if err := appendUndoneLog(d.o.UndoneLog, e.ID); err != nil {
			_ = s.Data(res)
			return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNWRITABLE", Error: fmt.Sprintf("undid %s but could not record it: %v", e.ID, err)}
		}
		orig := e
		orig.UndoneBy = &u.ID
		if err := d.o.Journal.Append(orig); err != nil {
			// The change is undone on the phone but not marked: stop, so the
			// same inverse is not run again by mistake. (The finished undo
			// record already counts as done for SelectUndo.)
			_ = s.Data(res)
			return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNWRITABLE", Error: fmt.Sprintf("undid %s but could not record it: %v", e.ID, err)}
		}
		fmt.Fprintf(s.Stderr(), "✓ %s\n", classify.Printable(e.Description))
	}
	for _, it := range res.Skipped {
		fmt.Fprintf(s.Stderr(), "– %s: %s\n", classify.Printable(it.Description), it.Error)
	}
	_ = s.Data(res)
	if len(res.Failed) > 0 {
		return ipc.Exit{Code: 1, ErrorCode: "UNDO_FAILED", Error: fmt.Sprintf("%d of %d inverses failed", len(res.Failed), len(plan))}
	}
	if ctx.Err() != nil {
		return ipc.Exit{Code: 1, ErrorCode: "UNDO_FAILED", Error: "cancelled"}
	}
	return ipc.Exit{}
}

func undoPrefix(lang string) string {
	if lang == "es" {
		return "Deshacer: "
	}
	return "Undo: "
}

// readUndoneLog reads the ids in the undone log ("" = no log).
func readUndoneLog(path string) (map[string]bool, error) {
	done := map[string]bool{}
	if path == "" {
		return done, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return done, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if id := strings.TrimSpace(line); id != "" {
			done[id] = true
		}
	}
	return done, nil
}

func appendUndoneLog(path, id string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(id + "\n"); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
