package emutest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// Tapjacking (O1) on the emulator: a window of another app, drawn over
// ADBuddy, that lets touches through but covers the button they land on. The
// input dispatcher delivers them marked as obscured, and every surface that
// grants (ConsentScreen, the notification access screen, the 🔴 dialog, the
// chat's menu) must refuse them (ObscuredTouchGuard). The overlay is
// TapjackOverlayActivity of the instrumented tests' APK
// (android/app/src/androidTest/…/TapjackOverlayActivity.java): another package,
// so another uid (a window of the app's own uid never obscures it), faint
// enough (alpha 0.3) for Android 12+ to let the touch through instead of
// dropping it. Each refusal has its positive control: the same tap on the same
// button, once the overlay is gone, does grant.

const (
	// tapjackPkg is the instrumented tests' package, whose APK carries the overlay.
	tapjackPkg      = phoneapp.Package + ".test"
	tapjackActivity = tapjackPkg + "/" + phoneapp.Package + ".TapjackOverlayActivity"
	// tapjackTag is the overlay's logcat tag and tapjackTitle its window's title.
	tapjackTag   = "ADBuddyTapjack"
	tapjackTitle = "ADBuddyTapjackOverlay"
	// refusedWait is how long a refused tap is given to show an effect anyway.
	refusedWait = 4 * time.Second
)

// The app's English texts the tapjacking test taps or looks for (values/strings.xml).
const (
	txtAccept       = "Accept"                              // consent_accept
	txtLetRead      = "Let the agent read my notifications" // access_confirm
	txtNotNow       = "Not now"                             // onboarding_skip
	txtDone         = "Done"                                // onboarding_done
	txtStep2        = "Step 2 of 2"
	txtMessage      = "Message the agent"
	txtMoreOptions  = "More options"     // action_menu
	txtMenuSettings = "Settings"         // menu_settings
	txtMenuNew      = "New conversation" // menu_new_conversation
)

// overlayLog reads the overlay's logcat lines (tag tapjackTag) for the start
// with nonce: whether it says the window is shown, or why it refused.
func overlayLog(logcat, nonce string) (shown bool, refused string) {
	for _, line := range strings.Split(logcat, "\n") {
		if !strings.Contains(line, "nonce="+nonce) {
			continue
		}
		if strings.Contains(line, "overlay shown nonce="+nonce) {
			shown = true
		} else if _, why, ok := strings.Cut(line, "refused nonce="+nonce); ok && refused == "" {
			refused = strings.TrimSpace(strings.TrimPrefix(why, ":"))
		}
	}
	return shown, refused
}

// overlayWin is what `dumpsys window windows` says of one window.
type overlayWin struct {
	pkg      string
	ownerUID int
	typ      string // ty= of its attributes: APPLICATION_OVERLAY, or a number
	visible  bool   // it has a surface and is on screen and visible
}

var (
	winHeaderRe = regexp.MustCompile(`^\s*Window #\d+ Window\{\S+ u\d+ (.*)\}:\s*$`)
	winOwnerRe  = regexp.MustCompile(`\bmOwnerUid=(\d+)\b.*\bpackage=(\S+)`)
	winTypeRe   = regexp.MustCompile(`\bty=(\S+)`)
)

// overlayWindow finds the window titled title in `dumpsys window windows`.
func overlayWindow(dump, title string) (overlayWin, bool) {
	var w overlayWin
	found, in := false, false
	var surface, onScreen, isVisible bool
	for _, line := range strings.Split(dump, "\n") {
		if m := winHeaderRe.FindStringSubmatch(line); m != nil {
			if in {
				break // the next window
			}
			in = m[1] == title
			found = found || in
			continue
		}
		if !in {
			continue
		}
		if m := winOwnerRe.FindStringSubmatch(line); m != nil && w.pkg == "" {
			w.ownerUID, _ = strconv.Atoi(m[1])
			w.pkg = m[2]
		}
		if m := winTypeRe.FindStringSubmatch(line); m != nil && w.typ == "" {
			w.typ = m[1]
		}
		surface = surface || strings.Contains(line, "mHasSurface=true")
		onScreen = onScreen || strings.Contains(line, "isOnScreen=true")
		isVisible = isVisible || strings.TrimSpace(line) == "isVisible=true"
	}
	w.visible = surface && onScreen && isVisible
	return w, found
}

