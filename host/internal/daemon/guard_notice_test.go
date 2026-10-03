package daemon

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// V45: in a game, a tap and a screenshot were refused between others that
// ran. The app says the screen is protected for about 8 s after any
// notification, and while an app window shows no content: the guard still
// refuses then, and says that it may pass.

const mayPass = "wait 10 seconds and try once more; if it is still blocked, ask the user to do this part"

func TestScreenProtectedReasonSaysItMayPass(t *testing.T) {
	for _, pkg := range []string{"", "com.supercell.clashroyale"} {
		got := screenProtectedReason(pkg)
		// It starts as before: the agent's context and the skills quote it.
		for _, w := range []string{"something on the screen is protected (a protected app, a notification or a window ADBuddy cannot read", "A notification that just arrived counts for about 8 seconds", mayPass} {
			if !strings.Contains(got, w) {
				t.Errorf("%q: missing %q in %q", pkg, w, got)
			}
		}
	}
}

func TestScreenProtectedByTheAppBlocksUntilItIsNot(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	game := bridge.UIForeground{Package: "com.supercell.clashroyale", Activity: "com.supercell.titan.GameApp", Protected: true}
	a.results[bridge.UIOpForeground] = game
	a.results[bridge.UIOpClick] = bridge.UIClicked{Clicked: true, Bounds: [4]int{672, 1440, 673, 1441}, How: "gesture"}
	e.adb.out["exec-out screencap -p"] = pngBytes(t)
	// While the app says so: the helpers and raw commands stop, with why
	// and what to do.
	for _, op := range []string{bridge.UIOpClick, OpScreenshot, bridge.UIOpDump} {
		if _, exit := e.helper(t, op, helperArgs(op)); exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" ||
			!strings.Contains(exit.Error, "com.supercell.clashroyale is in front") || !strings.Contains(exit.Error, mayPass) {
			t.Errorf("%s: %+v", op, exit)
		}
	}
	for _, argv := range [][]string{{"shell", "input", "tap", "672", "1440"}, {"exec-out", "screencap", "-p"}, {"shell", "input", "keyevent", "3"}, openGame} {
		if s, exit := e.exec(t, argv...); exit.Code != 126 || !strings.Contains(s.errOut.String(), mayPass) {
			t.Errorf("%q: %+v %q", argv, exit, s.errOut.String())
		}
	}
	if r := ranOnPhone(e); len(r) != 0 {
		t.Fatalf("adb ran %q", r)
	}
	for _, c := range a.called() {
		if strings.HasPrefix(c, bridge.UIOpClick) || strings.HasPrefix(c, bridge.UIOpDump) {
			t.Fatalf("the app was asked to %q", c)
		}
	}
	// A moment later it no longer says so: the same commands run.
	game.Protected = false
	a.results[bridge.UIOpForeground] = game
	if _, exit := e.helper(t, bridge.UIOpClick, xy(672, 1440)); exit.Code != 0 {
		t.Fatalf("tap: %+v", exit)
	}
	if _, exit := e.helper(t, OpScreenshot, helperArgs(OpScreenshot)); exit.Code != 0 {
		t.Fatalf("screenshot: %+v", exit)
	}
	if s, exit := e.exec(t, "shell", "input", "tap", "672", "1440"); exit.Code != 0 {
		t.Fatalf("input tap: %+v %q", exit, s.errOut.String())
	}
}
