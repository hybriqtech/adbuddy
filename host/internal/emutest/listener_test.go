package emutest

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// T6 of the manual validations (M5c-22, 23, 24, 26, 28 in the app's part, 29
// and 32): ADBuddy's notification listener and the user's confirmation on the
// emulator. The host allows and disallows the listener with `cmd
// notification` (T1-1: it binds and unbinds in under 2 s), kills the app's
// process, reboots, and taps ADBuddy's own confirm button with uiautomator;
// the app's half (listenerGateTest, listenerContentTest) reads, in the app's
// process, what `adbuddy notifications` would get. Every new binding of the
// listener clears the user's mark (M5c-32 b): whether a step cleared it is
// read from the app's file (markOnDisk) or over the real channel, never by
// the app's half, whose am instrument binds the listener anew.

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
	accessNeedsTap    = "Android allows it; your confirmation is missing"
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
// instrumentation arguments, and returns what it read. The system binds the
// listener again in that fresh process and the new binding clears the
// user's mark (onListenerConnected): its `mark` is false whatever happened
// before, so it never proves that something else cleared it (markOnDisk and
// the real channel do); with -e confirm 1 the test confirms in its own process.
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

// markTrueRe is the user's mark set, as SharedPreferences writes it in
// shared_prefs/notification_consent.xml (NotificationConsent's KEY).
var markTrueRe = regexp.MustCompile(`<boolean\s+name="confirmed"\s+value="true"\s*/>`)

// markInPrefs reports whether the preferences file xml holds the user's mark
// set. No file, or no key, is no mark.
func markInPrefs(xml string) bool { return markTrueRe.MatchString(xml) }

func TestMarkInPrefs(t *testing.T) {
	for _, c := range []struct {
		xml  string
		want bool
	}{
		{"<?xml version='1.0' encoding='utf-8' standalone='yes' ?>\n<map>\n    <boolean name=\"confirmed\" value=\"true\" />\n</map>\n", true},
		{"<?xml version='1.0' encoding='utf-8' standalone='yes' ?>\n<map>\n    <boolean name=\"confirmed\" value=\"false\" />\n</map>\n", false},
		{"<map><boolean name=\"other\" value=\"true\" /></map>", false},
		{"", false},
	} {
		if got := markInPrefs(c.xml); got != c.want {
			t.Errorf("markInPrefs(%q) = %v, want %v", c.xml, got, c.want)
		}
	}
}

// markOnDisk reports the user's mark as the app's own file holds it (run-as:
// the debug build allows it), WITHOUT starting or restarting the app's
// process. That is the way to see whether something cleared the mark: every
// gateRead restarts the process (am instrument), the system binds the
// listener again inside it and that binding clears the mark
// (onListenerConnected), whatever happened before. revoke() writes with
// commit(), and confirm()'s apply() is on disk once confirmOnScreen returns.
func (e *emu) markOnDisk(t *testing.T) bool {
	t.Helper()
	return markInPrefs(e.shell(t, "run-as "+phoneapp.Package+" cat shared_prefs/notification_consent.xml 2>/dev/null; true"))
}

// waitMarkCleared waits up to 10 s for the app's file to lose the user's mark
// (the listener's callback runs a moment after the system binds or unbinds
// it), and fails the step if it does not.
func (e *emu) waitMarkCleared(t *testing.T, step string) {
	t.Helper()
	if !poll(10*time.Second, func() bool { return !e.markOnDisk(t) }) {
		t.Errorf("%s: the user's mark is still set in the app's file 10 s later (no new tap was made)", step)
	}
}

