package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/screenshot"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// The agent's helpers (M5 spec §2-§5): adbuddy screen, tap, type, foreground
// and screenshot. They go through the same doors as an adb command (pause, a
// 🔴 question waiting, admission, the guard) and each one leaves a journal
// entry. The phone app does them (Options.UI); without it, adb does what it
// can.

// Errors of Options.UI besides a *UIError and the context's.
var (
	// ErrNoApp: no app is connected; nothing was sent.
	ErrNoApp = errors.New("the ADBuddy app is not connected")
	// ErrUITimeout: the app did not answer before the request expired.
	ErrUITimeout = errors.New("the ADBuddy app did not answer in time")
)

// UIError is an error the app answered (protocol spec §8.1). Text comes from
// the phone: escape it before printing.
type UIError struct{ Code, Text string }

func (e *UIError) Error() string { return e.Code + ": " + e.Text }

// OpScreenshot is the helper the host does itself (M5 spec §3.6).
const OpScreenshot = "screenshot"

// ScreenshotArgs are screenshot's args.
type ScreenshotArgs struct {
	Scale  float64 `json:"scale"`
	Format string  `json:"format"`
}

// ScreenshotResult is screenshot's data; the image goes on stdout.
type ScreenshotResult struct {
	Format string `json:"format"`
	Bytes  int    `json:"bytes"`
}

// Exit codes and error codes of the helpers (M5 spec §8).
const (
	exitUsage     = 2
	exitTransient = 75
)

// maxCapture bounds what screencap may send (a 4K PNG is far below).
const maxCapture = 64 << 20

// helper is what the daemon knows of each helper.
type helper struct {
	rule   string
	level  classify.Level
	title  classify.Text // {target} is replaced by what it acts on
	change bool          // counts as a change in flight (it can press Allow)
}

var helpers = map[string]helper{
	bridge.UIOpDump:          {"helper.screen", classify.Green, classify.Text{EN: "Read the screen", ES: "Leer la pantalla"}, false},
	bridge.UIOpForeground:    {"helper.foreground", classify.Green, classify.Text{EN: "Read the app in the foreground", ES: "Leer la app en primer plano"}, false},
	bridge.UIOpNotifications: {"helper.notifications", classify.Green, classify.Text{EN: "Read the notifications", ES: "Leer las notificaciones"}, false},
	OpScreenshot:             {"helper.screenshot", classify.Green, classify.Text{EN: "Take a screenshot", ES: "Hacer una captura de pantalla"}, false},
	bridge.UIOpClick:         {"helper.tap", classify.Yellow, classify.Text{EN: "Tap {target}", ES: "Tocar {target}"}, true},
	bridge.UIOpSetText:       {"helper.type", classify.Yellow, classify.Text{EN: "Type {target}", ES: "Escribir {target}"}, true},
}

// uiFailure is how a helper ended badly: the exit code, the error code for
// the CLI (UI_*) and the message.
type uiFailure struct {
	exit int
	code string
	msg  string
}

func fail(exit int, code, msg string) *uiFailure { return &uiFailure{exit, code, msg} }

