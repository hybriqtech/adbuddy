package emutest

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// TEMPORARY (audit/adbuddy-yolo, ADB-03): diagnostics for the unlock right after lockWithPIN. Reverted before the PR is ready.

// probeLockScreen logs what the system says about the lock screen, read-only.
func (e *emu) probeLockScreen(t *testing.T, label string) {
	t.Helper()
	t.Logf("DIAG[%s] at %s", label, time.Now().Format("15:04:05"))
	e.logShell(t, "dumpsys window | grep -E 'mCurrentFocus|isKeyguardShowing|mKeyguardShowing|mDreamingLockscreen|mKeyguardGoingAway|mKeyguardOccluded|mAodShowing' | head -20")
	e.logShell(t, "dumpsys trust | head -20")
	e.logShell(t, "dumpsys power | grep -E 'mWakefulness=|mWakefulnessChanging|Display Power'")
	e.logShell(t, "dumpsys activity activities | grep -E 'topResumedActivity|mResumedActivity'")
	e.logShell(t, "dumpsys notification --noredact | grep -c 'NotificationRecord'")
	nodes := e.screen(t)
	rows := 0
	for _, n := range nodes {
		if strings.Contains(n.ID, "expandableNotificationRow") || strings.Contains(n.ID, "notification_row") {
			rows++
		}
	}
	t.Logf("DIAG[%s] %d nodes, %d notification rows, bouncer=%v", label, len(nodes), rows, bouncer(nodes))
}

// tryBouncer runs act and waits up to 6 s for the bouncer; it logs the outcome.
func (e *emu) tryBouncer(t *testing.T, name string, act func()) bool {
	t.Helper()
	start := time.Now()
	act()
	ok := poll(6*time.Second, func() bool { return bouncer(e.screen(t)) })
	t.Logf("DIAG step %q: bouncer=%v after %s", name, ok, time.Since(start).Round(100*time.Millisecond))
	return ok
}

// unlockDiag is unlock with a ladder of ways to bring up the bouncer; the first one that works is named in the log
// and the test goes on from there.
func (e *emu) unlockDiag(t *testing.T) {
	t.Helper()
	e.probeLockScreen(t, "before")
	w, h := e.screenSize(t)
	x, y0, y1 := swipeUpPoints(w, h)
	swipe := func() { e.shell(t, fmt.Sprintf("input swipe %d %d %d %d 300", x, y0, x, y1)) }
	steps := []struct {
		name string
		act  func()
	}{
		{"A swipe as unlock() does", func() { swipe(); time.Sleep(4 * time.Second); swipe() }},
		{"B long fast swipe from 90% to 20%", func() {
			e.shell(t, fmt.Sprintf("input swipe %d %d %d %d 150", x, h*9/10, x, h/5))
		}},
		{"C swipe with a notification on the lock screen", func() {
			e.shell(t, "cmd notification post -t DiagTitle diagtag 'diag text'")
			time.Sleep(2 * time.Second)
			e.probeLockScreen(t, "after posting")
			swipe()
		}},
		{"D KEYCODE_MENU", func() { e.shell(t, "input keyevent KEYCODE_MENU") }},
		{"E wm dismiss-keyguard", func() { e.shell(t, "wm dismiss-keyguard") }},
		{"F lock again, then swipe", func() { e.lock(t); swipe() }},
	}
	for _, s := range steps {
		if e.tryBouncer(t, s.name, s.act) {
			t.Logf("DIAG RESULT: the bouncer came with step %q", s.name)
			e.unlockOnBouncer(t)
			return
		}
	}
	e.probeLockScreen(t, "after")
	e.logShell(t, "logcat -d -b all -t 600 | grep -iE 'keyguard|bouncer|lockscreen|NotificationPanel|StatusBarKeyguard' | tail -80")
	t.Fatalf("DIAG RESULT: no step brought up the bouncer; nodes %v", slices.Compact(nodeIDs(e.screen(t))))
}

func nodeIDs(nodes []uiNode) []string {
	var ids []string
	for _, n := range nodes {
		if n.ID != "" {
			ids = append(ids, n.ID)
		}
	}
	return ids
}
