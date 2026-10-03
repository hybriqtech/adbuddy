package emutest

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// T6 of the manual validations (M5c-22, 23, 24, 26, 28 in the app's part, 29
// and 32): ADBuddy's notification listener and the user's confirmation on the
// emulator. The host allows and disallows the listener with `cmd
// notification` (T1-1: it binds and unbinds in under 2 s), kills the app's
// process, reboots, and taps ADBuddy's own confirm button with uiautomator;
// the app's half (listenerGateTest, listenerContentTest) reads, in the app's
// process, what `adbuddy notifications` would get.

const (
	listenerComponent = phoneapp.Package + "/" + phoneapp.Package + ".listener.AgentNotificationListener"

	// listenerGateTest reads the system's state, the user's mark and the gate
	// (android/app/src/androidTest/…/NotificationListenerOnEmulatorTest.kt).
	listenerGateTest = phoneapp.Package + ".NotificationListenerOnEmulatorTest#gate"
	// listenerContentTest posts the app's own notifications and checks what
	// the listener and the report make of them.
	listenerContentTest = phoneapp.Package + ".NotificationListenerOnEmulatorTest#content"

	// fillerTitle is the title of the host's `cmd notification post` fillers
	// (FILLER_TITLE in the instrumented test).
	fillerTitle = "T6 filler"
)

// The texts of ADBuddy's notification access screen, in English (the tests
// set the app's own language): its state card (stateTitle in
// NotificationAccessScreen.kt) and its confirm button.
const (
	accessAllowed     = "Allowed"
	accessNeedsTap    = "Allowed in the system, not confirmed here"
	accessNotAllowed  = "Not allowed"
	accessUnknown     = "Unknown"
	accessConfirm     = "Let the agent read my notifications"
	accessOpenSystem  = "Open system settings"
	accessStepHeading = "Step 2 of 2"
	// accessRestrictedHint begins the hint for a greyed-out switch.
	accessRestrictedHint = "If the switch is greyed out"
)

// The texts of the app's `unavailable` answers to `notifications`
// (UiExecutor.kt): a beginning of each.
const (
	notAllowedText   = "the user has not allowed notification access to ADBuddy"
	notConfirmedText = "the user has to confirm it in the ADBuddy app"
)

// listenerEmu installs the app and its instrumented tests, lets the app post
// notifications and puts it in English, and disallows the listener at
// cleanup (before the uninstall, which runs last).
func listenerEmu(t *testing.T) *emu {
	t.Helper()
	e := newEmu(t)
	e.installApp(t)
	e.listenerSetup(t)
	return e
}

// listenerSetup is listenerEmu for an app already installed (openLink's).
func (e *emu) listenerSetup(t *testing.T) {
	t.Helper()
	e.installTests(t)
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")
	e.shell(t, "cmd locale set-app-locales "+phoneapp.Package+" --locales en")
	e.atCleanup(t, "shell", "cmd notification disallow_listener "+listenerComponent)
}

// allowListener allows (or disallows) ADBuddy's listener, as the system's
// switch does, and waits up to 10 s for the system to bind (or unbind) it and
// to write enabled_notification_listeners: the setting comes a moment after
// the binding (under 0.5 s, measured), and until then the app's two sources
// disagree (NotificationAccess.combine: unknown).
func (e *emu) allowListener(t *testing.T, allow bool) {
	t.Helper()
	cmd := "allow_listener"
	if !allow {
		cmd = "disallow_listener"
	}
	e.shell(t, "cmd notification "+cmd+" "+listenerComponent)
	e.waitBound(t, allow, 10*time.Second)
	if !poll(10*time.Second, func() bool { return e.listenerListed(t) == allow }) {
		t.Fatalf("enabled_notification_listeners lists ADBuddy's listener = %v 10 s after %s", !allow, cmd)
	}
}

// listenerListed reports whether enabled_notification_listeners lists
// ADBuddy's listener, as the shell reads it.
func (e *emu) listenerListed(t *testing.T) bool {
	t.Helper()
	return strings.Contains(e.shell(t, "settings get secure enabled_notification_listeners"), listenerComponent)
}