// M5c-32: revoking the access clears the user's mark even when nobody is
// looking (no screen open, no `adbuddy notifications`), and allowed again it
// takes a new tap. (a) With the app alive: the listener is unbound in a live
// process and clears the mark itself (onListenerDisconnected, onDestroy).
// (b) With the app dead: the process is gone when the access is revoked, and
// the restart the system had scheduled binds the listener again without
// unbinding it (measured on API 34): the new binding clears the mark
// (onListenerConnected). The mark is read from the app's file (markOnDisk)
// and over the real channel, never by a gateRead, which restarts the process.
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
		// The live listener's own unbinding cleared it (onListenerDisconnected), in the file.
		e.waitMarkCleared(t, "revoked with the app alive")
		// And read in a fresh process with the access off: no listener, so no
		// new binding clears it there, and `mark` is read before the gate's own
		// onRevoked does.
		g := e.gateRead(t)
		wantGate(t, "revoked with the app alive", g, map[string]string{
			"listed": "NO", "connected": "false", "state": "NOT_GRANTED",
			"mark": "false", "effective": "OFF", "gate": "NoAccess",
		})
		e.allowListener(t, true)
		// The system's state allowed again (the mark here proves nothing: a gateRead's binding clears it anyway).
		g = e.gateRead(t)
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
		// With the real channel: the mark is checked by `notifications`, which
		// does not restart the app's process.
		l := openLink(t)
		e := l.e
		e.listenerSetup(t)
		e.allowListener(t, true)
		e.confirmOnScreen(t)
		if !e.markOnDisk(t) {
			t.Fatal("the user's mark is not in the app's file after the tap: the case is not tested")
		}
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
		// What the system does next (measured on API 34, 2026-10-03): the
		// restart it had scheduled still runs, and it binds the listener it has
		// just disallowed, without ever calling onListenerDisconnected. Before
		// the fix the mark survived that and, allowed again (the agent can, with
		// `adbuddy tap` in Settings), the gate read without a new tap (M5c-32 b).
		// The new binding now clears it (onListenerConnected).
		time.Sleep(3 * time.Second)
		t.Logf("3 s after the revocation: process %q, listener bound %v, mark in the file %v", e.pidNow(), e.listenerBound(t), e.markOnDisk(t))
		e.allowListener(t, true)
		// No gateRead before these checks: it restarts the process and its own
		// binding would clear the mark whatever happened here.
		e.waitMarkCleared(t, "revoked with the app dead and allowed again")
		wantUnavailable(t, "revoked with the app dead and allowed again", l.notifications(t, "b_1"), notConfirmedText)
		g := e.gateRead(t)
		wantGate(t, "revoked with the app dead and allowed again", g, map[string]string{
			"listed": "YES", "connected": "true", "state": "GRANTED",
			"mark": "false", "effective": "NEEDS_CONFIRMATION", "gate": "NotConfirmed",
		})
	})
}

// notifications asks the app for the notifications over the real channel, as
// `adbuddy notifications` does (UiExecutor and the gate of UiHelpers.install).
// After am instrument, which restarts the app's process and so drops the
// channel, it hands the app the token again first, as serve does.
func (l *link) notifications(t *testing.T, id string) bridge.UIResponse {
	t.Helper()
	if !l.connected() {
		n := len(l.Hellos())
		l.handshake(t)
		l.waitHellos(t, n+1, 40*time.Second)
	}
	return l.uiRequest(t, id, bridge.UIOpNotifications, bridge.Empty{})
}

// restarted waits for the channel to drop after am instrument restarted the
// app's process.
func (l *link) restarted(t *testing.T) {
	t.Helper()
	if !poll(15*time.Second, func() bool { return !l.connected() }) {
		t.Fatal("the channel is still up 15 s after am instrument restarted the app")
	}
}

// wantUnavailable fails unless r is `unavailable` with a text that contains why.
func wantUnavailable(t *testing.T, step string, r bridge.UIResponse, why string) {
	t.Helper()
	if r.OK || r.Error == nil || r.Error.Code != bridge.CodeUnavailable || !strings.Contains(r.Error.Text, why) {
		t.Errorf("%s: notifications answered %+v (error %+v), want unavailable with %q", step, r, r.Error, why)
	}
}

