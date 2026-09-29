package classify

import (
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// DisplayCommand renders an adb command the way the phone will receive it,
// so the person confirming it reads what will really run: after `shell` adb
// joins the words with spaces; `exec-out` sends its first argument as is and
// quotes the rest; `wait-for-*` runs the rest of the line. Any other argument
// is one word and is quoted when needed. The result is escaped with Printable.
func DisplayCommand(argv []string) string {
	return "adb " + Printable(displayArgs(argv))
}

func displayArgs(argv []string) string {
	_, rest := SplitGlobal(argv)
	prefix := shq.Join(argv[:len(argv)-len(rest)]...)
	var body string
	switch {
	case len(rest) == 0:
	case rest[0] == "shell":
		body = strings.Join(rest, " ")
	case rest[0] == "exec-out" && len(rest) > 1:
		body = "exec-out " + rest[1]
		for _, a := range rest[2:] {
			body += " " + shq.Quote(a) // the same word adb sends, quoted only when needed
		}
	case strings.HasPrefix(rest[0], "wait-for-") && len(rest) > 1:
		body = rest[0] + " " + displayArgs(rest[1:])
	default:
		body = shq.Join(rest...)
	}
	if prefix == "" {
		return body
	}
	if body == "" {
		return prefix
	}
	return prefix + " " + body
}
