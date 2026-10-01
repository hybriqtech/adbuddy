package uidump

import (
	"os"
	"reflect"
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