// M5c-22, M5c-23 and M5c-28 (the gate's states): with the access not allowed,
// allowed without the user's tap, confirmed, allowed but not bound, revoked
// and allowed again, what the system says (the setting and its own answer),
// what the app reads in a fresh process (listenerGateTest), what its screen
// shows and what `notifications` answers over the real channel.
func TestTheListenerGateStates(t *testing.T) {
	l := openLink(t)
	e := l.e
	e.listenerSetup(t)

	// Not allowed: the system does not list it, the app says so.
	if e.listenerListed(t) {
		t.Fatal("a fresh install is listed in enabled_notification_listeners")
	}
	wantUnavailable(t, "not allowed", l.notifications(t, "n_1"), notAllowedText)
	g := e.gateRead(t)
	wantGate(t, "not allowed", g, map[string]string{
		"api": "false", "listed": "NO", "connected": "false", "state": "NOT_GRANTED",
		"mark": "false", "effective": "OFF", "gate": "NoAccess",
	})
	if s := g["setting"]; s != "NO" && s != "null" && s != "empty" {
		t.Errorf("not allowed: the app reads the setting as %q, want NO, null or empty", s)
	}
	l.restarted(t)
	e.openAccessStep(t)
	if nodes := e.waitAccessState(t, accessNotAllowed); hasConfirm(nodes) {
		t.Errorf("not allowed, yet the screen offers the confirm button: %+v", nodes)
	}

	// Allowed in the system: the listener binds although it is not exported
	// (the system binds it), and still nothing is read without the user's tap.
	e.allowListener(t, true)
	wantUnavailable(t, "allowed, not confirmed", l.notifications(t, "n_2"), notConfirmedText)
	g = e.gateRead(t)
	wantGate(t, "allowed, not confirmed", g, map[string]string{
		"setting": "YES", "api": "true", "listed": "YES", "connected": "true", "state": "GRANTED",
		"mark": "false", "effective": "NEEDS_CONFIRMATION", "gate": "NotConfirmed",
	})
	l.restarted(t)

	// The user's tap: the gate reads.
	e.confirmOnScreen(t)
	r := l.notifications(t, "n_3")
	if !r.OK {
		t.Errorf("confirmed: notifications answered %+v (error %+v), want the list", r, r.Error)
	} else {
		list := payload[bridge.UINotifications](t, bridge.Message{Type: bridge.TypeUIResponse, Payload: r.Result})
		t.Logf("confirmed: notifications gives %d (truncated %v)", len(list.Notifications), list.Truncated)
	}

	// Allowed but not bound (the listener asks the system to unbind it; the
	// access stays allowed): Unknown, and the read says no at once instead of
	// waiting. Unbinding clears the mark (onListenerDisconnected), so bound
	// again it needs a new tap. am instrument restarts the process and the
	// system's new binding there clears the tap above (onListenerConnected,
	// M5c-32 b): the instrumented test confirms in its own process once the
	// listener is bound (-e confirm 1), as the button would, and reads.
	g = e.gateRead(t, "-e confirm 1", "-e unbind 1")
	wantGate(t, "a fresh process", g, map[string]string{"fresh.connected": "true", "fresh.mark": "false"})
	wantGate(t, "confirmed", g, map[string]string{
		"listed": "YES", "connected": "true", "state": "GRANTED", "mark": "true", "effective": "ON",
	})
	if !strings.HasPrefix(g["gate"], "Ok:") {
		t.Errorf("confirmed: the gate reads %q, want Ok", g["gate"])
	}
	wantGate(t, "allowed but not bound", g, map[string]string{
		"unbound.disconnected": "true", "unbound.setting": "YES", "unbound.api": "true", "unbound.listed": "YES",
		"unbound.connected": "false", "unbound.state": "UNKNOWN", "unbound.effective": "UNKNOWN",
		"unbound.read": "null", "unbound.mark": "false", "unbound.gate": "NotConfirmed",
		"rebound.listed": "YES", "rebound.connected": "true", "rebound.state": "GRANTED",
		"rebound.mark": "false", "rebound.effective": "NEEDS_CONFIRMATION", "rebound.gate": "NotConfirmed",
	})
	if ms, err := strconv.Atoi(g["unbound.readMs"]); err != nil || ms >= 1000 {
		t.Errorf("allowed but not bound: read() took %q ms, want under 1000 (it must not hang)", g["unbound.readMs"])
	}
	l.restarted(t)

	// Confirmed again, then revoked: not allowed, and allowed again it asks again.
	e.confirmOnScreen(t)
	if !e.markOnDisk(t) {
		t.Fatal("the user's mark is not in the app's file after the tap: the revocation is not tested")
	}
	e.allowListener(t, false)
	// The listener's unbinding cleared it before anyone asked (the gate's own onRevoked comes with n_4).
	e.waitMarkCleared(t, "revoked")
	wantUnavailable(t, "revoked", l.notifications(t, "n_4"), notAllowedText)
	g = e.gateRead(t)
	wantGate(t, "revoked", g, map[string]string{
		"api": "false", "listed": "NO", "state": "NOT_GRANTED", "mark": "false", "effective": "OFF", "gate": "NoAccess",
	})
	l.restarted(t)
	e.allowListener(t, true)
	wantUnavailable(t, "allowed again", l.notifications(t, "n_5"), notConfirmedText)
	e.openAccessStep(t)
	if nodes := e.waitAccessState(t, accessNeedsTap); !hasConfirm(nodes) {
		t.Errorf("allowed again, the screen does not offer the confirm button: %+v", nodes)
	}

	// After a force-stop the setting still lists the listener.
	e.shell(t, "input keyevent KEYCODE_HOME")
	e.shell(t, "am force-stop "+phoneapp.Package)
	if !e.listenerListed(t) {
		t.Error("after a force-stop, the listener is no longer in enabled_notification_listeners")
	}
}

