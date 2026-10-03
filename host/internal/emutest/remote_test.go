package emutest

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)

// tsIP is a Tailscale address (100.64.0.0/10) for the remote controller. The
// emulator is never reached through it: Enable only refuses to run without one
// (M5b, ErrNoTailscale).
const tsIP = "100.64.0.5"

// memStore is the controller's config.toml, in memory.
type memStore struct {
	mu    sync.Mutex
	until time.Time
}

func (s *memStore) TailscaleIP() string { return tsIP }

func (s *memStore) Until() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.until
}

func (s *memStore) SetUntil(t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.until = t
	return nil
}

// fakeClock is the controller's now: the 12 h pass without waiting for them.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// tryShell runs command on the emulator and returns its output, or "" when
// adb fails (the emulator away while adbd restarts).
func (e *emu) tryShell(command string) string {
	ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
	defer cancel()
	out, _, code, err := e.adb.Output(ctx, "shell", command)
	if err != nil || code != 0 {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
}

// adbdNow returns adbd's pid and service.adb.tcp.port, read by one shell
// command, so an empty port is never a failed read; ok is false when adb
// failed. A new pid is a restarted adbd: that is how these tests see whether
// `adb tcpip` or `adb usb` ran, both of which restart it (the emulator is away
// for about 2 s, which sampling `adb devices` every second can miss).
func (e *emu) adbdNow() (pid, port string, ok bool) {
	out := e.tryShell(`echo "$(pidof adbd)|$(getprop service.adb.tcp.port)"`)
	pid, port, ok = strings.Cut(out, "|")
	return pid, port, ok && pidRe.MatchString(pid)
}

// adbd returns adbd's pid and service.adb.tcp.port now, or fails.
func (e *emu) adbd(t *testing.T) (pid, port string) {
	t.Helper()
	pid, port, ok := e.adbdNow()
	if !ok {
		t.Fatalf("could not read adbd's pid and port (emulator %q)", e.state())
	}
	return pid, port
}

// restartHint is what to do when the emulator does not come back after a
// change of adbd's port: service.adb.tcp.port does not survive a cold boot.
const restartHint = "if the emulator stays away, stop it with scripts/emulator.sh stop and start it again (same ADBUDDY_EMU_* variables): service.adb.tcp.port does not survive a cold boot"

// restartedTo waits up to 60 s for adbd to run with a pid other than before
// and a service.adb.tcp.port in want, and returns the new pid.
func (e *emu) restartedTo(t *testing.T, before string, want ...string) string {
	t.Helper()
	var pid, port string
	if !poll(60*time.Second, func() bool {
		var ok bool
		pid, port, ok = e.adbdNow()
		return ok && pid != before && slices.Contains(want, port)
	}) {
		t.Fatalf("60 s on, adbd has pid %q (before %s) and service.adb.tcp.port %q, want a new pid and one of %q (emulator %q); %s",
			pid, before, port, want, e.state(), restartHint)
	}
	return pid
}

// sameFor is how long sameAdbd watches adbd: adb usb answers before adbd
// restarts (1-2 s later), so one read right after an order could still see
// the old pid.
const sameFor = 5 * time.Second

// sameAdbd checks, every half second for sameFor, that adbd keeps pid and
// port and answers: nothing restarted it.
func (e *emu) sameAdbd(t *testing.T, pid, port string) {
	t.Helper()
	deadline := time.Now().Add(sameFor)
	for {
		now, p, ok := e.adbdNow()
		if !ok || now != pid || p != port {
			t.Errorf("adbd has pid %q and service.adb.tcp.port %q (read ok %v), want the same %s and %q for %v: something restarted it", now, p, ok, pid, port, sameFor)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// adbdTests skips the tests that restart the emulator's adbd (adb tcpip, adb
// usb, ctl.restart, persist.adb.tcp.port) unless ADBUDDY_EMULATOR_ADBD=1, as
// TestCutAccessAndReactivate asks for ADBUDDY_EMULATOR_CUT: the CI keeps its
// AVD between jobs, and a port other than 5555 or a persist property left
// behind would outlive the job. They run on a development emulator only.
func adbdTests(t *testing.T) {
	t.Helper()
	if os.Getenv("ADBUDDY_EMULATOR_ADBD") != "1" {
		t.Skip("set ADBUDDY_EMULATOR_ADBD=1 to run it, on a development emulator only (it restarts adbd)")
	}
}

// portClosed says whether service.adb.tcp.port reads as no network port, as
// remote and device.EnsureNoTCPPort take it (empty or 0).
func portClosed(p string) bool { return p == "" || p == "0" }

// closedAtStartAndEnd requires adbd without a TCP port before the test and
// closes it at cleanup if the test left it open (adb usb). adb usb leaves
// service.adb.tcp.port at "0" where it could have been "" before (both mean
// closed, portClosed).
func (e *emu) closedAtStartAndEnd(t *testing.T) {
	t.Helper()
	if _, p := e.adbd(t); !portClosed(p) {
		t.Fatalf("service.adb.tcp.port = %q before the test: a tcpip mode left over; close it with `adb usb`", p)
	}
	t.Cleanup(func() {
		c := *e
		c.ctx = context.WithoutCancel(e.ctx)
		pid, p, ok := c.adbdNow()
		if !ok {
			t.Errorf("could not read adbd's pid and port at the cleanup (emulator %q): the port may be left open; %s", c.state(), restartHint)
			return
		}
		if portClosed(p) {
			return
		}
		ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
		defer cancel()
		_, _, _, _ = c.adb.Output(ctx, "usb")
		if !poll(60*time.Second, func() bool {
			now, p, ok := c.adbdNow()
			return ok && now != pid && portClosed(p)
		}) {
			t.Errorf("service.adb.tcp.port still open after the cleanup's adb usb; %s", restartHint)
		}
	})
}

// Validation 12 of M5b, what the emulator can show of it: the remote
// controller of serve, against the emulator's real adbd. Enable saves the
// expiry and opens port 5555 (adb tcpip); Reconcile with the expiry ahead
// keeps it and runs nothing; Disable closes it (adb usb) and clears the
// expiry; with the 12 h passed the mode is Closing, never off, and Expire
// closes it; a stale expiry with the port closed is cleared without adb usb,
// and one that passed with the port open is closed by Reconcile. On the
// emulator adbd always listens on 5555 (its own transport, M2.5b S5): what
// counts is service.adb.tcp.port, never the socket. Tailscale with mobile
// data, and serve reconnecting through it, stay on the Pixel.
func TestTheRemoteModeOpensAndClosesThePort(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)
	st, clk := &memStore{}, &fakeClock{t: time.Now().UTC()}
	ctl := remote.New(e.adb, st, clk.now)
	want := func(what string, got, want remote.State, err error) {
		t.Helper()
		if err != nil || got != want {
			t.Fatalf("%s: state %+v, %v; want %+v", what, got, err, want)
		}
	}
	on := func() remote.State {
		return remote.State{Enabled: true, Address: tsIP + ":" + remote.Port, ExpiresAt: clk.now().Add(remote.Lifetime)}
	}

	pid, _ := e.adbd(t)
	s, err := ctl.Enable(e.ctx)
	want("Enable", s, on(), err)
	pid = e.restartedTo(t, pid, remote.Port)
	if !st.Until().Equal(clk.now().Add(remote.Lifetime)) {
		t.Errorf("saved expiry %v, want now + %v", st.Until(), remote.Lifetime)
	}
	if s := e.state(); s != "device" {
		t.Errorf("the emulator is %q after adb tcpip: it keeps its serial (M2.5b S5)", s)
	}

	// serve starts again with the expiry ahead: kept, and adbd left alone.
	s, err = ctl.Reconcile(e.ctx)
	want("Reconcile with the expiry ahead", s, on(), err)
	e.sameAdbd(t, pid, remote.Port)

	s, err = ctl.Disable(e.ctx)
	want("Disable", s, remote.State{}, err)
	pid = e.restartedTo(t, pid, "0", "")
	if !st.Until().IsZero() {
		t.Errorf("the expiry %v is still saved after Disable", st.Until())
	}

	// The 12 h pass: Closing (never off) until Expire closes the port.
	s, err = ctl.Enable(e.ctx)
	want("Enable again", s, on(), err)
	pid = e.restartedTo(t, pid, remote.Port)
	clk.add(remote.Lifetime + time.Second)
	if s := ctl.State(); !s.Enabled || !s.Closing {
		t.Errorf("past the expiry with the port open: %+v, want Enabled and Closing", s)
	}
	s, changed, err := ctl.Expire(e.ctx)
	if err != nil || !changed || s != (remote.State{}) {
		t.Fatalf("Expire: %+v, changed %v, %v", s, changed, err)
	}
	pid = e.restartedTo(t, pid, "0", "")

	// A stale expiry (the phone restarted and the port closed meanwhile):
	// Reconcile clears it without adb usb.
	_ = st.SetUntil(clk.now().Add(time.Hour))
	_, port := e.adbd(t)
	s, err = ctl.Reconcile(e.ctx)
	want("Reconcile with the port closed", s, remote.State{}, err)
	e.sameAdbd(t, pid, port)
	if !st.Until().IsZero() {
		t.Errorf("the stale expiry %v is still saved", st.Until())
	}

	// An expiry that passed while serve was stopped, with the port open:
	// Reconcile closes it now.
	s, err = ctl.Enable(e.ctx)
	want("Enable a third time", s, on(), err)
	pid = e.restartedTo(t, pid, remote.Port)
	clk.add(remote.Lifetime + time.Second)
	s, err = ctl.Reconcile(e.ctx)
	want("Reconcile past the expiry", s, remote.State{}, err)
	e.restartedTo(t, pid, "0", "")
}

// Validation 37 of the v1 deferred B (VB-37): "off" with the remote mode off
// does not restart adbd when the port reads closed (on a phone that would cut
// its connection for nothing), and closes a port opened by hand, 5555 or any
// other (7777), with adb usb. adbd's pid tells a restart (adbdNow).
func TestOffWithTheModeOffOnlyClosesAnOpenPort(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)
	ctl := remote.New(e.adb, &memStore{}, nil)
	t.Run("closed", func(t *testing.T) {
		pid, port := e.adbd(t)
		if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
			t.Fatalf("Disable: %+v, %v", s, err)
		}
		e.sameAdbd(t, pid, port)
	})
	// 7777 last: the emulator's transport with a port other than 5555 has not
	// been measured (restartHint if it does not come back).
	for _, p := range []string{remote.Port, "7777"} {
		t.Run("open by hand on "+p, func(t *testing.T) {
			pid, _ := e.adbd(t)
			e.run(t, "tcpip", p)
			pid = e.restartedTo(t, pid, p)
			if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
				t.Fatalf("Disable: %+v, %v", s, err)
			}
			e.restartedTo(t, pid, "0")
		})
	}
}

