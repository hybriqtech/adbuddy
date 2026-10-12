package emutest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// What `tap --screen` and `screen --settle` show on the real screen, through the
// host's own daemon with the real app (docs/superpowers/plans/2026-10-04-latencia-settle2.md,
// S2): the screen after a tap is the new one, a single click in the journal,
// Back never ends blocked, a protected app opening ends with the note and no
// second tap, and a tap that does nothing says so after settleIdle. It checks;
// it is not a measurement (that is TestLatencySettle). Skipped with no emulator.

// settleIdleFloor is what a tap with no effect must wait at least: the daemon's
// settleIdle (2 s; not reachable from here) minus a margin.
const settleIdleFloor = 1500 * time.Millisecond

// helperScreen is helper with Screen: true (tap --screen, screen --settle): the
// exit and the data the daemon wrote.
func (h *latencyHost) helperScreen(op string, args any) (ipc.Exit, []byte) {
	raw, _ := json.Marshal(args)
	s := &guardStream{}
	exit := h.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUI, UIOp: op, UIArgs: raw, Argv: []string{"adbuddy", op}, Cwd: h.ws,
		Env: map[string]string{"ADBUDDY_SOURCE": "agent"}, Screen: true}, s)
	return exit, s.data
}

// clicks counts the taps in the journal (an entry is written twice, running and
// then its end, with the same id).
func (h *latencyHost) clicks(t *testing.T) int {
	t.Helper()
	entries, _, err := h.j.Load()
	if err != nil {
		t.Fatalf("journal: %v", err)
	}
	ids := map[string]bool{}
	for _, e := range entries {
		if e.Rule == "helper.tap" {
			ids[e.ID] = true
		}
	}
	return len(ids)
}

// restoreAnimationScaleAtCleanup leaves the animation scale key as the test found
// it. A scale that was not set cannot be restored by deleting it alone: the window
// manager keeps its current value when the setting disappears (it reads it with the
// current value as the default), so the emulator would stay at the slowed-down scale
// and every later test would run its animations that slow (TestTerminalWorksOffline
// never saw "Step 2 of 2" in CI). The system default, 1.0, is put first and the key
// is deleted after it.
func (e *emu) restoreAnimationScaleAtCleanup(t *testing.T, key string) {
	t.Helper()
	prev := e.shell(t, "settings get global "+key)
	if prev != "null" && prev != "" {
		e.restoreAtCleanup(t, "global", key, prev)
		return
	}
	e.restoreAtCleanup(t, "global", key, "null") // runs last: deletes the key
	e.restoreAtCleanup(t, "global", key, "1.0")  // runs first: back to the default
}

// describeScreen is what a log line says of the screen a wait ended with:
// whether it settled, whether it changed, and how many elements are outside the
// system windows (none: only the system's bars, a transition between two screens).
func describeScreen(s *daemon.ScreenSettled) string {
	if s == nil {
		return ", no screen"
	}
	changed := "unknown"
	if s.Changed != nil {
		changed = strconv.FormatBool(*s.Changed)
	}
	inApp := 0
	for _, e := range s.Elements {
		if e.Window != "system" {
			inApp++
		}
	}
	return fmt.Sprintf(", settled %v, changed %s, %d elements (%d outside the system windows)", s.Settled, changed, len(s.Elements), inApp)
}

func screenHas(d bridge.UIDump, text string) bool {
	for _, e := range d.Elements {
		if strings.Contains(e.Text, text) || strings.Contains(e.Desc, text) {
			return true
		}
	}
	return false
}