// ui serves a KindUI request.
func (d *Daemon) ui(ctx context.Context, req ipc.Request, s Stream) ipc.Exit {
	h, ok := helpers[req.UIOp]
	if !ok {
		return ipc.Exit{Code: exitUsage, ErrorCode: "UI_INVALID", Error: "unknown helper " + classify.Printable(req.UIOp)}
	}
	target, err := checkUIArgs(req.UIOp, req.UIArgs)
	if err != nil {
		return ipc.Exit{Code: exitUsage, ErrorCode: "UI_INVALID", Error: err.Error()}
	}
	// The defaults are decided here, before the journal entry and the helper
	// run, so the journal records what was really done.
	req.UIArgs = d.withDefaults(req.UIOp, req.UIArgs)
	now := d.o.Now().UTC()
	e := journal.Entry{
		ID: journal.NewID("j_", now), TS: now, Session: d.o.Session, Serial: d.identity(), Source: journal.SourceUser,
		Argv: helperArgv(req.UIOp, req.UIArgs), Level: h.level.String(), Rule: h.rule,
		Description: strings.ReplaceAll(h.title.In(d.o.Lang), "{target}", target),
	}
	if req.Env["ADBUDDY_SOURCE"] == journal.SourceAgent {
		e.Source = journal.SourceAgent
	}
	if d.o.Paused() {
		return d.blockUI(e, PausedMessage)
	}
	if h.change {
		if err := d.o.Journal.Writable(); err != nil {
			return d.blockUI(e, "the journal cannot be written ("+err.Error()+"); without a journal no changes are allowed")
		}
	}
	// Like any adb command: refused while a 🔴 question waits, and a change
	// counts in flight so the next question waits for it.
	release, other := d.admit(e.Description, h.change)
	if other != "" {
		return d.blockUI(e, other)
	}
	defer release()
	// foreground is what the guard asks; notifications do not read the screen
	// (the app empties what a protected app says, and the host does it again).
	firstBlocked := false
	if req.UIOp != bridge.UIOpForeground && req.UIOp != bridge.UIOpNotifications {
		if why := d.guardForeground(ctx, nil); why != "" {
			if ctx.Err() != nil { // the agent or serve went away meanwhile
				return d.abandonUI(ctx, e)
			}
			if !(req.Screen && req.UIOp == bridge.UIOpDump) {
				return d.blockUI(e, why)
			}
			// screen --settle: maybe a transition (a window with no content for
			// a moment). The wait tries again with the guard on every attempt,
			// and its first valid read is the screen it starts from.
			firstBlocked = true
		}
	}
	guarded := time.Now() // right after the guard said yes
	if d.o.Paused() {     // paused while the guard looked
		return d.blockUI(e, PausedMessage)
	}
	// An act with Screen: read the screen before it, the one the wait starts
	// from. That read is only compared (never returned, written or journaled)
	// and it does not weaken the doors of the act: the context and the pause
	// are looked at again, the guard too if the read took longer than
	// baselineSlack, and if the app says something protected is in front the
	// act does not happen.
	var base *bridge.UIDump
	if req.Screen && (req.UIOp == bridge.UIOpClick || req.UIOp == bridge.UIOpSetText) {
		var denied *uiFailure
		base, denied = d.baselineScreen(ctx)
		if ctx.Err() != nil { // the agent or serve went away: no act
			return d.abandonUI(ctx, e)
		}
		if d.o.Paused() { // paused while the start was read: no act
			return d.blockUI(e, PausedMessage)
		}
		if denied != nil { // the app says there is something protected in front: no act
			return d.blockUI(e, denied.msg)
		}
		if time.Since(guarded) > baselineSlack { // the guard looked too long ago: look again before acting
			if why := d.guardForeground(ctx, nil); why != "" {
				if ctx.Err() != nil {
					return d.abandonUI(ctx, e)
				}
				return d.blockUI(e, why)
			}
			if d.o.Paused() {
				return d.blockUI(e, PausedMessage)
			}
		}
	}
	if h.change {
		running := e
		running.Status = journal.StatusRunning
		if err := d.o.Journal.Append(running); err != nil {
			return d.blockUI(e, "the journal cannot be written ("+err.Error()+"); without a journal no changes are allowed")
		}
	}

	var data any
	var via string
	var f *uiFailure
	if firstBlocked {
		// screen --settle after a block: nothing was read yet.
		var done *ipc.Exit
		if data, via, f, done = d.settleScreen(ctx, e, settleIn{args: req.UIArgs, start: time.Now(), adopt: true}); done != nil {
			return *done
		}
	} else {
		data, via, f = d.runUI(ctx, req.UIOp, req.UIArgs, s)
		// screen with Screen: wait for it to settle; the read above is the start.
		if f == nil && req.Screen && req.UIOp == bridge.UIOpDump {
			if first, ok := asDump(data); ok {
				var done *ipc.Exit
				if data, via, f, done = d.settleScreen(ctx, e, settleIn{args: req.UIArgs, base: &first, first: &first, via: via, start: time.Now()}); done != nil {
					return *done
				}
			}
		}
	}
	code := 0
	if f != nil {
		code = f.exit
		e.Status, e.Reason = journal.StatusError, f.msg
		if f.exit == ExitDenied {
			e.Status = journal.StatusBlocked
		}
	} else {
		e.Status, e.Reason = journal.StatusOK, "via "+via
	}
	e.ExitCode = &code
	d.record(e)
	if f != nil {
		return ipc.Exit{Code: f.exit, ErrorCode: f.code, Error: f.msg}
	}
	if data != nil {
		if req.Screen && f == nil && (req.UIOp == bridge.UIOpClick || req.UIOp == bridge.UIOpSetText) {
			release() // the change is done: a question must not wait for the screen to settle
			data = d.withScreen(ctx, req, data, base)
		}
		if err := s.Data(data); err != nil {
			return ipc.Exit{Code: 1, ErrorCode: "UI_FAILED", Error: err.Error()}
		}
	}
	return ipc.Exit{}
}