// liveListeners returns the components of the "Live notification listeners"
// section of `dumpsys notification`: the listeners the system has bound now.
func liveListeners(dump string) []string {
	var live []string
	in := false
	for _, line := range strings.Split(dump, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Live notification listeners") {
			in = true
			continue
		}
		if !in {
			continue
		}
		// The section's entries are indented "ComponentInfo{pkg/cls} (user N): …"
		// lines; the next section's heading ends it.
		m := componentInfoRe.FindStringSubmatch(trimmed)
		if m == nil {
			if trimmed != "" {
				break
			}
			continue
		}
		live = append(live, m[1])
	}
	return live
}

var componentInfoRe = regexp.MustCompile(`^ComponentInfo\{([^}]+)\}`)

func TestLiveListeners(t *testing.T) {
	dump := "    Allowed notification listeners:\n" +
		"      tech.hybriq.adbuddy/tech.hybriq.adbuddy.listener.AgentNotificationListener\n" +
		"    Live notification listeners (2):\n" +
		"      ComponentInfo{com.android.launcher3/com.android.launcher3.notification.NotificationListener} (user 0): android.service.notification.INotificationListener$Stub$Proxy@1 SYSTEM\n" +
		"      ComponentInfo{tech.hybriq.adbuddy/tech.hybriq.adbuddy.listener.AgentNotificationListener} (user 0): android.service.notification.INotificationListener$Stub$Proxy@2\n" +
		"    Snoozed notification listeners:\n" +
		"      ComponentInfo{com.example/com.example.Snoozed}\n"
	got := liveListeners(dump)
	if len(got) != 2 || got[1] != listenerComponent {
		t.Errorf("liveListeners = %q, want the launcher's and ADBuddy's (not the snoozed one)", got)
	}
	if got := liveListeners(""); len(got) != 0 {
		t.Errorf("liveListeners of nothing = %q", got)
	}
	if got := liveListeners("    Live notification listeners (0):\n    Snoozed notification listeners:\n      ComponentInfo{" + listenerComponent + "}\n"); len(got) != 0 {
		t.Errorf("no live listener, yet %q", got)
	}
}

// listenerBound reports whether the system has ADBuddy's listener bound now.
func (e *emu) listenerBound(t *testing.T) bool {
	t.Helper()
	for _, c := range liveListeners(e.shell(t, "dumpsys notification")) {
		if c == listenerComponent {
			return true
		}
	}
	return false
}

// waitBound waits up to d for the listener to be bound (or not), or fails.
func (e *emu) waitBound(t *testing.T, bound bool, d time.Duration) {
	t.Helper()
	if !poll(d, func() bool { return e.listenerBound(t) == bound }) {
		t.Fatalf("ADBuddy's listener bound = %v %d s after, want %v", !bound, int(d/time.Second), bound)
	}
}

var gateStatusRe = regexp.MustCompile(`(?m)^INSTRUMENTATION_STATUS: adbuddy\.(\S+?)=(.*)$`)

// parseGate reads the status lines listenerGateTest sends, `adbuddy.<name>=<value>`.
func parseGate(out string) map[string]string {
	g := map[string]string{}
	for _, m := range gateStatusRe.FindAllStringSubmatch(out, -1) {
		g[m[1]] = strings.TrimSpace(m[2])
	}
	return g
}

func TestParseGate(t *testing.T) {
	out := "INSTRUMENTATION_STATUS: adbuddy.gate=NotConfirmed\n" +
		"INSTRUMENTATION_STATUS: adbuddy.unbound.state=UNKNOWN\n" +
		"INSTRUMENTATION_STATUS: class=tech.hybriq.adbuddy.NotificationListenerOnEmulatorTest\n" +
		"INSTRUMENTATION_STATUS: adbuddy.setting=YES\n" +
		"INSTRUMENTATION_STATUS_CODE: 2\n"
	g := parseGate(out)
	if len(g) != 3 || g["gate"] != "NotConfirmed" || g["unbound.state"] != "UNKNOWN" || g["setting"] != "YES" {
		t.Errorf("parseGate = %v", g)
	}
}

