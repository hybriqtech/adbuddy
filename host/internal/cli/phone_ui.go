package cli

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
)

// uiArea is the agent's helpers (M5, protocol spec §8; uilink.go). The remote
// mode joins it in M5b.
func (l *phoneLink) uiArea() area {
	if l.ui == nil { // a link built by hand, as in tests
		l.ui = newUILink("", io.Discard)
	}
	if l.srv != nil {
		l.ui.setServer(l.srv)
	}
	return l.ui.area()
}

// helpers gives the daemon its way to the app and the protected apps (M5
// spec §2, §4.3). ui is nil without the app: then adb does what it can.
func (l *phoneLink) helpers() (ui func(context.Context, string, json.RawMessage) (json.RawMessage, error), protected func(string) bool) {
	if l.ui == nil {
		l.uiArea()
	}
	if l.srv != nil {
		ui = l.ui.Call
	}
	return ui, l.ui.Protected
}

// protectedAppsPath is where the protected apps are kept between sessions: in
// the data folder, out of the agent's environment.
func protectedAppsPath(e *env) string { return filepath.Join(e.dirs.Data, "protected.json") }