// hostReadsPersistPort says whether remote and device.EnsureNoTCPPort read
// persist.adb.tcp.port. They do not (remote.go and device.go read only
// service.adb.tcp.port; pendientes, diferidos B, T2). Hypothesis the reboot
// step measures: after a reboot with persist set, service.adb.tcp.port reads
// empty while adbd listens on the persist port (AOSP adbd reads persist only
// when the service property is empty), and the host reads "closed". If it is
// measured so, it is a finding for the controller; with a fix, set this to
// true and the reboot step then wants Disable to close the port.
const hostReadsPersistPort = false

// rebootAndWait reboots the emulator and waits up to 3 min for it to boot
// (with -no-snapshot the AVD keeps its data: T1-9).
func (e *emu) rebootAndWait(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
	defer cancel()
	_, _, _, _ = e.adb.Output(ctx, "reboot")
	e.waitState(func(s string) bool { return s != "device" }, 30*time.Second)
	if !poll(3*time.Minute, func() bool {
		return e.state() == "device" && e.tryShell("getprop sys.boot_completed") == "1"
	}) {
		t.Fatalf("the emulator did not boot 3 min after adb reboot (state %q)", e.state())
	}
}

// Validation 41 of the v1 deferred B (VB-41): persist.adb.tcp.port against
// adb usb and adbd's restarts, measured on the emulator's adbd. The emulator
// always listens on 5555, so which port adbd really opens is not visible here:
// the test records the properties and what the host does with them. The
// reboot step runs only with ADBUDDY_EMULATOR_REBOOT=1 (a minute or two more).
func TestPersistTCPPortAgainstUSBAndRestarts(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)
	persist := func() string { return e.shell(t, "su 0 getprop persist.adb.tcp.port") }
	if p := persist(); p != "" {
		t.Fatalf("persist.adb.tcp.port = %q before the test: left over; clear it with `adb shell su 0 setprop persist.adb.tcp.port ''`", p)
	}
	// Cleanups run last first: the order is the persist clear, the adbd restart
	// (only if the reboot step ran: adbd listens on the persist port until it
	// restarts), then closedAtStartAndEnd's adb usb.
	rebooted := false
	t.Cleanup(func() {
		if !rebooted {
			return
		}
		c := *e
		c.ctx = context.WithoutCancel(e.ctx)
		if c.state() != "device" {
			return
		}
		ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
		defer cancel()
		_, _, _, _ = c.adb.Output(ctx, "shell", "su 0 setprop ctl.restart adbd")
	})
	e.atCleanup(t, "shell", "su 0 setprop persist.adb.tcp.port ''")

	// Can the shell user set it without su? Measured and logged, not
	// required (the classifier makes `setprop persist.adb…` 🔴 anyway). It is
	// emptied again at once, so the measurement below starts from nothing.
	out, _, code, _ := e.adb.Output(e.ctx, "shell", "setprop persist.adb.tcp.port 5555")
	t.Logf("setprop persist.adb.tcp.port 5555 as shell: exit %d %s; persist now %q", code, strings.TrimSpace(out), persist())
	e.shell(t, "su 0 setprop persist.adb.tcp.port ''")
	if p := persist(); p != "" {
		t.Fatalf("persist.adb.tcp.port = %q after emptying it", p)
	}

	// If the shell attempt above restarted adbd, wait until it is stable
	// before taking the pid that restartedBySetprop compares with.
	pid, port := e.adbd(t)
	e.sameAdbd(t, pid, port)
	e.shell(t, "su 0 setprop persist.adb.tcp.port 5555")
	restartedBySetprop := poll(10*time.Second, func() bool {
		now, _, ok := e.adbdNow()
		return ok && now != pid
	})
	t.Logf("setting persist.adb.tcp.port restarted adbd: %v", restartedBySetprop)
	pid, after := e.adbd(t)
	if after != port {
		t.Errorf("service.adb.tcp.port went from %q to %q when only persist.adb.tcp.port was set", port, after)
	}

	// adbd restarted by init, not by adb: the service property stays.
	_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
	pid = e.restartedTo(t, pid, port)

	// adb tcpip and adb usb: usb sets the service property to 0.
	e.run(t, "tcpip", remote.Port)
	pid = e.restartedTo(t, pid, remote.Port)
	e.run(t, "usb")
	pid = e.restartedTo(t, pid, "0")
	t.Logf("persist.adb.tcp.port after adb usb: %q", persist())

	// With the service property at 0 a restart keeps it at 0 (AOSP adbd then
	// ignores persist), and "off" with the mode off leaves adbd alone.
	_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
	pid = e.restartedTo(t, pid, "0")
	ctl := remote.New(e.adb, &memStore{}, nil)
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
		t.Fatalf("Disable: %+v, %v", s, err)
	}
	e.sameAdbd(t, pid, "0")

	if os.Getenv("ADBUDDY_EMULATOR_REBOOT") != "1" {
		t.Log("the reboot step needs ADBUDDY_EMULATOR_REBOOT=1")
		return
	}
	// After a reboot the service property is empty and persist is still set:
	// adbd takes the persist port, and the host reads "closed".
	if p := persist(); p != remote.Port {
		t.Logf("persist.adb.tcp.port = %q before the reboot (adb usb cleared it?): the reboot step has nothing to show", p)
		return
	}
	rebooted = true
	e.rebootAndWait(t)
	pid, port = e.adbd(t)
	t.Logf("after the reboot: service.adb.tcp.port %q, persist.adb.tcp.port %q", port, persist())
	if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
		t.Fatalf("Disable after the reboot: %+v, %v", s, err)
	}
	switch {
	case !portClosed(port):
		// The service property reflects persist: the host sees the port and closes it.
		e.restartedTo(t, pid, "0")
	case hostReadsPersistPort:
		e.restartedTo(t, pid, "0")
	default:
		e.sameAdbd(t, pid, port)
		t.Logf("FINDING (VB-41): after a reboot with persist.adb.tcp.port=%s, service.adb.tcp.port reads %q and the host takes the port as closed; Disable ran no adb usb", remote.Port, port)
	}
}

