package paths

import (
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolvePerOS(t *testing.T) {
	home := filepath.Join("h", "u")
	cases := []struct {
		goos      string
		env       map[string]string
		cfg, data string
	}{
		{"windows", map[string]string{"APPDATA": filepath.Join("C", "Roaming"), "LOCALAPPDATA": filepath.Join("C", "Local")},
			filepath.Join("C", "Roaming", "ADBuddy"), filepath.Join("C", "Local", "ADBuddy")},
		{"darwin", nil,
			filepath.Join(home, "Library", "Application Support", "ADBuddy"), filepath.Join(home, "Library", "Application Support", "ADBuddy")},
		{"linux", nil,
			filepath.Join(home, ".config", "adbuddy"), filepath.Join(home, ".local", "share", "adbuddy")},
		{"linux", map[string]string{"XDG_CONFIG_HOME": "xc", "XDG_DATA_HOME": "xd"},
			filepath.Join("xc", "adbuddy"), filepath.Join("xd", "adbuddy")},
	}
	for _, c := range cases {
		d, err := Resolve(env(c.env), c.goos, home)
		if err != nil {
			t.Fatal(err)
		}
		if d.Config != c.cfg || d.Data != c.data {
			t.Errorf("%s %v: got %+v, want cfg=%s data=%s", c.goos, c.env, d, c.cfg, c.data)
		}
	}
}

func TestResolveOverrides(t *testing.T) {
	d, err := Resolve(env(map[string]string{"ADBUDDY_CONFIG_DIR": "c", "ADBUDDY_DATA_DIR": "d"}), "linux", "home")
	if err != nil {
		t.Fatal(err)
	}
	if d.Config != "c" || d.Data != "d" {
		t.Fatalf("got %+v", d)
	}
	if d.ConfigFile() != filepath.Join("c", "config.toml") || d.RulesFile() != filepath.Join("c", "rules.yaml") {
		t.Fatalf("files: %s %s", d.ConfigFile(), d.RulesFile())
	}
}