// fillerCommand posts the host's five fillers of another app
// (com.android.shell), one by one under the system's rate of 5 a second per app.
const fillerCommand = `for i in 1 2 3 4 5; do cmd notification post -t '` + fillerTitle + `' t6fill$i "filler $i" >/dev/null; sleep 0.3; done`

// M5c-26: what the listener reads of the app's own notifications and what
// the report makes of it (listenerContentTest), with five fillers of another
// app posted by the host (`cmd notification post`). The fillers go with a
// force-stop of com.android.shell, which cancels its notifications.
func TestTheListenerContent(t *testing.T) {
	e := listenerEmu(t)
	e.allowListener(t, true)
	e.atCleanup(t, "shell", "am force-stop com.android.shell")
	e.shell(t, fillerCommand)
	ctx, cancel := context.WithTimeout(e.ctx, instrumentTimeout)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell", "am instrument -w -r -e class "+listenerContentTest+" -e adbuddyListener content "+instrumentRunner)
	out = strings.ReplaceAll(out, "\r", "")
	if err != nil || code != 0 {
		t.Fatalf("am instrument: exit %d, %v: %s\n%s", code, err, strings.TrimSpace(errOut), out)
	}
	if tests, _, _, failed := parseInstrument(out); failed || tests != 1 {
		t.Fatalf("%s did not pass (%d tests ran):\n%s", listenerContentTest, tests, out)
	}
	if list := e.shell(t, "cmd notification list"); strings.Contains(list, "|"+phoneapp.Package+"|74") {
		t.Errorf("notifications of the test are still posted after it:\n%s", list)
	}
	// am force-stop returns before the notification service has cancelled the
	// package's notifications: it does so when it handles the package-restart
	// broadcast, on its own handler (measured on the emulator: 0.05-1 s, and
	// longer on a loaded runner). So the fillers are waited for, not read once.
	e.shell(t, "am force-stop com.android.shell")
	var list string
	if !poll(30*time.Second, func() bool {
		list = e.shell(t, "cmd notification list")
		return !strings.Contains(list, "t6fill")
	}) {
		t.Errorf("the host's fillers are still posted 30 s after force-stop of com.android.shell:\n%s", list)
	}
}

