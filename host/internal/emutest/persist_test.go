package emutest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
)

// persistPort is the port the test leaves in persist.adb.tcp.port: 5555, the
// emulator's own transport port (M2.5b S5), so a persist the cleanup failed to
// clear never moves the emulator's adbd to an unmeasured port.
const persistPort = remote.Port

// withPersist sets persist.adb.tcp.port (su 0: the shell user may be denied)
// and makes sure the cleanup empties it again and restarts adbd, so the
// emulator ends without a persist port nor an adbd that took one. Cleanups run
// last first: this one runs before closedAtStartAndEnd's adb usb.
func (e *emu) withPersist(t *testing.T) func() string {
	t.Helper()
	persist := func() string { return e.shell(t, "su 0 getprop persist.adb.tcp.port") }
	if p := persist(); p != "" {
		t.Fatalf("persist.adb.tcp.port = %q before the test: left over; clear it with `adb shell su 0 setprop persist.adb.tcp.port ''`", p)
	}
	t.Cleanup(func() {
		c := *e
		c.ctx = context.WithoutCancel(e.ctx)
		if c.state() != "device" {
			t.Errorf("the emulator is %q at the cleanup: persist.adb.tcp.port may be left at %s; %s", c.state(), persistPort, restartHint)
			return
		}
		ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
		defer cancel()
		_, _, _, _ = c.adb.Output(ctx, "shell", "su 0 setprop persist.adb.tcp.port ''")
		pid, _, _ := c.adbdNow()
		_, _, _, _ = c.adb.Output(ctx, "shell", "su 0 setprop ctl.restart adbd")
		if !poll(60*time.Second, func() bool {
			now, _, ok := c.adbdNow()
			return ok && now != pid && c.tryShell("su 0 getprop persist.adb.tcp.port") == ""
		}) {
			t.Errorf("persist.adb.tcp.port not empty, or adbd not back, after the cleanup; %s", restartHint)
		}
	})
	e.shell(t, "su 0 setprop persist.adb.tcp.port "+persistPort)
	if p := persist(); p != persistPort {
		t.Fatalf("persist.adb.tcp.port = %q after su 0 setprop, want %s", p, persistPort)
	}
	return persist
}

// closedAfterOff is what this test asks of the host's "off" with a persist
// port set: persist.adb.tcp.port emptied, service.adb.tcp.port closed (empty
// or 0: the values measured after adb usb and at boot, and the host's own
// criterion, portClosed; -1 is not measured and the host would take it as
// open, the safe side) and adbd restarted (a new pid that stays: it cannot
// still be listening on the persist port it took when it started).
func (e *emu) closedAfterOff(t *testing.T, persist func() string, before string) {
	t.Helper()
	if p := persist(); p != "" {
		t.Errorf("persist.adb.tcp.port = %q after off, want it emptied: adbd takes it again at its next start with service.adb.tcp.port empty (a reboot)", p)
	}
	pid, port, ok := e.adbdNow()
	if !ok {
		t.Fatalf("could not read adbd after off (emulator %q)", e.state())
	}
	if !portClosed(port) {
		t.Errorf("service.adb.tcp.port = %q after off, want empty or 0", port)
	}
	if pid == before {
		t.Errorf("adbd still has pid %s after off: it was not restarted, and may still listen on the persist port it took when it started", pid)
		return
	}
	e.sameAdbd(t, pid, port)
}

// Security of VB-41 (supervisor's ruling): "off" must leave adbd without a
// TCP port, persist.adb.tcp.port included. The host's two "off" paths, the
// real remote controller's Disable (mode off) and device.EnsureNoTCPPort (the
// invariant at serve's start), run against the emulator's adbd with
// persist.adb.tcp.port=5555. RED EXPECTED TODAY: remote.go and device.go read
// only service.adb.tcp.port and never touch persist, so persist stays 5555
// (and with service.adb.tcp.port empty they do not even restart adbd). A red
// run is the finding: the fix goes in its own branch and plan; this test is
// its acceptance test.
func TestOffClosesAPersistTCPPort(t *testing.T) {
	adbdTests(t)
	e := newEmu(t)
	e.closedAtStartAndEnd(t)

	// persist set and the port opened by hand (adb tcpip): Disable with the
	// mode off sees 5555 and runs adb usb, which leaves persist as it was.
	t.Run("Disable after adb tcpip", func(t *testing.T) {
		persist := e.withPersist(t)
		pid, _ := e.adbd(t)
		e.run(t, "tcpip", remote.Port)
		pid = e.restartedTo(t, pid, remote.Port)
		ctl := remote.New(e.adb, &memStore{}, nil)
		if s, err := ctl.Disable(e.ctx); err != nil || s != (remote.State{}) {
			t.Fatalf("Disable: %+v, %v", s, err)
		}
		poll(10*time.Second, func() bool { now, _, ok := e.adbdNow(); return ok && now != pid })
		e.closedAfterOff(t, persist, pid)
	})

	// persist set and service.adb.tcp.port empty, as after a reboot: adbd took
	// the persist port at its start (AOSP adbd reads persist only when the
	// service property is empty), and the host reads "closed".
	t.Run("EnsureNoTCPPort with service.adb.tcp.port empty", func(t *testing.T) {
		persist := e.withPersist(t)
		if os.Getenv("ADBUDDY_EMULATOR_REBOOT") == "1" {
			e.rebootAndWait(t)
		} else {
			// Without a reboot: the service property emptied and adbd
			// restarted by init. Not measured yet: if the property does not
			// read empty, this case needs ADBUDDY_EMULATOR_REBOOT=1.
			pid, _ := e.adbd(t)
			_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop service.adb.tcp.port ''")
			_, _, _, _ = e.adb.Output(e.ctx, "shell", "su 0 setprop ctl.restart adbd")
			e.restartedTo(t, pid, "", "0")
		}
		pid, port := e.adbd(t)
		if port != "" {
			t.Skipf("service.adb.tcp.port = %q, not empty: this case needs ADBUDDY_EMULATOR_REBOOT=1", port)
		}
		if p := persist(); p != persistPort {
			t.Fatalf("persist.adb.tcp.port = %q before off, want %s", p, persistPort)
		}
		restarted, err := device.EnsureNoTCPPort(e.ctx, e.adb, "")
		if err != nil {
			t.Fatalf("EnsureNoTCPPort: %v", err)
		}
		t.Logf("EnsureNoTCPPort ran adb usb: %v", restarted)
		poll(10*time.Second, func() bool { now, _, ok := e.adbdNow(); return ok && now != pid })
		e.closedAfterOff(t, persist, pid)
	})
}