// settleScreen is the wait of screen --settle: what ui sends and how it was
// read, or its failure; or done, when ui returns it as is: a block (nothing
// read before comes out) or the agent or serve gone.
func (d *Daemon) settleScreen(ctx context.Context, e journal.Entry, in settleIn) (data any, via string, f *uiFailure, done *ipc.Exit) {
	scr, how, why, sf := d.settle(ctx, in)
	switch {
	case sf != nil:
		return nil, "", sf, nil
	case why != "":
		x := d.blockUI(e, why)
		return nil, "", nil, &x
	case scr == nil: // the agent or serve went away
		x := d.abandonUI(ctx, e)
		return nil, "", nil, &x
	}
	return *scr, how + ", settled=" + strconv.FormatBool(scr.Settled), nil, nil
}

// abandonUI journals a helper the agent or serve gave up on before it ran.
func (d *Daemon) abandonUI(ctx context.Context, e journal.Entry) ipc.Exit {
	code := ExitDenied
	e.Status, e.Reason, e.ExitCode = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it ran", "abandoned: ADBuddy stopped before it ran"), &code
	d.record(e)
	return ipc.Exit{Code: ExitDenied, ErrorCode: "UI_BLOCKED", Error: "stopped"}
}

// blockUI refuses a helper without running it, as block does a command.
func (d *Daemon) blockUI(e journal.Entry, reason string) ipc.Exit {
	code := ExitDenied
	e.Status, e.Reason, e.ExitCode = journal.StatusBlocked, reason, &code
	d.record(e)
	return ipc.Exit{Code: ExitDenied, ErrorCode: "UI_BLOCKED", Error: "Blocked by ADBuddy: " + reason}
}

// checkUIArgs checks a helper's args and returns what the description says
// it acts on.
func checkUIArgs(op string, raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	strict := func(v any) error {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(v); err != nil {
			return fmt.Errorf("bad %s args: %w", op, err)
		}
		return nil
	}
	switch op {
	case bridge.UIOpDump:
		return "", strict(new(bridge.UIDumpArgs))
	case bridge.UIOpForeground, bridge.UIOpNotifications:
		return "", strict(new(bridge.Empty))
	case bridge.UIOpClick:
		var a bridge.UIClickArgs
		if err := strict(&a); err != nil {
			return "", err
		}
		n := 0
		for _, set := range []bool{a.ID != "", a.Text != "", a.X != nil || a.Y != nil} {
			if set {
				n++
			}
		}
		switch {
		case n != 1:
			return "", errors.New("tap needs exactly one of a text, --id or --xy")
		case a.ID != "":
			return "id " + quoteTarget(a.ID), nil
		case a.Text != "":
			return quoteTarget(a.Text), nil
		case a.X == nil || a.Y == nil || *a.X < 0 || *a.Y < 0:
			return "", errors.New("--xy needs two numbers of 0 or more")
		}
		return fmt.Sprintf("(%d,%d)", *a.X, *a.Y), nil
	case bridge.UIOpSetText:
		var a bridge.UISetTextArgs
		if err := strict(&a); err != nil {
			return "", err
		}
		if n := utf8.RuneCountInString(a.Text); n == 0 || n > bridge.MaxSetText || !utf8.ValidString(a.Text) {
			return "", fmt.Errorf("type needs a text of 1 to %d characters", bridge.MaxSetText)
		}
		return quoteTarget(a.Text), nil
	case OpScreenshot:
		var a ScreenshotArgs
		if err := strict(&a); err != nil {
			return "", err
		}
		// 0 and "" mean the default, which ui resolves (withDefaults).
		if !(a.Scale == 0 || 0 < a.Scale && a.Scale <= 1) || a.Format != "" && a.Format != screenshot.PNG && a.Format != screenshot.JPEG {
			return "", errors.New("screenshot needs a scale above 0 and at most 1, and the format png or jpeg")
		}
		return "", nil
	}
	return "", errors.New("unknown helper")
}