// M5c-29: the listener stays bound with the app in the background: after
// `su 0 kill -9` (am kill does not kill it, T1-9), after am force-stop and,
// with ADBUDDY_EMULATOR_REBOOT=1, after adb reboot (the AVD keeps its data),
// the system binds it again without opening the app. The user's mark does NOT
// survive any of them: each new binding clears it (onListenerConnected, the
// close of M5c-32 b), and the agent cannot read until the person taps again;
// one tap after the rebinding is enough, and no late binding takes it away.
// Before each step the person confirms; after it the mark is read from the
// app's file (markOnDisk), never by a gateRead, which restarts the process.
func TestTheListenerStaysBound(t *testing.T) {
	l := openLink(t)
	e := l.e
	e.listenerSetup(t)
	e.allowListener(t, true)

	confirmed := func(step string) {
		t.Helper()
		e.confirmOnScreen(t)
		if !e.markOnDisk(t) {
			t.Fatalf("before %s: the user's mark is not in the app's file after the tap", step)
		}
	}
	rebound := func(step, before string) {
		t.Helper()
		start := time.Now()
		var pid string
		if !poll(20*time.Second, func() bool {
			pid = e.pidNow()
			return pid != "" && pid != before && e.listenerBound(t)
		}) {
			t.Fatalf("after %s: the listener is not bound again in a new process in 20 s (pid %q, was %s)", step, pid, before)
		}
		t.Logf("after %s: bound again in process %s within %.1f s", step, pid, time.Since(start).Seconds())
		e.waitMarkCleared(t, "after "+step)
	}
	confirmed("kill -9")
	pid := e.pidOf(t)
	e.shell(t, "su 0 kill -9 "+pid)
	rebound("kill -9", pid)

	confirmed("am force-stop")
	pid = e.pidOf(t)
	e.shell(t, "am force-stop "+phoneapp.Package)
	rebound("am force-stop", pid)

	if os.Getenv("ADBUDDY_EMULATOR_REBOOT") == "1" {
		confirmed("adb reboot")
		e.rebootAndWait(t)
		// The first bind after the boot may fail and the next one binds (T1-9).
		if !poll(60*time.Second, func() bool { return e.listenerBound(t) }) {
			t.Fatal("after adb reboot: the listener is not bound 60 s after the boot")
		}
		t.Logf("after adb reboot: bound again (process %s)", e.pidNow())
		e.waitMarkCleared(t, "after adb reboot")
		// The reboot took the reverse of the channel's port with it.
		if err := phoneapp.Tunnel(e.ctx, e.adb, l.port); err != nil {
			t.Fatalf("adb reverse after the reboot: %v", err)
		}
	} else {
		t.Log("the reboot step needs ADBUDDY_EMULATOR_REBOOT=1")
	}

	// Rebound, the agent gets `unavailable` until the person taps again; one tap is enough.
	wantUnavailable(t, "after the kills, without a new tap", l.notifications(t, "s_1"), notConfirmedText)
	confirmed("the last read")
	r := l.notifications(t, "s_2")
	if !r.OK {
		t.Errorf("after the kills and a new tap: notifications answered %+v (error %+v), want the list", r, r.Error)
	}
	// No late binding took the tap away.
	time.Sleep(5 * time.Second)
	if !e.markOnDisk(t) {
		t.Error("after the kills and a new tap: the mark went again 5 s later without a revocation")
	}
}

// resumedActivity returns the resumed activity of `dumpsys activity
// activities` (its component), or "".
func resumedActivity(dump string) string {
	m := resumedRe.FindStringSubmatch(dump)
	if m == nil {
		return ""
	}
	return m[1]
}

var resumedRe = regexp.MustCompile(`(?m)(?:topResumedActivity|mResumedActivity)[=:].*? (\S+/\S+)[ }]`)

