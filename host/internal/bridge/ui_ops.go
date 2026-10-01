package bridge

import "time"

// The operations of ui.request and the shape of their args and result (M5
// spec §3). UIRequest.Args and UIResponse.Result carry them as raw JSON;
// TestUIOpFixtures checks each one against protocol/fixtures.

// UI operations.
const (
	UIOpDump          = "dump"
	UIOpClick         = "click"
	UIOpSetText       = "setText"
	UIOpForeground    = "foreground"
	UIOpNotifications = "notifications"
)

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
// field's.
type UISetTextArgs struct {
	Text string `json:"text"`
}

// UIDump is dump's result: the screen as the agent may see it. Elements is
// empty while the phone is locked.
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
// in screen pixels, clipped to what is on screen. Text and Hint are empty
// for a password field. Checked is absent when it cannot be checked.
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

// UIClicked is click's result. How is "action" (ACTION_CLICK on a node) or
// "gesture" (a tap at a point).
type UIClicked struct {
	Clicked bool   `json:"clicked"`
	Bounds  [4]int `json:"bounds"`
	How     string `json:"how"`
}

// UITextSet is setText's result.
type UITextSet struct {
	Set bool `json:"set"`
}

// UIForeground is foreground's result. Protected: the package is a
// protected app or ADBuddy itself.
type UIForeground struct {
	Package   string `json:"package"`
	Activity  string `json:"activity"`
	Protected bool   `json:"protected"`
}

// UINotifications is notifications' result.
type UINotifications struct {
	Notifications []UINotification `json:"notifications"`
}

// UINotification is one notification. Title and Text are empty for a
// protected app and for ADBuddy.
type UINotification struct {
	Key      string    `json:"key"`
	Package  string    `json:"package"`
	Title    string    `json:"title"`
	Text     string    `json:"text"`
	PostedAt time.Time `json:"postedAt"`
	Ongoing  bool      `json:"ongoing"`
}