// packageUID reads the uid of pkg from `cmd package list packages -U pkg`,
// which lists every package whose name contains pkg.
func packageUID(out, pkg string) (int, bool) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "package:"+pkg {
			if v, ok := strings.CutPrefix(f[1], "uid:"); ok {
				uid, err := strconv.Atoi(v)
				return uid, err == nil && uid > 0
			}
		}
	}
	return 0, false
}

// untrustedDrop reports whether the input dispatcher's log says it dropped a
// touch because pkg's window covered it (Android 12+'s untrusted-touch
// block): the touch then never reached the app, and its refusal would prove
// nothing of the app's guard.
func untrustedDrop(logcat, pkg string) bool {
	for _, line := range strings.Split(logcat, "\n") {
		if strings.Contains(strings.ToLower(line), "untrusted touch") && strings.Contains(line, pkg+"/") {
			return true
		}
	}
	return false
}

var prefTrueRe = regexp.MustCompile(`<boolean\s+name="confirmed"\s+value="true"\s*/>`)

// consentMarked reads NotificationConsent's file (shared_prefs/notification_consent.xml):
// whether the mark "confirmed" is set. A missing file is no mark.
func consentMarked(xml string) bool { return prefTrueRe.MatchString(xml) }

func TestTapjackParsing(t *testing.T) {
	const nonce = "0123456789abcdef"
	logcat := "10-03 10:00:00.000  4321  4321 E ADBuddyTapjack: refused nonce=fedcba9876543210: not an emulator (Build.HARDWARE=qcom)\n" +
		"10-03 10:00:01.000  4321  4321 I ADBuddyTapjack: overlay shown nonce=" + nonce + "\n"
	for _, c := range []struct {
		name, logcat, refused string
		shown                 bool
	}{
		{"shown", logcat, "", true},
		{"another start's refusal is not this one's", strings.SplitAfter(logcat, "\n")[0], "", false},
		{"refused", "E ADBuddyTapjack: refused nonce=" + nonce + ": SYSTEM_ALERT_WINDOW is not allowed (appops set x)\n",
			"SYSTEM_ALERT_WINDOW is not allowed (appops set x)", false},
		{"already shown", "I ADBuddyTapjack: overlay shown nonce=" + nonce + " (already)\n", "", true},
		{"nothing", "", "", false},
	} {
		shown, refused := overlayLog(c.logcat, nonce)
		if shown != c.shown || refused != c.refused {
			t.Errorf("%s: overlayLog = %v, %q; want %v, %q", c.name, shown, refused, c.shown, c.refused)
		}
	}

	// From a real API 34 dump (uidump/testdata/api34/adbuddy.windows.txt),
	// the overlay's window put among systemui's.
	win := func(title, owner, pkg, ty, surface, onScreen, visible string) string {
		return "  Window #7 Window{b97266e u0 " + title + "}:\n" +
			"    mDisplayId=0 rootTaskId=1 mSession=Session{ec0838c 765:u0a10168} mClient=android.os.BinderProxy@8abaf2b\n" +
			"    mOwnerUid=" + owner + " showForAllUsers=false package=" + pkg + " appop=SYSTEM_ALERT_WINDOW\n" +
			"    mAttrs={(0,0)(fillxfill) sim={adjust=pan} layoutInDisplayCutoutMode=always ty=" + ty + " fmt=TRANSLUCENT alpha=0.3\n" +
			"      fl=NOT_FOCUSABLE NOT_TOUCHABLE LAYOUT_IN_SCREEN HARDWARE_ACCELERATED\n" +
			"      bhv=DEFAULT}\n" +
			"    mViewVisibility=0x0 mHaveFrame=true mObscured=false\n" +
			"    mHasSurface=" + surface + " isReadyForDisplay()=" + surface + " mWindowRemovalAllowed=false\n" +
			"      mShownAlpha=0.3 mAlpha=0.3 mLastAlpha=0.3\n" +
			"    mForceSeamlesslyRotate=false seamlesslyRotate: pending=null    isOnScreen=" + onScreen + "\n" +
			"    isVisible=" + visible + "\n" +
			"    mPrepareSyncSeqId=0\n"
	}
	shell := win("ShellDropTarget", "10168", "com.android.systemui", "APPLICATION_OVERLAY", "false", "false", "false")
	next := "  Window #8 Window{f3f1784 u0 InputMethod}:\n    mOwnerUid=10146 showForAllUsers=false package=com.google.android.inputmethod.latin appop=NONE\n" +
		"    mHasSurface=true isReadyForDisplay()=true\n    isOnScreen=true\n    isVisible=true\n"
	up := win(tapjackTitle, "10191", tapjackPkg, "APPLICATION_OVERLAY", "true", "true", "true")
	for _, c := range []struct {
		name, dump string
		want       overlayWin
		found      bool
	}{
		{"shown", "WINDOW MANAGER WINDOWS (dumpsys window windows)\n" + shell + up + next,
			overlayWin{tapjackPkg, 10191, "APPLICATION_OVERLAY", true}, true},
		{"last window", shell + up, overlayWin{tapjackPkg, 10191, "APPLICATION_OVERLAY", true}, true},
		// The next window's lines are not the overlay's.
		{"no surface yet", win(tapjackTitle, "10191", tapjackPkg, "APPLICATION_OVERLAY", "false", "false", "false") + next,
			overlayWin{tapjackPkg, 10191, "APPLICATION_OVERLAY", false}, true},
		{"not visible", win(tapjackTitle, "10191", tapjackPkg, "APPLICATION_OVERLAY", "true", "true", "false") + next,
			overlayWin{tapjackPkg, 10191, "APPLICATION_OVERLAY", false}, true},
		{"gone", shell + next, overlayWin{}, false},
		{"empty", "", overlayWin{}, false},
	} {
		got, found := overlayWindow(c.dump, tapjackTitle)
		if got != c.want || found != c.found {
			t.Errorf("%s: overlayWindow = %+v, %v; want %+v, %v", c.name, got, found, c.want, c.found)
		}
	}

	list := "package:tech.hybriq.adbuddy.test uid:10191\npackage:tech.hybriq.adbuddy uid:10190\n"
	if uid, ok := packageUID(list, phoneapp.Package); !ok || uid != 10190 {
		t.Errorf("packageUID(app) = %d, %v; want 10190 (not the tests' package, whose name starts the same)", uid, ok)
	}
	if uid, ok := packageUID(list, tapjackPkg); !ok || uid != 10191 {
		t.Errorf("packageUID(tests) = %d, %v", uid, ok)
	}
	if _, ok := packageUID("package:tech.hybriq.adbuddy.test uid:10191\n", phoneapp.Package); ok {
		t.Error("packageUID found the app in a list without it")
	}

	drop := "10-03 10:00:02.000  512  600 W InputDispatcher: Dropping untrusted touch event due to " + tapjackPkg + "/10191\n"
	if !untrustedDrop(drop, tapjackPkg) {
		t.Error("untrustedDrop missed the dispatcher's drop")
	}
	if untrustedDrop(strings.ReplaceAll(drop, tapjackPkg, "com.other"), tapjackPkg) || untrustedDrop("", tapjackPkg) {
		t.Error("untrustedDrop saw a drop that is not the overlay's")
	}
	// The app's own package is a prefix of the tests': a drop due to the app is not the overlay's.
	if untrustedDrop(strings.ReplaceAll(drop, tapjackPkg+"/", phoneapp.Package+"/"), tapjackPkg) {
		t.Error("untrustedDrop took the app's package for the overlay's")
	}

	for xml, want := range map[string]bool{
		"<?xml version='1.0' encoding='utf-8' standalone='yes' ?>\n<map>\n    <boolean name=\"confirmed\" value=\"true\" />\n</map>\n": true,
		"<map>\n    <boolean name=\"confirmed\" value=\"false\" />\n</map>\n":                                                          false,
		"<map>\n    <boolean name=\"other\" value=\"true\" />\n</map>\n":                                                               false,
		"": false,
	} {
		if got := consentMarked(xml); got != want {
			t.Errorf("consentMarked(%q) = %v, want %v", xml, got, want)
		}
	}
}

