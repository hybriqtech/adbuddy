package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TrustFolder adds only the trust of the folder, keyed by its path with
// forward slashes, and keeps everything else.
func TestTrustFolder(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	ws := filepath.Join(dir, "ADBuddy", "pixel")
	os.WriteFile(cfg, []byte(`{"numStartups": 12345678901234567, "projects": {"/other": {"hasTrustDialogAccepted": false, "allowedTools": []}}, "userID": "u"}`), 0o600)
	wrote, err := TrustFolder(cfg, ws)
	if err != nil || !wrote {
		t.Fatalf("TrustFolder = %v, %v", wrote, err)
	}
	m := readJSON(t, cfg)
	key := filepath.ToSlash(ws)
	if p := m["projects"].(map[string]any)[key].(map[string]any); p["hasTrustDialogAccepted"] != true {
		t.Fatalf("project %v", p)
	}
	if other := m["projects"].(map[string]any)["/other"].(map[string]any); other["hasTrustDialogAccepted"] != false {
		t.Fatalf("another project changed: %v", other)
	}
	raw, _ := os.ReadFile(cfg)
	if !json.Valid(raw) || !strings.Contains(string(raw), "12345678901234567") || m["userID"] != "u" {
		t.Fatalf("the rest changed:\n%s", raw)
	}
	// Twice: nothing to write.
	if wrote, err := TrustFolder(cfg, ws); err != nil || wrote {
		t.Fatalf("second call = %v, %v", wrote, err)
	}
}

// A missing file, one that is not JSON or not what Claude Code writes, is
// left alone.
func TestTrustFolderLeavesOddFilesAlone(t *testing.T) {
	dir := t.TempDir()
	if wrote, err := TrustFolder(filepath.Join(dir, "missing.json"), dir); err != nil || wrote {
		t.Fatalf("missing file: %v %v", wrote, err)
	}
	for _, content := range []string{"not json", "[1,2]", `{"projects": "x"}`, `{"projects": {"` + filepath.ToSlash(dir) + `": 3}}`} {
		cfg := filepath.Join(dir, "c.json")
		os.WriteFile(cfg, []byte(content), 0o600)
		if wrote, err := TrustFolder(cfg, dir); err != nil || wrote {
			t.Fatalf("%q: %v %v", content, wrote, err)
		}
		if got, _ := os.ReadFile(cfg); string(got) != content {
			t.Fatalf("%q changed to %q", content, got)
		}
	}
}

func TestClaudeConfigFile(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	if got := ClaudeConfigFile(get, "/home/u"); got != filepath.Join("/home/u", ".claude.json") {
		t.Fatal(got)
	}
	env["CLAUDE_CONFIG_DIR"] = "/cfg"
	if got := ClaudeConfigFile(get, "/home/u"); got != filepath.Join("/cfg", ".claude.json") {
		t.Fatal(got)
	}
}

// Anything but spaces after the JSON object: not what Claude Code writes,
// the file is left alone.
func TestTrustFolderLeavesTrailingDataAlone(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	for _, content := range []string{`{"a": 1} {"b": 2}`, `{"a": 1}x`, `{"a": 1}]`, "{\"a\": 1}\n\n,"} {
		os.WriteFile(cfg, []byte(content), 0o600)
		if wrote, err := TrustFolder(cfg, dir); err != nil || wrote {
			t.Fatalf("%q: %v %v", content, wrote, err)
		}
		if got, _ := os.ReadFile(cfg); string(got) != content {
			t.Fatalf("%q changed to %q", content, got)
		}
	}
	// Spaces and new lines after it are fine.
	os.WriteFile(cfg, []byte("{\"a\": 1}\r\n \t\n"), 0o600)
	if wrote, err := TrustFolder(cfg, dir); err != nil || !wrote {
		t.Fatalf("trailing spaces: %v %v", wrote, err)
	}
}

// A configuration file that is a symbolic link stays one: its target gets
// the trust.
func TestTrustFolderThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real", "claude.json")
	os.MkdirAll(filepath.Dir(target), 0o700)
	os.WriteFile(target, []byte(`{"userID": "u"}`), 0o600)
	link := filepath.Join(dir, ".claude.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make symbolic links here: %v", err)
	}
	if wrote, err := TrustFolder(link, dir); err != nil || !wrote {
		t.Fatalf("TrustFolder = %v, %v", wrote, err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced: %v %v", fi.Mode(), err)
	}
	m := readJSON(t, target)
	if p := m["projects"].(map[string]any)[filepath.ToSlash(dir)].(map[string]any); p["hasTrustDialogAccepted"] != true || m["userID"] != "u" {
		t.Fatalf("target %v", m)
	}
}
