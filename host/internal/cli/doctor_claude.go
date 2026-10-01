package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
)

// checkClaudeHooks warns about the user's own Claude Code hooks that can
// answer the same request as ADBuddy's (spec M4 §3.4): a global PermissionRequest
// or PreToolUse hook may allow or deny a tool before the phone is asked. It
// only reads ~/.claude/settings.json (or $CLAUDE_CONFIG_DIR's) and never
// changes it. A file that is missing or unreadable is not a problem.
func checkClaudeHooks() check {
	dir, err := claudecode.ConfigDir(os.Getenv, os.UserHomeDir)
	if err != nil {
		return check{Name: "claude hooks", Status: checkOK, Detail: "no Claude Code settings found"}
	}
	path := filepath.Join(dir, "settings.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return check{Name: "claude hooks", Status: checkOK, Detail: "no global Claude Code hooks (" + path + " not read)"}
	}
	var s struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if json.Unmarshal(b, &s) != nil {
		return check{Name: "claude hooks", Status: checkOK, Detail: path + " is not readable as JSON: not checked"}
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
		return check{Name: "claude hooks", Status: checkOK, Detail: "no global PermissionRequest or PreToolUse hooks"}
	}
	return check{
		Name: "claude hooks", Status: checkWarn,
		Detail:   path + " has " + strings.Join(clash, " and ") + " hooks: if one answers the same request as ADBuddy, the phone may not be asked, or the two may disagree",
		Guidance: "Review those hooks in " + path + ". ADBuddy keeps classifying every adb command either way.",
	}
}
