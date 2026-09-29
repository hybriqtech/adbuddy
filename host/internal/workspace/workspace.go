// Package workspace creates the agent's environment (spec §7): context
// files, pre-approved permissions and the adb shim.
package workspace

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed templates/AGENTS.md templates/CLAUDE.md templates/NOTES.md
var templates embed.FS

// ProfileMarker is replaced by the device profile in AGENTS.md.
const ProfileMarker = "<!-- adbuddy:profile -->"

// Options for Init.
type Options struct {
	Dir     string // the environment directory
	Exe     string // the running adbuddy binary, copied as the shim
	Profile string // markdown describing the phone ("" = not collected yet)
	Force   bool   // rewrite AGENTS.md and CLAUDE.md (never NOTES.md)
}

// Result lists what Init changed, relative to Dir.
type Result struct {
	Dir     string   `json:"dir"`
	Created []string `json:"created"`
	Updated []string `json:"updated"`
}

// BinDir holds the shim; it goes first in the agent's PATH.
func BinDir(dir string) string { return filepath.Join(dir, ".adbuddy", "bin") }

// JournalDir holds the journal.
func JournalDir(dir string) string { return filepath.Join(dir, "journal") }

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Init creates or refreshes the environment. It is idempotent: files the
// user may have edited are only rewritten with Force, and NOTES.md never.
func Init(o Options) (Result, error) {
	res := Result{Dir: o.Dir}
	if o.Dir == "" || o.Exe == "" {
		return res, errors.New("workspace: Dir and Exe are required")
	}
	for _, d := range []string{o.Dir, BinDir(o.Dir), JournalDir(o.Dir), filepath.Join(o.Dir, ".claude")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return res, err
		}
	}
	profile := o.Profile
	if profile == "" {
		profile = "Not collected yet."
	}
	agents, _ := templates.ReadFile("templates/AGENTS.md")
	agents = bytes.Replace(agents, []byte(ProfileMarker), []byte(strings.TrimSpace(profile)), 1)
	claude, _ := templates.ReadFile("templates/CLAUDE.md")
	notes, _ := templates.ReadFile("templates/NOTES.md")

	files := []struct {
		name      string
		data      []byte
		overwrite bool
	}{
		{"AGENTS.md", agents, o.Force},
		{"CLAUDE.md", claude, o.Force},
		{"NOTES.md", notes, false},
	}
	for _, f := range files {
		changed, created, err := writeFile(filepath.Join(o.Dir, f.name), f.data, f.overwrite)
		if err != nil {
			return res, err
		}
		res.note(f.name, changed, created)
	}

	changed, created, err := mergeClaudeSettings(filepath.Join(o.Dir, ".claude", "settings.json"))
	if err != nil {
		return res, err
	}
	res.note(filepath.Join(".claude", "settings.json"), changed, created)

	for _, name := range []string{exe("adb"), exe("adbuddy")} {
		changed, created, err := copyIfDifferent(o.Exe, filepath.Join(BinDir(o.Dir), name))
		if err != nil {
			return res, err
		}
		res.note(filepath.Join(".adbuddy", "bin", name), changed, created)
	}
	return res, nil
}

func (r *Result) note(name string, changed, created bool) {
	switch {
	case created:
		r.Created = append(r.Created, name)
	case changed:
		r.Updated = append(r.Updated, name)
	}
}

func writeFile(path string, data []byte, overwrite bool) (changed, created bool, err error) {
	old, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, true, os.WriteFile(path, data, 0o600)
	case err != nil:
		return false, false, err
	case !overwrite || bytes.Equal(old, data):
		return false, false, nil
	}
	return true, false, os.WriteFile(path, data, 0o600)
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// AllowRules are the permissions ADBuddy pre-approves in Claude Code (spec §4.6).
var AllowRules = []string{"Bash(adb:*)", "Bash(adbuddy:*)"}

// mergeClaudeSettings adds AllowRules to permissions.allow, keeping every
// other setting the user has.
func mergeClaudeSettings(path string) (changed, created bool, err error) {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		created = true
	case err != nil:
		return false, false, err
	default:
		if err := json.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &settings); err != nil {
			return false, false, fmt.Errorf("%s is not valid JSON (%v); fix it or delete it", path, err)
		}
		if settings == nil {
			return false, false, fmt.Errorf("%s is not a JSON object; fix it or delete it", path)
		}
	}
	perms, _ := settings["permissions"].(map[string]any)
	if perms == nil {
		if _, exists := settings["permissions"]; exists {
			return false, false, fmt.Errorf("%s: \"permissions\" is not an object", path)
		}
		perms = map[string]any{}
	}
	allow, _ := perms["allow"].([]any)
	if allow == nil {
		if _, exists := perms["allow"]; exists {
			return false, false, fmt.Errorf("%s: \"permissions.allow\" is not a list", path)
		}
	}
	for _, rule := range AllowRules {
		found := false
		for _, a := range allow {
			if a == rule {
				found = true
			}
		}
		if !found {
			allow = append(allow, rule)
			changed = true
		}
	}
	if !changed && !created {
		return false, false, nil
	}
	perms["allow"] = allow
	settings["permissions"] = perms
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, false, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return false, false, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(out, '\n')); err != nil {
		tmp.Close()
		return false, false, err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return false, false, err
	}
	if err := tmp.Close(); err != nil {
		return false, false, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return false, false, err
	}
	return true, created, nil
}

// copyIfDifferent copies src to dst unless dst already has the same content.
func copyIfDifferent(src, dst string) (changed, created bool, err error) {
	want, err := fileHash(src)
	if err != nil {
		return false, false, err
	}
	have, err := fileHash(dst)
	if err == nil && have == want {
		return false, false, nil
	}
	created = errors.Is(err, fs.ErrNotExist)
	in, err := os.Open(src)
	if err != nil {
		return false, false, err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".shim-*")
	if err != nil {
		return false, false, err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return false, false, err
	}
	if err := tmp.Chmod(0o700); err != nil {
		tmp.Close()
		return false, false, err
	}
	if err := tmp.Close(); err != nil {
		return false, false, err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return false, false, fmt.Errorf("cannot update %s (is an agent still running in this environment?): %w", dst, err)
	}
	return true, created, nil
}

func fileHash(path string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
