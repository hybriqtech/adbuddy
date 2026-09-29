package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.ADBServerPort != 5038 || c.ConfirmTimeout.Duration != 2*time.Minute {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestLoadParsesValues(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	data := "agent = \"claude\"\nadb_path = \"/x/adb\"\nadb_server_port = 5040\nserial = \"abc\"\nconfirm_timeout = \"30s\"\nlanguage = \"es\"\n"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Agent != "claude" || c.ADBPath != "/x/adb" || c.ADBServerPort != 5040 || c.Serial != "abc" ||
		c.ConfirmTimeout.Duration != 30*time.Second || c.Language != "es" {
		t.Fatalf("got %+v", c)
	}
}

func TestLoadRejectsUnknownKeysAndBadValues(t *testing.T) {
	for _, data := range []string{
		"adb_sever_port = 1\n",
		"adb_server_port = 70000\n",
		"confirm_timeout = \"0s\"\n",
		"language = \"fr\"\n",
		"agent = \n",
	} {
		p := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(p); err == nil {
			t.Errorf("Load(%q) succeeded, want error", data)
		}
	}
}

func TestSaveRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	c := Default()
	c.Agent = "codex"
	c.Serial = "s1"
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != c {
		t.Fatalf("round trip: got %+v want %+v", got, c)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(p)
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}
}

func TestLangAndWorkspace(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := Default().Lang(env(map[string]string{"LANG": "es_ES.UTF-8"})); got != "es" {
		t.Fatalf("LANG es: %s", got)
	}
	if got := Default().Lang(env(nil)); got != "en" {
		t.Fatalf("no LANG: %s", got)
	}
	c := Default()
	c.Language = "es"
	if got := c.Lang(env(map[string]string{"LANG": "en_US"})); got != "es" {
		t.Fatalf("configured: %s", got)
	}
	if !strings.HasSuffix(Default().WorkspaceDir("home"), filepath.Join("ADBuddy", "default")) {
		t.Fatal(Default().WorkspaceDir("home"))
	}
}
