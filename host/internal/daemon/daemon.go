// Package daemon serves the shim's requests: classify, precheck, confirm,
// journal and run the real adb (spec §4). It fails closed: if the journal
// cannot be written, nothing 🟡 or 🔴 runs.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
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
	Serial     string // serial at start; the agent's ANDROID_SERIAL
	// RemoteActive says whether the remote mode is on (Closing counts as
	// on): then the default capture is JPEG. nil: off. Called concurrently.
	RemoteActive func() bool
	// Serials: the serials the agent may name besides Serial and the identity
	// (every one the session used: the agent keeps the ANDROID_SERIAL it
	// started with when the phone changes address). nil: only Serial.
	// Called concurrently.
	Serials func() []string
	// CurrentSerial is the serial adb lists the phone with now, which Ping
	// reports. nil: Serial. Called concurrently.
	CurrentSerial func() string
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
	OnNotice       func(string)        // what the person running serve should know (a 🔴 waiting for changes in flight); escaped; called concurrently
	// Workspace is the root of the agent's environment: adb pull and adb
	// bugreport may only write inside it. Empty: they always need confirmation.
	Workspace string
	// UndoneLog lists the undos started, finished or failed (see
	// readUndoneLog), outside the agent's folder, so deleting journal lines
	// cannot make undo run an inverse twice. Empty in tests that do not need it.
	UndoneLog string
	// Paused reports whether the person paused the agent from the phone:
	// then every command and undo is refused (spec §4.6). It is asked again
	// after every wait (the lock, a confirmation, each inverse), so a pause
	// also stops what was already waiting. Nil: never paused. Called
	// concurrently: it must be safe across goroutines (an atomic.Bool, say).
	Paused func() bool
	// UI runs a helper operation (bridge.UIOp*) on the phone app and returns
	// its result (M5 spec §2): its errors are ErrNoApp, ErrUITimeout, a
	// *UIError the app answered, or the context's. Nil: there is no app, and
	// adb does what it can. Called concurrently.
	UI func(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error)
	// Protected reports whether pkg is one of the user's protected apps (M5
	// spec §4.3); ADBuddy itself always is. Nil: only ADBuddy. Called
	// concurrently.
	Protected func(pkg string) bool
	Now       func() time.Time
	// Yolo is the state of the YOLO mode. Nil: New creates one (off).
	Yolo *Yolo
}

// Daemon handles IPC requests.
type Daemon struct {
	o Options
	// changes serialises commands with an inverse and undo: prechecks read
	// the phone's state, and another change in between would make the
	// recorded inverse wrong.
	changes sync.Mutex

	// asking is the 🔴 question waiting for an answer, on the phone or in
	// the terminal, or waiting for the changes in flight to finish before
	// it is asked. While there is one, every other command is refused: an
	// `input tap` sent meanwhile could press Allow on the phone.
	// inFlight are the 🟡/🔴 commands and undos admitted to adb and not
	// finished yet: a question is only shown once there are none, so a
	// `sleep 3; input tap` already running cannot press Allow either. 🟢
	// commands cannot inject touches and are not counted.
	askMu    sync.Mutex
	asking   *question
	inFlight map[*flight]bool
	idle     chan struct{} // closed when inFlight becomes empty

	// beforeRun, in tests, runs right before a command is admitted to adb.
	beforeRun func(argv []string)
}

// question is a 🔴 confirmation in progress.
type question struct {
	title    string
	withdraw context.CancelCauseFunc
	shown    bool // asked on the phone or in the terminal; guarded by askMu
	// own: asked through AskUser: serve's own questions (`adbuddy remote on`, the person's) and the 🔴 inverse of an undo
	// (the agent's). liftQuestion keeps the doors closed for both while they wait: it fails closed, and a question
	// of an undo asked before YOLO was turned on stays the person's to answer.
	own bool
}

