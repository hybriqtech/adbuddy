package emutest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// The host's guard (M5 spec §4.3, daemon/guard.go) on the emulator's real
// screens (T5 of the manual validations plan): each test puts the emulator in a
// state (ADBuddy in front, the shade, the quick settings, the lock screen, a
// heads-up, a system dialog, the keyboard, a split screen, the bubble), reads
// what the guard reads there, and asks the host's own daemon, on the emulator's
// adb and without the app (it then reads dumpsys), what it decides. With
// ADBUDDY_GUARD_RECORD set to a folder the reads are written there as
// <state>.<read>.txt: host/internal/uidump/testdata/api34 was recorded so, and
// the tests of uidump and daemon check the guard against those files.

// guardRecordEnv names the folder the reads are recorded in.
const guardRecordEnv = "ADBUDDY_GUARD_RECORD"

// guardReads are the reads of the guard (daemon/guard.go) and of the fallback
// without the app (daemon/ui.go), word for word.
var guardReads = map[string]string{
	"focus":         "dumpsys window | grep -E 'mCurrentFocus'",
	"windows":       "dumpsys window windows",
	"activities":    "dumpsys activity activities",
	"power":         "dumpsys power",
	"accessibility": "dumpsys accessibility",
}

// wallpaperWindow is the title of the wallpaper's window on the API 34 image:
// a window of systemui, visible (mViewVisibility=0x0) behind every app, that
// fixedSystemUI (uidump/window.go) does not list (it has "ImageWallpaper").
// Without the app the guard then sees an unknown system window on every
// screen and blocks every command that touches or reads it.
// TODO(adjudicar): product finding of T5. Blocking more is safe, so the tests
// expect today's decision; once the title is listed, wallpaperReason becomes
// "the command runs".
const wallpaperWindow = "com.android.systemui.wallpapers.ImageWallpaper"

// wallpaperReason is today's refusal on a screen with nothing over the app.
const wallpaperReason = "a system window is on screen (" + wallpaperWindow + ")"

// read runs one of guardReads on the emulator as the guard does (adb shell,
// the output untouched) and records it as state's (not when state is "").
func (e *emu) read(t *testing.T, state, name string) string {
	t.Helper()
	cmd := guardReads[name]
	ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
	defer cancel()
	out, code, err := e.adb.Shell(ctx, cmd)
	if err != nil || code != 0 {
		t.Fatalf("%s: exit %d, %v", cmd, code, err)
	}
	e.keep(t, state, name, out)
	return out
}

// keep records out as the read name of state, when guardRecordEnv is set.
func (e *emu) keep(t *testing.T, state, name, out string) {
	t.Helper()
	dir := os.Getenv(guardRecordEnv)
	if dir == "" || state == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, state+"."+name+".txt"), []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

// screenXML is uiautomator's dump as the fallback reads it (daemon/ui.go),
// recorded as state's "uiautomator". windows adds --windows (every window,
// the bubble's too).
func (e *emu) screenXML(t *testing.T, state string, windows bool) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
	defer cancel()
	args := []string{"exec-out", "uiautomator", "dump", "/dev/tty"}
	if windows {
		args = []string{"exec-out", "uiautomator", "dump", "--windows", "/dev/tty"}
	}
	var out, errOut bytes.Buffer
	code, err := e.adb.Run(ctx, "", args, nil, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("uiautomator dump: exit %d, %v: %s", code, err, errOut.String())
	}
	e.keep(t, state, "uiautomator", out.String())
	return out.Bytes()
}

// screenState is what the guard's readers make of the four reads of a state.
type screenState struct {
	focus   []string // ParseFocus
	unknown []string // ParseSystemUIWindows
	resumed []string // the packages of Resumed, the top one first
	asleep  bool     // Asleep
}

// snapshot reads (and records) the guard's four reads in state and parses
// them; a read the guard would not understand fails the test.
func (e *emu) snapshot(t *testing.T, state string) screenState {
	t.Helper()
	var s screenState
	var ok bool
	if s.focus, ok = uidump.ParseFocus(e.read(t, state, "focus")); !ok {
		t.Fatalf("%s: the focused window is not understood", state)
	}
	if s.unknown, ok = uidump.ParseSystemUIWindows(e.read(t, state, "windows")); !ok {
		t.Fatalf("%s: dumpsys window windows is not understood", state)
	}
	for _, a := range uidump.Resumed(e.read(t, state, "activities")) {
		if !slices.Contains(s.resumed, a.Package) {
			s.resumed = append(s.resumed, a.Package)
		}
	}
	s.asleep = uidump.Asleep(e.read(t, state, "power"))
	t.Logf("%s: focus %q, system windows %q, resumed %q, asleep %v", state, s.focus, s.unknown, s.resumed, s.asleep)
	return s
}

// front returns the packages with a resumed activity, the top one first, as
// the guard reads them.
func (e *emu) front(t *testing.T) []string {
	t.Helper()
	var pkgs []string
	for _, a := range uidump.Resumed(e.read(t, "", "activities")) {
		if !slices.Contains(pkgs, a.Package) {
			pkgs = append(pkgs, a.Package)
		}
	}
	return pkgs
}

// waitFront waits up to 15 s for ok to accept the resumed packages, or fails.
func (e *emu) waitFront(t *testing.T, what string, ok func([]string) bool) []string {
	t.Helper()
	var pkgs []string
	if !poll(15*time.Second, func() bool { pkgs = e.front(t); return ok(pkgs) }) {
		t.Fatalf("not %s in 15 s: resumed %q", what, pkgs)
	}
	return pkgs
}

func first(pkg string) func([]string) bool {
	return func(p []string) bool { return len(p) > 0 && p[0] == pkg }
}

