package uidump

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func sample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseFocus(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want []string
		ok   bool
	}{
		{"shade", sample(t, "window_shade.txt"), []string{"NotificationShade"}, true},
		{"app", sample(t, "window_app.txt"), []string{"com.android.settings/com.android.settings.Settings"}, true},
		{"null", "  mCurrentFocus=null\n", []string{""}, true},
		{"one per display", "  mCurrentFocus=Window{1 u0 com.example.bank/.Main}\n  mCurrentFocus=Window{2 u0 NotificationShade}\n", []string{"com.example.bank/.Main", "NotificationShade"}, true},
		{"title with spaces", "  mCurrentFocus=Window{1 u0 Application Not Responding: com.x}\n", []string{"Application Not Responding: com.x"}, true},
		{"no line", "", nil, false},
		{"nothing about focus", "mFocusedApp=null\n", nil, false},
		{"unreadable", sample(t, "window_unreadable.txt"), nil, false},
		{"one line understood, one not", "  mCurrentFocus=Window{1 u0 com.x/.Y}\n  mCurrentFocus=garbage\n", nil, false},
	}
	for _, c := range cases {
		got, ok := ParseFocus(c.out)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q %v, want %q %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestIsShade(t *testing.T) {
	for focus, want := range map[string]bool{
		"NotificationShade":                     true,
		"StatusBar":                             true,
		"":                                      false, // mCurrentFocus=null
		"com.android.settings/.Settings":        false,
		"com.android.systemui/.RecentsActivity": false,
		"NotificationShadeX":                    false,
	} {
		if IsShade(focus) != want {
			t.Errorf("IsShade(%q) = %v", focus, !want)
		}
	}
}

func TestParseSystemUIWindows(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		unknown []string
		ok      bool
	}{
		{"fixed ones, one hidden", sample(t, "windows_systemui_fixed.txt"), nil, true},
		{"unknown and visible", sample(t, "windows_systemui_unknown.txt"), []string{"NotificationShade"}, true},
		{"unknown but hidden", sample(t, "windows_systemui_hidden.txt"), nil, true},
		{"no systemui", sample(t, "windows_no_systemui.txt"), nil, true},
		{"unreadable", sample(t, "windows_unreadable.txt"), nil, false},
		{"empty", "", nil, false},
		{"a window without a package", "  Window #0 Window{1 u0 HeadsUp}:\n    mViewVisibility=0x0\n", nil, false},
		{"systemui without visibility", "  Window #0 Window{1 u0 HeadsUp}:\n    package=com.android.systemui appop=NONE\n", nil, false},
	}
	for _, c := range cases {
		got, ok := ParseSystemUIWindows(c.out)
		if ok != c.ok || !reflect.DeepEqual(got, c.unknown) {
			t.Errorf("%s: got %q %v, want %q %v", c.name, got, ok, c.unknown, c.ok)
		}
	}
}

func TestWindowFormatVariations(t *testing.T) {
	crlf := strings.ReplaceAll(sample(t, "windows_systemui_unknown.txt"), "\n", "\r\n")
	if got, ok := ParseSystemUIWindows(crlf); !ok || !reflect.DeepEqual(got, []string{"NotificationShade"}) {
		t.Errorf("CRLF windows: %q %v", got, ok)
	}
	if got, ok := ParseFocus("  mCurrentFocus=Window{4b0f3a2 u0 NotificationShade}\r\n"); !ok || !reflect.DeepEqual(got, []string{"NotificationShade"}) {
		t.Errorf("CRLF focus: %q %v", got, ok)
	}
	// A work profile: u10.
	if got, ok := ParseFocus("  mCurrentFocus=Window{4b0f3a2 u10 NotificationShade}\n"); !ok || !IsShade(got[0]) {
		t.Errorf("u10 focus: %q %v", got, ok)
	}
	work := strings.ReplaceAll(sample(t, "windows_systemui_unknown.txt"), "Window{dd33ee u0 NotificationShade}", "Window{dd33ee u10 NotificationShade}")
	if got, ok := ParseSystemUIWindows(work); !ok || !reflect.DeepEqual(got, []string{"NotificationShade"}) {
		t.Errorf("u10 windows: %q %v", got, ok)
	}
	// A title with } is not understood: not ok, so the guard fails closed.
	if _, ok := ParseFocus("  mCurrentFocus=Window{1 u0 a}b}\n"); ok {
		t.Error("focus with } in the title is ok")
	}
	if _, ok := ParseSystemUIWindows("  Window #0 Window{1 u0 a}b}:\n    package=com.android.systemui\n    mViewVisibility=0x0\n"); ok {
		t.Error("window with } in the title is ok")
	}
}

// A window line that is not understood must not merge into the block before
// it (that window would vanish).
func TestAWindowHeaderNotUnderstoodIsNotOK(t *testing.T) {
	out := sample(t, "windows_systemui_fixed.txt") + "  Window #9 Window{zz u0 HeadsUp\n    mOwnerUid=1 package=com.android.systemui\n    mViewVisibility=0x0\n"
	if _, ok := ParseSystemUIWindows(out); ok {
		t.Error("broken header is ok")
	}
}

func TestFixedSystemUIWindowsOfARealPhone(t *testing.T) {
	out := ""
	for _, title := range []string{"StatusBar", "NavigationBar0", "ImageWallpaper", "NavigationBarEdgePanel", "ScreenDecorHwcOverlay", "ScreenDecorHwcOverlayBottom", "com.android.systemui.wallpapers.ImageWallpaper"} {
		out += "  Window #0 Window{1 u0 " + title + "}:\n    mOwnerUid=1 package=com.android.systemui appop=NONE\n    mViewVisibility=0x0\n"
	}
	if got, ok := ParseSystemUIWindows(out); !ok || len(got) != 0 {
		t.Errorf("%q %v", got, ok)
	}
}

// The long wallpaper title is fixed only for systemui: the same title in the
// window of another package is not a systemui window and is ignored (the
// parser only looks at windows of com.android.systemui).
func TestTheLongWallpaperTitleIsFixedOnlyForSystemUI(t *testing.T) {
	const title = "com.android.systemui.wallpapers.ImageWallpaper"
	win := func(pkg string) string {
		return `  Window #0 Window{1 u0 ` + title + `}:
    mOwnerUid=1 package=` + pkg + ` appop=NONE
    mViewVisibility=0x0
`
	}
	if got, ok := ParseSystemUIWindows(win(SystemUIPackage)); !ok || len(got) != 0 {
		t.Errorf("systemui: %q %v", got, ok)
	}
	if got, ok := ParseSystemUIWindows(win("com.evil.app")); !ok || len(got) != 0 {
		t.Errorf("other package: %q %v", got, ok)
	}
}