// blocks is why other commands are refused while q waits, in the shim's
// format (Blocked by ADBuddy: <reason>). askMu must be held.
func (q *question) blocks() string {
	if !q.shown {
		return `waiting to ask about "` + q.title + `"`
	}
	return `waiting for your answer to "` + q.title + `"`
}

// errWithdrawn is why a question is withdrawn when the agent is paused.
var errWithdrawn = i18n.M("confirm.cause.paused").Err()

// flight is a change admitted to adb.
type flight struct{ title string }

// ask makes q the question waiting for an answer. If another one is
// waiting, q is not registered and ask returns why (see blocks).
func (d *Daemon) ask(q *question) (busy string) {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	if d.asking != nil {
		return d.asking.blocks()
	}
	d.asking = q
	return ""
}

// show marks q as asked, right before it shows up.
func (d *Daemon) show(q *question) {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	q.shown = true
}

// answered removes q once its answer is in. An allowed question's command
// is admitted to adb in the same step, so a 🔴 that registers right after
// waits for it instead of blocking it; call the returned release when it
// finishes.
func (d *Daemon) answered(q *question, allowed bool) (release func()) {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	if d.asking == q {
		d.asking = nil
	}
	if !allowed {
		return func() {}
	}
	return d.flyLocked(q.title)
}

// admit lets a command reach adb unless a question is waiting (then it
// returns why, see blocks). A change (change true) counts as in flight
// until release is called.
func (d *Daemon) admit(title string, change bool) (release func(), other string) {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	if d.asking != nil {
		return nil, d.asking.blocks()
	}
	if !change {
		return func() {}, ""
	}
	return d.flyLocked(title), ""
}

// flyLocked adds a change in flight; askMu must be held.
func (d *Daemon) flyLocked(title string) (release func()) {
	f := &flight{title: title}
	if d.inFlight == nil {
		d.inFlight = map[*flight]bool{}
	}
	if len(d.inFlight) == 0 {
		d.idle = make(chan struct{})
	}
	d.inFlight[f] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			d.askMu.Lock()
			defer d.askMu.Unlock()
			delete(d.inFlight, f)
			if len(d.inFlight) == 0 {
				close(d.idle)
			}
		})
	}
}