func notFirst(pkg string) func([]string) bool {
	return func(p []string) bool { return len(p) > 0 && p[0] != pkg }
}

// focus returns the titles of the focused windows, as the guard reads them.
func (e *emu) focus(t *testing.T) []string {
	t.Helper()
	f, ok := uidump.ParseFocus(e.read(t, "", "focus"))
	if !ok {
		t.Fatal("the focused window is not understood")
	}
	return f
}

// waitFocus waits up to 15 s for ok to accept the focused windows, or fails.
func (e *emu) waitFocus(t *testing.T, what string, ok func([]string) bool) []string {
	t.Helper()
	var f []string
	if !poll(15*time.Second, func() bool { f = e.focus(t); return ok(f) }) {
		t.Fatalf("not %s in 15 s: focus %q", what, f)
	}
	return f
}

// systemWindows returns the unknown visible windows of systemui now.
func (e *emu) systemWindows(t *testing.T) []string {
	t.Helper()
	w, ok := uidump.ParseSystemUIWindows(e.read(t, "", "windows"))
	if !ok {
		t.Fatal("dumpsys window windows is not understood")
	}
	return w
}

func shade(f []string) bool   { return slices.ContainsFunc(f, uidump.IsShade) }
func noShade(f []string) bool { return !shade(f) }

func focusedOn(pkg string) func([]string) bool {
	return func(f []string) bool { return len(f) == 1 && strings.HasPrefix(f[0], pkg+"/") }
}

// settingsPkg is Settings, the app the tests put in front: not protected.
const settingsPkg = "com.android.settings"

// openSettings closes whatever is open and puts Settings in front, at its top.
func (e *emu) openSettings(t *testing.T) {
	t.Helper()
	e.shell(t, "cmd statusbar collapse")
	e.shell(t, "input keyevent KEYCODE_HOME")
	e.shell(t, "am start -W -S -n "+settingsPkg+"/.Settings")
	e.waitFront(t, "Settings in front", first(settingsPkg))
	e.waitFocus(t, "Settings focused", focusedOn(settingsPkg))
}

// longerUITimeout makes heads-ups and the volume dialog stay on screen 15 s
// (Accessibility > Time to take action), so the test reads them before they
// go; restored at cleanup.
func (e *emu) longerUITimeout(t *testing.T) {
	t.Helper()
	const key = "accessibility_interactive_ui_timeout_ms"
	e.restoreAtCleanup(t, "secure", key, e.shell(t, "settings get secure "+key))
	e.shell(t, "settings put secure "+key+" 15000")
}

// hostGuard is the host's daemon on the emulator's adb, as serve builds it. Its
// guard reads dumpsys when there is no app (ui nil). Nothing the tests run
// through it asks for a confirmation (a question fails the test, and is
// denied), but with allowing: then the person says yes, and what the guard
// decides after that is what the test looks at.
type hostGuard struct {
	d   *daemon.Daemon
	ws  string
	ctx context.Context
	q   *questions
}

type questions struct {
	t     *testing.T
	allow bool
	asked []string
}

func (q *questions) Confirm(_ context.Context, r confirm.Request) confirm.Decision {
	q.asked = append(q.asked, r.RawCommand)
	if q.allow {
		return confirm.Allow
	}
	q.t.Errorf("the guard's test was asked to confirm %q (%s)", r.Title, r.RawCommand)
	return confirm.Deny
}

func newHostGuard(t *testing.T, e *emu, ui func(context.Context, string, json.RawMessage) (json.RawMessage, error)) *hostGuard {
	t.Helper()
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	q := &questions{t: t}
	return &hostGuard{ws: ws, ctx: e.ctx, q: q, d: daemon.New(daemon.Options{
		Classifier: cls, ADB: e.adb, Journal: journal.OpenSigned(filepath.Join(ws, "journal"), []byte("emutest")),
		Confirmer: q, Serial: e.adb.Serial, Session: "emutest", Lang: "en", Workspace: ws, UI: ui,
	})}
}

// allowing makes the person say yes to every question from now on.
func (g *hostGuard) allowing() { g.q.allow = true }

// guardStream is the agent's side of a request.
type guardStream struct {
	out, errOut bytes.Buffer
	data        []byte
}

func (s *guardStream) Stdin() io.Reader  { return strings.NewReader("") }
func (s *guardStream) Stdout() io.Writer { return &s.out }
func (s *guardStream) Stderr() io.Writer { return &s.errOut }
func (s *guardStream) Data(v any) error {
	b, err := json.Marshal(v)
	s.data = append(s.data, b...)
	return err
}

