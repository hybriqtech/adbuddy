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
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/precheck"
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

// otherDevice is why a change made on another phone is not undone: its
// inverse would be applied to the active phone. identity is the active
// phone's (see Options.Identity). Records older than the serial field have
// none and are accepted.
func otherDevice(e journal.Entry, identity string) string {
	if e.Serial == "" || samePhone(e.Serial, identity) {
		return ""
	}
	return "recorded on another phone (" + e.Serial + "): undo it by hand on that phone"
}

// samePhone reports whether a record's serial names the phone known as
// identity: the same name, or the same mDNS guid (records written before
// identities kept the session's serial, maybe renamed "<guid> (2)…"). An
// ip:port says nothing about which phone answered there: only an exact
// match counts.
func samePhone(recorded, identity string) bool {
	if recorded == identity {
		return true
	}
	g, ok := pairing.GuidOf(recorded)
	h, ok2 := pairing.GuidOf(identity)
	return ok && ok2 && g == h
}

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
// this ADBuddy, made on the active phone, finished with exit code 0, with an
// inverse, not undone yet.
func skipReason(e journal.Entry, state map[string]string, identity string) string {
	switch {
	case e.Tampered:
		return skipUntrusted
	case otherDevice(e, identity) != "":
		return otherDevice(e, identity)
	case e.Status != journal.StatusOK || state[e.ID] == journal.StatusRunning:
		return skipUncertain
	case e.Inverse == nil:
		return skipNoInverse
	}
	return ""
}