// waitChanges waits until no change is in flight before q is asked, and
// says so once. It ends early when ctx does (the agent left, or a pause
// withdrew q).
func (d *Daemon) waitChanges(ctx context.Context, q *question) error {
	told := false
	for {
		d.askMu.Lock()
		if len(d.inFlight) == 0 {
			d.askMu.Unlock()
			return nil
		}
		idle, running := d.idle, ""
		for f := range d.inFlight {
			running = f.title
			break
		}
		d.askMu.Unlock()
		if !told {
			told = true
			d.o.OnNotice(d.lang().T("daemon.waiting", classify.Visible(running), classify.Visible(q.title)))
		}
		select {
		case <-idle:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// waitingFor returns why commands are refused while a question waits (see
// blocks), or "" when none does.
func (d *Daemon) waitingFor() string {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	if d.asking == nil {
		return ""
	}
	return d.asking.blocks()
}

// CancelConfirmations withdraws the question waiting for an answer, if any.
// Call it after the person pauses from the phone (once Paused reports true):
// the command then ends blocked as paused at once, instead of when the
// question expires.
func (d *Daemon) CancelConfirmations() {
	d.askMu.Lock()
	defer d.askMu.Unlock()
	if d.asking != nil {
		d.asking.withdraw(errWithdrawn)
	}
}

// ErrAskBusy is AskUser's error when another question is waiting for an
// answer (the 🔴 of a command, or another of serve's own).
var ErrAskBusy = errors.New("another question is open")

// ErrAskPaused is AskUser's error when the agent is paused from the phone,
// before the question or while it was asked: whatever the answer, it counts
// for nothing.
var ErrAskPaused = errors.New(PausedMessage)

// AskUser asks the user a question of serve's own (`adbuddy remote on`)
// through the same door as a 🔴 one: while it waits for its answer, and
// while it waits for the changes in flight to end before it shows up, every
// command and undo of the agent is refused (an `input tap` could press Allow
// on the phone). One question at a time: if another one is waiting it
// returns ErrAskBusy without asking. A pause withdraws it
// (CancelConfirmations) and ends it with ErrAskPaused. The question expires
// timeout after it shows up (r.ExpiresAt is set then); timeout <= 0 means
// the daemon's ConfirmTimeout. On an error the decision is Deny.
func (d *Daemon) AskUser(ctx context.Context, r confirm.Request, timeout time.Duration) (confirm.Decision, error) {
	if timeout <= 0 {
		timeout = d.o.ConfirmTimeout
	}
	qctx, withdraw := context.WithCancelCause(ctx)
	defer withdraw(nil)
	q := &question{title: r.Title, withdraw: withdraw, own: true}
	if busy := d.ask(q); busy != "" {
		return confirm.Deny, fmt.Errorf("%w: %s", ErrAskBusy, busy)
	}
	// A pause that came before the question was registered did not
	// withdraw it: look again now that it is.
	if d.paused() {
		d.answered(q, false)
		return confirm.Deny, ErrAskPaused
	}
	if err := d.waitChanges(qctx, q); err != nil {
		d.answered(q, false)
		if d.paused() || withdrawnByPause(qctx) {
			return confirm.Deny, ErrAskPaused
		}
		return confirm.Deny, err
	}
	d.show(q)
	cctx, cancel := context.WithTimeout(qctx, timeout)
	r.ExpiresAt = d.o.Now().UTC().Add(timeout)
	decision := d.o.Confirmer.Confirm(cctx, r)
	cancel()
	// Nothing of serve's own runs as a change in flight: the caller acts
	// after this returns.
	d.answered(q, false)
	if d.paused() || withdrawnByPause(qctx) {
		return confirm.Deny, ErrAskPaused
	}
	return decision, nil
}

// withdrawnByPause reports whether a pause withdrew the question of qctx
// (see CancelConfirmations).
func withdrawnByPause(qctx context.Context) bool {
	return errors.Is(context.Cause(qctx), errWithdrawn)
}

// abandoned picks the journal's reason for a 🔴 that ends because its
// request did: byAgent when the agent left, byServe when serve stopped
// (Ctrl+C), which is not the agent's doing.
func abandoned(ctx context.Context, byAgent, byServe string) string {
	if errors.Is(context.Cause(ctx), ipc.ErrClosed) {
		return byServe
	}
	return byAgent
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
	if o.OnNotice == nil {
		o.OnNotice = func(string) {}
	}
	if o.Paused == nil {
		o.Paused = func() bool { return false }
	}
	if o.ConfirmTimeout <= 0 {
		o.ConfirmTimeout = 2 * time.Minute
	}
	if o.Yolo == nil {
		o.Yolo = NewYolo(o.Now, nil)
	}
	d := &Daemon{o: o}
	o.Yolo.Subscribe(d.yoloEvent)
	return d
}

// lang is the language of what serve tells the person (Options.Lang).
func (d *Daemon) lang() i18n.Lang { return i18n.Lang(d.o.Lang) }

// Yolo is the state of the YOLO mode, for the parts of serve that turn it off
// or tell the phone (the daemon is created after the phone link).
func (d *Daemon) Yolo() *Yolo { return d.o.Yolo }

// paused reports whether the person paused the agent from the phone (Options.Paused).
// A pause also turns YOLO off, for good: resuming does not bring it back. It is
// the only way the daemon asks about the pause.
func (d *Daemon) paused() bool {
	if !d.o.Paused() {
		return false
	}
	d.o.Yolo.Off(YoloOffPause)
	return true
}

// identity is the name the journal and undo know the phone by.
func (d *Daemon) identity() string {
	if d.o.Identity != "" {
		return d.o.Identity
	}
	return d.o.Serial
}

// currentSerial is the serial adb lists the phone with now.
func (d *Daemon) currentSerial() string {
	if d.o.CurrentSerial != nil {
		return d.o.CurrentSerial()
	}
	return d.o.Serial
}

// knownSerial reports whether s names the active phone for the agent: this
// session's serial, the identity, or a serial used earlier in the session.
func (d *Daemon) knownSerial(s string) bool {
	if s == d.o.Serial || s == d.identity() {
		return true
	}
	if d.o.Serials != nil {
		for _, k := range d.o.Serials() {
			if s == k {
				return true
			}
		}
	}
	return false
}

// Handle serves one request.
func (d *Daemon) Handle(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	switch req.Kind {
	case ipc.KindExec:
		return d.exec(ctx, req, s)
	case ipc.KindUndo:
		return d.undo(ctx, req, s)
	case ipc.KindUI:
		return d.ui(ctx, req, s)
	case ipc.KindPing:
		_ = s.Data(ipc.Ping{Version: d.o.Version, Session: d.o.Session, Serial: d.currentSerial(), Identity: d.identity(), Journal: d.o.Journal.Dir(), Paused: d.paused()})
		return ipc.Exit{}
	}
	return ipc.Exit{Code: 1, ErrorCode: "UNKNOWN_REQUEST", Error: "unknown request " + req.Kind}
}

func (d *Daemon) exec(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	now := d.o.Now().UTC()
	e := journal.Entry{ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Serial: d.identity(), Source: journal.SourceUser, Argv: req.Argv, Yolo: d.o.Yolo.Active()}
	if req.Env["ADBUDDY_SOURCE"] == journal.SourceAgent {
		e.Source = journal.SourceAgent
	}
	// What YOLO lets through is noted here and told to the agent right before adb runs.
	ctx, notes := withYoloNotes(ctx)

	// release ends this command's time in flight (see admit).
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	g, args := classify.SplitGlobal(req.Argv)
	res := d.o.Classifier.Classify(args) // pure: nothing reaches adb yet
	if d.paused() {
		p := e // say what was attempted, at its level
		p.Level, p.Rule, p.Description = res.Level.String(), res.Primary.Rule, res.Description(d.o.Lang)
		return d.block(p, s, PausedMessage)
	}
	if t := d.waitingFor(); t != "" && !d.liftQuestion(ctx, t) {
		p := e
		p.Level, p.Rule, p.Description = res.Level.String(), res.Primary.Rule, res.Description(d.o.Lang)
		return d.block(p, s, t)
	}
	// Options that reach another adb server are never passed on (see classify.SplitGlobal): under YOLO the command runs through ADBuddy's own.
	if len(g.Forbidden) > 0 && !d.lift(ctx, "the adb option "+classify.Printable(g.Forbidden[0])+" is not allowed and was dropped: it ran through ADBuddy's own adb server") {
		return d.block(e, s, "the adb option "+classify.Printable(g.Forbidden[0])+" is not allowed: ADBuddy only talks to the active phone through its own adb server")
	}
	// The phone may be named by this session's serial or by its identity;
	// either way adb runs with -s Serial (the -s given is not passed on). Under
	// YOLO another phone named runs on the active one: the host's adb only reaches that.
	for _, serial := range []string{g.Serial, req.Env["ANDROID_SERIAL"]} {
		if serial != "" && !d.knownSerial(serial) && !d.lift(ctx, "it named "+classify.Printable(serial)+", which is not the active phone ("+d.identity()+"): it ran on the active phone") {
			return d.block(e, s, "only the active phone can be used ("+d.identity()+")")
		}
	}

	e.Level, e.Rule, e.Description = res.Level.String(), res.Primary.Rule, res.Description(d.o.Lang)
	level, reason := res.Level, classify.Text{} // reason: why the PC or the phone raised it, for the detail of the question
	if len(res.PCWrites) > 0 {
		// pull and bugreport write on the PC from the agent's working
		// directory, which the classifier does not know.
		if why := d.checkPCWrites(req.Cwd, res.PCWrites); !why.Empty() {
			level, reason = classify.Red, why
			e.Level = level.String()
		}
	}

	if level >= classify.Yellow {
		if err := d.o.Journal.Writable(); err != nil && !d.lift(ctx, noRecord) {
			return d.block(e, s, "the journal cannot be written ("+err.Error()+"); without a journal no changes are allowed")
		}
	}
	var prep precheck.Prepared
	if res.Op != nil {
		d.changes.Lock()
		defer d.changes.Unlock()
		if d.paused() { // paused while waiting for the lock
			return d.block(e, s, PausedMessage)
		}
		prep = precheck.Prepare(ctx, d.o.ADB, res.Op, level, precheck.Env{Cwd: req.Cwd, BackupDir: d.o.Journal.BackupDir(e.ID),
			MakeBackupDir: func() (string, error) { return d.o.Journal.MakeBackupDir(e.ID) }})
		level = prep.Level
		reason = classify.Join(reason, prep.Reason)
		e.Level = level.String()
		if prep.Inverse != nil {
			e.Inverse = &journal.Inverse{Argv: prep.Inverse}
		}
		e.Backups = prep.Backups
	}

	redWhy := "this asked for your confirmation (" + classify.Printable(e.Description) + ") and ran without it"
	if !reason.Empty() {
		redWhy += "; " + classify.Printable(reason.In(d.o.Lang))
	}
	redLifted := level == classify.Red && d.lift(ctx, redWhy)
	if level == classify.Red && !redLifted {
		detail := res.Detail(d.o.Lang)
		if !reason.Empty() {
			detail = strings.TrimSpace(detail + "\n" + classify.Printable(reason.In(d.o.Lang)))
		}
		// Key lets the phone allow the same rule with the same arguments from
		// the same folder for the rest of the session. A 🔴 that comes from
		// the state of the phone or the PC (a precheck, a write on the PC)
		// may be fine now and not next time: no key, so it is always asked.
		// Nor one whose effect may come from stdin (A4-1): the key does not
		// cover it, and the next stdin could be anything. This does not
		// depend on the classification (classify.StdinDecides).
		key := ""
		stdin := classify.StdinDecides(args)
		if reason.Empty() && !stdin {
			key = e.Rule + "\x00" + req.Cwd + "\x00" + strings.Join(req.Argv, "\x00")
		}
		qctx, withdraw := context.WithCancelCause(ctx)
		defer withdraw(nil)
		q := &question{title: e.Description, withdraw: withdraw}
		if busy := d.ask(q); busy != "" {
			return d.block(e, s, busy)
		}
		// A pause that came before the question was registered did not
		// withdraw it: look again now that it is.
		if d.paused() {
			d.answered(q, false)
			return d.block(e, s, PausedMessage)
		}
		// Registered first, so nothing new starts; then the changes
		// already running must end before the question shows up.
		if err := d.waitChanges(qctx, q); err != nil {
			d.answered(q, false)
			// Withdrawn by a pause, even one already resumed.
			if d.paused() || withdrawnByPause(qctx) {
				return d.block(e, s, PausedMessage)
			}
			code := ExitDenied
			e.Status, e.Reason, e.ExitCode = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it was asked", "abandoned: ADBuddy stopped before asking"), &code
			d.record(e)
			return ipc.Exit{Code: ExitDenied}
		}
		// The expiry counts from when the question is shown; from now on the
		// refused commands say it waits for an answer.
		d.show(q)
		asked := d.o.Now().UTC()
		cctx, cancel := context.WithTimeout(qctx, d.o.ConfirmTimeout)
		decision := d.o.Confirmer.Confirm(cctx, confirm.Request{
			ID: e.ID, Title: e.Description, Detail: detail, RawCommand: classify.DisplayCommand(req.Argv),
			ExpiresAt: asked.Add(d.o.ConfirmTimeout), Special: res.Special, Key: key, Stdin: stdin,
		})
		cancel()
		release = d.answered(q, decision == confirm.Allow)
		// Paused while it was asked: whatever the answer (or none, if the
		// pause withdrew it, even if it was resumed since), it ends blocked.
		if d.paused() || withdrawnByPause(qctx) {
			return d.block(e, s, PausedMessage)
		}
		if decision != confirm.Allow {
			e.Status, e.Reason = journal.StatusDenied, "denied by user"
			msg := "Denied by user: " + e.Description
			switch decision {
			case confirm.Nobody:
				e.Reason = "denied: nobody to confirm (serve --confirm deny or no terminal)"
				msg = "Denied: nobody could confirm: " + e.Description
			case confirm.Timeout:
				e.Status, e.Reason = journal.StatusTimeout, "confirmation expired"
				msg += " (confirmation expired)"
			}
			if errors.Is(context.Cause(ctx), ipc.ErrClosed) {
				// Ctrl+C on serve while asking: nobody said no. The shim's
				// message stays the literal one.
				e.Status, e.Reason = journal.StatusDenied, "abandoned: ADBuddy stopped while asking"
			}
			code := ExitDenied
			e.ExitCode = &code
			d.record(e)
			fmt.Fprintln(s.Stderr(), msg)
			return ipc.Exit{Code: ExitDenied}
		}
		if ctx.Err() != nil { // allowed, but the agent is gone: nobody waits for the result
			code := ExitDenied
			e.Status, e.Reason, e.ExitCode = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it ran", "abandoned: ADBuddy stopped before it ran"), &code
			d.record(e)
			return ipc.Exit{Code: ExitDenied}
		}
	}

	if d.beforeRun != nil {
		d.beforeRun(req.Argv)
	}
	// Paused during the confirmation (an Allow may come after the pause) or
	// the prechecks: refuse before any running record and before adb.
	if d.paused() {
		return d.block(e, s, PausedMessage)
	}
	// Admitted to adb, unless a question started after this command got
	// past the door. A 🔴 was admitted with its Allow.
	if level != classify.Red || redLifted {
		r, other := d.admit(e.Description, level >= classify.Yellow)
		if other != "" {
			if !d.liftQuestion(ctx, other) {
				return d.block(e, s, other)
			}
			r = d.flyAnyway(level >= classify.Yellow, e.Description)
		}
		release = r
	}
	// The guard (M5 spec §4.3), after the doors: nothing touches or reads the
	// screen with a protected app, or ADBuddy, in front.
	if why := d.guard(ctx, res, args); why != "" {
		return d.refuse(ctx, e, s, why)
	}
	// The notice of the on-screen frame, before the last pause check (a pause
	// that arrives meanwhile blocks) and after the guard (no notice for what it refuses).
	if readsScreen(res) {
		if why := d.showNotice(ctx); why != "" && !d.lift(ctx, why) {
			return d.refuse(ctx, e, s, why)
		}
	}
	if d.paused() { // paused while the guard looked
		return d.block(e, s, PausedMessage)
	}
	if level >= classify.Yellow {
		running := e
		running.Status = journal.StatusRunning
		if err := d.o.Journal.Append(running); err != nil && !d.lift(ctx, noRecord) {
			return d.block(e, s, "the journal cannot be written ("+err.Error()+"); without a journal no changes are allowed")
		}
	}

	notes.tell(s.Stderr())
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
			d.o.OnWarning(d.lang().T("daemon.inverse_failed", e.ID, err.Error()))
		} else if inv != nil {
			e.Inverse = &journal.Inverse{Argv: inv}
		}
	}
	d.record(e)
	return ipc.Exit{Code: code}
}

// PausedMessage explains why nothing runs while paused.
const PausedMessage = "paused from the phone: nothing runs until you resume it there"

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
		d.o.OnWarning(d.lang().T("daemon.journal_failed", e.ID, err.Error()))
	}
	d.o.OnEntry(e)
}