// exec runs argv as the agent's adb through the shim; a refusal's reason is in
// the exit's Error.
func (g *hostGuard) exec(argv ...string) ipc.Exit {
	s := &guardStream{}
	exit := g.d.Handle(g.ctx, ipc.Request{Kind: ipc.KindExec, Argv: argv, Cwd: g.ws, Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	if exit.Error == "" {
		exit.Error = s.errOut.String()
	}
	return exit
}

// helper runs one of the agent's helpers (adbuddy screen, tap…) and returns
// its exit and data.
func (g *hostGuard) helper(op string, args any) (ipc.Exit, string) {
	raw, _ := json.Marshal(args)
	s := &guardStream{}
	exit := g.d.Handle(g.ctx, ipc.Request{Kind: ipc.KindUI, UIOp: op, UIArgs: raw, Argv: []string{"adbuddy", op}, Cwd: g.ws,
		Env: map[string]string{"ADBUDDY_SOURCE": "agent"}}, s)
	return exit, string(s.data)
}

// blocks checks that the guard refuses argv because of why.
func (g *hostGuard) blocks(t *testing.T, why string, argv ...string) {
	t.Helper()
	if exit := g.exec(argv...); exit.Code != daemon.ExitDenied || !strings.Contains(exit.Error, why) {
		t.Errorf("%q: exit %d %q, want blocked because %q", argv, exit.Code, exit.Error, why)
	}
}

// helperBlocks checks that the guard refuses the helper op because of why.
func (g *hostGuard) helperBlocks(t *testing.T, why, op string, args any) {
	t.Helper()
	if exit, _ := g.helper(op, args); exit.Code != daemon.ExitDenied || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, why) {
		t.Errorf("adbuddy %s: %+v, want blocked because %q", op, exit, why)
	}
}

// passes checks that the guard lets argv run, and that it ran.
func (g *hostGuard) passes(t *testing.T, argv ...string) {
	t.Helper()
	if exit := g.exec(argv...); exit.Code != 0 {
		t.Errorf("%q: exit %d %q, want it to run", argv, exit.Code, exit.Error)
	}
}

var (
	screencap = []string{"exec-out", "screencap", "-p"}
	tapMiddle = []string{"shell", "input", "tap", "540", "1200"}
	dumpUI    = []string{"shell", "uiautomator", "dump"}
	keyBack   = []string{"shell", "input", "keyevent", "4"}
	keyHome   = []string{"shell", "input", "keyevent", "3"}
	keyWake   = []string{"shell", "input", "keyevent", "224"}
)

func xyArgs(x, y int) bridge.UIClickArgs { return bridge.UIClickArgs{X: &x, Y: &y} }

// onlyWallpaper checks that the only unknown system window is the wallpaper's.
func onlyWallpaper(t *testing.T, s screenState) {
	t.Helper()
	if !slices.Equal(s.unknown, []string{wallpaperWindow}) {
		t.Errorf("unknown system windows %q, want only the wallpaper's", s.unknown)
	}
}

// TestTheGuardOnRealScreens: the guard without the app on the emulator's real
// screens: ADBuddy in front and Home (M5c-28, V44), the shade and the quick
// settings closed with Back (M5b-14), the keyboard and a system dialog (M5a-7),
// a split screen with ADBuddy in one half and the way out of it (M5a-7, T1-11),
// the screen off, Doze and the lock screen with a PIN (M5b-14 (a), M5a-9).
// Every read is recorded (M5a-6, M5a-9).
func TestTheGuardOnRealScreens(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	e.atCleanup(t, "shell", "cmd statusbar collapse")
	e.atCleanup(t, "shell", "input keyevent KEYCODE_WAKEUP")
	g := newHostGuard(t, e, nil)

	// Settings in front, nothing over it. The command should run; today the
	// wallpaper's window blocks it (wallpaperWindow).
	t.Run("settings", func(t *testing.T) {
		e.openSettings(t)
		s := e.snapshot(t, "settings")
		if !slices.Equal(s.resumed, []string{settingsPkg}) || shade(s.focus) || s.asleep {
			t.Errorf("Settings in front: %+v", s)
		}
		onlyWallpaper(t, s)
		if _, err := uidump.Find(e.screenXML(t, "settings", false), "", "Network & internet", nil); err != nil {
			t.Errorf("Find Network & internet in Settings: %v", err)
		}
		g.blocks(t, wallpaperReason, screencap...)
		g.helperBlocks(t, wallpaperReason, bridge.UIOpDump, bridge.UIDumpArgs{})
	})

	// ADBuddy in front (V44): nothing touches or reads it; Home alone leaves.
	t.Run("adbuddy", func(t *testing.T) {
		e.openSettings(t)
		e.shell(t, "am start -W -n "+phoneapp.Activity)
		e.waitFront(t, "ADBuddy in front", first(phoneapp.Package))
		e.waitFocus(t, "ADBuddy focused", focusedOn(phoneapp.Package))
		s := e.snapshot(t, "adbuddy")
		if len(s.resumed) == 0 || s.resumed[0] != phoneapp.Package {
			t.Errorf("resumed %q, want ADBuddy first", s.resumed)
		}
		if _, err := uidump.Find(e.screenXML(t, "adbuddy", false), "", "Not now", nil); err != uidump.ErrProtected {
			t.Errorf("Find Not now on ADBuddy's screen: %v, want ErrProtected", err)
		}
		const why = "ADBuddy itself is in front"
		for _, argv := range [][]string{screencap, tapMiddle, dumpUI, keyBack} {
			g.blocks(t, why, argv...)
		}
		g.helperBlocks(t, why, bridge.UIOpClick, xyArgs(540, 1200))
		g.helperBlocks(t, why, bridge.UIOpDump, bridge.UIDumpArgs{})
		g.helperBlocks(t, why, daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: 1, Format: "png"})
		g.passes(t, keyHome...)
		e.waitFront(t, "ADBuddy gone after Home", notFirst(phoneapp.Package))
	})

	// The shade and the quick settings: a code may be on them. Back closes them
	// (from the quick settings, one Back goes back to the shade).
	for _, c := range []struct {
		state, open string
		backs       int
	}{
		{"shade", "cmd statusbar expand-notifications", 1},
		{"quick-settings", "cmd statusbar expand-settings", 2},
	} {
		t.Run(c.state, func(t *testing.T) {
			e.openSettings(t)
			e.shell(t, c.open)
			e.waitFocus(t, "the shade focused", shade)
			time.Sleep(time.Second) // the animation
			s := e.snapshot(t, c.state)
			if !slices.Equal(s.focus, []string{"NotificationShade"}) || !slices.Contains(s.unknown, "NotificationShade") {
				t.Errorf("%s: %+v", c.state, s)
			}
			const why = "the notification shade is open"
			for _, argv := range [][]string{screencap, tapMiddle, dumpUI, {"shell", "input", "keyevent", "--longpress", "3"}} {
				g.blocks(t, why, argv...)
			}
			g.helperBlocks(t, why, bridge.UIOpDump, bridge.UIDumpArgs{})
			for i := 0; i < c.backs; i++ {
				g.passes(t, keyBack...)
				time.Sleep(time.Second)
			}
			e.waitFocus(t, "the shade closed by Back", noShade)
			g.blocks(t, wallpaperReason, screencap...)
		})
	}

	// The keyboard over Settings' search: the input method's window is not
	// systemui's, and changes nothing.
	t.Run("keyboard", func(t *testing.T) {
		e.openSettings(t)
		e.atCleanup(t, "shell", "am force-stop com.google.android.settings.intelligence")
		e.shell(t, "am start -W -a android.settings.APP_SEARCH_SETTINGS")
		if !poll(15*time.Second, func() bool { return strings.Contains(e.shell(t, "dumpsys input_method"), "mInputShown=true") }) {
			t.Fatal("no keyboard on screen 15 s after opening Settings' search")
		}
		s := e.snapshot(t, "keyboard")
		if !strings.Contains(e.read(t, "keyboard", "windows"), " u0 InputMethod}:") || len(s.resumed) == 0 || s.resumed[0] != "com.google.android.settings.intelligence" {
			t.Errorf("keyboard: %+v", s)
		}
		onlyWallpaper(t, s)
		g.blocks(t, wallpaperReason, screencap...)
		e.shell(t, "am force-stop com.google.android.settings.intelligence")
	})

	// A system dialog: the volume panel, systemui's, over Settings. One volume
	// key while it is hidden only shows it; the volume is checked unchanged.
	t.Run("volume-dialog", func(t *testing.T) {
		e.openSettings(t)
		e.longerUITimeout(t)
		vol := e.shell(t, "settings get system volume_music_speaker")
		e.shell(t, "input keyevent KEYCODE_VOLUME_DOWN")
		if !poll(10*time.Second, func() bool { return slices.Contains(e.systemWindows(t), "VolumeDialogImpl") }) {
			t.Fatal("the volume dialog is not on screen 10 s after the volume key")
		}
		g.blocks(t, "VolumeDialogImpl", screencap...)
		s := e.snapshot(t, "volume-dialog")
		if !slices.Contains(s.unknown, "VolumeDialogImpl") || shade(s.focus) {
			t.Errorf("volume dialog: %+v", s)
		}
		if !poll(30*time.Second, func() bool { return !slices.Contains(e.systemWindows(t), "VolumeDialogImpl") }) {
			t.Error("the volume dialog is still on screen 30 s later")
		}
		if now := e.shell(t, "settings get system volume_music_speaker"); now != vol {
			t.Errorf("the volume changed from %s to %s", vol, now)
		}
	})

	// A split screen with ADBuddy in one half (T1-11): the guard sees both
	// halves and refuses, and Home is no way out there. The way out measured:
	// Home and opening an app leave both tasks full screen.
	t.Run("split", func(t *testing.T) {
		e.openSettings(t)
		e.shell(t, "am start -W -n "+phoneapp.Activity)
		e.waitFront(t, "ADBuddy in front", first(phoneapp.Package))
		e.shell(t, "dumpsys activity service SystemUIService WMShell splitscreen moveToSideStage "+e.taskOf(t, settingsPkg)+" 1")
		e.waitFront(t, "both halves resumed", func(p []string) bool {
			return len(p) == 2 && slices.Contains(p, settingsPkg) && slices.Contains(p, phoneapp.Package)
		})
		s := e.snapshot(t, "split")
		if len(s.resumed) != 2 {
			t.Errorf("split: %+v", s)
		}
		for _, argv := range [][]string{screencap, tapMiddle, keyHome} {
			g.blocks(t, phoneapp.Package, argv...)
		}
		e.shell(t, "input keyevent KEYCODE_HOME")
		// Home must have ended (neither half resumed) before an app opens: an
		// app opened during the animation brings the pair back.
		e.waitFront(t, "the launcher after Home", func(p []string) bool {
			return len(p) > 0 && !slices.Contains(p, settingsPkg) && !slices.Contains(p, phoneapp.Package)
		})
		time.Sleep(2 * time.Second)
		e.shell(t, "am start -W -n "+settingsPkg+"/.Settings")
		e.waitFront(t, "Settings alone after leaving the split screen", func(p []string) bool { return slices.Equal(p, []string{settingsPkg}) })
		// The split root's own two tasks (no app, invisible) keep their halves.
		for _, line := range strings.Split(e.shell(t, "am stack list"), "\n") {
			if (strings.Contains(line, ": "+settingsPkg+"/") || strings.Contains(line, ": "+phoneapp.Package+"/")) && !strings.Contains(line, "bounds=[0,0][1080,2400]") {
				t.Errorf("a task is still in a half after Home and am start: %s", strings.TrimSpace(line))
			}
		}
	})

	// The screen off and Doze (M5a-9): only the exact wake key goes through.
	t.Run("screen-off", func(t *testing.T) {
		e.openSettings(t)
		e.sleepScreen(t)
		s := e.snapshot(t, "screen-off")
		// The focus stays on Settings or goes null a moment later: either is not the shade.
		if !s.asleep || shade(s.focus) {
			t.Errorf("screen off: %+v", s)
		}
		g.blocks(t, wallpaperReason, screencap...)
		g.passes(t, keyWake...)
		e.waitFocus(t, "Settings focused after the wake key", focusedOn(settingsPkg))

		e.sleepScreen(t)
		e.atCleanup(t, "shell", "dumpsys deviceidle unforce")
		if out := e.shell(t, "dumpsys deviceidle force-idle"); !strings.Contains(out, "deep idle") {
			t.Fatalf("force-idle: %s", out)
		}
		if s := e.snapshot(t, "doze"); !s.asleep {
			t.Errorf("Doze: %+v", s)
		}
		e.shell(t, "dumpsys deviceidle unforce")
		e.wakeScreen(t)
	})

	// The lock screen with a PIN (M5b-14 (a)): it is the shade; Back, Home and,
	// with the screen off, the exact wake key go through, and nothing else.
	t.Run("locked", func(t *testing.T) {
		e.openSettings(t)
		e.lockWithPIN(t)
		e.sleepScreen(t)
		if !poll(10*time.Second, func() bool { return e.locked(t) }) {
			t.Fatal("not locked 10 s after the screen went off with a PIN")
		}
		s := e.snapshot(t, "locked-off")
		if !s.asleep || !shade(s.focus) {
			t.Errorf("locked, screen off: %+v", s)
		}
		const why = "the notification shade is open"
		g.blocks(t, why, screencap...)
		g.passes(t, keyWake...)
		if !poll(10*time.Second, func() bool { return !uidump.Asleep(e.read(t, "", "power")) }) {
			t.Fatal("the wake key did not turn the screen on")
		}
		time.Sleep(time.Second)
		s = e.snapshot(t, "locked")
		if s.asleep || !shade(s.focus) || !e.locked(t) {
			t.Errorf("locked, screen on: %+v", s)
		}
		for _, argv := range [][]string{screencap, tapMiddle, dumpUI, keyWake} {
			g.blocks(t, why, argv...)
		}
		g.helperBlocks(t, why, bridge.UIOpDump, bridge.UIDumpArgs{})
		g.passes(t, keyBack...)
		g.passes(t, keyHome...)
		if !e.locked(t) {
			t.Error("Back and Home unlocked the device")
		}
	})
}

