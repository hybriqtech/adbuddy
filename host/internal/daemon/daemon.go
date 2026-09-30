// Package daemon serves the shim's requests: classify, precheck, confirm,
// journal and run the real adb (spec §4). It fails closed: if the journal
// cannot be written, nothing 🟡 or 🔴 runs.
package daemon

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/precheck"
)

// ExitDenied is the shim's exit code when a command is denied, expires or is
// blocked (spec §4.5).
const ExitDenied = 126

// Executor runs the real adb for the active device.
type Executor interface {
	precheck.Device
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// Stream is the client side of a request (implemented by *ipc.Conn).
type Stream interface {
	Stdin() io.Reader
	Stdout() io.Writer
	Stderr() io.Writer
	Data(v any) error
}

// Options configure a Daemon.
type Options struct {
	Classifier *classify.Classifier
	ADB        Executor
	Journal    *journal.Store
	Confirmer  confirm.Confirmer
	Serial     string // active device, as adb lists it this session: every command runs with -s Serial
	// Identity is the phone's stable name: its mDNS name
	// (<guid>._adb-tls-connect._tcp) even when adb lists it this session as
	// "<guid> (2)…" or as ip:port. The journal records it, undo compares it,
	// and the agent may name the phone with it. Empty: Serial.
	Identity       string
	Session        string
	Lang           string
	ConfirmTimeout time.Duration
	Version        string
	OnEntry        func(journal.Entry) // final records, for the live view; fields are untrusted, escape before printing; called concurrently
	OnWarning      func(string)        // problems the person running serve should see; called concurrently
	// Workspace is the root of the agent's environment: adb pull and adb
	// bugreport may only write inside it. Empty: they always need confirmation.
	Workspace string
	// UndoneLog lists the undos started, finished or failed (see
	// readUndoneLog), outside the agent's folder, so deleting journal lines
	// cannot make undo run an inverse twice. Empty in tests that do not need it.
	UndoneLog string
	Now       func() time.Time
}

// Daemon handles IPC requests.
type Daemon struct {
	o Options
	// changes serialises commands with an inverse and undo: prechecks read
	// the phone's state, and another change in between would make the
	// recorded inverse wrong.
	changes sync.Mutex
}

// New returns a Daemon; missing optional fields get defaults.
func New(o Options) *Daemon {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.OnEntry == nil {
		o.OnEntry = func(journal.Entry) {}
	}
	if o.OnWarning == nil {
		o.OnWarning = func(string) {}
	}
	if o.ConfirmTimeout <= 0 {
		o.ConfirmTimeout = 2 * time.Minute
	}
	return &Daemon{o: o}
}

// identity is the name the journal and undo know the phone by.
func (d *Daemon) identity() string {
	if d.o.Identity != "" {
		return d.o.Identity
	}
	return d.o.Serial
}

// Handle serves one request.
func (d *Daemon) Handle(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	switch req.Kind {
	case ipc.KindExec:
		return d.exec(ctx, req, s)
	case ipc.KindUndo:
		return d.undo(ctx, req, s)
	case ipc.KindPing:
		_ = s.Data(ipc.Ping{Version: d.o.Version, Session: d.o.Session, Serial: d.o.Serial, Identity: d.identity(), Journal: d.o.Journal.Dir()})
		return ipc.Exit{}
	}
	return ipc.Exit{Code: 1, ErrorCode: "UNKNOWN_REQUEST", Error: "unknown request " + req.Kind}
}

func (d *Daemon) exec(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	now := d.o.Now().UTC()
	e := journal.Entry{ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Serial: d.identity(), Source: journal.SourceUser, Argv: req.Argv}
	if req.Env["ADBUDDY_SOURCE"] == journal.SourceAgent {
		e.Source = journal.SourceAgent
	}

	g, args := classify.SplitGlobal(req.Argv)
	if len(g.Forbidden) > 0 {
		return d.block(e, s, "the adb option "+classify.Printable(g.Forbidden[0])+" is not allowed: ADBuddy only talks to the active phone through its own adb server")
	}
	// The phone may be named by this session's serial or by its identity;
	// either way adb runs with -s Serial (the -s given is not passed on).
	for _, serial := range []string{g.Serial, req.Env["ANDROID_SERIAL"]} {
		if serial != "" && serial != d.o.Serial && serial != d.identity() {
			return d.block(e, s, "only the active phone can be used ("+d.identity()+")")
		}
	}

	res := d.o.Classifier.Classify(args)
	e.Level, e.Rule, e.Description = res.Level.String(), res.Primary.Rule, res.Description(d.o.Lang)
	level, reason := res.Level, ""
	if len(res.PCWrites) > 0 {
		// pull and bugreport write on the PC from the agent's working
		// directory, which the classifier does not know.
		if why := d.checkPCWrites(req.Cwd, res.PCWrites); why != "" {
			level, reason = classify.Red, why
			e.Level = level.String()
		}
	}

	if level >= classify.Yellow {
		if err := d.o.Journal.Writable(); err != nil {
			return d.block(e, s, "the journal cannot be written ("+err.Error()+"); without a journal no changes are allowed")
		}
	}
	var prep precheck.Prepared
	if res.Op != nil {
		d.changes.Lock()
		defer d.changes.Unlock()
		prep = precheck.Prepare(ctx, d.o.ADB, res.Op, level, precheck.Env{Cwd: req.Cwd, BackupDir: d.o.Journal.BackupDir(e.ID)})
		level = prep.Level
		if prep.Reason != "" {
			reason = strings.TrimPrefix(reason+"; "+prep.Reason, "; ")
		}
		e.Level = level.String()
		if prep.Inverse != nil {
			e.Inverse = &journal.Inverse{Argv: prep.Inverse}
		}
		e.Backups = prep.Backups
	}

	if level == classify.Red {
		detail := res.Detail(d.o.Lang)
		if reason != "" {
			detail = strings.TrimSpace(detail + "\n" + classify.Printable(reason))
		}
		cctx, cancel := context.WithTimeout(ctx, d.o.ConfirmTimeout)
		decision := d.o.Confirmer.Confirm(cctx, confirm.Request{
			ID: e.ID, Title: e.Description, Detail: detail, RawCommand: classify.DisplayCommand(req.Argv),
			ExpiresAt: now.Add(d.o.ConfirmTimeout), Special: res.Special,
		})
		cancel()
		if decision != confirm.Allow {
			e.Status, e.Reason = journal.StatusDenied, "denied by user"
			msg := "Denied by user: " + e.Description
			if decision == confirm.Timeout {
				e.Status, e.Reason = journal.StatusTimeout, "confirmation expired"
				msg += " (confirmation expired)"
			}
			code := ExitDenied
			e.ExitCode = &code
			d.record(e)
			fmt.Fprintln(s.Stderr(), msg)
			return ipc.Exit{Code: ExitDenied}
		}
		if ctx.Err() != nil { // allowed, but the agent is gone: nobody waits for the result
			code := ExitDenied
			e.Status, e.Reason, e.ExitCode = journal.StatusDenied, "abandoned by the agent before it ran", &code
			d.record(e)
			return ipc.Exit{Code: ExitDenied}
		}
	}

	if level >= classify.Yellow {
		running := e
		running.Status = journal.StatusRunning
		if err := d.o.Journal.Append(running); err != nil {
			return d.block(e, s, "the journal cannot be written ("+err.Error()+"); without a journal no changes are allowed")
		}
	}

	runArgs := append(append([]string{}, g.Pass...), args...)
	code, err := d.o.ADB.Run(ctx, req.Cwd, runArgs, s.Stdin(), s.Stdout(), s.Stderr())
	switch {
	case err != nil:
		code = 1
		e.Status, e.Reason = journal.StatusError, err.Error()
		fmt.Fprintf(s.Stderr(), "adbuddy: could not run adb: %v\n", err)
	case code == 0:
		e.Status = journal.StatusOK
	default:
		if code < 0 { // killed by a signal
			code = 1
		}
		e.Status = journal.StatusError
	}
	e.ExitCode = &code
	if prep.After != nil && code == 0 {
		if inv, err := prep.After(ctx); err != nil {
			d.o.OnWarning("could not compute the inverse of " + e.ID + ": " + err.Error())
		} else if inv != nil {
			e.Inverse = &journal.Inverse{Argv: inv}
		}
	}
	d.record(e)
	return ipc.Exit{Code: code}
}

// block refuses a command without running it.
func (d *Daemon) block(e journal.Entry, s Stream, reason string) ipc.Exit {
	code := ExitDenied
	e.Status, e.Reason, e.ExitCode = journal.StatusBlocked, reason, &code
	if e.Level == "" {
		e.Level = classify.Red.String()
	}
	d.record(e)
	fmt.Fprintln(s.Stderr(), "Blocked by ADBuddy: "+reason)
	return ipc.Exit{Code: ExitDenied}
}

// record appends the final record and shows it in the live view.
func (d *Daemon) record(e journal.Entry) {
	if err := d.o.Journal.Append(e); err != nil {
		d.o.OnWarning("journal write failed for " + e.ID + ": " + err.Error())
	}
	d.o.OnEntry(e)
}
