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
const aloneReason = "a command that touches or reads the screen must run alone: one simple command per adb call, without ;, &&, ||, |, &, loops or $(…), without wrappers (watch, flock, sh -c…), and one key per input keyevent"

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
	if !runsUnwrapped(res.Findings[0], args) {
		return aloneReason
	}
	return d.guardForeground(ctx, res.Findings[0].Words)
}

// runsUnwrapped reports whether the shell text of args starts with the
// guarded command f itself: an allowlist, because a wrapper (watch, flock,
// sh -c, env, nice, timeout, nohup, busybox, VAR=x…) would let it act after
// the guard looked, and again in whatever app is then in front.
func runsUnwrapped(f classify.Finding, args []string) bool {
	if len(f.Words) == 0 {
		return false
	}
	i := 0
	for i < len(args) && args[i] != "shell" && args[i] != "exec-out" {
		i++
	}
	if i == len(args) {
		return false
	}
	i++
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		i++
	}
	fields := strings.Fields(strings.Join(args[i:], " "))
	return len(fields) > 0 && path.Base(fields[0]) == path.Base(f.Words[0])
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
	if cmd == "dumpsys" {
		return false, dumpsysReadsScreen(f.Words[1:])
	}
	return false, screenReads[cmd]
}

// dumpsysValueOptions are dumpsys's own options that take a value; the
// others it has (--proto, --pid) take none.
var dumpsysValueOptions = map[string]bool{"-t": true, "--timeout": true, "--pri": true, "--priority": true, "--skip": true}

// dumpsysFlagOptions take no value.
var dumpsysFlagOptions = map[string]bool{"--proto": true, "--pid": true}

// dumpsysSafeActivity are the `dumpsys activity` arguments known not to print
// the view tree or the texts on screen. It is an allowlist: `top` and any
// argument a future Android adds read the screen.
var dumpsysSafeActivity = map[string]bool{
	"activities": true, "a": true, "services": true, "providers": true, "broadcasts": true, "processes": true,
}

// dumpsysReadsScreen reports whether `dumpsys args…` may print what is on
// screen: a dump of everything (no service, only options) includes
// `activity top`, and so does `activity` with any argument not in
// dumpsysSafeActivity. -l and --help only list or explain. An option it does
// not know fails closed. (args are the words after dumpsys.)
func dumpsysReadsScreen(args []string) bool {
	i := 0
	for ; i < len(args) && strings.HasPrefix(args[i], "-"); i++ {
		switch o := args[i]; {
		case o == "-l" || o == "--help":
			return false
		case dumpsysValueOptions[o]:
			i++
		case dumpsysFlagOptions[o]:
		default:
			return true
		}
	}
	if i >= len(args) {
		return true // no service: everything is dumped
	}
	if args[i] != "activity" {
		return false
	}
	return i+1 >= len(args) || !dumpsysSafeActivity[args[i+1]]
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

// closingKeys are the only keys that may be pressed while the shade is open:
// Home and Back close it.
var closingKeys = map[string]bool{"3": true, "4": true, "KEYCODE_HOME": true, "KEYCODE_BACK": true}

// closesShade reports whether words are exactly `input keyevent <key>` with
// one of closingKeys and no option: `--longpress 3` opens the assistant, and
// every other key could act on the shade. (The guard has already required
// that the command runs alone and with one key.)
func closesShade(words []string) bool {
	return len(words) == 3 && path.Base(words[0]) == "input" && words[1] == "keyevent" && closingKeys[words[2]]
}

// The fixed commands the guard asks the phone for the shade (no text of the
// agent goes into them).
const (
	focusCmd   = "dumpsys window | grep -E 'mCurrentFocus'"
	windowsCmd = "dumpsys window windows"
	powerCmd   = "dumpsys power"
)

// guardForeground returns why nothing may touch or read the screen now, or "".
// words are those of the one command that asks (nil for a helper): the exact
// key that closes the shade (closesShade) skips the shade rule, and the exact
// key that wakes the screen (wakesScreen) is let through while it is off.
func (d *Daemon) guardForeground(ctx context.Context, words []string) string {
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
	if why := d.systemUIReason(ctx, f.viaApp, closesShade(words)); why != "" && !(wakesScreen(words) && d.screenOff(ctx)) {
		return why
	}
	return ""
}

// systemUIReason returns why the system interface on top of the apps
// protects the screen, or "": the notification shade or the quick settings
// open (a 2FA code can be on them), and, when the app does not tell, a window
// of systemui that is not a fixed one (a heads-up notification, with no app
// name here). It fails closed: what it cannot read blocks. The caller lifts
// the block only for a wake key with the screen off (the lock screen is the shade then).
func (d *Daemon) systemUIReason(ctx context.Context, viaApp, closing bool) string {
	const unknown = "; ADBuddy does not touch or read the screen without knowing"
	out, code, err := d.o.ADB.Shell(ctx, focusCmd)
	focus, ok := uidump.ParseFocus(out)
	if err != nil || code != 0 || !ok {
		return "could not tell whether the notification shade is open" + unknown
	}
	if closing {
		return ""
	}
	for _, title := range focus {
		if uidump.IsShade(title) {
			return "the notification shade is open; close it with Back (input keyevent 4) or ask the user to do this part"
		}
	}
	if viaApp {
		return "" // the app's foreground says if a notification is on screen
	}
	out, code, err = d.o.ADB.Shell(ctx, windowsCmd)
	windows, ok := uidump.ParseSystemUIWindows(out)
	if err != nil || code != 0 || !ok {
		return "could not tell whether a system window is on screen" + unknown
	}
	if len(windows) > 0 {
		for i, w := range windows {
			windows[i] = classify.Printable(w)
		}
		return "a system window is on screen (" + strings.Join(windows, ", ") + "), maybe a notification; wait for it to go away or ask the user to do this part"
	}
	return ""
}

// screenOff reports whether dumpsys power says the screen is off.
func (d *Daemon) screenOff(ctx context.Context) bool {
	out, code, err := d.o.ADB.Shell(ctx, powerCmd)
	return err == nil && code == 0 && uidump.Asleep(out)
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
	viaApp       bool   // the app said it, not dumpsys
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
			return front{packages: []string{fg.Package}, activity: fg.Activity, appProtected: fg.Protected, viaApp: true}, nil
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
	power, code, err := d.o.ADB.Shell(ctx, powerCmd)
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

// wakeKeys are the keys that turn the screen on.
var wakeKeys = map[string]bool{"224": true, "26": true, "KEYCODE_WAKEUP": true, "KEYCODE_POWER": true}

// wakesScreen reports whether words are exactly `input keyevent <key>` with
// one of wakeKeys and no option. With the screen off (the lock screen lives
// in the shade then) only this may pass what would block: anything else is
// judged as with the screen on.
func wakesScreen(words []string) bool {
	return len(words) == 3 && path.Base(words[0]) == "input" && words[1] == "keyevent" && wakeKeys[words[2]]
}
