package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// noticeFailed is what showNotice says when the app answers that it cannot put
// its notice up. It is the block message; under YOLO the read goes on, so the
// line the agent gets says liftedNoticeFailed instead (liftNotice).
const noticeFailed = "ADBuddy can't show the on-screen notice, so it won't read the screen; ask the user to do this part"

// liftedNoticeFailed is the YOLO line for noticeFailed: the read goes ahead.
const liftedNoticeFailed = "the on-screen notice could not be shown"

// liftNotice is lift for a showNotice refusal: whether YOLO lets the read go
// on. The line it tells the agent must not say the read is refused.
func (d *Daemon) liftNotice(ctx context.Context, why string) bool {
	if why == noticeFailed {
		why = liftedNoticeFailed
	}
	return d.lift(ctx, why)
}

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
	return noticeFailed
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

// screenshotVeilMs is how long the app keeps its notice off the screen for a
// screenshot at most (it puts it back by itself when the time is up). The
// capture takes 0.2 to 0.9 s on the phones measured.
const screenshotVeilMs = 2000

// restoreWait bounds the call that puts the notice back after a capture.
const restoreWait = 3 * time.Second

// veilNotice asks the app to take its on-screen notice off the screen for the
// screencap that follows, so the frame and the pill do not end up in the
// image. It returns what to call once the capture is done (it puts the
// notice back at once; the app would anyway when screenshotVeilMs is up) and
// whether the notice may be in the image all the same. A pause the app
// reports is a failure: a paused session reads nothing. Not being able to
// take the notice off never refuses the capture: the person saw it go up
// before (showNotice), and the agent is told the image may carry it.
func (d *Daemon) veilNotice(ctx context.Context) (restore func(), inImage bool, f *uiFailure) {
	raw, _ := json.Marshal(bridge.UIHideNoticeArgs{Ms: screenshotVeilMs})
	_, err := d.callApp(ctx, bridge.UIOpHideNotice, raw)
	var ue *UIError
	switch {
	case err == nil:
		return d.restoreNotice, false, nil
	case errors.Is(err, ErrNoApp), errors.As(err, &ue) && ue.Code == bridge.CodeUnavailable:
		return func() {}, false, nil // no app: no notice on the screen
	case errors.As(err, &ue) && ue.Code == bridge.CodeUnsupported:
		return func() {}, true, nil // an older app cannot take it off
	case errors.As(err, &ue) && ue.Code == bridge.CodeProtected: // the only reason hideNotice gives it is a pause
		return nil, false, fail(ExitDenied, "UI_BLOCKED", "Blocked by ADBuddy: "+PausedMessage)
	}
	// Failed, expired, no answer in time: it may or may not be off the screen. Put it back.
	return d.restoreNotice, true, nil
}

// restoreNotice puts the app's notice back after a capture. Best effort: the
// app does it by itself when the time it was given is up, and not for a
// paused session.
func (d *Daemon) restoreNotice() {
	ctx, cancel := context.WithTimeout(context.Background(), restoreWait)
	defer cancel()
	_, _ = d.callApp(ctx, bridge.UIOpNotice, nil)
}