// TestTheGuardWithAHeadsUp (the sample of M5b-17): a heads-up of an app over
// Settings shows NotificationShade visible in dumpsys window windows, and the
// guard without the app refuses because of it; once the heads-up is gone the
// notification stays in the shade and only the fixed windows (and the
// wallpaper's) are left.
func TestTheGuardWithAHeadsUp(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	e.installTests(t)
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	e.longerUITimeout(t)
	g := newHostGuard(t, e, nil)
	e.openSettings(t)

	release := e.guardPost(t, "headsup", headsUpID)
	if !poll(10*time.Second, func() bool { return slices.Contains(e.systemWindows(t), "NotificationShade") }) {
		t.Fatal("no heads-up (NotificationShade visible) 10 s after the notification")
	}
	g.blocks(t, "a system window is on screen (NotificationShade", screencap...)
	s := e.snapshot(t, "headsup")
	if !slices.Contains(s.unknown, "NotificationShade") || shade(s.focus) || !slices.Equal(s.resumed, []string{settingsPkg}) {
		t.Errorf("heads-up: %+v", s)
	}

	if !poll(40*time.Second, func() bool { return !slices.Contains(e.systemWindows(t), "NotificationShade") }) {
		t.Fatal("the heads-up is still on screen 40 s later")
	}
	if !strings.Contains(e.shell(t, "cmd notification list"), "|"+phoneapp.Package+"|"+headsUpID+"|") {
		t.Error("the notification is gone with the heads-up")
	}
	s = e.snapshot(t, "headsup-gone")
	onlyWallpaper(t, s)
	g.blocks(t, wallpaperReason, screencap...)
	release()
}