// withDefaults fills in what a helper's args leave to the default (already
// checked): a screenshot is half size, PNG, or JPEG with the remote mode on.
func (d *Daemon) withDefaults(op string, raw json.RawMessage) json.RawMessage {
	if op != OpScreenshot {
		return raw
	}
	var a ScreenshotArgs
	_ = json.Unmarshal(raw, &a)
	if a.Scale == 0 {
		a.Scale = 0.5
	}
	if a.Format == "" {
		a.Format = screenshot.PNG
		if d.o.RemoteActive != nil && d.o.RemoteActive() {
			a.Format = screenshot.JPEG
		}
	}
	out, err := json.Marshal(a)
	if err != nil {
		return raw
	}
	return out
}

// helperArgv is the command the journal records for a helper, made here from
// its operation and args: what the client sends is not trusted, and a typed
// text is cut as in the description.
func helperArgv(op string, raw json.RawMessage) []string {
	switch op {
	case bridge.UIOpDump:
		var a bridge.UIDumpArgs
		_ = json.Unmarshal(raw, &a)
		if a.All {
			return []string{"adbuddy", "screen", "--all"}
		}
		return []string{"adbuddy", "screen"}
	case bridge.UIOpClick:
		var a bridge.UIClickArgs
		_ = json.Unmarshal(raw, &a)
		switch {
		case a.ID != "":
			return []string{"adbuddy", "tap", "--id", a.ID}
		case a.Text != "":
			return []string{"adbuddy", "tap", a.Text}
		case a.X != nil && a.Y != nil:
			return []string{"adbuddy", "tap", "--xy", strconv.Itoa(*a.X) + "," + strconv.Itoa(*a.Y)}
		}
	case bridge.UIOpSetText:
		var a bridge.UISetTextArgs
		_ = json.Unmarshal(raw, &a)
		t := a.Text
		if utf8.RuneCountInString(t) > 60 {
			t = string([]rune(t)[:60]) + "…"
		}
		return []string{"adbuddy", "type", t}
	case bridge.UIOpForeground:
		return []string{"adbuddy", "foreground"}
	case OpScreenshot:
		var a ScreenshotArgs
		_ = json.Unmarshal(raw, &a)
		return []string{"adbuddy", "screenshot", "--scale", strconv.FormatFloat(a.Scale, 'g', -1, 64), "--format", a.Format}
	}
	return []string{"adbuddy", op}
}

// quoteTarget quotes what a tap or type acts on for the journal, cut to 60
// characters and escaped.
func quoteTarget(s string) string {
	if utf8.RuneCountInString(s) > 60 {
		s = string([]rune(s)[:60]) + "…"
	}
	return `"` + classify.Printable(s) + `"`
}

// fallsBack reports whether adb should do what the app could not: the app is
// not there, its accessibility service is off, or (only for what reads) it
// did not answer in time. A tap that may have happened is never repeated.
func fallsBack(err error, reads bool) bool {
	var ue *UIError
	return errors.Is(err, ErrNoApp) || errors.As(err, &ue) && ue.Code == bridge.CodeUnavailable ||
		reads && errors.Is(err, ErrUITimeout)
}

// appFailure turns an error of Options.UI into the helper's failure.
func appFailure(ctx context.Context, err error) *uiFailure {
	var ue *UIError
	switch {
	case errors.As(err, &ue):
		text := classify.Printable(ue.Text)
		switch ue.Code {
		case bridge.CodeNotFound:
			return fail(1, "UI_NOT_FOUND", text)
		case bridge.CodeAmbiguous:
			return fail(1, "UI_AMBIGUOUS", text+" Use --id or --xy.")
		case bridge.CodeProtected:
			return fail(ExitDenied, "UI_PROTECTED", text)
		case bridge.CodeExpired:
			return fail(exitTransient, "UI_TIMEOUT", "the request expired before the app took it")
		case bridge.CodeUnavailable:
			return fail(exitTransient, "UI_UNAVAILABLE", text)
		}
		return fail(1, "UI_FAILED", classify.Printable(ue.Code)+": "+text)
	case errors.Is(err, ErrUITimeout):
		return fail(exitTransient, "UI_TIMEOUT", "the ADBuddy app did not answer in time: the action may or may not have happened; look at the screen before trying again")
	case ctx.Err() != nil:
		return fail(1, "UI_FAILED", "stopped: "+ctx.Err().Error())
	}
	return fail(1, "UI_FAILED", err.Error())
}