// gateRead runs listenerGateTest (am instrument restarts the app's process,
// as T1 saw: what it reads is what a fresh process reads) with the extra
// instrumentation arguments, and returns what it read.
func (e *emu) gateRead(t *testing.T, extra ...string) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, instrumentTimeout)
	defer cancel()
	cmd := "am instrument -w -r -e class " + listenerGateTest + " -e adbuddyListener gate"
	for _, a := range extra {
		cmd += " " + a
	}
	out, errOut, code, err := e.adb.Output(ctx, "shell", cmd+" "+instrumentRunner)
	out = strings.ReplaceAll(out, "\r", "")
	if err != nil || code != 0 {
		t.Fatalf("am instrument: exit %d, %v: %s\n%s", code, err, strings.TrimSpace(errOut), out)
	}
	if tests, _, _, failed := parseInstrument(out); failed || tests != 1 {
		t.Fatalf("%s did not pass (%d tests ran):\n%s", listenerGateTest, tests, out)
	}
	g := parseGate(out)
	t.Logf("the app reads: %v", g)
	return g
}

// wantGate fails for each name of want whose value g does not have.
func wantGate(t *testing.T, step string, g, want map[string]string) {
	t.Helper()
	for k, v := range want {
		if g[k] != v {
			t.Errorf("%s: the app reads %s=%q, want %q (all: %v)", step, k, g[k], v, g)
		}
	}
}

// openAccessStep opens the app on its notification access screen: the first
// run's step 2 of 2 (the first step is skipped with "Not now"). It goes home
// at cleanup.
func (e *emu) openAccessStep(t *testing.T) {
	t.Helper()
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	e.shell(t, "am start -W -n "+phoneapp.Activity)
	var last []uiNode
	if !poll(30*time.Second, func() bool {
		last = e.screen(t)
		for _, n := range last {
			if n.Text == accessStepHeading {
				return true
			}
		}
		for _, n := range last {
			if n.Text == "Not now" {
				e.tap(t, n)
				return false
			}
		}
		return false
	}) {
		t.Fatalf("the notification access step is not on screen in 30 s: %+v", last)
	}
}

// accessState returns the state ADBuddy's access screen shows among nodes (its
// card's title), or "".
func accessState(nodes []uiNode) string {
	for _, n := range nodes {
		// The card merges its title and detail for a screen reader: the title may
		// come first in a longer text.
		for _, s := range []string{accessNeedsTap, accessNotAllowed, accessUnknown, accessAllowed} {
			if n.Text == s || strings.HasPrefix(n.Text, s+"\n") || strings.HasPrefix(n.Text, s+", ") {
				return s
			}
		}
	}
	return ""
}

func TestAccessState(t *testing.T) {
	for _, c := range []struct {
		texts []string
		want  string
	}{
		{[]string{"Notification access", accessNeedsTap, accessConfirm}, accessNeedsTap},
		{[]string{accessAllowed, "The agent can read your notifications."}, accessAllowed},
		{[]string{accessAllowed + ", The agent can read your notifications."}, accessAllowed},
		{[]string{"Checking…"}, ""},
		{[]string{accessNotAllowed}, accessNotAllowed},
		{[]string{"Allowed apps"}, ""},
	} {
		var nodes []uiNode
		for _, s := range c.texts {
			nodes = append(nodes, uiNode{Text: s})
		}
		if got := accessState(nodes); got != c.want {
			t.Errorf("accessState(%q) = %q, want %q", c.texts, got, c.want)
		}
	}
}

// waitAccessState waits up to 15 s for the access screen to show want, or fails.
func (e *emu) waitAccessState(t *testing.T, want string) []uiNode {
	t.Helper()
	var last []uiNode
	if !poll(15*time.Second, func() bool {
		last = e.screen(t)
		return accessState(last) == want
	}) {
		t.Fatalf("the access screen shows %q, want %q: %+v", accessState(last), want, last)
	}
	return last
}

// hasConfirm reports whether the confirm button is among nodes.
func hasConfirm(nodes []uiNode) bool {
	for _, n := range nodes {
		if n.Text == accessConfirm {
			return true
		}
	}
	return false
}

// confirmOnScreen opens ADBuddy's access screen, which must ask for the
// confirmation, taps "Let the agent read my notifications" as the user does
// (input tap: the agent's `adbuddy tap` cannot, ADBuddy is protected), waits
// for "Allowed" and goes home: from then on nobody looks (the screen stops
// observing once it is not started).
func (e *emu) confirmOnScreen(t *testing.T) {
	t.Helper()
	e.openAccessStep(t)
	e.waitAccessState(t, accessNeedsTap)
	e.tap(t, e.waitNode(t, "the confirm button", 10*time.Second, func(n uiNode) bool { return n.Text == accessConfirm }))
	e.waitAccessState(t, accessAllowed)
	e.shell(t, "input keyevent KEYCODE_HOME")
	// SharedPreferences.apply writes on its own thread: the activity's onStop waits for it.
	time.Sleep(time.Second)
}