// quiet runs command on the emulator with a context of its own (it also works
// at cleanup, after e.ctx is canceled) and returns its output, or an error.
func (e *emu) quiet(command string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(e.ctx), time.Minute)
	defer cancel()
	out, _, code, err := e.adb.Output(ctx, "shell", command)
	return strings.TrimSpace(strings.ReplaceAll(out, "\r", "")), err == nil && code == 0
}

// uidOf returns the uid of an installed package, or fails.
func (e *emu) uidOf(t *testing.T, pkg string) int {
	t.Helper()
	out := e.shell(t, "cmd package list packages -U "+pkg)
	uid, ok := packageUID(out, pkg)
	if !ok {
		t.Fatalf("no uid of %s in %q", pkg, out)
	}
	return uid
}

// overlay is TapjackOverlayActivity's window on the emulator.
type overlay struct {
	e     *emu
	since string // the emulator's clock when it went up, for logcat -T
	gone  bool
}

// obscure puts the overlay over whatever is on screen and checks that it is
// there: its own log line, and its window in dumpsys, visible, of the tests'
// package and of a uid that is not the app's (appUID). The app, if it was in
// front, is in front again when it returns. The overlay goes at cleanup, if
// remove did not take it away before.
func (e *emu) obscure(t *testing.T, appUID int) *overlay {
	t.Helper()
	// No leftover of an earlier start: a fresh process, whose window is this start's.
	e.shell(t, "am force-stop "+tapjackPkg)
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatal(err)
	}
	nonce := hex.EncodeToString(raw[:])
	since := e.shell(t, "date +%s") + ".000"
	front := e.front(t)
	o := &overlay{e: e, since: since}
	t.Cleanup(func() { o.remove(t) })
	e.shell(t, "am start -n "+tapjackActivity+" --es nonce "+nonce)
	var logcat string
	if !poll(15*time.Second, func() bool {
		logcat = e.shell(t, "logcat -d -T "+since+" -s "+tapjackTag)
		shown, refused := overlayLog(logcat, nonce)
		if refused != "" {
			t.Fatalf("the overlay refused to show: %s", refused)
		}
		return shown
	}) {
		t.Fatalf("the overlay did not say it is shown 15 s after am start:\n%s", logcat)
	}
	var w overlayWin
	if !poll(10*time.Second, func() bool {
		var found bool
		w, found = overlayWindow(e.shell(t, "dumpsys window windows"), tapjackTitle)
		return found && w.visible
	}) {
		t.Fatalf("the overlay's window is not visible in dumpsys window windows 10 s after it said it is shown: %+v", w)
	}
	if w.pkg != tapjackPkg || w.typ != "APPLICATION_OVERLAY" {
		t.Fatalf("the overlay's window is %+v, want a window of %s of type APPLICATION_OVERLAY", w, tapjackPkg)
	}
	// The whole point: the input dispatcher only marks touches as obscured by another uid's window.
	if w.ownerUID == appUID {
		t.Fatalf("the overlay's window has the app's own uid %d: it would not obscure the app", appUID)
	}
	if len(front) > 0 && front[0] == phoneapp.Package {
		e.waitFront(t, "ADBuddy in front again under the overlay", first(phoneapp.Package))
	}
	return o
}

