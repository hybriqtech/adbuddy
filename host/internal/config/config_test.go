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

// address is the last IP:port the phone answered on; empty or IP:port.
func TestAddress(t *testing.T) {
	for _, ok := range []string{"", "100.64.0.5:41234", "192.0.2.10:5555", "[fd7a:115c:a1e0::1]:41234"} {
		c := Default()
		c.Address = ok
		if err := c.Validate(); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"100.64.0.5", "phone.local:41234", "100.64.0.5:0", "100.64.0.5:65536", "100.64.0.5:x", " 100.64.0.5:41234"} {
		c := Default()
		c.Address = bad
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "address") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	p := filepath.Join(t.TempDir(), "config.toml")
	c := Default()
	c.Address = "100.64.0.5:41234"
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if got, err := Load(p); err != nil || got.Address != c.Address || !strings.Contains(string(data), `address = "100.64.0.5:41234"`) {
		t.Fatalf("%+v %v\n%s", got, err, data)
	}
}