// SelectUndo picks the entries to undo, newest first. target is "last",
// "session" or an entry id; session is the session for "session"; identity
// is the active phone's (see Options.Identity).
func SelectUndo(entries []journal.Entry, target, session, identity string) (plan []journal.Entry, skipped []UndoItem, err error) {
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
			if why := skipReason(e, state, identity); why != "" {
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
			if why := skipReason(e, state, identity); why != "" {
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
		case otherDevice(e, identity) != "":
			return nil, nil, &UndoError{"OTHER_DEVICE", e.ID + " was " + otherDevice(e, identity)}
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

// undo runs inverses in reverse order without asking again (spec §4.4), except
// the 🔴 ones: those ask like any 🔴 command. It holds the changes lock, so
// two undos (or an undo and a change) never race.
func (d *Daemon) undo(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	if d.paused() {
		return ipc.Exit{Code: ExitDenied, ErrorCode: "PAUSED", Error: PausedMessage}
	}
	ctx, notes := withYoloNotes(ctx)
	yoloAtStart := d.o.Yolo.Active()
	if t := d.waitingFor(); t != "" && !d.liftQuestion(ctx, t) {
		return ipc.Exit{Code: ExitDenied, ErrorCode: "WAITING_FOR_ANSWER", Error: t}
	}
	d.changes.Lock()
	defer d.changes.Unlock()
	if d.paused() { // paused while waiting for the lock
		return ipc.Exit{Code: ExitDenied, ErrorCode: "PAUSED", Error: PausedMessage}
	}
	// Admitted as a change in flight: a question waits for the inverses.
	// A question started while undo waited for the lock stops it here.
	release, other := d.admit(undoPrefix(d.o.Lang)+req.Target, true)
	if other != "" {
		if !d.liftQuestion(ctx, other) {
			return ipc.Exit{Code: ExitDenied, ErrorCode: "WAITING_FOR_ANSWER", Error: other}
		}
		release = d.flyAnyway(true, undoPrefix(d.o.Lang)+req.Target)
	}
	// release is replaced while a 🔴 inverse is asked (see below).
	defer func() { release() }()
	entries, _, err := d.o.Journal.Load()
	if err != nil {
		return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNREADABLE", Error: err.Error()}
	}
	session := req.Session
	if session == "" {
		session = d.o.Session
	}
	logged, err := readUndoneLog(d.o.UndoneLog)
	if err != nil {
		return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNREADABLE", Error: err.Error()}
	}
	for i := range entries {
		if logged[entries[i].ID] == undoneDone && entries[i].UndoneBy == nil {
			id := "(recorded outside the journal)"
			entries[i].UndoneBy = &id
		}
	}
	for id, st := range logged {
		if st == undoneRunning {
			// An undo that started and never finished, even if the journal
			// lost its record: SelectUndo treats the change as uncertain.
			entries = append(entries, journal.Entry{Source: journal.SourceUndo, Undoes: id, Status: journal.StatusRunning})
		}
	}
	plan, skipped, err := SelectUndo(entries, req.Target, session, d.identity())
	res := newUndoResult()
	var reappeared []string // moves not undone because their source exists again
	denied := 0             // 🔴 inverses the user did not confirm
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
		if d.paused() {
			_ = s.Data(res)
			return ipc.Exit{Code: ExitDenied, ErrorCode: "PAUSED", Error: fmt.Sprintf("%s; undid %d change(s) before stopping", PausedMessage, len(res.Undone))}
		}
		item := UndoItem{ID: e.ID, Description: e.Description}
		if err := validInverse(e.Inverse.Argv, d.o.Journal.BackupDir(e.ID)); err != nil {
			item.Error = "refused: " + err.Error()
			res.Failed = append(res.Failed, item)
			fmt.Fprintf(s.Stderr(), "✗ %s: %s\n", classify.Printable(e.Description), classify.Printable(item.Error))
			continue
		}
		// The journal is the agent's to forge (it can read the signing secret):
		// the inverse must undo the change the record claims, and goes through
		// the same classifier and confirmation as any command of the agent.
		kind := ""
		oargs := e.Argv
		_, oargs = classify.SplitGlobal(oargs)
		if len(oargs) > 0 {
			if r := d.o.Classifier.Classify(oargs); r.Op != nil {
				kind = r.Op.Kind
			}
		}
		if !inverseFitsKind(kind, e.Inverse.Argv) {
			item.Error = "refused: the inverse does not undo the change recorded (" + kind + ")"
			res.Failed = append(res.Failed, item)
			fmt.Fprintf(s.Stderr(), "✗ %s: %s\n", classify.Printable(e.Description), classify.Printable(item.Error))
			continue
		}
		level, rule := "yellow", "undo" // what the journal says: 🔴 only when the user was asked
		if ires := d.o.Classifier.Classify(e.Inverse.Argv); ires.Level == classify.Red {
			level, rule = ires.Level.String(), "undo-confirmed"
			if d.lift(ctx, "the inverse of "+classify.Printable(e.Description)+" asked for your confirmation ("+classify.Printable(ires.Description(d.o.Lang))+") and ran without it") {
				rule = "undo-yolo" // nobody was asked
			} else {
				// Not asked while this undo is itself in flight: the question
				// waits for the changes in flight, this one included.
				release()
				release = func() {}
				// What the person reads comes from the classifier, never from the
				// journal: the agent can forge its records.
				detail := d.lang().T("undo.confirm.detail") + "\n" + classify.Printable(ires.Detail(d.o.Lang))
				decision, askErr := d.AskUser(ctx, confirm.Request{
					ID: journal.NewID("j_", d.o.Now().UTC()), Title: ires.Description(d.o.Lang), Detail: detail,
					RawCommand: classify.DisplayCommand(e.Inverse.Argv), Special: ires.Special,
				}, 0)
				if errors.Is(askErr, ErrAskPaused) {
					_ = s.Data(res)
					return ipc.Exit{Code: ExitDenied, ErrorCode: "PAUSED", Error: fmt.Sprintf("%s; undid %d change(s) before stopping", PausedMessage, len(res.Undone))}
				}
				if errors.Is(askErr, ErrAskYolo) { // YOLO was turned on while it waited: run undo again, it will not ask
					_ = s.Data(res)
					return ipc.Exit{Code: ExitDenied, ErrorCode: "CONFIRMATION_WITHDRAWN", Error: fmt.Sprintf("%s; undid %d change(s) before stopping", YoloWithdrawnMessage, len(res.Undone))}
				}
				if errors.Is(askErr, ErrAskBusy) { // never asked: not a denial
					_ = s.Data(res)
					return ipc.Exit{Code: ExitDenied, ErrorCode: "WAITING_FOR_ANSWER", Error: askErr.Error()}
				}
				if ctx.Err() != nil {
					break // the client left, or serve stopped, while asking
				}
				// Back in flight, whatever the answer, while the loop goes on.
				r, other := d.admit(undoPrefix(d.o.Lang)+req.Target, true)
				if other != "" {
					_ = s.Data(res)
					return ipc.Exit{Code: ExitDenied, ErrorCode: "WAITING_FOR_ANSWER", Error: other}
				}
				release = r
				why := ""
				switch {
				case askErr != nil:
					why = "denied: " + askErr.Error()
				case decision == confirm.Nobody:
					why = "denied: nobody could confirm"
				case decision == confirm.Timeout:
					why = "denied: confirmation expired"
				case decision != confirm.Allow:
					why = "denied by user"
				}
				if why != "" {
					item.Error = why
					res.Skipped = append(res.Skipped, item)
					denied++
					continue
				}
				// Paused while it was asked (an Allow may come after the pause).
				if d.paused() {
					_ = s.Data(res)
					return ipc.Exit{Code: ExitDenied, ErrorCode: "PAUSED", Error: fmt.Sprintf("%s; undid %d change(s) before stopping", PausedMessage, len(res.Undone))}
				}
			}
		}
		if src, ok := moveBackTarget(e.Inverse.Argv); ok {
			// Something may have been created where the file was: moving it
			// back would overwrite it, or move it inside a new folder.
			k, err := precheck.Kind(ctx, d.o.ADB, src)
			if err != nil {
				item.Error = "could not check " + src + " before moving it back: " + err.Error()
				res.Failed = append(res.Failed, item)
				fmt.Fprintf(s.Stderr(), "✗ %s: %s\n", classify.Printable(e.Description), classify.Printable(item.Error))
				continue
			}
			if k != "none" {
				item.Error = src + " exists again: undo it by hand"
				res.Skipped = append(res.Skipped, item)
				reappeared = append(reappeared, e.ID+" "+item.Error)
				continue
			}
		}
		now := d.o.Now().UTC()
		u := journal.Entry{
			ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Serial: d.identity(), Source: journal.SourceUndo,
			Argv: e.Inverse.Argv, Level: level, Rule: rule, Description: undoPrefix(d.o.Lang) + e.Description, Undoes: e.ID,
			Yolo: yoloAtStart || rule == "undo-yolo", // YOLO could expire between the lift and here
		}
		item.UndoID = u.ID
		// The mark goes first, outside the agent's reach: if the host dies
		// after the inverse, the change stays uncertain even when the agent
		// deletes the journal lines.
		if err := appendUndoneLog(d.o.UndoneLog, "running "+e.ID); err != nil {
			_ = s.Data(res)
			return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNWRITABLE", Error: fmt.Sprintf("the undo log cannot be written (%v); undid %d change(s) before stopping", err, len(res.Undone))}
		}
		running := u
		running.Status = journal.StatusRunning
		if err := d.o.Journal.Append(running); err != nil {
			_ = appendUndoneLog(d.o.UndoneLog, "failed "+e.ID) // nothing ran; if this fails too, it stays uncertain
			_ = s.Data(res)
			return ipc.Exit{Code: 1, ErrorCode: "JOURNAL_UNWRITABLE", Error: fmt.Sprintf("the journal cannot be written (%v); undid %d change(s) before stopping", err, len(res.Undone))}
		}
		var errb bytes.Buffer
		notes.tell(s.Stderr()) // right before it runs, not when the undo ends blocked
		code, runErr := d.o.ADB.Run(ctx, "", e.Inverse.Argv, nil, io.Discard, &errb)
		if runErr != nil && code == 0 {
			code = 1
		}
		u.ExitCode = &code
		if runErr != nil || code != 0 {
			if ctx.Err() != nil {
				// The stop cut the inverse off: adb may have applied it before
				// dying. Leave the running mark (no "failed") so the change
				// stays uncertain instead of inviting a retry.
				_ = s.Data(res)
				return ipc.Exit{Code: 1, ErrorCode: "UNDO_FAILED", Error: fmt.Sprintf("stopped while undoing %s; it is uncertain, check the phone", e.ID)}
			}
			u.Status = journal.StatusError
			msg := strings.TrimSpace(errb.String())
			if runErr != nil {
				msg = runErr.Error()
			}
			u.Reason, item.Error = msg, msg
			res.Failed = append(res.Failed, item)
			d.record(u)
			// It may be retried; if this mark cannot be written it stays uncertain.
			_ = appendUndoneLog(d.o.UndoneLog, "failed "+e.ID)
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
		fmt.Fprintf(s.Stderr(), "– %s: %s\n", classify.Printable(it.Description), classify.Printable(it.Error))
	}
	_ = s.Data(res)
	if len(res.Failed) > 0 {
		return ipc.Exit{Code: 1, ErrorCode: "UNDO_FAILED", Error: fmt.Sprintf("%d of %d inverses failed", len(res.Failed), len(plan))}
	}
	if ctx.Err() != nil {
		return ipc.Exit{Code: 1, ErrorCode: "UNDO_FAILED", Error: "cancelled"}
	}
	if len(res.Undone) == 0 && denied > 0 {
		return ipc.Exit{Code: ExitDenied, ErrorCode: "UNDO_DENIED", Error: fmt.Sprintf("%d inverse(s) needed your confirmation and did not get it", denied)}
	}
	if len(res.Undone) == 0 && len(reappeared) > 0 {
		return ipc.Exit{Code: 1, ErrorCode: "UNCERTAIN_STATE", Error: strings.Join(reappeared, "; ")}
	}
	return ipc.Exit{}
}

// moveBackTarget returns src for an inverse `shell mv <dst> <src>` (already
// accepted by validInverse).
func moveBackTarget(argv []string) (string, bool) {
	if len(argv) != 4 || argv[0] != "shell" {
		return "", false
	}
	if w, ok := unquote(argv[1]); !ok || w != "mv" {
		return "", false
	}
	return unquote(argv[3])
}

func undoPrefix(lang string) string { return i18n.Lang(lang).T("undo.prefix") }

// States in the undone log. Its lines are "running <id>" (written and synced
// before the inverse runs), "<id>" (undone) and "failed <id>" (the inverse
// failed, so the change may be undone again).
const (
	undoneRunning = "running"
	undoneDone    = "done"
)

// readUndoneLog reads the undone log ("" = no log) into the state of each id.
func readUndoneLog(path string) (map[string]string, error) {
	state := map[string]string{}
	if path == "" {
		return state, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) == 1:
			state[f[0]] = undoneDone
		case len(f) == 2 && f[0] == "running" && state[f[1]] != undoneDone:
			state[f[1]] = undoneRunning
		case len(f) == 2 && f[0] == "failed" && state[f[1]] == undoneRunning:
			delete(state, f[1])
		}
	}
	return state, nil
}

// appendUndoneLog appends one line to the undone log and syncs it.
func appendUndoneLog(path, line string) error {
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
	if _, err := f.WriteString(line + "\n"); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