// shot saves a screenshot named name when ADBUDDY_EMUTEST_SHOTS names a folder.
func (e *emu) shot(t *testing.T, name string) {
	t.Helper()
	if dir := os.Getenv("ADBUDDY_EMUTEST_SHOTS"); dir != "" {
		e.screencap(t, filepath.Join(dir, name))
	}
}

// notDropped fails if the system dropped a touch because of the overlay:
// then nothing reached the app, and a refusal says nothing of its guard.
func (o *overlay) notDropped(t *testing.T) {
	t.Helper()
	if log := o.e.shell(t, "logcat -d -T "+o.since+" -s InputDispatcher"); untrustedDrop(log, tapjackPkg) {
		t.Fatalf("the system dropped the touch as untrusted (the overlay is too opaque?): the app never got it\n%s", log)
	}
}

// remove takes the overlay away (the tests' package force-stopped: its process
// dies with its window) and waits until its window is gone. Idempotent.
func (o *overlay) remove(t *testing.T) {
	t.Helper()
	if o.gone {
		return
	}
	o.gone = true
	if o.e.state() != "device" {
		return
	}
	if _, ok := o.e.quiet("am force-stop " + tapjackPkg); !ok {
		t.Errorf("am force-stop %s failed: the overlay may still be up", tapjackPkg)
	}
	if !poll(10*time.Second, func() bool {
		dump, ok := o.e.quiet("dumpsys window windows")
		_, found := overlayWindow(dump, tapjackTitle)
		return ok && !found
	}) {
		t.Errorf("the overlay's window is still there 10 s after am force-stop %s", tapjackPkg)
	}
}

