package emutest

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// The overlay of M5 spec §6 (the frame, the pill, the mark) on the emulator,
// through the real channel with ADBuddy's accessibility service on: M5a-3,
// M5a-7 (the status bar with a 🔴 waiting), M5a-8 and M5b-19.

// a11yLink is openLink with ADBuddy's accessibility service on (restored at
// cleanup) and Settings in front, once the app answers a dump.
func a11yLink(t *testing.T) *link {
	t.Helper()
	l := openLink(t)
	e := l.e
	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, l.v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	e.openSettings(t)
	l.appDump(t)
	return l
}

var uiSeq int

// uiCall sends a ui.request and returns the app's answer.
func (l *link) uiCall(t *testing.T, op string, args any) bridge.UIResponse {
	t.Helper()
	uiSeq++
	return l.uiRequest(t, "g_"+strconv.Itoa(uiSeq), op, args)
}

// appDump returns the app's dump of the screen, waiting up to 20 s for its
// service to answer.
func (l *link) appDump(t *testing.T) bridge.UIDump {
	t.Helper()
	var r bridge.UIResponse
	if !poll(20*time.Second, func() bool {
		r = l.uiCall(t, bridge.UIOpDump, bridge.UIDumpArgs{})
		return r.OK
	}) {
		t.Fatalf("the app does not dump the screen: %+v", r.Error)
	}
	return payload[bridge.UIDump](t, bridge.Message{Type: "dump", Payload: r.Result})
}

// element returns the element of d whose text is text, or fails.
func element(t *testing.T, d bridge.UIDump, text string) bridge.UIElement {
	t.Helper()
	for _, el := range d.Elements {
		if el.Text == text {
			return el
		}
	}
	t.Fatalf("no %q on screen: %+v", text, d.Elements)
	return bridge.UIElement{}
}

// freePoint returns a point of the app's window that no clickable element of
// d covers (a list that scrolls may): a tap there does nothing.
func freePoint(t *testing.T, d bridge.UIDump) (int, int) {
	t.Helper()
	for y := 300; y < d.Height-300; y += 20 {
		for x := 40; x < d.Width-40; x += 40 {
			free := true
			for _, el := range d.Elements {
				b := el.Bounds
				if (el.Click || el.Edit) && x >= b[0] && x < b[2] && y >= b[1] && y < b[3] {
					free = false
					break
				}
			}
			if free {
				return x, y
			}
		}
	}
	t.Fatal("no free point on screen")
	return 0, 0
}

