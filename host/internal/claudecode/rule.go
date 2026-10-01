package claudecode

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
	in := input(raw)
	var d string
	switch {
	case tool == "Bash" && str(in, "command") != "":
		d = str(in, "command")
	case str(in, "file_path") != "":
		d = str(in, "file_path")
	case str(in, "notebook_path") != "":
		d = str(in, "notebook_path")
	case str(in, "url") != "":
		d = str(in, "url")
	default:
		d = string(raw)
	}
	return cut(d, maxDetail)
}

// AlwaysRule returns the rule "yes, always" would add, and whether it may be
// offered at all (spec M4 §5.3). It is the exact command of a Bash and nothing
// broader: no wildcard is ever built from it. It is refused (false) for any
// other tool; for a command that is long, spans lines, has a character that
// changes the syntax of a Claude Code rule or chains commands; and for any
// command that could touch adb, the shim or the phone, whose every call the
// shim classifies and Claude Code must not wave through for good.
func AlwaysRule(tool string, raw json.RawMessage) (string, bool) {
	if tool != "Bash" {
		return "", false
	}
	cmd := strings.TrimSpace(str(input(raw), "command"))
	if cmd == "" || len(cmd) > maxRule || !utf8.ValidString(cmd) {
		return "", false
	}
	if strings.ContainsAny(cmd, "\r\n\t()*\\\"';&|$`<>") || strings.Contains(cmd, ":*") {
		return "", false
	}
	low := strings.ToLower(cmd)
	for _, w := range []string{"adb", "android_serial", "adbuddy"} {
		if strings.Contains(low, w) {
			return "", false
		}
	}
	return cmd, true
}