// callApp runs op on the app; ErrNoApp when there is none.
func (d *Daemon) callApp(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
	if d.o.UI == nil {
		return nil, ErrNoApp
	}
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	return d.o.UI(ctx, op, args)
}

// runUI does the helper, by the app or by adb, and returns the data for the
// client and how it was done.
func (d *Daemon) runUI(ctx context.Context, op string, raw json.RawMessage, s Stream) (any, string, *uiFailure) {
	switch op {
	case OpScreenshot:
		var a ScreenshotArgs
		_ = json.Unmarshal(raw, &a)
		return d.screenshot(ctx, a, s)
	case bridge.UIOpForeground:
		f, err := d.inFront(ctx)
		if err != nil {
			return nil, "", fail(exitTransient, "UI_UNAVAILABLE", err.Error())
		}
		fg := bridge.UIForeground{Package: f.first(), Activity: f.activity, Protected: f.appProtected}
		for _, p := range f.packages {
			fg.Protected = fg.Protected || d.protected(p)
		}
		return fg, "the app or adb", nil
	case bridge.UIOpNotifications:
		// No adb fallback: dumpsys notification --noredact would print every app's text.
		return d.notifications(ctx, raw)
	}
	res, err := d.callApp(ctx, op, raw)
	if err == nil {
		v, err := checkResult(op, res)
		if err != nil {
			return nil, "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable: "+err.Error())
		}
		return v, "the app", nil
	}
	if !fallsBack(err, op == bridge.UIOpDump) {
		return nil, "", appFailure(ctx, err)
	}
	switch op {
	case bridge.UIOpDump:
		var a bridge.UIDumpArgs
		_ = json.Unmarshal(raw, &a)
		out, f := d.uiautomator(ctx)
		if f != nil {
			return nil, "", f
		}
		dump, err := uidump.Parse(out, a.All, d.protected)
		if err != nil {
			return nil, "", fail(1, "UI_FAILED", err.Error())
		}
		return dump, "adb", nil
	case bridge.UIOpClick:
		var a bridge.UIClickArgs
		_ = json.Unmarshal(raw, &a)
		return d.tapByADB(ctx, a)
	case bridge.UIOpSetText:
		var a bridge.UISetTextArgs
		_ = json.Unmarshal(raw, &a)
		return d.typeByADB(ctx, a.Text)
	}
	return nil, "", fail(1, "UI_FAILED", "unknown helper")
}

// checkResult decodes the app's result into its type: what reaches the CLI
// is always well formed.
func checkResult(op string, raw json.RawMessage) (any, error) {
	if len(raw) > bridge.MaxUIResult {
		return nil, fmt.Errorf("a result of %d bytes", len(raw))
	}
	v := bridge.UIOps[op].Result()
	if err := json.Unmarshal(raw, v); err != nil {
		return nil, err
	}
	if d, ok := v.(*bridge.UIDump); ok && d.Elements == nil {
		d.Elements = []bridge.UIElement{}
	}
	return v, nil
}

// uiautomator dumps the screen by adb, unless ADBuddy's accessibility
// service is on: uiautomator would disconnect it (M5 spec §5).
func (d *Daemon) uiautomator(ctx context.Context) ([]byte, *uiFailure) {
	out, code, err := d.o.ADB.Shell(ctx, "dumpsys accessibility")
	if err != nil || code != 0 {
		return nil, fail(exitTransient, "UI_UNAVAILABLE", "could not check ADBuddy's accessibility service on the phone")
	}
	if uidump.ServiceOn(out) {
		return nil, fail(exitTransient, "UI_UNAVAILABLE", "ADBuddy's accessibility service is running but the app is not connected: open the ADBuddy app and try again")
	}
	var buf, errb bytes.Buffer
	code, err = d.o.ADB.Run(ctx, "", []string{"exec-out", "uiautomator", "dump", "/dev/tty"}, nil, &limited{w: &buf, n: maxCapture}, &errb)
	if err != nil || code != 0 {
		return nil, fail(1, "UI_FAILED", "uiautomator dump failed: "+classify.Printable(strings.TrimSpace(errb.String())))
	}
	return buf.Bytes(), nil
}