func TestResumedActivity(t *testing.T) {
	dump := "  ResumedActivity: ActivityRecord{1 u0 x/.Y t1}\n" +
		"    topResumedActivity=ActivityRecord{a1b2 u0 com.android.settings/.Settings$NotificationAccessDetailsActivity t42}\n"
	if got := resumedActivity(dump); got != "com.android.settings/.Settings$NotificationAccessDetailsActivity" {
		t.Errorf("resumedActivity = %q", got)
	}
	if got := resumedActivity("nothing"); got != "" {
		t.Errorf("resumedActivity of nothing = %q", got)
	}
}

// accessSwitch returns the system's "Allow notification access" switch on
// screen, if any.
func accessSwitch(nodes []uiNode) (uiNode, bool) {
	for _, n := range nodes {
		if n.ID == "android:id/switch_widget" {
			return n, true
		}
	}
	return uiNode{}, false
}

// M5c-24, on API 34: the screen's shortcut ("Open system settings",
// NotificationAccessStatus.openSystemSettings) opens ADBuddy's own entry of
// the system's notification access, not the list, with its switch usable (an
// adb install is not restricted on API 34, T1-6). Then, once, whether
// `appops set … ACCESS_RESTRICTED_SETTINGS ignore` greys it out as a
// restricted setting does (measured: it does). The app's hint for that case
// is on its screen while the access is not allowed.
func TestTheShortcutOpensADBuddysEntry(t *testing.T) {
	e := listenerEmu(t)
	e.atCleanup(t, "shell", "am force-stop com.android.settings")
	open := func() []uiNode {
		t.Helper()
		e.openAccessStep(t)
		// Not allowed: the screen says what to do with a greyed-out switch.
		e.waitNode(t, "the restricted settings hint", 15*time.Second, func(n uiNode) bool { return strings.HasPrefix(n.Text, accessRestrictedHint) })
		e.tap(t, e.waitNode(t, accessOpenSystem, 15*time.Second, func(n uiNode) bool { return n.Text == accessOpenSystem }))
		var nodes []uiNode
		if !poll(15*time.Second, func() bool {
			nodes = e.screen(t)
			_, ok := accessSwitch(nodes)
			return ok
		}) {
			t.Fatalf("no notification access switch on screen 15 s after %q: %+v", accessOpenSystem, nodes)
		}
		return nodes
	}
	nodes := open()
	act := resumedActivity(e.shell(t, "dumpsys activity activities"))
	t.Logf("the shortcut opens %s", act)
	if !strings.HasPrefix(act, "com.android.settings/") || !strings.Contains(act, "Detail") {
		t.Errorf("the shortcut opens %q, want Settings' detail of one listener", act)
	}
	var adbuddy, others bool
	for _, n := range nodes {
		adbuddy = adbuddy || n.Text == "ADBuddy"
		// The list shows a row per app that has a listener: the launcher's is one.
		others = others || strings.Contains(n.Text, "Launcher") || strings.Contains(n.Text, "Pixel")
	}
	if !adbuddy || others {
		t.Errorf("the shortcut's screen names ADBuddy %v and other apps %v, want only ADBuddy's entry: %+v", adbuddy, others, nodes)
	}
	if sw, _ := accessSwitch(nodes); !sw.Enabled {
		t.Errorf("ADBuddy's switch is greyed out after an adb install on API 34 (T1-6 measured it enabled): %+v", sw)
	}

	// The restricted case, simulated once.
	e.atCleanup(t, "shell", "appops set "+phoneapp.Package+" ACCESS_RESTRICTED_SETTINGS default")
	e.shell(t, "appops set "+phoneapp.Package+" ACCESS_RESTRICTED_SETTINGS ignore")
	e.shell(t, "am force-stop com.android.settings")
	nodes = open()
	// Measured on 2026-10-03: greyed out, as a restricted setting is.
	if sw, _ := accessSwitch(nodes); sw.Enabled {
		t.Errorf("with ACCESS_RESTRICTED_SETTINGS ignore, ADBuddy's switch is still enabled (measured greyed out): %+v", sw)
	}
}
