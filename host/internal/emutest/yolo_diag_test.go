package emutest

import (
	"fmt"
	"testing"
	"time"
)

// TEMPORARY (audit/adbuddy-yolo, ADB-03): how often each way of bringing up the bouncer works, and whether the PIN
// field has the focus. Reverted before the PR is ready.

const pinEntryID = "com.android.systemui:id/pinEntry"

var diagRound int

// diagPin returns the PIN field's text and focus ("-" and false when there is none).
func diagPin(nodes []uiNode) (text string, focused, found bool) {
	for _, n := range nodes {
		if n.ID == pinEntryID {
			return n.Text, n.Focused, true
		}
	}
	return "-", false, false
}

// diagTry runs act, waits 6 s for the bouncer and, when it shows, types the PIN the way unlockOnBouncer does and
// logs one RESULT line. It leaves the device on a fresh lock screen.
func (e *emu) diagTry(t *testing.T, label string, act func()) bool {
	t.Helper()
	nodes := e.screen(t)
	_, _, pb := diagPin(nodes)
	e.logShell(t, "dumpsys window | grep -E 'mCurrentFocus|isKeyguardShowing|mDreamingLockscreen|mShowingDream' | head -8")
	start := time.Now()
	act()
	var seen time.Duration
	shown := poll(6*time.Second, func() bool {
		nodes = e.screen(t)
		_, _, found := diagPin(nodes)
		seen = time.Since(start)
		return found
	})
	if !shown {
		t.Logf("DIAG RESULT round=%d %s: no bouncer in 6 s (bouncer before: %v)", diagRound, label, pb)
		e.logShell(t, "logcat -d -b all -t 600 | grep -E 'Bouncer|sysui_lockscreen_gesture|KeyguardViewMediator|Dream' | grep -v Accessibility | tail -25")
		return false
	}
	_, focus1, _ := diagPin(nodes)
	time.Sleep(2 * time.Second)
	_, focus2, _ := diagPin(e.screen(t))
	e.shell(t, "input text "+testPIN)
	typed := poll(3*time.Second, func() bool { txt, _, _ := diagPin(e.screen(t)); return txt != "" && txt != "-" })
	tapFixed := false
	if !typed {
		// Does a tap on the field give it the focus and let the PIN in?
		for _, n := range e.screen(t) {
			if n.ID == pinEntryID {
				e.tap(t, n)
			}
		}
		time.Sleep(time.Second)
		_, focus3, _ := diagPin(e.screen(t))
		e.shell(t, "input text "+testPIN)
		tapFixed = poll(3*time.Second, func() bool { txt, _, _ := diagPin(e.screen(t)); return txt != "" && txt != "-" })
		t.Logf("DIAG tap on the field: focused after the tap=%v, PIN typed after it=%v", focus3, tapFixed)
	}
	e.shell(t, "input keyevent KEYCODE_ENTER")
	unlocked := poll(10*time.Second, func() bool { return !e.locked(t) })
	t.Logf("DIAG RESULT round=%d %s: bouncer after %s, focused at first sight=%v and 2 s later=%v, typed=%v, typed after a tap=%v, unlocked=%v",
		diagRound, label, seen.Round(100*time.Millisecond), focus1, focus2, typed, tapFixed, unlocked)
	if !unlocked {
		e.logShell(t, "logcat -d -b all -t 600 | grep -E 'Bouncer|sysui_lockscreen_gesture|KeyguardViewMediator|Dream' | grep -v Accessibility | tail -25")
	}
	e.lock(t)
	return true
}

func TestDiagUnlockPaths(t *testing.T) {
	diagRound++
	l := openLink(t)
	e := l.e
	e.lockWithPIN(t)
	w, h := e.screenSize(t)
	swipe := fmt.Sprintf("input swipe %d %d %d %d 150", w/2, h*9/10, w/2, h/5) // new
	if diagRound%2 == 1 {
		swipe = fmt.Sprintf("input swipe %d %d %d %d 300", w/2, h*3/4, w/2, h/3) // old
	}
	t.Logf("DIAG round %d swipe %q", diagRound, swipe)

	// The TestYolo situation: the lock screen as lockWithPIN leaves it, swipes as unlock() repeats them.
	tries := 0
	for tries < 3 {
		tries++
		if e.diagTry(t, fmt.Sprintf("A fresh lock screen, swipe %d", tries), func() { e.shell(t, swipe) }) {
			break
		}
	}
	if tries == 3 {
		e.diagTry(t, "A+ dismiss-keyguard after the swipes failed", func() { e.shell(t, "wm dismiss-keyguard") })
	}
	// What the notification tests do: the system asks to dismiss the keyguard.
	e.lock(t)
	e.diagTry(t, "B dismiss-keyguard on a lock screen from lock()", func() { e.shell(t, "wm dismiss-keyguard") })
	// A swipe on a lock screen from lock(), not from lockWithPIN.
	e.lock(t)
	e.diagTry(t, "C swipe on a lock screen from lock()", func() { e.shell(t, swipe) })
}