// consentNow is the app's setup consent, as the host reads it.
func (e *emu) consentNow(t *testing.T) string {
	t.Helper()
	res, err := phoneapp.Call(e.ctx, e.adb, "getSetupConsent")
	if err != nil {
		t.Fatalf("getSetupConsent: %v", err)
	}
	return res["consent"]
}

// stillThere waits refusedWait and checks that the screen still shows the
// node match finds: the refused tap neither closed nor changed it.
func (e *emu) stillThere(t *testing.T, what string, match func(uiNode) bool) uiNode {
	t.Helper()
	time.Sleep(refusedWait)
	nodes := e.screen(t)
	i := slices.IndexFunc(nodes, match)
	if i < 0 {
		t.Fatalf("%s is gone after a tap the overlay covered: %+v", what, nodes)
	}
	return nodes[i]
}

func textIs(s string) func(uiNode) bool { return func(n uiNode) bool { return n.Text == s } }

// O1 on the emulator, automated (fix-tapjacking-consent): with another app's
// window over ADBuddy that lets touches through, a tap on a button that grants
// is refused, and the same tap with the window gone grants. (a) The setup
// consent's Accept leaves the consent pending and the screen open. (c) The
// notification access screen's "Let the agent read my notifications" leaves
// no mark. (b) In the chat, which keeps working under a harmless overlay (its
// menu opens: the control that touches do reach the app), the menu's items
// and the 🔴 dialog's Allow are refused: no confirm.reply.
func TestAnObscuringOverlayRefusesTheTapsThatGrant(t *testing.T) {
	l := openLink(t)
	e := l.e
	e.installTests(t)
	// LIFO: before the tests' package is uninstalled, its process is gone.
	e.atCleanup(t, "shell", "am force-stop "+tapjackPkg)
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	e.shell(t, "appops set "+tapjackPkg+" SYSTEM_ALERT_WINDOW allow")
	appUID := e.uidOf(t, phoneapp.Package)
	// The system's "allowed" for the notification listener: the screen then asks the person to confirm.
	e.atCleanup(t, "shell", "cmd notification disallow_listener "+listenerComponent)
	e.shell(t, "cmd notification allow_listener "+listenerComponent)

	t.Run("consent", func(t *testing.T) {
		// Pending, as `adbuddy setup` leaves it (AskConsent) before it opens the screen.
		if _, err := phoneapp.Call(e.ctx, e.adb, "requestSetup", "host:s:emutest"); err != nil {
			t.Fatalf("requestSetup: %v", err)
		}
		// The overlay first, as an attacker's would be: then the screen opens under it.
		o := e.obscure(t, appUID)
		e.shell(t, "am start -n "+phoneapp.Activity+" -a "+phoneapp.SetupAction)
		accept := e.waitNode(t, txtAccept, 20*time.Second, textIs(txtAccept))
		e.shot(t, "tapjack-consent.png")
		e.tap(t, accept)
		accept = e.stillThere(t, "the consent screen", textIs(txtAccept))
		o.notDropped(t)
		if c := e.consentNow(t); c != string(phoneapp.ConsentPending) {
			t.Fatalf("the consent is %q after a tap on Accept under the overlay, want pending", c)
		}

		o.remove(t)
		e.tap(t, accept)
		var c string
		if !poll(10*time.Second, func() bool { c = e.consentNow(t); return c == string(phoneapp.ConsentGranted) }) {
			t.Fatalf("the same Accept without the overlay left the consent %q, want granted", c)
		}
	})

	t.Run("notification access", func(t *testing.T) {
		// A clean start: the app shows its first-run steps (not done yet), the
		// second of which is the notification access screen. Its service comes
		// back by itself (MainActivity restarts it with the saved link).
		e.shell(t, "am force-stop "+phoneapp.Package)
		e.shell(t, "am start -W -n "+phoneapp.Activity)
		e.tap(t, e.waitNode(t, txtNotNow+" (step 1)", 60*time.Second, textIs(txtNotNow)))
		e.waitNode(t, txtStep2, 30*time.Second, textIs(txtStep2))
		e.waitNode(t, txtLetRead, 20*time.Second, textIs(txtLetRead))
		if e.markOnDisk(t) {
			t.Fatal("the mark is set before any tap")
		}

		o := e.obscure(t, appUID)
		// Found again under the overlay: the overlay's own start paused the app,
		// and a tap on a button that is not drawn yet would be refused by nothing.
		confirm := e.waitNode(t, txtLetRead, 15*time.Second, textIs(txtLetRead))
		e.shot(t, "tapjack-notification-access.png")
		e.tap(t, confirm)
		confirm = e.stillThere(t, "the confirm button", textIs(txtLetRead))
		o.notDropped(t)
		if e.markOnDisk(t) {
			t.Fatal("a tap under the overlay set the mark")
		}

		o.remove(t)
		e.tap(t, confirm)
		if !poll(10*time.Second, func() bool { return e.markOnDisk(t) }) {
			t.Fatal("the same tap without the overlay did not set the mark")
		}
		// Confirmed: the step offers Done only, which ends the first-run steps.
		e.tap(t, e.waitNode(t, txtDone, 15*time.Second, textIs(txtDone)))
	})

	t.Run("chat", func(t *testing.T) {
		e.shell(t, "am start -W -n "+phoneapp.Activity)
		// Past whatever first-run step an earlier failure left on screen.
		var nodes []uiNode
		if !poll(60*time.Second, func() bool {
			nodes = e.screen(t)
			if hasText(nodes, txtMessage) {
				return true
			}
			if i := slices.IndexFunc(nodes, func(n uiNode) bool { return n.Text == txtNotNow || n.Text == txtDone }); i >= 0 {
				e.tap(t, nodes[i])
			}
			return false
		}) {
			t.Fatalf("the chat is not on screen in 60 s: %+v", nodes)
		}
		if !poll(30*time.Second, l.connected) {
			t.Fatal("the app is not connected to the host 30 s after it opened")
		}

		o := e.obscure(t, appUID)
		// The control: the chat itself keeps working under the overlay (its
		// window has no guard), so touches do reach the app.
		e.tap(t, e.waitNode(t, txtMoreOptions, 15*time.Second, func(n uiNode) bool { return n.Desc == txtMoreOptions }))
		item := e.waitNode(t, "the chat's menu opened under the overlay (if not, the system blocks every touch and the refusals below prove nothing)",
			10*time.Second, textIs(txtMenuSettings))
		o.notDropped(t)
		// Its items act (Pause, Cut access/Restore access…): refused.
		e.shot(t, "tapjack-menu.png")
		e.tap(t, item)
		e.stillThere(t, "the chat's menu", textIs(txtMenuNew))
		e.shell(t, "input keyevent KEYCODE_BACK")
		e.waitNodes(t, "the chat without its menu", 10*time.Second, func(nodes []uiNode) bool {
			return hasText(nodes, txtMessage) && !hasText(nodes, txtMenuNew)
		})

		q := question("j_tapjack", "Restart the phone", "adb shell reboot")
		l.ask(t, q)
		var allow uiNode
		if !poll(15*time.Second, func() bool {
			var ok bool
			nodes = e.screen(t)
			allow, ok = dialogButton(nodes, txtAllow)
			return ok
		}) {
			t.Fatalf("no Allow in the question's dialog in 15 s: %+v", nodes)
		}
		if !allow.Enabled {
			t.Fatalf("Allow is off for a short command: the refusal below would prove nothing: %+v", nodes)
		}
		e.shot(t, "tapjack-dialog.png")
		l.drain()
		e.tap(t, allow)
		if m, ok := l.take(bridge.TypeConfirmReply, refusedWait); ok {
			t.Fatalf("Allow under the overlay answered the question: %s", m.Payload)
		}
		o.notDropped(t)
		allow, ok := dialogButton(e.screen(t), txtAllow)
		if !ok {
			t.Fatal("the question's dialog is gone after a tap the overlay covered")
		}

		o.remove(t)
		e.tap(t, allow)
		if r := l.confirmReply(t, 10*time.Second); r.ID != q.ID || r.Decision != "allow" {
			t.Errorf("the same Allow without the overlay sent %+v", r)
		}
		l.settle(t, q.ID)
	})
}
