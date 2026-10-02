package claudecode

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
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

// ToolSummary is ToolSummaryIn in English.
func ToolSummary(tool string, raw json.RawMessage) string { return ToolSummaryIn(i18n.EN, tool, raw) }

// ToolSummaryIn is the one line the phone shows of a tool use (spec M4 §4.2):
// the verb, in the language of the PC, and what it works on, which is the
// agent's and is never translated; never the whole input, never a path with
// folders. At most 120 characters.
func ToolSummaryIn(lang i18n.Lang, tool string, raw json.RawMessage) string {
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
		return oneLine(lang.T("tool.read", base("file_path")), maxSummary)
	case "Edit", "MultiEdit":
		return oneLine(lang.T("tool.edit", base("file_path")), maxSummary)
	case "Write":
		return oneLine(lang.T("tool.write", base("file_path")), maxSummary)
	case "NotebookEdit":
		return oneLine(lang.T("tool.edit", base("notebook_path")), maxSummary)
	case "Grep":
		return oneLine(lang.T("tool.search", str(in, "pattern")), maxSummary)
	case "Glob":
		return oneLine(lang.T("tool.find", str(in, "pattern")), maxSummary)
	case "WebFetch":
		return oneLine(lang.T("tool.fetch", domainOf(str(in, "url"))), maxSummary)
	case "WebSearch":
		return oneLine(lang.T("tool.web_search", str(in, "query")), maxSummary)
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
	switch tool {
	case "Bash":
		if str(in, "command") != "" {
			return str(in, "command")
		}
	case "Read", "Edit", "MultiEdit", "Write":
		if str(in, "file_path") != "" {
			return str(in, "file_path")
		}
	case "NotebookEdit":
		if str(in, "notebook_path") != "" {
			return str(in, "notebook_path")
		}
	case "WebFetch":
		if str(in, "url") != "" {
			return str(in, "url")
		}
	}
	// Any other tool (MCP included) shows its whole input: a path or a URL
	// does not say what a method or a body would do.
	return string(raw)
}

// shownRune is false for what the person cannot be trusted to see as it is: a
// control (but the newline and the tab), a format character (bidi, zero width),
// a separator that is not the ASCII space, or what is not valid text.
func shownRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t' || r == ' ':
		return true
	case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r), unicode.Is(unicode.Z, r),
		unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r):
		return false
	case r == 0x3164, r == 0x115f, r == 0x1160, r == 0xffa0, r == 0x2800, r == 0x034f: // fillers that draw nothing
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

// maxBlanks is the longest run of spaces and tabs the card's detail may hold:
// more would push text out of view.
const maxBlanks = 8

// detailShown is shown plus what hides text with plain blanks: a long run of
// spaces or tabs, or blanks at the end of a line.
func detailShown(s string) bool {
	if !shown(s) {
		return false
	}
	run := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			run++
			if run > maxBlanks {
				return false
			}
			continue
		}
		if run > 0 && s[i] == '\n' {
			return false
		}
		run = 0
	}
	return run == 0
}

// CardSafe reports whether the phone's card shows what a "yes" would approve
// (spec M4 §5.1, review I3): the detail must fit whole (the card cuts it and a
// "yes" approves all of it) and neither it nor what the title is made of may
// hold controls, bidi or invisible characters. When it is false the hook does
// not ask the phone: the question stays on the terminal.
func CardSafe(tool string, raw json.RawMessage) bool {
	d := fullDetail(tool, raw)
	if len(d) > maxDetail || !detailShown(d) {
		return false
	}
	in := input(raw)
	for _, k := range []string{"description", "pattern", "query"} {
		if !shown(str(in, k)) {
			return false
		}
	}
	if tool == "Bash" && !bashRestOff(in) {
		return false
	}
	switch tool {
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		if configPath(str(in, "file_path")) || configPath(str(in, "notebook_path")) {
			return false
		}
	}
	return true
}

// configPath reports whether a file tool would write what configures the agent
// or git: a component .claude or .git, or a .mcp.json (V04, round 1). The card
// shows the path, not the content, and such a write could add rules, hooks or
// servers that skip every later question. It looks at every component as
// written, in any case and with either slash, so it holds before and after
// cleaning the path (cleaning only drops components) and when one of them is a
// link. Windows also reaches a name with trailing dots or spaces, a stream
// (name:…) or its 8.3 short name (CLAUDE~1, GIT~1, MCP~1.JSO).
func configPath(p string) bool {
	for _, c := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		c = strings.ToLower(c)
		if i := strings.IndexByte(c, ':'); i >= 0 {
			c = c[:i]
		}
		c = strings.TrimRight(c, ". ")
		switch c {
		case ".claude", ".git", ".mcp.json":
			return true
		}
		for _, short := range []string{"claude~", "git~", "mcp~"} {
			if rest, ok := strings.CutPrefix(c, short); ok && rest != "" && rest[0] >= '0' && rest[0] <= '9' {
				return true
			}
		}
	}
	return false
}

// bashShown are the fields of a Bash input its card accounts for: the command
// is the detail, the description the title, and the timeout does not change
// what runs.
var bashShown = map[string]bool{"command": true, "description": true, "timeout": true}

// bashRestOff reports whether every other field of a Bash input is off (absent,
// null or false). One that is on may change how the command runs where the
// card does not say (dangerouslyDisableSandbox runs it out of the sandbox,
// run_in_background leaves it running after the answer), and so may one this
// host does not know: a "yes" would approve the whole input (V04).
func bashRestOff(in map[string]any) bool {
	for k, v := range in {
		if !bashShown[k] && v != nil && v != false {
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
// them; su, sudo, doas, pkexec, run0 and expo are short or common inside other words and are
// searched as words. No lexical filter covers `make install`, a script of the
// project or an alias made before with `ln -s …/adb /tmp/x` (spec M4 §5.3).
var (
	phoneSubstrings = []string{"adb", "adbuddy", "fastboot", "scrcpy", "gradle", "platform-tools", "android_serial", "android", "flutter", "bundletool", "react-native"}
	phoneWords      = []string{"su", "sudo", "doas", "pkexec", "run0", "expo"}
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
