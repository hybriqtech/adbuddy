package cli

import (
	"context"
	"io"
	"slices"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/device"
)

// Waiting times of the host invariant (variables so tests can shorten them).
var (
	usbDropWait   = 5 * time.Second        // how long adbd may take to drop after `adb usb`
	invariantPoll = 250 * time.Millisecond // how often `adb devices` is asked meanwhile
)

// applyInvariant applies the host invariant (spec §3.3) to runner's phone:
// no adb network port while remote mode is off. If `adb usb` had to close
// it, adbd restarts: this waits for the old connection to drop (at most
// usbDropWait; the adb server may keep listing it for a moment), then for
// the phone to come back (at most wait), and checks that the port is really
// closed, except keep (remote mode's port, "" when it is off). command is
// what the user runs again after a failure. When ctx ends, the error is whatever the step returned: callers turn it into
// Interrupted. A persist.adb.tcp.port that names a port (VB-41) is said once
// in log, from the last read: ADBuddy cannot clear it, and adbd listens on it
// again when the phone restarts.
func applyInvariant(ctx context.Context, runner adb.Runner, wait time.Duration, log io.Writer, command, keep string) error {
	_, err := invariantTCP(ctx, runner, wait, log, command, keep)
	return err
}

// invariantTCP is applyInvariant that also returns the last read of the port:
// serve gives it to the remote controller (Note) and to the watcher of the
// connection, so both know what persist.adb.tcp.port holds from the start.
func invariantTCP(ctx context.Context, runner adb.Runner, wait time.Duration, log io.Writer, command, keep string) (device.TCP, error) {
	t, restarted, err := device.EnsureNoTCPPort(ctx, runner, keep)
	if err != nil {
		return t, NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix", err.Error())
	}
	if !restarted {
		tellPersist(log, t.Persist, false)
		return t, nil
	}
	tell(log, "invariant.closed_adb_s_network_port")
	if err := waitDropped(ctx, runner, usbDropWait); err != nil {
		return t, err
	}
	if _, err := runner.WaitSerial(ctx, runner.Serial, wait); err != nil {
		if ctx.Err() != nil {
			return t, ctx.Err()
		}
		return t, TransientErrorT("DEVICE_NOT_FOUND", "err.invariant.device_not_found", "err.invariant.device_not_found.fix", command)
	}
	// Only read the port now: another `adb usb` would restart adbd again.
	t, err = device.ReadTCP(ctx, runner)
	if err != nil {
		if ctx.Err() != nil {
			return t, ctx.Err()
		}
		return t, NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.invariant.device_unreadable.fix_2", err.Error())
	}
	if t.Now != "" && t.Now != keep {
		// Restarting the phone closes a port of `adb tcpip`, but opens
		// persist's again: that fix only without persist.
		if t.Persist != "" {
			return t, NewErrorT("DEVICE_PORT_OPEN", "err.invariant.device_port_open", "err.invariant.device_port_open.persist_fix", command, t.Persist)
		}
		return t, NewErrorT("DEVICE_PORT_OPEN", "err.invariant.device_port_open", "err.invariant.device_port_open.fix", command)
	}
	tellPersist(log, t.Persist, false)
	return t, nil
}

// tellPersist says in log that persist.adb.tcp.port names a port (VB-41:
// ADBuddy did not set it, cannot clear it, and adbd listens on it again when
// the phone restarts), or that it could not be read (unknown). Nothing when
// neither.
func tellPersist(log io.Writer, persist string, unknown bool) {
	switch {
	case persist != "":
		tell(log, "remote.persist.log", persist)
	case unknown:
		tell(log, "remote.persist.unknown_log")
	}
}

// waitDropped waits until runner's phone is no longer in state `device`, or
// until timeout: if the old connection never shows as gone, the wait for the
// phone goes on anyway. It only fails when ctx ends.
func waitDropped(ctx context.Context, runner adb.Runner, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		// A hanging `adb devices` is cut at the deadline too.
		octx, cancel := context.WithDeadline(ctx, deadline)
		online, err := runner.Online(octx)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil && !hasSerial(online, runner.Serial) {
			return nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil
		}
		t := time.NewTimer(min(invariantPoll, left))
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func hasSerial(ds []adb.Device, serial string) bool {
	return slices.ContainsFunc(ds, func(d adb.Device) bool { return d.Serial == serial })
}
