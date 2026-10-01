package cli

import (
	"context"
	"fmt"
	"io"
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
// closed, except keep (remote mode's port, "" when it is off). command is what the user runs again after a failure. When ctx
// ends, the error is whatever the step returned: callers turn it into
// Interrupted.
func applyInvariant(ctx context.Context, runner adb.Runner, wait time.Duration, log io.Writer, command, keep string) error {
	restarted, err := device.EnsureNoTCPPort(ctx, runner, keep)
	if err != nil {
		return NewError("DEVICE_UNREADABLE", err.Error(), "Check that the phone is unlocked and still connected.")
	}
	if !restarted {
		return nil
	}
	fmt.Fprintln(log, "Closed adb's network port on the phone (adb usb); waiting for it to reconnect…")
	if err := waitDropped(ctx, runner, usbDropWait); err != nil {
		return err
	}
	if _, err := runner.WaitSerial(ctx, runner.Serial, wait); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return TransientError("DEVICE_NOT_FOUND", "the phone did not come back after `adb usb`",
			"Check that wireless debugging is still on, then run `"+command+"` again.")
	}
	// Only read the port now: another `adb usb` would restart adbd again.
	port, err := device.TCPPort(ctx, runner)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return NewError("DEVICE_UNREADABLE", err.Error(), "Check that the phone is unlocked and still connected.")
	}
	if port != "" && port != keep {
		return NewError("DEVICE_PORT_OPEN", "adb's network port on the phone is still open after `adb usb`",
			"Restart the phone (that closes a port opened with `adb tcpip`), turn wireless debugging on again and run `"+command+"` again.")
	}
	return nil
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
	for _, d := range ds {
		if d.Serial == serial {
			return true
		}
	}
	return false
}
