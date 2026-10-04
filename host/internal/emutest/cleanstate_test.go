package emutest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/uidump"
)

// dirtyState says why the emulator is not in the state every test starts
// from (screen on, an app or the launcher focused, no shade, no lock screen),
// or "" when it is. focusOK is false when the focus was not understood.
func dirtyState(focus []string, focusOK, asleep bool) string {
	if asleep {
		return "the screen off"
	}
	if !focusOK {
		return "an unreadable focus"
	}
	for _, f := range focus {
		low := strings.ToLower(f)
		switch {
		case f == "":
			return "no focused window"
		case uidump.IsShade(f):
			return fmt.Sprintf("the notification shade open (focus %q)", f)
		case strings.Contains(low, "keyguard") || strings.Contains(low, "bouncer"):
			return fmt.Sprintf("the lock screen (focus %q)", f)
		}
	}
	return ""
}

// once registers key in seen and reports whether it was new: the registry of
// the tests that already have their clean-state cleanup.
func once(mu *sync.Mutex, seen map[any]bool, key any) bool {
	mu.Lock()
	defer mu.Unlock()
	if seen[key] {
		return false
	}
	seen[key] = true
	return true
}

var (
	cleanMu   sync.Mutex
	cleanSeen = map[any]bool{}
)

// screenState reads the focus and the power state without failing a test.
func (e *emu) screenState(ctx context.Context) (focus []string, focusOK, asleep bool, err error) {
	out, code, err := e.adb.Shell(ctx, guardReads["focus"])
	if err != nil || code != 0 {
		return nil, false, false, fmt.Errorf("%s: exit %d, %v", guardReads["focus"], code, err)
	}
	focus, focusOK = uidump.ParseFocus(out)
	out, code, err = e.adb.Shell(ctx, guardReads["power"])
	if err != nil || code != 0 {
		return focus, focusOK, false, fmt.Errorf("%s: exit %d, %v", guardReads["power"], code, err)
	}
	return focus, focusOK, uidump.Asleep(out), nil
}

// cleanState brings the emulator back to the state every test starts from:
// screen on, shade collapsed, the launcher in front, the keyguard dismissed
// when there is no lock credential. It never touches a credential (lockWithPIN
// clears its own PIN) and does not depend on the test's context, which may be
// over. It waits up to 15 s and returns what is still wrong, or "".
func (e *emu) cleanState() string {
	c := *e
	c.ctx = context.WithoutCancel(e.ctx)
	if c.state() != "device" {
		return "the emulator not online"
	}
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Minute)
	defer cancel()
	noLock, _ := c.noLock(ctx)
	act := func() {
		_, _, _, _ = c.adb.Output(ctx, "shell", "input keyevent KEYCODE_WAKEUP")
		if noLock {
			_, _, _, _ = c.adb.Output(ctx, "shell", "wm dismiss-keyguard")
		}
		_, _, _, _ = c.adb.Output(ctx, "shell", "cmd statusbar collapse")
		_, _, _, _ = c.adb.Output(ctx, "shell", "input keyevent KEYCODE_HOME")
	}
	act()
	if !noLock {
		// With a credential the lock screen is expected (the test's own PIN, or
		// one lockWithPIN reports): only the screen and the shade are fixed here.
		return ""
	}
	var why string
	deadline := time.Now().Add(15 * time.Second)
	retry := time.Now().Add(6 * time.Second)
	for {
		focus, ok, asleep, err := c.screenState(ctx)
		if err != nil {
			why = "an unreadable state: " + err.Error()
		} else if why = dirtyState(focus, ok, asleep); why == "" {
			return ""
		}
		if time.Now().After(deadline) {
			return why
		}
		if time.Now().After(retry) {
			act()
			retry = time.Now().Add(6 * time.Second)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// cleanStateAtCleanup registers cleanState at the end of t, once per test
// however many helpers ask for it.
func (e *emu) cleanStateAtCleanup(t *testing.T) {
	t.Helper()
	if !once(&cleanMu, cleanSeen, t) {
		return
	}
	t.Cleanup(func() {
		cleanMu.Lock()
		delete(cleanSeen, t)
		cleanMu.Unlock()
		if why := e.cleanState(); why != "" {
			t.Logf("the emulator is left with %s", why)
		}
	})
}

// requireCleanState fails the test, at its start, when an earlier test left
// the emulator dirty and cleaning it did not work: one test's leftovers must
// not show up as a dozen unrelated failures.
func (e *emu) requireCleanState(t *testing.T) {
	t.Helper()
	if why := e.cleanState(); why != "" {
		t.Fatalf("the emulator was left with %s by an earlier test: restart it (scripts/emulator.sh) or find the test that left it so", why)
	}
}

func TestDirtyState(t *testing.T) {
	for _, c := range []struct {
		name   string
		focus  []string
		ok     bool
		asleep bool
		dirty  bool
	}{
		{"launcher", []string{"com.google.android.apps.nexuslauncher/com.google.android.apps.nexuslauncher.NexusLauncherActivity"}, true, false, false},
		{"settings", []string{"com.android.settings/com.android.settings.Settings"}, true, false, false},
		{"two displays clean", []string{"a/b", "c/d"}, true, false, false},
		{"shade", []string{"NotificationShade"}, true, false, true},
		{"status bar", []string{"StatusBar"}, true, false, true},
		{"empty focus", []string{""}, true, false, true},
		{"one display empty", []string{"a/b", ""}, true, false, true},
		{"keyguard", []string{"Keyguard"}, true, false, true},
		{"bouncer", []string{"com.android.systemui/com.android.systemui.keyguard.BouncerActivity"}, true, false, true},
		{"asleep", []string{"a/b"}, true, true, true},
		{"unreadable", nil, false, false, true},
	} {
		if got := dirtyState(c.focus, c.ok, c.asleep); (got != "") != c.dirty {
			t.Errorf("%s: dirtyState = %q, want dirty=%v", c.name, got, c.dirty)
		}
	}
}

func TestOnceRegistersOnce(t *testing.T) {
	var mu sync.Mutex
	seen := map[any]bool{}
	a, b := new(int), new(int)
	for i, c := range []struct {
		key  any
		want bool
	}{{a, true}, {a, false}, {b, true}, {a, false}, {b, false}} {
		if got := once(&mu, seen, c.key); got != c.want {
			t.Errorf("call %d: once = %v, want %v", i, got, c.want)
		}
	}
}