// bubbleScene is the bubble of ADBuddy's conversation (a test notification
// without the foreground service, which on API 34 is the only way it
// bubbles: T1-8) collapsed over Settings, with the guard up. It returns the
// emulator, the verified app, the guard and the release of the notification.
func bubbleScene(t *testing.T) (*emu, phoneapp.Verified, *hostGuard, func()) {
	e := newEmu(t)
	v := e.installApp(t)
	e.installTests(t)
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")
	e.shell(t, "cmd notification set_bubbles "+phoneapp.Package+" 1")
	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	g := newHostGuard(t, e, nil)
	e.openSettings(t)

	release := e.guardPost(t, "bubble", bubbleID)
	if !poll(15*time.Second, func() bool { return slices.Contains(e.systemWindows(t), "Bubbles!") }) {
		t.Fatal("no bubble window 15 s after the notification")
	}
	return e, v, g, release
}

// TestTheGuardWithTheCollapsedBubble (M3b-8 and M5c-34, the guard's part): the
// collapsed bubble over Settings; its systemui window "Bubbles!" blocks.
func TestTheGuardWithTheCollapsedBubble(t *testing.T) {
	e, _, g, release := bubbleScene(t)
	s := e.snapshot(t, "bubble-collapsed")
	if !slices.Equal(s.resumed, []string{settingsPkg}) || !slices.Contains(s.unknown, "Bubbles!") {
		t.Errorf("collapsed bubble: %+v", s)
	}
	g.blocks(t, "Bubbles!", screencap...)
	release()
}

// bubbleTapAttempts is how many times the expanded-bubble test taps the bubble.
const bubbleTapAttempts = 3

// tapCenter returns the point at the center of bounds ([l, t, r, b]).
func tapCenter(l, tp, r, bt int) (x, y int) { return (l + r) / 2, (tp + bt) / 2 }