// overlayFrameRe finds the windows of ADBuddy without an activity (the
// overlay's) and their frames in dumpsys window windows.
var overlayFrameRe = regexp.MustCompile(`(?s)Window #\d+ Window\{\S+ u0 ` + regexp.QuoteMeta(phoneapp.Package) + `\}:.*?mViewVisibility=(0x[0-9a-f]+).*? frame=\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

// overlayWindows returns the frames of the overlay's visible windows.
func (e *emu) overlayWindows(t *testing.T) [][4]int {
	t.Helper()
	out := e.read(t, "", "windows")
	var frames [][4]int
	for _, block := range strings.Split(out, "  Window #")[1:] {
		m := overlayFrameRe.FindStringSubmatch("Window #" + block)
		if m == nil || m[1] != "0x0" {
			continue
		}
		var f [4]int
		for i := range f {
			f[i], _ = strconv.Atoi(m[i+2])
		}
		frames = append(frames, f)
	}
	return frames
}

// waitOverlay waits up to 5 s for the overlay (the frame and the pill) and
// returns the pill's frame: the smaller one.
func (e *emu) waitOverlay(t *testing.T) [4]int {
	t.Helper()
	var frames [][4]int
	if !poll(5*time.Second, func() bool { frames = e.overlayWindows(t); return len(frames) == 2 }) {
		var heads []string
		for _, line := range strings.Split(e.read(t, "", "windows"), "\n") {
			if strings.Contains(line, "Window #") || strings.Contains(line, phoneapp.Package) {
				heads = append(heads, strings.TrimSpace(line))
			}
		}
		t.Fatalf("the overlay is not on screen: ADBuddy's windows %v\n%s", frames, strings.Join(heads, "\n"))
	}
	area := func(f [4]int) int { return (f[2] - f[0]) * (f[3] - f[1]) }
	if area(frames[0]) < area(frames[1]) {
		return frames[0]
	}
	return frames[1]
}

func TestTheOverlayOnTheEmulator(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	home := l.appDump(t)
	x, y := freePoint(t, home)

	// M5a-8: two taps at a point within 4 s: the frame, all over the screen,
	// is not one of the windows the app looks at, so the second is not
	// "protected" (nor the first).
	for i := 1; i <= 2; i++ {
		if r := l.uiCall(t, bridge.UIOpClick, xyArgs(x, y)); !r.OK {
			t.Fatalf("tap %d at the free point (%d,%d): %+v", i, x, y, r.Error)
		}
	}
	pill := e.waitOverlay(t)
	t.Logf("overlay up; the pill at %v", pill)

	// M5a-3: a tap of the person (input tap) goes through the frame: it opens
	// Network & internet in Settings, below.
	net := element(t, home, "Network & internet")
	cx, cy := (net.Bounds[0]+net.Bounds[2])/2, (net.Bounds[1]+net.Bounds[3])/2
	e.shell(t, "input tap "+strconv.Itoa(cx)+" "+strconv.Itoa(cy))
	if len(e.overlayWindows(t)) != 2 {
		t.Error("the overlay was gone right after the tap: it did not go through it")
	}
	if !poll(10*time.Second, func() bool {
		d := l.appDump(t)
		for _, el := range d.Elements {
			if el.Text == "Internet" {
				return true
			}
		}
		return false
	}) {
		t.Errorf("Network & internet did not open under the overlay")
	}

	// M5b-19: the overlay is TYPE_ACCESSIBILITY_OVERLAY, which needs no
	// SYSTEM_ALERT_WINDOW: with that permission denied the agent still acts and
	// the overlay still shows. FAILED ("can't show the on-screen notice") is
	// only reached when addView fails, which the shell cannot cause.
	e.atCleanup(t, "shell", "appops set "+phoneapp.Package+" SYSTEM_ALERT_WINDOW default")
	e.shell(t, "appops set "+phoneapp.Package+" SYSTEM_ALERT_WINDOW deny")
	time.Sleep(5 * time.Second) // the overlay hides 4 s after the last action
	if r := l.uiCall(t, bridge.UIOpClick, xyArgs(x, y)); !r.OK {
		t.Errorf("a tap with SYSTEM_ALERT_WINDOW denied: %+v", r.Error)
	}
	pill = e.waitOverlay(t)
	e.shell(t, "appops set "+phoneapp.Package+" SYSTEM_ALERT_WINDOW default")

	// M5a-3: the pill pauses the agent: the app sends pause to the host.
	l.drain()
	e.shell(t, "input tap "+strconv.Itoa((pill[0]+pill[2])/2)+" "+strconv.Itoa((pill[1]+pill[3])/2))
	l.waitFor(t, bridge.TypePause, 10*time.Second)
	if !poll(5*time.Second, func() bool { return len(e.overlayWindows(t)) == 0 }) {
		t.Error("the overlay is still up after the pill was tapped")
	}
	// The app takes itself as paused when the host says so (session state):
	// this host, the test, does not, so the app is not checked to refuse now.
}

// M5a-7, the status bar with a 🔴 waiting: a tap at a point of the status bar
// while a question of ADBuddy waits is refused as protected.
func TestTheStatusBarWithAQuestionWaiting(t *testing.T) {
	l := a11yLink(t)
	e := l.e
	if r := l.uiCall(t, bridge.UIOpClick, xyArgs(540, 2)); !r.OK {
		t.Logf("a tap on the status bar with nothing waiting: %+v", r.Error)
	}
	l.send(t, bridge.TypeConfirmReq, bridge.ConfirmRequest{
		ID: "j_guard", Level: "red", Title: "Restart the phone", Detail: "may restart",
		RawCommand: "adb shell reboot", ExpiresAt: time.Now().Add(2 * time.Minute).UTC(),
	})
	e.atCleanup(t, "shell", "cmd statusbar collapse")
	if !poll(20*time.Second, func() bool {
		return notificationRecord(e.shell(t, "dumpsys notification --noredact"), phoneapp.Package, "confirm") != ""
	}) {
		t.Fatal("no question notified 20 s after the confirm.request")
	}
	// Past the heads-up's 8 s of NotificationWindow: what refuses is the question.
	time.Sleep(10 * time.Second)
	r := l.uiCall(t, bridge.UIOpClick, xyArgs(540, 2))
	if r.OK || r.Error == nil || r.Error.Code != bridge.CodeProtected {
		t.Fatalf("a tap on the status bar with a question waiting: %+v, want protected", r)
	}
	t.Logf("refused: %s", r.Error.Text)
}