// pidNow returns the app's process now, or "" when there is none.
func (e *emu) pidNow() string {
	pid := e.tryShell("pidof " + phoneapp.Package)
	if !pidRe.MatchString(pid) {
		return ""
	}
	return pid
}

// M5c-32: revoking the access clears the user's mark even when nobody is
// looking (no screen open, no `adbuddy notifications`). (a) With the app
// alive: the listener is unbound in a live process and clears the mark
// itself (onListenerDisconnected, onDestroy). (b) With the app dead: the
// process is gone when the access is revoked.
func TestRevokingClearsTheMark(t *testing.T) {
	t.Run("a_alive", func(t *testing.T) {
		e := listenerEmu(t)
		e.allowListener(t, true)
		e.confirmOnScreen(t)
		pid := e.pidOf(t)
		e.allowListener(t, false)
		if now := e.pidNow(); now != pid {
			t.Fatalf("the app's process went from %s to %q while the access was revoked: not the live app's case", pid, now)
		}
		e.allowListener(t, true)
		g := e.gateRead(t)
		wantGate(t, "revoked and allowed again with the app alive", g, map[string]string{
			"listed": "YES", "connected": "true", "state": "GRANTED",
			"mark": "false", "effective": "NEEDS_CONFIRMATION", "gate": "NotConfirmed",
		})
		// And the screen asks again.
		e.openAccessStep(t)
		if nodes := e.waitAccessState(t, accessNeedsTap); !hasConfirm(nodes) {
			t.Errorf("the access screen does not offer the confirm button again: %+v", nodes)
		}
	})

	t.Run("b_dead", func(t *testing.T) {
		e := listenerEmu(t)
		e.allowListener(t, true)
		e.confirmOnScreen(t)
		pid := e.pidOf(t)
		// The process must be dead when the access is revoked, and stay so. Not
		// with am force-stop: the system asks to bind the listener again within
		// 10 ms of it, and binds it even after a disallow_listener that comes
		// right behind (measured: "Disallowing notification listener" and then
		// "Start proc … for service AgentNotificationListener" and "service
		// connected"). After kill -9 it waits 1 s before restarting the service
		// ("Scheduling restart of crashed service … in 1000ms", T1-9): revoked
		// within that second, in the same shell, nothing restarts it.
		out := e.shell(t, "su 0 kill -9 "+pid+"; cmd notification disallow_listener "+listenerComponent+"; pidof "+phoneapp.Package+" || echo dead")
		if !strings.HasSuffix(out, "dead") {
			t.Fatalf("the app's process was back when the access was revoked (%q): not the dead app's case", out)
		}
		// What the system does next (measured, 2026-10-03): the restart it had
		// scheduled still runs, and it binds the listener it has just
		// disallowed, without ever calling onListenerDisconnected.
		time.Sleep(3 * time.Second)
		t.Logf("3 s after the revocation: process %q, listener bound %v", e.pidNow(), e.listenerBound(t))
		e.allowListener(t, true)
		g := e.gateRead(t)
		wantGate(t, "revoked with the app dead and allowed again", g, map[string]string{
			"listed": "YES", "connected": "true", "state": "GRANTED",
		})
		// TODO(adjudicar): FINDING (M5c-32 b, the risk ruling 15 accepted),
		// measured on API 34 on 2026-10-03: the mark survives a revocation with
		// the app dead, and allowed again the gate reads without a new tap. This
		// pins today's behaviour; the cheap fix (clear the mark in
		// onListenerConnected too: the system's late restart above calls it)
		// would turn it into mark=false, gate=NotConfirmed, at the cost of a tap
		// after every rebind (reboot, update, kill), and would also change what
		// TestTheListenerStaysBound expects.
		wantGate(t, "revoked with the app dead and allowed again (today's behaviour, the accepted risk)", g, map[string]string{
			"mark": "true", "effective": "ON",
		})
		if !strings.HasPrefix(g["gate"], "Ok:") {
			t.Errorf("revoked with the app dead and allowed again: the gate reads %q, today Ok (the accepted risk)", g["gate"])
		}
		t.Logf("FINDING (M5c-32 b, ruling 15's accepted risk): the mark survived a revocation with the app dead; the gate reads %s without a new tap", g["gate"])
	})
}
