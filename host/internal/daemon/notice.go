package daemon

import (
	"context"
	"errors"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// showNotice asks the app to put up its on-screen notice before the host reads
// the screen by adb (screenshot, raw screencap/screenrecord/uiautomator,
// dumpsys that dumps the view). Allowlist: only these let the read go on.
// Anything else (failed, protected because of a pause, invalid, expired, a
// channel error) refuses, as tap and type do.
func (d *Daemon) showNotice(ctx context.Context) string {
	_, err := d.callApp(ctx, bridge.UIOpNotice, nil)
	var ue *UIError
	switch {
	case err == nil:
		return ""
	case errors.As(err, &ue) && ue.Code == bridge.CodeUnsupported:
		return "" // an older app that answers: it cannot show the notice, but it is not stuck
	case errors.Is(err, ErrNoApp), errors.As(err, &ue) && ue.Code == bridge.CodeUnavailable:
		// No app, or accessibility off: nobody to tell... unless the
		// accessibility service is still bound (the app is stuck, not absent),
		// or it cannot be checked (fail closed, as uiautomator() does).
		on, serr := d.serviceOn(ctx)
		switch {
		case serr != nil:
			return "could not check ADBuddy's accessibility service, so ADBuddy cannot tell whether it can show the on-screen notice; ask the user to do this part"
		case on:
			return "the ADBuddy app does not answer but its accessibility service is on, so it cannot show the on-screen notice; ask the user to open the ADBuddy app"
		}
		return ""
	case errors.Is(err, ErrUITimeout): // 10 s: the app hung; the overlay answers in 2 s at most
		return "the ADBuddy app did not answer in time, so ADBuddy cannot show the on-screen notice; ask the user to open the ADBuddy app"
	}
	return "ADBuddy can't show the on-screen notice, so it won't read the screen; ask the user to do this part"
}

// readsScreen reports whether any command of res reads the screen.
func readsScreen(res classify.Result) bool {
	for _, f := range res.Findings {
		if _, reads := screenAction(f); reads {
			return true
		}
	}
	return false
}
