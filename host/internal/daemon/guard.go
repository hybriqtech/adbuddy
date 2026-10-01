package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// The guard (M5 spec §4.3): nothing touches or reads the screen while a
// protected app, or ADBuddy itself, is in front (in any half of a split
// screen), and nothing opens a protected app. A command that touches or
// reads the screen runs alone, one key at a time, so it cannot open an app
// and act on it after the guard looked. It fails closed: when what is in
// front cannot be known, what it guards does not run.

// foregroundWait is how long the guard waits for the app to say what is in
// front before asking adb (a variable so tests can shorten it).
var foregroundWait = 3 * time.Second

// screenReads are the commands that read the screen; the classifier marks
// the ones that act on it (Finding.UI: input, am start, monkey).
var screenReads = map[string]bool{"screencap": true, "screenrecord": true, "uiautomator": true}

// aloneReason is why a guarded command must run alone.
const aloneReason = "a command that touches or reads the screen must run alone: one simple command per adb call, without ;, &&, ||, |, &, loops or $(…), and one key per input keyevent"

// guard returns why the command res (args, as the agent gave them after the
// global options) must not run, or "".
func (d *Daemon) guard(ctx context.Context, res classify.Result, args []string) string {
	guarded := false
	for _, f := range res.Findings {
		acts, reads := screenAction(f)
		if !acts && !reads {
			continue
		}
		guarded = true
		if !acts {
			continue
		}
		if manyKeys(f.Words) {
			return aloneReason
		}
		// am start and monkey may name the app they open, as a component,
		// a package or inside an intent URI.
		for _, w := range f.Words[1:] {
			for _, tok := range strings.FieldsFunc(w, func(r rune) bool { return strings.ContainsRune("/;=:#,{} \t", r) }) {
				if d.protected(tok) {
					return protectedReason(tok)
				}
			}
		}
	}
	if !guarded {
		return ""
	}
	line := strings.Join(args, " ")
	if len(res.Findings) > 1 || strings.ContainsAny(line, ";&|`\n") || strings.Contains(line, "$(") {
		return aloneReason
	}
	return d.guardForeground(ctx)
}

// screenAction reports whether f acts on the screen (input, cmd input, am
// start, monkey) or reads it (screencap, screenrecord, uiautomator).
func screenAction(f classify.Finding) (acts, reads bool) {
	if len(f.Words) == 0 {
		return false, false
	}
	cmd := path.Base(f.Words[0])
	if f.UI || cmd == "cmd" && len(f.Words) > 1 && f.Words[1] == "input" {
		return true, false
	}
	return false, screenReads[cmd]
}

// manyKeys reports whether an input command presses more than one key:
// `input keyevent 187 22 66` could switch to an app and act in it at once.
func manyKeys(words []string) bool {
	for i, w := range words {
		if w != "keyevent" && w != "keycombination" {
			continue
		}
		keys := 0
		for _, k := range words[i+1:] {
			if !strings.HasPrefix(k, "-") {
				keys++
			}
		}
		return keys > 1
	}
	return false
}

// guardForeground returns why nothing may touch or read the screen now, or "".
func (d *Daemon) guardForeground(ctx context.Context) string {
	f, err := d.inFront(ctx)
	if err != nil {
		return "could not tell which app is in the foreground (" + err.Error() + "); ADBuddy does not touch or read the screen without knowing"
	}
	for _, pkg := range f.packages {
		if d.protected(pkg) {
			return protectedReason(pkg)
		}
	}
	if f.appProtected {
		return protectedReason(f.first())
	}
	return ""
}

// refuse ends a command the guard stopped: blocked, or abandoned when the
// agent or serve went away while the guard looked.
func (d *Daemon) refuse(ctx context.Context, e journal.Entry, s Stream, why string) ipc.Exit {
	if ctx.Err() != nil {
		code := ExitDenied
		e.Status, e.Reason, e.ExitCode = journal.StatusDenied, abandoned(ctx, "abandoned by the agent before it ran", "abandoned: ADBuddy stopped before it ran"), &code
		d.record(e)
		return ipc.Exit{Code: ExitDenied}
	}
	return d.block(e, s, why)
}

func protectedReason(pkg string) string {
	return classify.Printable(pkg) + " is a protected app; ask the user to do this part"
}

// protected reports whether pkg is ADBuddy or one of the user's protected apps.
func (d *Daemon) protected(pkg string) bool {
	return pkg == uidump.AppPackage || d.o.Protected != nil && d.o.Protected(pkg)
}

// front is what is in front: every package with a resumed activity (none:
// the screen is off), the top one first.
type front struct {
	packages     []string
	activity     string // the top one's, when known
	appProtected bool   // the app says something on screen is protected
}

func (f front) first() string {
	if len(f.packages) == 0 {
		return ""
	}
	return f.packages[0]
}

// inFront asks the app what is in front and, when it cannot say, adb. An
// empty package from the app does not count (its window had no content).
// From dumpsys, no resumed activity counts as nothing in front only when
// dumpsys power says the screen is off; otherwise it is unknown.
func (d *Daemon) inFront(ctx context.Context) (front, error) {
	if d.o.UI != nil {
		fctx, cancel := context.WithTimeout(ctx, foregroundWait)
		raw, err := d.o.UI(fctx, bridge.UIOpForeground, json.RawMessage("{}"))
		cancel()
		var fg bridge.UIForeground
		if err == nil && json.Unmarshal(raw, &fg) == nil && fg.Package != "" {
			return front{packages: []string{fg.Package}, activity: fg.Activity, appProtected: fg.Protected}, nil
		}
	}
	if ctx.Err() != nil {
		return front{}, ctx.Err()
	}
	out, code, err := d.o.ADB.Shell(ctx, "dumpsys activity activities")
	if err != nil || code != 0 || !strings.Contains(out, "ACTIVITY MANAGER ACTIVITIES") {
		return front{}, errForegroundUnknown
	}
	var f front
	for _, a := range uidump.Resumed(out) {
		if len(f.packages) == 0 {
			f.activity = a.Name
		}
		if !contains(f.packages, a.Package) {
			f.packages = append(f.packages, a.Package)
		}
	}
	if len(f.packages) > 0 {
		return f, nil
	}
	power, code, err := d.o.ADB.Shell(ctx, "dumpsys power")
	if err != nil || code != 0 || !uidump.Asleep(power) {
		return front{}, errNothingResumed
	}
	return f, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Why inFront could not tell.
var (
	errForegroundUnknown = errors.New("neither the ADBuddy app nor dumpsys answered")
	errNothingResumed    = errors.New("no app is resumed and the screen is not off: maybe the lock screen")
)