// tapByADB taps with input tap: at the point, or at the middle of the element
// uiautomator finds by id or text.
func (d *Daemon) tapByADB(ctx context.Context, a bridge.UIClickArgs) (any, string, *uiFailure) {
	var x, y int
	var b [4]int
	if a.X != nil {
		x, y = *a.X, *a.Y
		b = [4]int{x, y, x + 1, y + 1}
	} else {
		out, f := d.uiautomator(ctx)
		if f != nil {
			return nil, "", f
		}
		var err error
		b, err = uidump.Find(out, a.ID, a.Text, d.protected)
		var amb *uidump.AmbiguousError
		switch {
		case errors.Is(err, uidump.ErrNotFound):
			return nil, "", fail(1, "UI_NOT_FOUND", "no element on the screen matches")
		case errors.Is(err, uidump.ErrProtected):
			return nil, "", fail(ExitDenied, "UI_PROTECTED", "the element belongs to ADBuddy or a protected app")
		case errors.As(err, &amb):
			return nil, "", fail(1, "UI_AMBIGUOUS", amb.Error())
		case err != nil:
			return nil, "", fail(1, "UI_FAILED", err.Error())
		}
		x, y = uidump.Center(b)
	}
	if f := d.shellRun(ctx, []string{"shell", "input", "tap", strconv.Itoa(x), strconv.Itoa(y)}); f != nil {
		return nil, "", f
	}
	return bridge.UIClicked{Clicked: true, Bounds: b, How: "gesture"}, "adb", nil
}

// typeByADB types with input text: printable ASCII only (M5 spec §5).
func (d *Daemon) typeByADB(ctx context.Context, text string) (any, string, *uiFailure) {
	for _, r := range text {
		if r < 0x20 || r > 0x7e {
			return nil, "", fail(exitTransient, "UI_UNAVAILABLE", "without the ADBuddy app only plain ASCII can be typed: Unicode needs the ADBuddy app")
		}
	}
	if strings.Contains(text, "%s") {
		return nil, "", fail(exitTransient, "UI_UNAVAILABLE", "without the ADBuddy app %s cannot be typed (input text reads it as a space)")
	}
	// input text reads %s as a space; the shell on the phone reads the rest
	// as one quoted word.
	if f := d.shellRun(ctx, []string{"shell", "input text " + shq.Quote(strings.ReplaceAll(text, " ", "%s"))}); f != nil {
		return nil, "", f
	}
	return bridge.UITextSet{Set: true}, "adb", nil
}

// shellRun runs an adb command of the helpers' own and fails with its stderr.
func (d *Daemon) shellRun(ctx context.Context, args []string) *uiFailure {
	var errb bytes.Buffer
	code, err := d.o.ADB.Run(ctx, "", args, nil, io.Discard, &errb)
	if err != nil || code != 0 {
		msg := strings.TrimSpace(errb.String())
		if err != nil {
			msg = err.Error()
		}
		return fail(1, "UI_FAILED", "adb failed: "+classify.Printable(msg))
	}
	return nil
}

// screenshot takes the capture by adb, scales it and sends it on stdout.
func (d *Daemon) screenshot(ctx context.Context, a ScreenshotArgs, s Stream) (any, string, *uiFailure) {
	var buf, errb bytes.Buffer
	code, err := d.o.ADB.Run(ctx, "", []string{"exec-out", "screencap", "-p"}, nil, &limited{w: &buf, n: maxCapture}, &errb)
	if err != nil || code != 0 {
		return nil, "", fail(1, "UI_FAILED", "screencap failed: "+classify.Printable(strings.TrimSpace(errb.String())))
	}
	img, err := screenshot.Scale(buf.Bytes(), a.Scale, a.Format)
	if err != nil {
		return nil, "", fail(1, "UI_FAILED", err.Error())
	}
	if _, err := s.Stdout().Write(img); err != nil {
		return nil, "", fail(1, "UI_FAILED", err.Error())
	}
	return ScreenshotResult{Format: a.Format, Bytes: len(img)}, "adb", nil
}

// limited fails a write past n bytes.
type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(p []byte) (int, error) {
	if len(p) > l.n {
		return 0, errors.New("more output than expected")
	}
	l.n -= len(p)
	return l.w.Write(p)
}
