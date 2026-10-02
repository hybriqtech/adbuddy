package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"

	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// checkClaudeHooks warns about the user's own Claude Code hooks that can
// answer the same request as ADBuddy's (spec M4 §3.4): a global PermissionRequest
// or PreToolUse hook may allow or deny a tool before the phone is asked. It
// only reads ~/.claude/settings.json (or $CLAUDE_CONFIG_DIR's) and never
// changes it. A file that is missing or unreadable is not a problem.
func checkClaudeHooks() check {
	dir, err := claudecode.ConfigDir(os.Getenv, os.UserHomeDir)
	if err != nil {
		return mk("claude hooks", checkOK, i18n.M("doctor.hooks.no_settings"))
	}
	path := filepath.Join(dir, "settings.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return mk("claude hooks", checkOK, i18n.M("doctor.hooks.not_read", path))
	}
	var s struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if json.Unmarshal(b, &s) != nil {
		return mk("claude hooks", checkOK, i18n.M("doctor.hooks.not_json", path))
	}
	var clash []string
	for _, ev := range []string{claudecode.EventPermissionRequest, "PreToolUse"} {
		var entries []json.RawMessage
		if json.Unmarshal(s.Hooks[ev], &entries) == nil && len(entries) > 0 {
			clash = append(clash, ev)
		}
	}
	sort.Strings(clash)
	if len(clash) == 0 {
		return mk("claude hooks", checkOK, i18n.M("doctor.hooks.none"))
	}
	detail := i18n.M("doctor.hooks.clash_1", path, clash[0])
	if len(clash) == 2 {
		detail = i18n.M("doctor.hooks.clash_2", path, clash[0], clash[1])
	}
	return mk("claude hooks", checkWarn, detail).withGuide(i18n.M("doctor.hooks.review", path))
}
