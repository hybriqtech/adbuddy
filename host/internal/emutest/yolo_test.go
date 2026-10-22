package emutest

import (
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

// The app's English texts the YOLO tests look for (values/strings.xml).
const (
	yoloTxtMenu          = "More options"
	yoloTxtMenuItem      = "YOLO mode…"
	yoloTxtTitle         = "Turn on YOLO mode?"
	yoloTxtQuarter       = "15 minutes"
	yoloTxtTurnOn        = "Turn on"
	yoloTxtAuthTitle     = "Turn on YOLO mode" // the title the app gives the system's credential screen
	yoloTxtNoLockTitle   = "Your phone has no screen lock"
	yoloTxtNoLockConfirm = "Turn on anyway"
	yoloTxtBand          = "YOLO mode · "
	yoloTxtTurnOff       = "Turn off"
	yoloTxtNotifTitle    = "YOLO mode is on"
	yoloChannel          = "yolo"
)

// yoloNotification returns the record of the YOLO notification in a dump of
// the notification service, or "".
func yoloNotification(dump string) string {
	return notificationRecord(dump, phoneapp.Package, yoloChannel)
}

// rect is the bounds of a node of a uiautomator dump.
type rect struct{ l, t, r, b int }

var (
	nodeTagRe = regexp.MustCompile(`<node [^>]*>`)
	attrRe    = regexp.MustCompile(`(\S+?)="([^"]*)"`)
)

// boundsWhere returns the bounds of the first node of the dump for which
// match(text, desc, class) holds (uiNode keeps only the middle of its bounds).
func boundsWhere(dump string, match func(text, desc, class string, r rect) bool) (rect, bool) {
	for _, tag := range nodeTagRe.FindAllString(dump, -1) {
		attrs := map[string]string{}
		for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
			attrs[m[1]] = m[2]
		}
		m := boundsRe.FindStringSubmatch(attrs["bounds"])
		if m == nil {
			continue
		}
		var v [4]int
		for i := range v {
			v[i], _ = strconv.Atoi(m[i+1])
		}
		r := rect{v[0], v[1], v[2], v[3]}
		if match(attrs["text"], attrs["content-desc"], attrs["class"], r) {
			return r, true
		}
	}
	return rect{}, false
}

// buttonOf returns the node that holds the label `text` (Compose draws
// the label as a TextView inside the clickable View, and an empty Button node
// beside it, which is always enabled), from a screen listing.
func buttonOf(nodes []uiNode, text string) (uiNode, bool) {
	return dialogButton(nodes, text) // the clickable View that holds the label: it carries the enabled state
}

// waitButton waits for the Button labelled text and, when enabled is set, for it to be enabled.
func (e *emu) waitButton(t *testing.T, what string, d time.Duration, text string, enabled bool) uiNode {
	t.Helper()
	var found uiNode
	var last []uiNode
	if !poll(d, func() bool {
		last = e.screen(t)
		b, ok := buttonOf(last, text)
		found = b
		return ok && (!enabled || b.Enabled)
	}) {
		t.Fatalf("no %s on screen in %d s: %+v", what, int(d/time.Second), last)
	}
	return found
}

