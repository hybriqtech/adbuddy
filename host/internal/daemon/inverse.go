package daemon

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// validInverse accepts only the inverse shapes package precheck writes. The
// journal lives in the agent's folder: even with signed records, undo never
// runs a command that precheck could not have produced.
func validInverse(argv []string, backupDir string) error {
	if len(argv) == 0 {
		return errors.New("empty inverse")
	}
	switch argv[0] {
	case "uninstall":
		if len(argv) == 2 && packageName(argv[1]) {
			return nil
		}
	case "push":
		if len(argv) == 3 && argv[1] == filepath.Join(backupDir, "original") && strings.HasPrefix(argv[2], "/") {
			return nil
		}
	case "shell":
		words := make([]string, 0, len(argv)-1)
		for _, tok := range argv[1:] {
			w, ok := unquote(tok)
			if !ok {
				return fmt.Errorf("unexpected word %q in an inverse", tok)
			}
			words = append(words, w)
		}
		if shellInverse(words) {
			return nil
		}
	}
	return fmt.Errorf("not an inverse ADBuddy writes: %q", argv)
}

// unquote reverses shq.Quote: safe characters, or single-quoted parts joined
// by \'. Anything else (spaces outside quotes, $, ;…) is refused.
func unquote(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < len(tok); {
		switch {
		case tok[i] == '\'':
			j := strings.IndexByte(tok[i+1:], '\'')
			if j < 0 {
				return "", false
			}
			b.WriteString(tok[i+1 : i+1+j])
			i += j + 2
		case strings.HasPrefix(tok[i:], `\'`):
			b.WriteByte('\'')
			i += 2
		case safeByte(tok[i]):
			b.WriteByte(tok[i])
			i++
		default:
			return "", false
		}
	}
	return b.String(), true
}

func safeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("_@%+=:,./-", c) >= 0
}

func packageName(s string) bool {
	if !strings.Contains(s, ".") || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_') {
			return false
		}
	}
	return true
}

func userID(s string) bool {
	if s == "current" || s == "cur" {
		return true
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func absPath(s string) bool { return strings.HasPrefix(s, "/") && !strings.ContainsRune(s, 0) }

// shellInverse matches the shell commands precheck builds as inverses.
func shellInverse(w []string) bool {
	n := len(w)
	withUser := func(i int) int { // skips an optional "--user U" at w[i]
		if i+1 < n && w[i] == "--user" && userID(w[i+1]) {
			return i + 2
		}
		return i
	}
	ns := func(s string) bool { return s == "system" || s == "secure" || s == "global" }
	switch {
	case n >= 1 && w[0] == "settings":
		i := withUser(1)
		switch {
		case i+4 == n && w[i] == "put" && ns(w[i+1]):
			return true
		case i+3 == n && w[i] == "delete" && ns(w[i+1]):
			return true
		}
	case n >= 3 && w[0] == "pm" && w[1] == "enable":
		i := withUser(2)
		return i+1 == n && packageName(w[i])
	case n == 6 && w[0] == "cmd" && w[1] == "package" && w[2] == "install-existing" && w[3] == "--user" && userID(w[4]):
		return packageName(w[5])
	case n >= 5 && w[0] == "appops" && w[1] == "set":
		i := withUser(2)
		if i+3 == n && packageName(w[i]) {
			switch w[i+2] {
			case "allow", "ignore", "deny", "default", "foreground", "errored":
				return true
			}
		}
	case n == 4 && w[0] == "dumpsys" && w[1] == "deviceidle" && w[2] == "whitelist":
		return len(w[3]) > 1 && (w[3][0] == '+' || w[3][0] == '-') && packageName(w[3][1:])
	case n == 3 && w[0] == "mv":
		return absPath(w[1]) && absPath(w[2])
	case n == 2 && w[0] == "rmdir":
		return absPath(w[1])
	case n == 3 && w[0] == "rm" && w[1] == "-f":
		return absPath(w[2])
	}
	return false
}
