package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ClaudeConfigFile is where Claude Code keeps its per-folder settings:
// $CLAUDE_CONFIG_DIR/.claude.json, or ~/.claude.json.
func ClaudeConfigFile(getenv func(string) string, home string) string {
	if d := getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, ".claude.json")
	}
	return filepath.Join(home, ".claude.json")
}

// TrustFolder marks dir as trusted in Claude Code's configuration file, so it
// does not open with its folder trust dialog, whose default answer exits
// (spec M3 §3.3). It only adds projects["<dir with />"].hasTrustDialogAccepted
// and replaces the file atomically; a missing file, or one that is not a JSON
// object, is left alone. It reports whether it wrote.
func TrustFolder(configFile, dir string) (bool, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	key := filepath.ToSlash(abs)
	data, err := os.ReadFile(configFile)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var cfg map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // numbers come back as written
	if err := dec.Decode(&cfg); err != nil || cfg == nil {
		return false, nil
	}
	projects, ok := cfg["projects"].(map[string]any)
	if !ok {
		if _, exists := cfg["projects"]; exists {
			return false, nil // not what Claude Code writes: leave it
		}
		projects = map[string]any{}
		cfg["projects"] = projects
	}
	project, ok := projects[key].(map[string]any)
	if !ok {
		if _, exists := projects[key]; exists {
			return false, nil
		}
		project = map[string]any{}
		projects[key] = project
	}
	if project["hasTrustDialogAccepted"] == true {
		return false, nil
	}
	project["hasTrustDialogAccepted"] = true
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	info, err := os.Stat(configFile)
	if err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(configFile), ".claude.json.adbuddy-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name()) // after a successful rename it no longer exists
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	_ = os.Chmod(tmp.Name(), info.Mode().Perm())
	if err := os.Rename(tmp.Name(), configFile); err != nil {
		return false, err
	}
	return true, nil
}
