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
	"github.com/hybriqtech/adbuddy/host/internal/shq"
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
	Classifier     *classify.Classifier
	ADB            Executor
	Journal        *journal.Store
	Confirmer      confirm.Confirmer
	Serial         string // active device
	Session        string
	Lang           string
	ConfirmTimeout time.Duration
	Version        string
	OnEntry        func(journal.Entry) // final records, for the live view; fields are untrusted, escape before printing; called concurrently
	OnWarning      func(string)        // problems the person running serve should see; called concurrently
	Now            func() time.Time
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

// Handle serves one request.
func (d *Daemon) Handle(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	switch req.Kind {
	case ipc.KindExec:
		return d.exec(ctx, req, s)
	case ipc.KindUndo:
		return d.undo(ctx, req, s)
	case ipc.KindPing:
		_ = s.Data(ipc.Ping{Version: d.o.Version, Session: d.o.Session, Serial: d.o.Serial, Journal: d.o.Journal.Dir()})
		return ipc.Exit{}
	}
	return ipc.Exit{Code: 1, ErrorCode: "UNKNOWN_REQUEST", Error: "unknown request " + req.Kind}
}

func (d *Daemon) exec(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	now := d.o.Now().UTC()
	e := journal.Entry{ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Source: journal.SourceUser, Argv: req.Argv}
	if req.Env["ADBUDDY_SOURCE"] == journal.SourceAgent {
		e.Source = journal.SourceAgent
	}

	g, args := classify.SplitGlobal(req.Argv)
	if len(g.Forbidden) > 0 {
		return d.block(e, s, "the adb option "+classify.Printable(g.Forbidden[0])+" is not allowed: ADBuddy only talks to the active phone through its own adb server")
	}
	for _, serial := range []string{g.Serial, req.Env["ANDROID_SERIAL"]} {
		if serial != "" && serial != d.o.Serial {
			return d.block(e, s, "only the active phone can be used ("+d.o.Serial+")")
		}
	}

	res := d.o.Classifier.Classify(args)
	e.Level, e.Rule, e.Description = res.Level.String(), res.Primary.Rule, res.Description(d.o.Lang)
	level, reason := res.Level, ""

	if res.Level >= classify.Yellow {
		if err := d.o.Journal.Writable(); err != nil {
			return d.block(e, s, "the journal cannot be written ("+err.Error()+"); without a journal no changes are allowed")
		}
	}
	var prep precheck.Prepared
	if res.Op != nil {
		d.changes.Lock()
		defer d.changes.Unlock()
		prep = precheck.Prepare(ctx, d.o.ADB, res.Op, level, precheck.Env{Cwd: req.Cwd, BackupDir: d.o.Journal.BackupDir(e.ID)})
		level, reason = prep.Level, prep.Reason
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
			ID: e.ID, Title: e.Description, Detail: detail, RawCommand: rawCommand(req.Argv),
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

// rawCommand shows the command as the person should read it. After `shell`
// adb joins the words with spaces, so they are shown as they are; elsewhere
// each argument is one word and is quoted when needed.
func rawCommand(argv []string) string {
	_, rest := classify.SplitGlobal(argv)
	if len(rest) > 0 && rest[0] == "shell" {
		return "adb " + classify.Printable(strings.Join(argv, " "))
	}
	return "adb " + classify.Printable(shq.Join(argv...))
}
