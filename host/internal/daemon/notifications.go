package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// adbuddy notifications (M5 spec §3.5). Only the app can read them (its
// notification listener), and there is no adb fallback: `dumpsys notification
// --noredact` prints the text of every app's notifications, protected apps
// included. The app already empties the title and text of ADBuddy and of the
// protected apps; the host does it again with its own list, so a stale list
// or a wrong app never lets a 2FA code out through this door.

// hiddenNotificationKey stands in for the key of a notification whose
// content is hidden: a key carries the app's own tag, which can hold text.
const hiddenNotificationKey = "hidden"

// noNotificationAccess is what the agent is told when the app cannot read the
// notifications: no access, or allowed in the system but not confirmed in the
// app. The host's own text, so it covers both of the app's fixed texts.
const noNotificationAccess = "ADBuddy cannot read the notifications yet: the user has not allowed or confirmed it. " +
	"Ask them to allow ADBuddy in the ADBuddy app (Settings, Notification access; if it says it is allowed in the system, " +
	"they have to confirm it there, you cannot do it for them) or in the phone's Settings, Notifications, Device & app notifications. " +
	"If it is already allowed, turning it off and on again fixes it."

// notifications asks the app for the active notifications.
func (d *Daemon) notifications(ctx context.Context, raw json.RawMessage) (any, string, *uiFailure) {
	res, err := d.callApp(ctx, bridge.UIOpNotifications, raw)
	if err != nil {
		switch {
		case errors.Is(err, ErrNoApp):
			return nil, "", fail(exitTransient, "UI_UNAVAILABLE", "reading notifications needs the ADBuddy app on the phone, connected to this PC: ask the user to open it")
		case errors.Is(err, ErrUITimeout):
			return nil, "", fail(exitTransient, "UI_TIMEOUT", "the ADBuddy app did not answer in time: try again")
		}
		// The app's error text may repeat a notification, and the decoder's
		// error quotes what it could not parse: nothing of it goes through.
		// Not even the access message: the host says it with its own text.
		if ctx.Err() != nil {
			return nil, "", fail(1, "UI_FAILED", "stopped")
		}
		var ue *UIError
		if errors.As(err, &ue) && ue.Code == bridge.CodeUnavailable {
			return nil, "", fail(exitTransient, "UI_UNAVAILABLE", noNotificationAccess)
		}
		return nil, "", fail(1, "UI_FAILED", "the ADBuddy app could not read the notifications")
	}
	v, err := checkResult(bridge.UIOpNotifications, res)
	if err != nil {
		return nil, "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
	}
	n, ok := v.(*bridge.UINotifications)
	if !ok {
		return nil, "", fail(1, "UI_FAILED", "the ADBuddy app answered something unreadable")
	}
	d.cleanNotifications(n)
	return n, "the app", nil
}

// cleanNotifications enforces on the app's answer what the protocol promises:
// at most MaxNotifications, text cut to MaxNotificationText, and the content
// (key, title and text) of ADBuddy, of a protected app, and of a package that
// is not one, emptied.
func (d *Daemon) cleanNotifications(n *bridge.UINotifications) {
	if len(n.Notifications) > bridge.MaxNotifications {
		n.Notifications = n.Notifications[:bridge.MaxNotifications]
		n.Truncated = true
	}
	if n.Notifications == nil {
		n.Notifications = []bridge.UINotification{}
	}
	for i := range n.Notifications {
		x := &n.Notifications[i]
		if !bridge.ValidPackage(x.Package) {
			x.Package = ""
		}
		if x.Package == "" || d.protected(x.Package) {
			x.Key, x.Title, x.Text = hiddenNotificationKey+"-"+strconv.Itoa(i), "", ""
			continue
		}
		x.Title, x.Text = cutRunes(x.Title, bridge.MaxNotificationText), cutRunes(x.Text, bridge.MaxNotificationText)
	}
}

// cutRunes cuts s to n code points.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
