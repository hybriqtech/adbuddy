package bridge

import (
	"regexp"
	"time"
)

// The operations of ui.request and the shape of their args and result (M5
// spec §3). UIRequest.Args and UIResponse.Result carry them as raw JSON;
// UIOps says which type each operation's are, and TestUIOps checks it, the
// limits and ValidPackage against protocol/fixtures/ui.json, which the app's
// tests read too.

// UI operations.
const (
	UIOpDump          = "dump"
	UIOpClick         = "click"
	UIOpSetText       = "setText"
	UIOpForeground    = "foreground"
	UIOpNotifications = "notifications"
	UIOpNotice        = "notice"
	UIOpHideNotice    = "hideNotice"
)

// UIOp makes new values of an operation's args and result.
type UIOp struct {
	Args   func() any
	Result func() any
}

// UIOps maps each operation to the types of its args and result.
var UIOps = map[string]UIOp{
	UIOpDump:          {func() any { return new(UIDumpArgs) }, func() any { return new(UIDump) }},
	UIOpClick:         {func() any { return new(UIClickArgs) }, func() any { return new(UIClicked) }},
	UIOpSetText:       {func() any { return new(UISetTextArgs) }, func() any { return new(UITextSet) }},
	UIOpForeground:    {func() any { return new(Empty) }, func() any { return new(UIForeground) }},
	UIOpNotifications: {func() any { return new(Empty) }, func() any { return new(UINotifications) }},
	UIOpNotice:        {func() any { return new(Empty) }, func() any { return new(Empty) }},
	UIOpHideNotice:    {func() any { return new(UIHideNoticeArgs) }, func() any { return new(Empty) }},
}

// Limits of the helpers (M5 spec §3, §4.3 and protocol spec §9). Text is
// counted in Unicode code points, the same in Go and Kotlin.
const (
	MaxUIResult         = 512 << 10 // bytes of a ui.response's result, as encoded JSON (UTF-8)
	MaxNotifications    = 50        // notifications in one result; more set Truncated
	MaxNotificationText = 1024      // code points of a notification's title or text
	MaxSetText          = 4096      // code points of setText's text
	MaxProtectedApps    = 1000      // packages in device.info's protectedApps
	MaxPackage          = 255       // characters of a package name
	MaxHideNoticeMs     = 5000      // milliseconds hideNotice may keep the app's notice off the screen
)

var packageRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

// ValidPackage reports whether s is an Android package name: at least two
// dot-separated parts of letters, digits and underscores, each starting with
// a letter, at most MaxPackage characters.
func ValidPackage(s string) bool {
	return len(s) <= MaxPackage && packageRE.MatchString(s)
}

// UIDumpArgs are dump's args. All keeps every visible node, not only the
// informative or usable ones.
type UIDumpArgs struct {
	All bool `json:"all,omitempty"`
}

// UIClickArgs are click's args: exactly one of ID, Text, or X and Y.
type UIClickArgs struct {
	ID   string `json:"id,omitempty"`
	Text string `json:"text,omitempty"`
	X    *int   `json:"x,omitempty"`
	Y    *int   `json:"y,omitempty"`
}

// UISetTextArgs are setText's args: the text that replaces the focused
// field's, at most MaxSetText code points.
type UISetTextArgs struct {
	Text string `json:"text"`
}

// UIHideNoticeArgs are hideNotice's args: how long, from 1 to MaxHideNoticeMs
// milliseconds, the app keeps its on-screen notice (frame and pill) off the
// screen so that a capture by adb does not carry it. The app puts it back by
// itself when the time is up, unless the agent was paused meanwhile.
type UIHideNoticeArgs struct {
	Ms int `json:"ms"`
}

// UIDump is dump's result: the screen as the agent may see it. Elements is
// empty while the phone is locked. Package is empty when unknown.
type UIDump struct {
	Locked    bool        `json:"locked"`
	Package   string      `json:"package"`
	Activity  string      `json:"activity"`
	Width     int         `json:"width"`
	Height    int         `json:"height"`
	Rotation  int         `json:"rotation"`
	Truncated bool        `json:"truncated"`
	Elements  []UIElement `json:"elements"`
}

// UIElement is one element of a dump. Bounds are left, top, right, bottom
// in screen pixels, clipped to what is on screen (always four: check before
// use, Go fills missing ones with 0). Text and Hint are empty for a password
// field. Checked is absent when it cannot be checked.
type UIElement struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Desc     string `json:"desc"`
	Hint     string `json:"hint"`
	Class    string `json:"class"`
	Bounds   [4]int `json:"bounds"`
	Click    bool   `json:"click"`
	Edit     bool   `json:"edit"`
	Scroll   bool   `json:"scroll"`
	Focused  bool   `json:"focused"`
	Checked  *bool  `json:"checked,omitempty"`
	Enabled  bool   `json:"enabled"`
	Password bool   `json:"password"`
	Window   string `json:"window"` // app, dialog, ime, system or split
}

// UIClicked is click's result. How is "action" (ACTION_CLICK on a node,
// Bounds are the node's) or "gesture" (a tap at a point, Bounds are
// [x, y, x+1, y+1]). Clicked is always true: a click that did not happen is
// an error, never ok with false.
type UIClicked struct {
	Clicked bool   `json:"clicked"`
	Bounds  [4]int `json:"bounds"`
	How     string `json:"how"`
}

// UITextSet is setText's result. Set is always true, as UIClicked.Clicked.
type UITextSet struct {
	Set bool `json:"set"`
}

// UIForeground is foreground's result. An empty Package means unknown, and
// the host's guard then refuses (M5 spec §4.3). Protected is the app's
// opinion: the host refuses when it is true or when the package is in its
// own list or is ADBuddy.
type UIForeground struct {
	Package   string `json:"package"`
	Activity  string `json:"activity"`
	Protected bool   `json:"protected"`
}

// UINotifications is notifications' result: at most MaxNotifications, the
// newest first; Truncated when there were more.
type UINotifications struct {
	Notifications []UINotification `json:"notifications"`
	Truncated     bool             `json:"truncated"`
}

// UINotification is one notification. Title and Text are empty for a
// protected app and for ADBuddy, and cut to MaxNotificationText code points.
type UINotification struct {
	Key      string    `json:"key"`
	Package  string    `json:"package"`
	Title    string    `json:"title"`
	Text     string    `json:"text"`
	PostedAt time.Time `json:"postedAt"`
	Ongoing  bool      `json:"ongoing"`
}