// expandBubble taps the bubble until ADBuddy is the first resumed activity,
// re-reading the bubble's bounds on each attempt, and returns "" or, if it
// never expanded, the last state seen.
func (e *emu) expandBubble(t *testing.T) string {
	t.Helper()
	last := "no attempt"
	for i := 1; i <= bubbleTapAttempts; i++ {
		// --windows dumps one hierarchy per window: the bubble's bounds are
		// read from its node.
		m := bubbleRe.FindSubmatch(e.screenXML(t, "", true))
		if m == nil {
			last = fmt.Sprintf("attempt %d: no bubble_view in uiautomator dump --windows", i)
			time.Sleep(2 * time.Second)
			continue
		}
		l, _ := strconv.Atoi(string(m[1]))
		tp, _ := strconv.Atoi(string(m[2]))
		r, _ := strconv.Atoi(string(m[3]))
		bt, _ := strconv.Atoi(string(m[4]))
		x, y := tapCenter(l, tp, r, bt)
		e.shell(t, "input tap "+strconv.Itoa(x)+" "+strconv.Itoa(y))
		var s []string
		if poll(8*time.Second, func() bool {
			s = e.front(t)
			return first(phoneapp.Package)(s)
		}) {
			return ""
		}
		last = fmt.Sprintf("attempt %d, tap at %d,%d: resumed %q", i, x, y, s)
	}
	return last
}

// TestTheGuardWithTheExpandedBubble (M3b-8 and M5c-34, the guard's part): the
// bubble expanded, ChatBubbleActivity is resumed first and the guard refuses
// as with ADBuddy in front, with the accessibility service off and on. The
// tap that expands the bubble is a race with systemui on some emulators: if
// it does not expand in three attempts the test skips; if it does, every
// check is hard.
func TestTheGuardWithTheExpandedBubble(t *testing.T) {
	e, v, g, release := bubbleScene(t)
	if last := e.expandBubble(t); last != "" {
		release()
		t.Skipf("the bubble did not expand on this emulator (%s)", last)
	}
	e.waitFocus(t, "the bubble focused", focusedOn(phoneapp.Package))
	check := func(state string) {
		t.Helper()
		s := e.snapshot(t, state)
		if len(s.resumed) < 2 || s.resumed[0] != phoneapp.Package || !slices.Contains(s.resumed, settingsPkg) || !focusedOn(phoneapp.Package)(s.focus) {
			t.Errorf("%s: %+v", state, s)
		}
		const why = "ADBuddy itself is in front"
		for _, argv := range [][]string{screencap, tapMiddle, dumpUI} {
			g.blocks(t, why, argv...)
		}
		g.helperBlocks(t, why, bridge.UIOpClick, xyArgs(540, 1200))
	}
	check("bubble-expanded")

	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	var a11y string
	if !poll(20*time.Second, func() bool { a11y = e.read(t, "", "accessibility"); return a11yBound(a11y) }) {
		t.Fatalf("ADBuddy's service is not bound 20 s after enabling it:\n%s", a11y)
	}
	check("bubble-expanded-a11y")
	e.read(t, "bubble-expanded-a11y", "accessibility")
	e.shell(t, "input keyevent KEYCODE_BACK")
	release()
}

// bubbleRe finds the bounds of the bubble in uiautomator's dump.
var bubbleRe = regexp.MustCompile(`resource-id="com\.android\.systemui:id/bubble_view"[^>]*bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"`)

// guardDone ends GuardOnEmulatorTest.
const guardDone = "tech.hybriq.adbuddy.test.GUARD_DONE"

// The ids of GuardOnEmulatorTest's notifications.
const (
	headsUpID = "7302"
	bubbleID  = "7303"
)

// guardPost runs GuardOnEmulatorTest with kind (headsup or bubble) until its
// notification id is posted, and returns what releases it: the release
// (idempotent, and at cleanup too) waits for the instrumented test to end and
// checks that it passed and took its notification away.
func (e *emu) guardPost(t *testing.T, kind, id string) (release func()) {
	t.Helper()
	type result struct {
		out, errOut string
		code        int
		err         error
	}
	done := make(chan result, 1)
	// Not canceled with the test: the release, at cleanup too, ends it.
	runner, ctx := e.adb, context.WithoutCancel(e.ctx)
	go func() {
		c, cancel := context.WithTimeout(ctx, instrumentTimeout)
		defer cancel()
		out, errOut, code, err := runner.Output(c, "shell", "am instrument -w -r -e class "+guardTest+
			" -e adbuddyGuard "+kind+" -e holdMs 240000 "+instrumentRunner)
		done <- result{strings.ReplaceAll(out, "\r", ""), errOut, code, err}
	}()
	key := "|" + phoneapp.Package + "|" + id + "|"
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		ctx, cancel := context.WithTimeout(context.WithoutCancel(e.ctx), 3*time.Minute)
		defer cancel()
		_, _, _, _ = e.adb.Output(ctx, "shell", "am broadcast -a "+guardDone+" -p "+phoneapp.Package)
		select {
		case r := <-done:
			if tests, _, _, failed := parseInstrument(r.out); r.err != nil || r.code != 0 || failed || tests != 1 {
				t.Errorf("%s (%s) did not pass: exit %d, %v: %s\n%s", guardTest, kind, r.code, r.err, r.errOut, r.out)
			}
		case <-time.After(2 * time.Minute):
			t.Errorf("am instrument did not end 2 min after %s", guardDone)
		}
		out, _, _, _ := e.adb.Output(ctx, "shell", "cmd notification list")
		if strings.Contains(out, key) {
			t.Errorf("the notification %s is still posted after the test", key)
		}
	}
	t.Cleanup(release)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(500 * time.Millisecond) {
		if strings.Contains(e.shell(t, "cmd notification list"), key) {
			return release
		}
		select {
		case r := <-done:
			released = true
			t.Fatalf("am instrument ended before the notification was posted: exit %d, %v: %s\n%s", r.code, r.err, r.errOut, r.out)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the notification %s is not posted 90 s after am instrument", key)
		}
	}
}