// cardText returns what the remote card says among nodes: one of its
// texts in English (the harness sets the app to English), or "".
func cardText(nodes []uiNode) string {
	for _, n := range nodes {
		switch {
		case n.Text == "Off", n.Text == "On", n.Text == "Working…", n.Text == "Not connected to the PC",
			strings.HasPrefix(n.Text, "Unknown:"), strings.HasPrefix(n.Text, "Closing…"), strings.HasPrefix(n.Text, "Until "):
			return n.Text
		}
	}
	return ""
}

// Validation 13 of M5b, on the emulator, with the answer lost (M5b ruling
// 3): the remote card's switch sends remote.set; the harness, as serve, runs
// the real Enable (adb tcpip 5555 restarts adbd and drops the tunnel) and does
// not answer, as when the answer is lost with the tunnel. The app gets no
// reply and marks the state unknown (RemoteCard.kt:69); once it is back, for
// 5 s, the card must say Unknown, never the "Off" it kept through the
// disconnection; then the host's remote.state (serve republishes it on
// attach, M5b-20) turns it to "Until …". It logs the seconds from adb tcpip
// until the app is back: the emulator's, a lower bound of the Pixel's (adbd is
// back in ~2 s here and the harness reopens the tunnel at once, keeping the
// token; serve looks every 15 s and hands over a new one, and a phone comes
// back over Wi-Fi). "Never Off" as logic is RemoteUiTest's and
// BridgeStateTest's on the JVM; this is the integration with a real drop.
func TestTheCardSaysUnknownWhenTheAnswerIsLost(t *testing.T) {
	adbdTests(t)
	l := openLink(t)
	e := l.e
	e.closedAtStartAndEnd(t)
	ctl := remote.New(e.adb, &memStore{}, nil)
	// As serve on attach (M5b-20): the mode as the host has it, off.
	l.send(t, bridge.TypeRemoteState, bridge.RemoteState{})
	l.openChat(t)
	e.waitNode(t, "the card saying Off", 15*time.Second, func(n uiNode) bool { return n.Text == "Off" })
	e.tap(t, e.waitNode(t, "the card's switch", 5*time.Second, func(n uiNode) bool { return n.Class == "android.widget.Switch" }))
	e.tap(t, e.waitNode(t, "Turn on", 10*time.Second, func(n uiNode) bool { return n.Text == "Turn on" }))
	req := l.waitFor(t, bridge.TypeRemoteSet, 10*time.Second)
	if p := payload[bridge.RemoteSet](t, req); !p.Enabled {
		t.Fatalf("remote.set %s, want enabled", req.Payload)
	}

	start := time.Now()
	hellos := len(l.Hellos())
	pid, _ := e.adbd(t)
	s, err := ctl.Enable(e.ctx)
	if err != nil || !s.Enabled {
		t.Fatalf("Enable: %+v, %v", s, err)
	}
	e.restartedTo(t, pid, remote.Port)
	// adbd's restart takes the reverse with it: reopened as serve's tunnel
	// check does (phone.go), without its new token.
	if open, err := phoneapp.TunnelOpen(e.ctx, e.adb, l.port); err != nil || !open {
		t.Logf("the reverse did not survive adb tcpip (open %v, %v): reopened", open, err)
		if err := phoneapp.Tunnel(e.ctx, e.adb, l.port); err != nil {
			t.Fatalf("adb reverse: %v", err)
		}
	}
	l.waitHellos(t, hellos+1, 90*time.Second)
	t.Logf("M5b-13 on the emulator: the app was back %.1f s after adb tcpip", time.Since(start).Seconds())

	// Back, and no state from the host yet: unknown, never the old off.
	var seen []string
	note := func() string {
		text := cardText(e.screen(t))
		if text != "" && (len(seen) == 0 || seen[len(seen)-1] != text) {
			seen = append(seen, text)
		}
		return text
	}
	poll(5*time.Second, func() bool { note(); return false })
	if slices.Contains(seen, "Off") {
		t.Errorf("after the lost answer the card said Off: %q", seen)
	}
	if len(seen) == 0 || !strings.HasPrefix(seen[len(seen)-1], "Unknown:") {
		t.Errorf("after the lost answer the card said %q, want Unknown", seen)
	}

	// The host speaks: the card says Until …
	l.send(t, bridge.TypeRemoteState, bridge.RemoteState{Enabled: true, Address: s.Address, ExpiresAt: s.ExpiresAt})
	if !poll(20*time.Second, func() bool { return strings.HasPrefix(note(), "Until ") }) {
		t.Errorf("the card did not say Until … 20 s after remote.state; it said %q", seen)
	}
}
