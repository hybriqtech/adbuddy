package claudecode

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxSummary = 120
	maxDetail  = 2000
	maxRule    = 200
)

// input reads the tool_input of a tool as a map; anything else is empty.
func input(raw json.RawMessage) map[string]any {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// oneLine puts s on one line, at most n runes.
func oneLine(s string, n int) string {
	s = Normalize(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// ToolSummary is the one line the phone shows of a tool use (spec M4 §4.2):
// the verb and what it works on, never the whole input, never a path with
// folders. At most 120 characters.
func ToolSummary(tool string, raw json.RawMessage) string {
	in := input(raw)
	base := func(key string) string {
		p := str(in, key)
		if p == "" {
			return ""
		}
		return filepath.Base(strings.ReplaceAll(p, "\\", "/"))
	}
	switch tool {
	case "Bash":
		if d := str(in, "description"); strings.TrimSpace(d) != "" {
			return oneLine(d, maxSummary)
		}
		return oneLine(str(in, "command"), maxSummary)
	case "Read":
		return oneLine("Read "+base("file_path"), maxSummary)
	case "Edit", "MultiEdit":
		return oneLine("Edit "+base("file_path"), maxSummary)
	case "Write":
		return oneLine("Write "+base("file_path"), maxSummary)
	case "NotebookEdit":
		return oneLine("Edit "+base("notebook_path"), maxSummary)
	case "Grep":
		return oneLine("Search "+str(in, "pattern"), maxSummary)
	case "Glob":
		return oneLine("Find "+str(in, "pattern"), maxSummary)
	case "WebFetch":
		return oneLine("Fetch "+domainOf(str(in, "url")), maxSummary)
	case "WebSearch":
		return oneLine("Search the web: "+str(in, "query"), maxSummary)
	}
	if rest, ok := strings.CutPrefix(tool, "mcp__"); ok {
		server, name, _ := strings.Cut(rest, "__")
		return oneLine(server+": "+name, maxSummary)
	}
	return oneLine(tool, maxSummary)
}

// domainOf is the host of a URL as written, or "" when it has none.
func domainOf(u string) string {
	_, rest, ok := strings.Cut(u, "://")
	if !ok {
		return ""
	}
	host, _, _ := strings.Cut(rest, "/")
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	return strings.ToLower(host)
}

// ToolDetail is what the person reads to decide a permission (spec M4 §5.1):
// the whole command of a Bash, the path of a file tool, else the input as
// JSON, at most 2000 characters.
func ToolDetail(tool string, raw json.RawMessage) string {
	return cut(fullDetail(tool, raw), maxDetail)
}

func fullDetail(tool string, raw json.RawMessage) string {
	in := input(raw)
	switch {
	case tool == "Bash" && str(in, "command") != "":
		return str(in, "command")
	case str(in, "file_path") != "":
		return str(in, "file_path")
	case str(in, "notebook_path") != "":
		return str(in, "notebook_path")
	case str(in, "url") != "":
		return str(in, "url")
	}
	return string(raw)
}

// shownRune is false for what the person cannot be trusted to see as it is: a
// control (but the newline and the tab), a format character (bidi, zero width),
// a separator that is not the ASCII space, or what is not valid text.
func shownRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t' || r == ' ':
		return true
	case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Z, r):
		return false
	}
	return true
}

func shown(s string) bool {
	for _, r := range s {
		if !shownRune(r) {
			return false
		}
	}
	return true
}

// CardSafe reports whether the phone's card shows what a "yes" would approve
// (spec M4 §5.1, review I3): the detail must fit whole (the card cuts it and a
// "yes" approves all of it) and neither it nor what the title is made of may
// hold controls, bidi or invisible characters. When it is false the hook does
// not ask the phone: the question stays on the terminal.
func CardSafe(tool string, raw json.RawMessage) bool {
	d := fullDetail(tool, raw)
	if len(d) > maxDetail || !shown(d) {
		return false
	}
	in := input(raw)
	for _, k := range []string{"description", "pattern", "query"} {
		if !shown(str(in, k)) {
			return false
		}
	}
	return true
}

// ruleRune is the whitelist of what a "yes, always" rule may be made of:
// ASCII letters and digits, the space and a few signs that mean nothing to a
// shell or to the syntax of a Claude Code rule. Anything else (wildcards,
// quotes, expansions, controls, bidi, any non-ASCII) and the rule is not
// offered: when in doubt, the question stays on the terminal.
func ruleRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune(" ._/=:@+,-", r)
}

// phoneWords are names of tools that reach the phone (or root) by a way the
// shim does not see: a rule that mentions one is never offered. They are
// searched as a substring, in any case, so a path or a suffix does not hide
// them; su and sudo are too short for that and are searched as words.
var (
	phoneSubstrings = []string{"adb", "adbuddy", "fastboot", "scrcpy", "gradle", "platform-tools", "android_serial"}
	phoneWords      = []string{"su", "sudo"}
)

func mentionsPhone(cmd string) bool {
	low := strings.ToLower(cmd)
	for _, w := range phoneSubstrings {
		if strings.Contains(low, w) {
			return true
		}
	}
	words := strings.FieldsFunc(low, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') })
	for _, w := range words {
		for _, p := range phoneWords {
			if w == p {
				return true
			}
		}
	}
	return false
}

// AlwaysRule returns the rule "yes, always" would add, and whether it may be
// offered at all (spec M4 §5.3). It is the exact command of a Bash and nothing
// broader: no wildcard is ever built from it. It is offered only for a Bash
// command that is short, on one line and made only of the ruleRune whitelist
// (so no wildcard of bash can hide a word from the check below), and that does
// not mention a tool that reaches the phone or root (mentionsPhone), whose
// every call the shim classifies and Claude Code must not wave through for
// good.
func AlwaysRule(tool string, raw json.RawMessage) (string, bool) {
	if tool != "Bash" {
		return "", false
	}
	cmd := strings.Trim(str(input(raw), "command"), " \t\r\n")
	if cmd == "" || len(cmd) > maxRule {
		return "", false
	}
	for _, r := range cmd {
		if !ruleRune(r) {
			return "", false
		}
	}
	if strings.Contains(cmd, ":*") || mentionsPhone(cmd) {
		return "", false
	}
	return cmd, true
}