// sleepScreen turns the screen off and waits until dumpsys power says so.
func (e *emu) sleepScreen(t *testing.T) {
	t.Helper()
	e.shell(t, "input keyevent KEYCODE_SLEEP")
	if !poll(10*time.Second, func() bool { return uidump.Asleep(e.read(t, "", "power")) }) {
		t.Fatal("the screen is not off 10 s after KEYCODE_SLEEP")
	}
}

// wakeScreen turns the screen on.
func (e *emu) wakeScreen(t *testing.T) {
	t.Helper()
	e.shell(t, "input keyevent KEYCODE_WAKEUP")
	if !poll(10*time.Second, func() bool { return !uidump.Asleep(e.read(t, "", "power")) }) {
		t.Fatal("the screen is not on 10 s after KEYCODE_WAKEUP")
	}
}

var taskRe = regexp.MustCompile(`taskId=(\d+): (\S+)/`)

// taskOf returns the id of pkg's task in `am stack list`, or fails.
func (e *emu) taskOf(t *testing.T, pkg string) string {
	t.Helper()
	out := e.shell(t, "am stack list")
	for _, m := range taskRe.FindAllStringSubmatch(out, -1) {
		if m[2] == pkg {
			return m[1]
		}
	}
	t.Fatalf("no task of %s in am stack list:\n%s", pkg, out)
	return ""
}

// a11yBound reports whether dumpsys accessibility shows ADBuddy's service
// bound (not only enabled): the bound ones are listed by label.
func a11yBound(dump string) bool {
	_, after, ok := strings.Cut(dump, "Bound services:{")
	if !ok {
		return false
	}
	bound, _, _ := strings.Cut(after, "\n")
	return strings.Contains(bound, "label=ADBuddy")
}

// Validation 5 of M5a: without the app, adbuddy screen reads the screen with
// uiautomator while ADBuddy's accessibility service is off, and answers
// UI_UNAVAILABLE while it is on, without running uiautomator: the service
// stays bound. dumpsys accessibility in each case is recorded
// (uidump.ServiceOn, M5a-9). The guard in front of the fallback would refuse
// on every screen today (wallpaperWindow), so the app only answers what is in
// front (Settings, as it would); every other helper finds no app.
func TestTheFallbackWithoutTheApp(t *testing.T) {
	e := newEmu(t)
	v := e.installApp(t)
	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	g := newHostGuard(t, e, func(_ context.Context, op string, _ json.RawMessage) (json.RawMessage, error) {
		if op == bridge.UIOpForeground {
			return json.RawMessage(`{"package":"` + settingsPkg + `","activity":"` + settingsPkg + `.Settings","protected":false}`), nil
		}
		return nil, daemon.ErrNoApp
	})
	e.openSettings(t)

	if a := e.read(t, "a11y-off", "accessibility"); uidump.ServiceOn(a) || a11yBound(a) {
		t.Fatal("ADBuddy's service is on before it was enabled")
	}
	if exit, data := g.helper(bridge.UIOpDump, bridge.UIDumpArgs{}); exit.Code != 0 || !strings.Contains(data, `"package":"`+settingsPkg+`"`) {
		t.Errorf("adbuddy screen with the service off: %+v %.200s, want uiautomator's dump of Settings", exit, data)
	}

	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	var a11y string
	if !poll(20*time.Second, func() bool { a11y = e.read(t, "", "accessibility"); return a11yBound(a11y) }) {
		t.Fatalf("ADBuddy's service is not bound 20 s after enabling it:\n%s", a11y)
	}
	e.keep(t, "a11y-on", "accessibility", a11y)
	if !uidump.ServiceOn(a11y) {
		t.Error("ServiceOn is false with the service bound")
	}
	if exit, _ := g.helper(bridge.UIOpDump, bridge.UIDumpArgs{}); exit.Code == 0 || exit.ErrorCode != "UI_UNAVAILABLE" {
		t.Errorf("adbuddy screen with the service on and no app: %+v, want UI_UNAVAILABLE", exit)
	}
	if a := e.read(t, "", "accessibility"); !a11yBound(a) {
		t.Errorf("the service is no longer bound after adbuddy screen:\n%s", a)
	}
}

// VB-36: Android decodes %XX in an android-app:// URI's host (am prints the
// intent with the name decoded) and in an intent: URI's values, where an
// encoded package or component opens that app. The guard decodes before it
// compares (daemon/guard.go, decodings): ADBuddy written so is blocked.
func TestAnEncodedAppURIOpensTheApp(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	// Otherwise ADBuddy opens on the system's question about notifications.
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	g := newHostGuard(t, e, nil)
	e.openSettings(t)
	e.shell(t, "input keyevent KEYCODE_HOME")
	e.waitFront(t, "Settings gone", notFirst(settingsPkg))

	if out := e.shell(t, "am start -W -a android.intent.action.VIEW -d android-app://com%2Eandroid%2Esettings"); !strings.Contains(out, "dat=android-app://com.android.settings/") {
		t.Errorf("am does not show the android-app:// host decoded:\n%s", out)
	}
	e.shell(t, "input keyevent KEYCODE_HOME")
	e.shell(t, "am start -W 'intent:#Intent;action=android.intent.action.MAIN;category=android.intent.category.LAUNCHER;package=com%2Eandroid%2Esettings;end'")
	e.waitFront(t, "Settings opened by its encoded package", first(settingsPkg))
	e.shell(t, "input keyevent KEYCODE_HOME")
	e.shell(t, "am start -W 'intent:#Intent;component=tech%2Ehybriq%2Eadbuddy/%2EMainActivity;end'")
	e.waitFront(t, "ADBuddy opened by its encoded component", first(phoneapp.Package))
	e.shell(t, "input keyevent KEYCODE_HOME")
	e.waitFront(t, "ADBuddy gone", notFirst(phoneapp.Package))

	// The classifier asks before an am start with a URI (🔴); the person says
	// yes here, and the guard still refuses.
	g.allowing()
	const why = phoneapp.Package + " is a protected app"
	for _, argv := range [][]string{
		{"shell", "am start -W 'intent:#Intent;component=tech%2Ehybriq%2Eadbuddy/%2EMainActivity;end'"},
		{"shell", "am start -W 'intent:#Intent;component=tech%252Ehybriq%252Eadbuddy/%252EMainActivity;end'"},
		{"shell", "am", "start", "-W", "-a", "android.intent.action.VIEW", "-d", "android-app://tech%2Ehybriq%2Eadbuddy"},
	} {
		g.blocks(t, why, argv...)
	}
	time.Sleep(2 * time.Second)
	if p := e.front(t); len(p) > 0 && p[0] == phoneapp.Package {
		t.Error("ADBuddy is in front after the guard blocked opening it")
	}
}

