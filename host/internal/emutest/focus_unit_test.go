package emutest

import "testing"

func TestEmptyFocus(t *testing.T) {
	for _, c := range []struct {
		name string
		f    []string
		want bool
	}{
		{"nil", nil, true},
		{"empty", []string{}, true},
		{"one empty", []string{""}, true},
		{"two empty", []string{"", ""}, true},
		{"settings", []string{"com.android.settings/com.android.settings.Settings"}, false},
		{"other window", []string{"com.other/.Main"}, false},
		{"empty and other", []string{"", "com.other/.Main"}, false},
	} {
		if got := emptyFocus(c.f); got != c.want {
			t.Errorf("%s: emptyFocus(%q) = %v, want %v", c.name, c.f, got, c.want)
		}
	}
}
