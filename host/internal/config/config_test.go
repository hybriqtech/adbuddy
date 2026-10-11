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

func TestRemoteFields(t *testing.T) {
	ok := Default()
	ok.TailscaleIP, ok.TailscalePort = "100.101.102.103", 41234
	ok.RemoteUntil = "2026-10-02T09:00:00Z"
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := ok.RemoteUntilTime(); !got.Equal(time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("until %v", got)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(path, ok); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil || back.TailscaleIP != ok.TailscaleIP || back.RemoteUntil != ok.RemoteUntil {
		t.Fatalf("%+v %v", back, err)
	}
	for name, mut := range map[string]func(*Config){
		"ip outside CGNAT":  func(c *Config) { c.TailscaleIP = "198.51.100.5" },
		"ip not an address": func(c *Config) { c.TailscaleIP = "tailscale" },
		"port without ip":   func(c *Config) { c.TailscaleIP, c.TailscalePort = "", 4000 },
		"port out of range": func(c *Config) { c.TailscalePort = 70000 },
		"until not a time":  func(c *Config) { c.RemoteUntil = "tomorrow" },
	} {
		c := ok
		mut(&c)
		if c.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if (Config{RemoteUntil: "x"}).RemoteUntilTime() != (time.Time{}) {
		t.Error("an unreadable until must read as none")
	}
}

func TestLangFollowsADBUDDYLangFirst(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	c := Default()
	c.Language = "es"
	if got := c.Lang(env(map[string]string{"ADBUDDY_LANG": "en", "LANG": "es_ES"})); got != "en" {
		t.Fatalf("ADBUDDY_LANG must win over config.language: %s", got)
	}
	if got := Default().Lang(env(map[string]string{"LC_ALL": "C", "LANG": "es_ES"})); got != "en" {
		t.Fatalf("LC_ALL=C decides: %s", got)
	}
}

func TestReadLanguage(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, c := range []struct{ name, content, want string }{
		{"es", "language = \"es\"\n", "es"},
		{"en", "language = \"en\"\n", "en"},
		{"other keys do not matter", "typo_key = 1\nserial = \"abc\"\nlanguage = \"es\"\n", "es"},
		{"unknown language", "language = \"fr\"\n", ""},
		{"no language", "serial = \"abc\"\n", ""},
		{"broken toml", "language = \n[[", ""},
	} {
		if got := ReadLanguage(write(c.name+".toml", c.content)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	if got := ReadLanguage(filepath.Join(dir, "missing.toml")); got != "" {
		t.Errorf("a missing file: %q", got)
	}
}

func TestAgentModelAndEffort(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte("agent_model = \"claude-sonnet-5-5\"\nagent_effort = \"medium\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil || c.AgentModel != "claude-sonnet-5-5" || c.AgentEffort != "medium" {
		t.Fatalf("%+v %v", c, err)
	}
	for key, data := range map[string]string{
		"agent_model":  "agent_model = \"--x\"\n",
		"agent_effort": "agent_effort = \"ultra\"\n",
	} {
		q := filepath.Join(dir, key+".toml")
		if err := os.WriteFile(q, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(q); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s: %v", key, err)
		}
	}
	// Absent: empty, and what Save writes loads again.
	d, err := Load(filepath.Join(dir, "none.toml"))
	if err != nil || d.AgentModel != "" || d.AgentEffort != "" {
		t.Fatalf("%+v %v", d, err)
	}
	if err := Save(filepath.Join(dir, "saved.toml"), d); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(dir, "saved.toml")); err != nil {
		t.Fatal(err)
	}
	// Filled in, they come back as they went.
	d.AgentModel, d.AgentEffort = "opus[1m]", "xhigh"
	rt := filepath.Join(dir, "roundtrip.toml")
	if err := Save(rt, d); err != nil {
		t.Fatal(err)
	}
	back, err := Load(rt)
	if err != nil || back.AgentModel != "opus[1m]" || back.AgentEffort != "xhigh" {
		t.Fatalf("%+v %v", back, err)
	}
}

// "inherit" is a valid agent_model and agent_effort and comes back as it went.
func TestAgentModelAndEffortInherit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	c := Default()
	c.AgentModel, c.AgentEffort = "inherit", "inherit"
	if err := Save(p, c); err != nil {
		t.Fatal(err)
	}
	back, err := Load(p)
	if err != nil || back.AgentModel != "inherit" || back.AgentEffort != "inherit" {
		t.Fatalf("%+v %v", back, err)
	}
}

// inherit is lower case only, and the error says it is accepted.
func TestAgentEffortInheritIsCaseSensitive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte("agent_effort = \"INHERIT\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "agent_effort") || !strings.Contains(err.Error(), "inherit") {
		t.Fatalf("%v", err)
	}
}
