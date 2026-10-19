package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"slices"
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
// screen), and nothing opens a protected app. With ADBuddy on top, the two
// ways out that press and read nothing of it go through (V44): Home alone
// and, when the app says what is on screen, opening another app by its
// package with monkey's launcher form. A command that touches or
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
		// a package or inside an intent URI, and %-encoded: Android decodes
		// %XX in a URI's host and in an intent URI's values. Every decoding
		// is looked at, and anything that cannot be in a package name splits
		// it (/, ;, =, ?, @, %…).
		for _, w := range f.Words[1:] {
			forms, ok := decodings(w)
			if !ok {
				return encodedReason
			}
			for _, form := range forms {
				for _, tok := range strings.FieldsFunc(form, notInPackageName) {
					if tok == uidump.AppPackage {
						return protectedReason(tok)
					}
					if d.userProtected(tok) {
						if d.lift(ctx, protectedReason(tok)) {
							continue
						}
						return protectedReason(tok)
					}
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

// encodedReason is why a word %-encoded more than maxDecodings times blocks:
// the app it opens cannot be told.
const encodedReason = "the app this command opens is hidden under too many layers of %-encoding; ADBuddy does not open an app without knowing which"

// maxDecodings is how many times decodings undoes the %-encoding of a word.
const maxDecodings = 4

// decodings returns w and each of its successive %-decodings (percentDecode),
// up to maxDecodings of them. ok is false when w still changes after that
// many: what it would end up naming is not known.
func decodings(w string) (forms []string, ok bool) {
	forms = []string{w}
	cur := w
	for range maxDecodings {
		next := percentDecode(cur)
		if next == cur {
			return forms, true
		}
		forms = append(forms, next)
		cur = next
	}
	return forms, percentDecode(cur) == cur
}

// percentDecode turns each %XX (two hex digits, in either case) into its
// byte and leaves any other % as it is. Android's Uri.decode turns a
// malformed escape into U+FFFD (it consumes the next character and adds a
// control byte), which no package name has either. Ours differs there, but it
// does not matter: the % splits the token (notInPackageName), and Android
// does not split a name at U+FFFD.
func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b.WriteByte(unhex(s[i+1])<<4 | unhex(s[i+2]))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// unhex is the value of the hex digit c (isHex(c)).
func unhex(c byte) byte {
	switch {
	case c <= '9':
		return c - '0'
	case c <= 'F':
		return c - 'A' + 10
	}
	return c - 'a' + 10
}

// notInPackageName reports whether r cannot be in an Android package name
// (ASCII letters, digits, _ and .).
func notInPackageName(r rune) bool {
	return !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '_' || r == '.')
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
// others it has (--proto, --pid) take none. --skip is not one: in dumpsys.cpp
// it is a switch and every word after it is a service to skip (the rest is
// dumped), so dumpsysReadsScreen guards it wherever it is.
var dumpsysValueOptions = map[string]bool{"-t": true, "-T": true, "--pri": true, "--priority": true}

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
	if slices.Contains(args, "--skip") {
		return true
	}
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

// homeKeys are the keys that go to the launcher.
var homeKeys = map[string]bool{"3": true, "KEYCODE_HOME": true}

// goesHome reports whether words are exactly `input keyevent <Home>` with no
// option: `--longpress 3` opens the assistant, which reads the screen, and
// any other spelling (`HOME`, `03`, `cmd input`, `input -d 0`) is judged as
// any other key. (The guard has already required that the command runs
// alone, unwrapped and with one key.)
func goesHome(words []string) bool {
	return len(words) == 3 && path.Base(words[0]) == "input" && words[1] == "keyevent" && homeKeys[words[2]]
}

// opensAnApp reports whether words are exactly
// `monkey -p <package> -c android.intent.category.LAUNCHER 1`: it opens the
// launcher entry of the app it names, written as a package name
// (bridge.ValidPackage: nothing to decode), and sends no other event. Any
// other form stays out: `am start` names any activity (a dialog would leave
// ADBuddy on screen to be read) and resolves intents with no package, monkey
// without -p may open ADBuddy, and another category or more events reach
// other screens or touch.
func opensAnApp(words []string) bool {
	return len(words) == 6 && path.Base(words[0]) == "monkey" && words[1] == "-p" && bridge.ValidPackage(words[2]) &&
		words[3] == "-c" && words[4] == "android.intent.category.LAUNCHER" && words[5] == "1"
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
	// V44: with ADBuddy itself on top (the user chatting with the agent on
	// the phone), the ways out go through: Home alone (goesHome) and, when
	// the app says what is in front, opening another app by its package
	// (opensAnApp; guard has already judged that package, as every word of
	// the command). Neither presses nor reads anything of ADBuddy: Back would
	// act on it, a long press opens the assistant, which reads the screen,
	// and an intent with no package may end in ADBuddy itself. Opening needs
	// the app because what is opened may leave ADBuddy on screen (a dialog, a
	// translucent activity): the app lists every window a tap can reach, so
	// the next command still sees ADBuddy if it can be touched, while dumpsys
	// only sees resumed activities. Any other protected app dumpsys sees (the
	// other half of a split screen) still blocks. The app marks ADBuddy's own
	// screen as protected, so that flag does not stop a way out either;
	// whatever else it may stand for (a notification, a window the service
	// cannot read, the other half of a split screen), leaving does not read
	// or touch it.
	// ADBuddy only in the other half, under another app, has no way out:
	// its own reason, which offers one, would only make the agent loop.
	leaving := f.first() == uidump.AppPackage && (goesHome(words) || f.viaApp && opensAnApp(words))
	for _, pkg := range f.packages {
		switch {
		case pkg == uidump.AppPackage && leaving:
			// Empty on purpose: a way out skips ADBuddy (and only ADBuddy).
		case pkg == uidump.AppPackage && pkg == f.first():
			return adbuddyInFrontReason(f.viaApp)
		case pkg == uidump.AppPackage:
			return protectedReason(pkg) // ADBuddy in the other half of a split screen: YOLO does not lift it
		case d.userProtected(pkg) && !d.lift(ctx, protectedReason(pkg)):
			return protectedReason(pkg)
		}
	}
	// What the APP marks as protected is never lifted by the host, YOLO or not: with the app connected, ADBuddy in
	// the other half of a split screen reaches the host only as this flag (inFront gives one package), and a window
	// the phone cannot read could be anything. It is the app that, under YOLO and with the link up, stops marking
	// the user's protected apps and the heads-up notifications (task A5); ADBuddy it keeps marking.
	if f.appProtected && !leaving {
		return screenProtectedReason(f.first())
	}
	// YOLO lifts a shade or a system window that is KNOWN to be there; not knowing what is in front (R1) still blocks.
	if why, unknown := d.systemUIReason(ctx, f.viaApp, closesShade(words)); why != "" && !(wakesScreen(words) && d.screenOff(ctx)) && (unknown || !d.lift(ctx, why)) {
		return why
	}
	return ""
}

// systemUIReason returns why the system interface on top of the apps
// protects the screen, or "": the notification shade or the quick settings
// open (a 2FA code can be on them), and, when the app does not tell, a window
// of systemui that is not a fixed one (a heads-up notification, with no app
// name here). It fails closed: what it cannot read blocks, and says so with unknown (YOLO never lifts that). The caller lifts
// the block only for a wake key with the screen off (the lock screen is the shade then).
func (d *Daemon) systemUIReason(ctx context.Context, viaApp, closing bool) (why string, unknown bool) {
	const unreadable = "; ADBuddy does not touch or read the screen without knowing"
	out, code, err := d.o.ADB.Shell(ctx, focusCmd)
	focus, ok := uidump.ParseFocus(out)
	if err != nil || code != 0 || !ok {
		return "could not tell whether the notification shade is open" + unreadable, true
	}
	if closing {
		return "", false
	}
	for _, title := range focus {
		if uidump.IsShade(title) {
			return "the notification shade is open; close it with Back (input keyevent 4) or ask the user to do this part", false
		}
	}
	if viaApp {
		return "", false // the app's foreground says if a notification is on screen
	}
	out, code, err = d.o.ADB.Shell(ctx, windowsCmd)
	windows, ok := uidump.ParseSystemUIWindows(out)
	if err != nil || code != 0 || !ok {
		return "could not tell whether a system window is on screen" + unreadable, true
	}
	if len(windows) > 0 {
		for i, w := range windows {
			windows[i] = classify.Printable(w)
		}
		return "a system window is on screen (" + strings.Join(windows, ", ") + "), maybe a notification; wait for it to go away or ask the user to do this part", false
	}
	return "", false
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

// protectedReason is why pkg, which is protected, blocks.
func protectedReason(pkg string) string {
	return classify.Printable(pkg) + " is a protected app; ask the user to do this part"
}

// adbuddyInFrontReason is why ADBuddy itself in front blocks: the user is
// most likely chatting with the agent there (V44). It starts as
// protectedReason does, which the docs and the agent's context quote, and
// gives the ways out; opening an app by its package only when the app says
// what is on screen (viaApp), as guardForeground lets it through only then.
func adbuddyInFrontReason(viaApp bool) string {
	out := "press Home alone (input keyevent 3) and then open the app you need"
	if viaApp {
		out = "press Home alone (input keyevent 3), or open the app you need with exactly monkey -p <package> -c android.intent.category.LAUNCHER 1"
	}
	return uidump.AppPackage + " is a protected app: ADBuddy itself is in front (the user is probably chatting with you there), and nothing touches its screen. To leave it, " + out + "; or ask the user to do this part"
}

// screenProtectedReason is why the app says the screen is protected: not
// always a protected app, but also a notification that just arrived (the app
// counts any for about 8 s, as it may be a heads-up with a code) or a window
// whose content the service cannot see. The host cannot tell which, and the
// likeliest, a notification, passes within seconds (V45): the agent is told
// to wait and try once more before asking the user. It starts as before,
// which the agent's context and the skills quote. pkg is only what is in
// front (may be "").
func screenProtectedReason(pkg string) string {
	in := ""
	if pkg != "" {
		in = "; " + classify.Printable(pkg) + " is in front"
	}
	return "something on the screen is protected (a protected app, a notification or a window ADBuddy cannot read" + in + "). A notification that just arrived counts for about 8 seconds: wait 10 seconds and try once more; if it is still blocked, ask the user to do this part"
}

// protected reports whether pkg is ADBuddy or one of the user's protected
// apps. Under YOLO the user's list does not apply (ADBuddy still does): it
// decides what the agent may read of a screen and what the notifications show.
func (d *Daemon) protected(pkg string) bool {
	return pkg == uidump.AppPackage || !d.o.Yolo.Active() && d.userProtected(pkg)
}

// userProtected reports whether pkg is in the user's own list, whatever YOLO says.
func (d *Daemon) userProtected(pkg string) bool {
	return d.o.Protected != nil && d.o.Protected(pkg)
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
// empty package from the app does not count (its window had no content)
// unless it also says the screen is protected: that blocks.
// From dumpsys, no resumed activity counts as nothing in front only when
// dumpsys power says the screen is off; otherwise it is unknown.
func (d *Daemon) inFront(ctx context.Context) (front, error) {
	if d.o.UI != nil {
		fctx, cancel := context.WithTimeout(ctx, foregroundWait)
		raw, err := d.o.UI(fctx, bridge.UIOpForeground, json.RawMessage("{}"))
		cancel()
		var fg bridge.UIForeground
		if err == nil && json.Unmarshal(raw, &fg) == nil && (fg.Package != "" || fg.Protected) {
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
