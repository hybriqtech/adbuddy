// Package shq quotes values for the phone's shell. `adb shell` joins its
// arguments with spaces into one command line (spec §4.3), so every value the
// host puts in a remote command must be quoted: a `$`, a `;` or a space would
// otherwise corrupt the value or inject commands.
package shq

import "strings"

// Quote returns s as one shell word. Words made only of safe characters are
// returned unchanged so journal entries stay readable; anything else is
// wrapped in single quotes; a single quote inside the value closes the
// quoting, is written escaped and reopens it.
func Quote(s string) string {
	if s != "" && isSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Join quotes every word and joins them with spaces.
func Join(words ...string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = Quote(w)
	}
	return strings.Join(q, " ")
}

// Args quotes every word and returns them as separate adb arguments, ready to
// follow "shell" in an adb argv: adb joins them back with spaces.
func Args(words ...string) []string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = Quote(w)
	}
	return q
}

func isSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_@%+=:,./-", r):
		default:
			return false
		}
	}
	return true
}

// Unquote reverses Quote: safe characters, or single-quoted parts joined by
// \'. It accepts only the subset of shell words that Quote produces, plus
// harmless variants (such as a safe word wrapped in quotes); anything else
// (spaces outside quotes, $, ;…) is refused, as is the empty word. It works
// byte by byte, so a multibyte character outside quotes is refused.
func Unquote(tok string) (string, bool) {
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