var statusBarRe = regexp.MustCompile(`(?s)type=(?:ITYPE_STATUS_BAR|statusBars)[^\n]*?frame=\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

// statusBarBottom is the bottom of the real status bar, read from the window
// manager's insets (0 and false when the dump does not tell).
func (e *emu) statusBarBottom(t *testing.T) (int, bool) {
	t.Helper()
	m := statusBarRe.FindStringSubmatch(e.shell(t, "dumpsys window"))
	if m == nil {
		return 0, false
	}
	b, _ := strconv.Atoi(m[4])
	return b, true
}

// checkBandInset (validation pending since A2: Robolectric cannot see insets)
// checks the band on the real screen: its content starts below the status
// bar, and it does not cover the header's menu button.
func (e *emu) checkBandInset(t *testing.T) {
	t.Helper()
	dump := e.dumpScreen(t)
	shot := filepath.Join(os.TempDir(), "adbuddy-yolo-band-"+strconv.FormatInt(time.Now().UnixNano(), 10)+".png") // outside the repo, kept for a person to look at
	e.screencap(t, shot)
	band, ok := boundsWhere(dump, func(text, _, _ string, _ rect) bool { return strings.HasPrefix(text, yoloTxtBand) })
	if !ok {
		t.Fatalf("no band with bounds in the dump (screenshot %s)", shot)
	}
	label, ok := boundsWhere(dump, func(text, _, _ string, _ rect) bool { return text == yoloTxtTurnOff })
	if !ok {
		t.Fatalf("no Turn off label in the band (screenshot %s)", shot)
	}
	// The label sits inside an empty Button node: its bounds are the band's real height.
	off, ok := boundsWhere(dump, func(_, _, class string, r rect) bool {
		return class == "android.widget.Button" && r.l <= label.l && r.t <= label.t && r.r >= label.r && r.b >= label.b
	})
	if !ok {
		t.Fatalf("no Turn off button in the band (screenshot %s)", shot)
	}
	menu, ok := boundsWhere(dump, func(_, desc, _ string, _ rect) bool { return desc == yoloTxtMenu })
	if !ok {
		t.Fatalf("the header's menu button is gone with the band up (screenshot %s)", shot)
	}
	t.Logf("band text %+v, band button %+v, menu button %+v (screenshot %s)", band, off, menu, shot)
	if bar, ok := e.statusBarBottom(t); !ok {
		t.Errorf("cannot read the status bar's height from dumpsys window (screenshot %s)", shot)
	} else {
		t.Logf("status bar bottom: %d", bar)
		if band.t < bar || off.t < bar {
			t.Errorf("the band's content (text %+v, button %+v) overlaps the status bar (bottom %d): screenshot %s", band, off, bar, shot)
		}
	}
	if menu.t < off.b {
		t.Errorf("the band (button %+v) covers the menu button %+v: screenshot %s", off, menu, shot)
	}
}

// yoloTurnOnFromTheMenu goes through the chat's menu and the dialog up to the tap on Turn on with 15 minutes
// chosen. There is no default duration, and the button waits for its arming time.
func (l *link) yoloTurnOnFromTheMenu(t *testing.T) {
	t.Helper()
	e := l.e
	// The dialog does not open while the host has not said what YOLO is (the app asks no question over an unknown state).
	l.send(t, bridge.TypeYoloState, bridge.YoloState{})
	e.tap(t, e.waitNode(t, "the menu", 30*time.Second, func(n uiNode) bool { return n.Desc == yoloTxtMenu }))
	e.tap(t, e.waitNode(t, "the YOLO item", 10*time.Second, func(n uiNode) bool { return n.Text == yoloTxtMenuItem }))
	e.waitNode(t, "the YOLO dialog", 10*time.Second, func(n uiNode) bool { return n.Text == yoloTxtTitle })
	if n := e.waitButton(t, "the button", 10*time.Second, yoloTxtTurnOn, false); n.Enabled {
		t.Fatalf("the Turn on button is enabled with no duration chosen")
	}
	e.tap(t, e.waitNode(t, "15 minutes", 10*time.Second, func(n uiNode) bool { return n.Text == yoloTxtQuarter }))
	time.Sleep(armingWait) // the button allows nothing for ConfirmUi.ARMING_MS after it is shown
	e.tap(t, e.waitButton(t, "the enabled button", 10*time.Second, yoloTxtTurnOn, true))
}

// bandGone reports whether no band is on the screen.
func (e *emu) bandGone(t *testing.T) bool {
	t.Helper()
	for _, n := range e.screen(t) {
		if strings.HasPrefix(n.Text, yoloTxtBand) {
			return false
		}
	}
	return true
}

// Validation of the YOLO mode on the emulator (plan 2026-10-18-modo-yolo.md,
// T9): the host plays the bridge (openLink). The person turns YOLO on from the
// chat's menu (dialog, a duration, the arming wait, and the phone's own lock:
// a PIN here); the app sends yolo.set only after that; the host answers
// yolo.state and the app paints its band and posts its notification, which is
// ongoing, local to the phone and has Turn off; the person turns it off from
// the notification, without any question; the app sends yolo.set off and, when
// the host says it is off, band and notification go.
func TestYoloFromTheAppToTheBandAndOffFromTheNotification(t *testing.T) {
	l := openLink(t)
	e := l.e
	// The last step of the activation is the phone's lock screen credential: set a PIN (cleared at cleanup) and unlock.
	// The lock screen has no notification here, and the swipe that used to be unlock's did not bring up the bouncer on it
	// (swipeUpPoints says what does).
	e.lockWithPIN(t)
	e.unlock(t)
	l.openChatPastOnboarding(t)

	l.yoloTurnOnFromTheMenu(t)
	// The system asks for the PIN. Until it is typed nothing goes to the host.
	e.waitNode(t, "the credential screen", 30*time.Second, func(n uiNode) bool { return n.Text == yoloTxtAuthTitle })
	if m, ok := l.take(bridge.TypeYoloSet, 3*time.Second); ok {
		t.Fatalf("yolo.set %s reached the host before the PIN was typed", m.Payload)
	}
	e.unlockOnBouncer(t) // types the test PIN and Enter

	req := l.waitFor(t, bridge.TypeYoloSet, 20*time.Second)
	if set := payload[bridge.YoloSet](t, req); set != (bridge.YoloSet{Minutes: 15}) {
		t.Fatalf("the app asked for %+v, want 15 minutes", set)
	}
	until := time.Now().Add(15 * time.Minute)
	l.publishRID(t, req, bridge.TypeYoloState, bridge.YoloState{Active: true, Until: until.UnixMilli()})

	// The band is on the screen, with the countdown, below the status bar and clear of the header's menu.
	e.waitNode(t, "the YOLO band", 20*time.Second, func(n uiNode) bool { return strings.HasPrefix(n.Text, yoloTxtBand) })
	e.checkBandInset(t)

	e.checkYoloNotification(t)

	// Off from the notification: one tap, no unlock, no question.
	e.tapInShade(t, yoloTxtNotifTitle, yoloTxtTurnOff)
	off := l.waitFor(t, bridge.TypeYoloSet, 20*time.Second)
	if set := payload[bridge.YoloSet](t, off); set != (bridge.YoloSet{Off: true}) {
		t.Fatalf("the notification asked for %+v, want off", set)
	}
	l.publishRID(t, off, bridge.TypeYoloState, bridge.YoloState{})

	// The band and the notification go when the host says it is off.
	if !poll(20*time.Second, func() bool { return e.bandGone(t) }) {
		t.Error("the band is still on the screen 20 s after the host said it is off")
	}
	if !poll(20*time.Second, func() bool { return yoloNotification(e.shell(t, "dumpsys notification --noredact")) == "" }) {
		t.Error("the YOLO notification is still up 20 s after the host said it is off")
	}
}

// Without a screen lock (R20, the person's decision) YOLO is turned on after a specific warning, armed: nothing goes
// to the host while the warning is up, and its button sends the request. The emulator has no lock credential by
// default (noLock checks it). Mutation: in ChatRoot, send yolo.set from onConfirm without YoloAuth.
func TestYoloWithNoScreenLockAsksFirst(t *testing.T) {
	l := openLink(t)
	e := l.e
	if ok, out := e.noLock(e.ctx); !ok {
		t.Skipf("the emulator has a lock credential (%s): this test needs none", out)
	}
	l.openChatPastOnboarding(t)
	l.yoloTurnOnFromTheMenu(t)
	e.waitNode(t, "the no-lock warning", 20*time.Second, func(n uiNode) bool { return n.Text == yoloTxtNoLockTitle })
	if m, ok := l.take(bridge.TypeYoloSet, 3*time.Second); ok {
		t.Fatalf("yolo.set %s reached the host while the warning was up", m.Payload)
	}
	time.Sleep(armingWait)
	e.tap(t, e.waitButton(t, "the warning's button", 10*time.Second, yoloTxtNoLockConfirm, true))
	req := l.waitFor(t, bridge.TypeYoloSet, 20*time.Second)
	if set := payload[bridge.YoloSet](t, req); set != (bridge.YoloSet{Minutes: 15}) {
		t.Fatalf("the app asked for %+v, want 15 minutes", set)
	}
}

// The mode outlives the link: with the host gone the band stays, as the mode
// stays on the host, and it comes back to the truth when the host does.
func TestYoloBandOutlivesTheLink(t *testing.T) {
	l := openLink(t)
	e := l.e
	l.openChatPastOnboarding(t)
	l.send(t, bridge.TypeYoloState, bridge.YoloState{Active: true, Session: true})
	e.waitNode(t, "the YOLO band", 30*time.Second, func(n uiNode) bool { return strings.HasPrefix(n.Text, yoloTxtBand) })
	e.checkBandInset(t)
	e.checkYoloNotification(t)
	n := len(l.Hellos())
	l.cut()
	time.Sleep(3 * time.Second)
	e.waitNode(t, "the YOLO band with the link down", 10*time.Second, func(n uiNode) bool { return strings.HasPrefix(n.Text, yoloTxtBand) })
	l.resume(t)
	l.waitHellos(t, n+1, 40*time.Second) // the app is back: what the host says now reaches it
	// The test host restarts its event numbers at 1 with the same session id (a real host that restarts gets a new
	// session), and the app drops events numbered at or below the last it saw: send enough for one to get through.
	for range 6 {
		l.send(t, bridge.TypeYoloState, bridge.YoloState{})
	}
	if !poll(30*time.Second, func() bool { return e.bandGone(t) }) {
		t.Error("the band is still on the screen after the host said it is off")
	}
}

// openChatPastOnboarding is link.openChat for the onboarding that ux/visual
// brought: the first step focuses its search field, so the keyboard covers
// "Not now" and a tap on it lands on a key. The keyboard is closed first
// (and the button looked up again, the layout moves).
func (l *link) openChatPastOnboarding(t *testing.T) {
	t.Helper()
	e := l.e
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	e.shell(t, "am start -W -n "+phoneapp.Activity)
	for step := 1; step <= 2; step++ {
		e.waitNode(t, "Not now (step "+strconv.Itoa(step)+")", 60*time.Second, func(n uiNode) bool { return n.Text == "Not now" })
		if strings.Contains(e.shell(t, "dumpsys input_method"), "mInputShown=true") {
			e.shell(t, "input keyevent KEYCODE_BACK")
			time.Sleep(time.Second)
		}
		e.tap(t, e.waitNode(t, "Not now (step "+strconv.Itoa(step)+")", 15*time.Second, func(n uiNode) bool { return n.Text == "Not now" }))
		want := "Step 2 of 2"
		if step == 2 {
			want = "Message the agent"
		}
		e.waitNode(t, want, 60*time.Second, func(n uiNode) bool { return n.Text == want })
	}
}

// checkYoloNotification waits for the YOLO notification and checks it: ongoing,
// local to the phone, with Turn off.
func (e *emu) checkYoloNotification(t *testing.T) {
	t.Helper()
	var rec string
	if !poll(20*time.Second, func() bool {
		rec = yoloNotification(e.shell(t, "dumpsys notification --noredact"))
		return rec != ""
	}) {
		t.Fatal("no YOLO notification 20 s after the host said it is on")
	}
	if !slices.Contains(recordActions(rec), yoloTxtTurnOff) {
		t.Errorf("the notification has no %q: %v", yoloTxtTurnOff, recordActions(rec))
	}
	if flags := recordFlags(rec); flags < 0 || flags&flagLocalOnly == 0 || flags&0x2 == 0 {
		t.Errorf("flags %#x: want local-only and ongoing", flags)
	}
}