// dumpsysReadForms are forms of dumpsys of the services O1-a names that the
// classifier lets run without asking (classify/checks_read.go, dumpsysReads).
var dumpsysReadForms = []string{
	"dumpsys activity", "dumpsys activity activities", "dumpsys activity recents", "dumpsys activity broadcasts",
	"dumpsys activity broadcast-stats", "dumpsys activity intents", "dumpsys activity permissions",
	"dumpsys activity processes", "dumpsys activity oom", "dumpsys activity lmk", "dumpsys activity lru",
	"dumpsys activity services", "dumpsys activity providers", "dumpsys activity top", "dumpsys activity starter",
	"dumpsys activity containers", "dumpsys activity lastanr", "dumpsys activity associations",
	"dumpsys activity exit-info", "dumpsys activity users", "dumpsys activity settings", "dumpsys activity binder-proxies",
	"dumpsys activity -p com.android.settings", "dumpsys activity com.android.settings",
	"dumpsys window", "dumpsys window windows", "dumpsys window displays", "dumpsys window tokens", "dumpsys window policy",
	"dumpsys window animator", "dumpsys window sessions", "dumpsys window lastanr", "dumpsys window input",
	"dumpsys window visible", "dumpsys window visible-apps", "dumpsys window containers", "dumpsys window -a",
	"dumpsys procstats", "dumpsys procstats --hours 3", "dumpsys procstats --last 1", "dumpsys procstats --current",
	"dumpsys procstats --details", "dumpsys procstats --full-details", "dumpsys procstats --active", "dumpsys procstats -c",
	"dumpsys procstats com.android.settings",
	"dumpsys package", "dumpsys package packages", "dumpsys package permissions", "dumpsys package features",
	"dumpsys package libraries", "dumpsys package preferred", "dumpsys package preferred-xml", "dumpsys package providers",
	"dumpsys package messages", "dumpsys package verifiers", "dumpsys package version", "dumpsys package installs",
	"dumpsys package shared-users", "dumpsys package queries", "dumpsys package domain-preferred-apps",
	"dumpsys package keysets", "dumpsys package frozen", "dumpsys package volumes", "dumpsys package dexopt",
	"dumpsys package compiler-stats", "dumpsys package changes", "dumpsys package service-permissions",
	"dumpsys package known-packages", "dumpsys package --checkin", "dumpsys package com.android.settings",
	"dumpsys meminfo", "dumpsys meminfo -a", "dumpsys meminfo -d", "dumpsys meminfo -c", "dumpsys meminfo -s",
	"dumpsys meminfo --oom", "dumpsys meminfo --checkin", "dumpsys meminfo --package com.android.settings",
	"dumpsys meminfo com.android.settings",
	"dumpsys batterystats", "dumpsys batterystats --charged", "dumpsys batterystats -c", "dumpsys batterystats --history",
	"dumpsys batterystats --daily", "dumpsys batterystats com.android.settings",
}

// phoneState is what O1-a compares before and after: every setting, and the
// files procstats and batterystats keep (a read that consumed them would
// rename or delete them).
func (e *emu) phoneState(t *testing.T) map[string]string {
	t.Helper()
	st := map[string]string{}
	for _, ns := range []string{"global", "secure", "system"} {
		st["settings "+ns] = e.shell(t, "settings list "+ns)
	}
	st["procstats files"] = e.shell(t, "su 0 ls /data/system/procstats")
	st["batterystats files"] = e.shell(t, "su 0 ls /data/system/ | grep -i batterystats; true")
	return st
}

// O1-a: the dumpsys reads of activity, window, procstats, package, meminfo and
// batterystats that the classifier lets run (🟢) change no setting and none of
// the files procstats and batterystats keep. Seen, not proven: that dumpsys
// changes nothing is AOSP's, and is checked against its source, not here.
func TestTheDumpsysReadsChangeNothingSeen(t *testing.T) {
	e := newEmu(t)
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	before := e.phoneState(t)
	for _, form := range dumpsysReadForms {
		if res := cls.Classify([]string{"shell", form}); res.Level != classify.Green {
			t.Errorf("%s is %v for the classifier, not a read it lets run", form, res.Level)
			continue
		}
		ctx, cancel := context.WithTimeout(e.ctx, 2*time.Minute)
		out, code, err := e.adb.Shell(ctx, form)
		cancel()
		if err != nil || code != 0 {
			t.Errorf("%s: exit %d, %v", form, code, err)
		}
		t.Logf("%s: %d bytes", form, len(out))
	}
	after := e.phoneState(t)
	for k, b := range before {
		if a := after[k]; a != b {
			t.Errorf("%s changed:\nbefore: %s\nafter:  %s", k, b, a)
		}
	}
}