func TestScreenAfterOnTheEmulator(t *testing.T) {
	l := openLink(t)
	e := l.e
	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, l.v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	var a11y string
	if !poll(20*time.Second, func() bool { a11y = e.read(t, "", "accessibility"); return a11yBound(a11y) }) {
		t.Fatalf("ADBuddy's service is not bound 20 s after enabling it:\n%s", a11y)
	}
	ui := l.appUI(t) // one for all hosts: its request ids must not repeat
	tapNet := bridge.UIClickArgs{Text: "Network & internet"}

	tapScreen := func(t *testing.T, h *latencyHost, args bridge.UIClickArgs) (ipc.Exit, daemon.ClickedScreen, time.Duration) {
		t.Helper()
		t0 := time.Now()
		exit, data := h.helperScreen(bridge.UIOpClick, args)
		took := time.Since(t0)
		var c daemon.ClickedScreen
		if exit.Code == 0 {
			if err := json.Unmarshal(data, &c); err != nil {
				t.Fatalf("tap --screen data %q: %v", data, err)
			}
		}
		t.Logf("tap --screen %+v: exit %d, %d ms%s", args, exit.Code, took.Milliseconds(), describeScreen(c.Screen))
		return exit, c, took
	}
	dumpSettle := func(t *testing.T, h *latencyHost) (ipc.Exit, daemon.ScreenSettled, time.Duration) {
		t.Helper()
		t0 := time.Now()
		exit, data := h.helperScreen(bridge.UIOpDump, bridge.UIDumpArgs{})
		took := time.Since(t0)
		var s daemon.ScreenSettled
		if exit.Code == 0 {
			if err := json.Unmarshal(data, &s); err != nil {
				t.Fatalf("screen --settle data %q: %v", data, err)
			}
		}
		t.Logf("screen --settle: exit %d, %d ms", exit.Code, took.Milliseconds())
		return exit, s, took
	}

	t.Run("a tap shows the new screen with one click in the journal", func(t *testing.T) {
		e.openSettings(t)
		time.Sleep(2 * time.Second)
		h := newLatencyHost(t, e, ui)
		exit, c, _ := tapScreen(t, h, tapNet)
		if exit.Code != 0 || c.Screen == nil {
			t.Fatalf("exit %+v, screen %v, note %q", exit, c.Screen, c.Note)
		}
		if c.Screen.Changed == nil || !*c.Screen.Changed {
			t.Errorf("changed = %v, want true", c.Screen.Changed)
		}
		// Settled is not required: on GitHub's runners Settings fills its summaries late and the
		// screen is often still changing at the 3 s cap (settled=false in 2 to 11 taps of 20, before bareApp too).
		if !screenHas(c.Screen.UIDump, "Internet") {
			t.Errorf("the screen after the tap is not the sub page (no \"Internet\"): %+v", c.Screen.UIDump.Elements)
		}
		if n := h.clicks(t); n != 1 {
			t.Errorf("%d clicks in the journal, want 1", n)
		}
	})

	// With the animations slowed down the transition lasts seconds: the screen
	// the tap leaves is on show long after the tap, so a wait that took the old
	// screen as settled, or ended before the new one, would fail here. At normal
	// speed the emulator changes screen before the first read and this cannot fail.
	//
	// What it checks is that the wait never says "settled" on a screen that is
	// not the new one, and that the tap happens once. At 8x the transition
	// (about 2.4 s here, longer on GitHub's runners) lasts as long as the wait's
	// cap (settleMax, 3 s) or longer, so the wait can end in three documented
	// ways: settled=false with whichever frame of the transition the cap fell on
	// (the old screen, or one with no app window yet); no screen, because the
	// guard blocked the window of the transition for longer than settleBlockWait,
	// with the note that the tap was done; or, once the new screen is drawn and
	// quiet, settled=true. A read with no app window never settles (bareApp).
	// Only a settled=true read is held to being the new screen, and a missing
	// screen to carrying the note that says the tap happened.
	t.Run("a slow transition never settles on the old screen nor taps twice", func(t *testing.T) {
		for _, k := range []string{"window_animation_scale", "transition_animation_scale", "animator_duration_scale"} {
			e.restoreAnimationScaleAtCleanup(t, k)
			e.shell(t, "settings put global "+k+" 8")
		}
		e.openSettings(t)
		time.Sleep(3 * time.Second)
		h := newLatencyHost(t, e, ui)
		for i := range 3 {
			exit, c, _ := tapScreen(t, h, tapNet)
			if exit.Code != 0 {
				t.Fatalf("run %d: exit %+v, note %q", i, exit, c.Note)
			}
			if c.Screen == nil {
				if !strings.HasPrefix(c.Note, "The action was done; do not repeat it") || !strings.Contains(c.Note, "Blocked by ADBuddy") {
					t.Errorf("run %d: no screen and no note that the tap was done: %q", i, c.Note)
				}
			} else if c.Screen.Settled && (c.Screen.Changed == nil || !*c.Screen.Changed || !screenHas(c.Screen.UIDump, "Internet")) {
				t.Errorf("run %d: settled on a screen that is not the new one (changed %v): %+v", i, c.Screen.Changed, c.Screen.UIDump.Elements)
			}
			e.shell(t, "input keyevent 4")
			time.Sleep(4 * time.Second)
		}
		if n := h.clicks(t); n != 3 {
			t.Errorf("%d clicks in the journal, want 3 (one per tap)", n)
		}
	})

	t.Run("Back 20 times is never blocked and shows the home", func(t *testing.T) {
		e.openSettings(t)
		time.Sleep(2 * time.Second)
		h := newLatencyHost(t, e, ui)
		for i := range 20 {
			if exit, c, _ := tapScreen(t, h, tapNet); exit.Code != 0 || c.Screen == nil {
				t.Fatalf("run %d: opening the sub page: exit %+v, note %q", i, exit, c.Note)
			}
			e.shell(t, "input keyevent 4")
			exit, s, _ := dumpSettle(t, h)
			if exit.Code != 0 {
				t.Fatalf("run %d: screen --settle after Back: exit %d %s %s", i, exit.Code, exit.ErrorCode, exit.Error)
			}
			if !screenHas(s.UIDump, "Network & internet") {
				t.Errorf("run %d: after Back the screen is not Settings' home (settled %v, changed %v): %+v", i, s.Settled, s.Changed, s.Elements)
			}
		}
	})

	t.Run("a protected app opening ends with the note and one tap", func(t *testing.T) {
		e.shell(t, "input keyevent KEYCODE_HOME")
		time.Sleep(2 * time.Second)
		h := newLatencyHostProtected(t, e, ui, func(pkg string) bool { return pkg == settingsPkg })
		exit, c, _ := tapScreen(t, h, bridge.UIClickArgs{Text: "Settings"})
		if exit.Code == 0 {
			if c.Screen != nil || !strings.Contains(c.Note, "The action was done; do not repeat it") {
				t.Errorf("screen %v, note %q, want no screen and the note", c.Screen, c.Note)
			}
			if n := h.clicks(t); n != 1 {
				t.Errorf("%d clicks in the journal, want 1", n)
			}
			return
		}
		// The launcher offers no "Settings" to tap: open it as an app would.
		t.Logf("no tap on the launcher's Settings (exit %d %s %s): monkey instead", exit.Code, exit.ErrorCode, exit.Error)
		// (The failed tap left its own entry in the journal, as an error: it does not count as a click.)
		e.shell(t, "input keyevent KEYCODE_HOME")
		time.Sleep(2 * time.Second)
		e.shell(t, "monkey -p "+settingsPkg+" -c android.intent.category.LAUNCHER 1")
		exit, _, _ = dumpSettle(t, h)
		if exit.Code != daemon.ExitDenied || !strings.Contains(exit.Error, "Blocked by ADBuddy") {
			t.Errorf("screen --settle on the protected app: exit %d %s %q, want blocked", exit.Code, exit.ErrorCode, exit.Error)
		}
	})

	t.Run("a tap with no effect says it did not change after settleIdle", func(t *testing.T) {
		e.openSettings(t)
		time.Sleep(2 * time.Second)
		h := newLatencyHost(t, e, ui)
		_, start, _ := dumpSettle(t, h)
		x, y, ok := emptySpot(start.UIDump)
		if !ok {
			t.Skip("no point of Settings' home outside every element that can be tapped or scrolled")
		}
		exit, c, took := tapScreen(t, h, bridge.UIClickArgs{X: &x, Y: &y})
		if exit.Code != 0 || c.Screen == nil {
			t.Fatalf("exit %+v, screen %v, note %q", exit, c.Screen, c.Note)
		}
		if c.Screen.Changed == nil || *c.Screen.Changed {
			t.Errorf("changed = %v, want false (a tap at %d,%d does nothing)", c.Screen.Changed, x, y)
		}
		if took < settleIdleFloor {
			t.Errorf("a tap with no effect came back after %v, want at least about settleIdle (%v)", took, settleIdleFloor)
		}
	})
}

// emptySpot is a point of the screen inside no element that can be tapped or
// scrolled (a tap there does nothing).
func emptySpot(d bridge.UIDump) (x, y int, ok bool) {
	if d.Width == 0 || d.Height == 0 {
		return 0, 0, false
	}
	for fy := 5; fy < 100; fy += 3 {
		for _, fx := range []int{50, 20, 80, 5, 95} {
			px, py := d.Width*fx/100, d.Height*fy/100
			free := true
			for _, el := range d.Elements {
				b := el.Bounds
				if (el.Click || el.Scroll || el.Edit) && px >= b[0] && px < b[2] && py >= b[1] && py < b[3] {
					free = false
					break
				}
			}
			if free {
				return px, py, true
			}
		}
	}
	return 0, 0, false
}
