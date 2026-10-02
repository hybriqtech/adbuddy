package classify

import (
	"path"
	"strings"
)

// StdinDecides reports whether what an adb command does may come from its
// stdin: the agent's, which the daemon forwards to adb and nobody sees. The
// key of "allow during the session" is the rule and the arguments, never the
// stdin, so such a command is never allowed for the session (A4-1).
//
// It is a second barrier, independent of the rules and of the shell
// evaluator: it only looks at the words, and errs on the side of yes. A yes
// only costs asking again. argv is without adb's global options (SplitGlobal).
//
// Yes for: adb shell or exec-out without a command (an interactive shell) or
// with options ADBuddy does not know; adb exec-in (it pipes stdin into the
// command); a line on the phone that names a shell, su, eval, source or `.`
// (code from stdin), xargs, read or a command substitution (arguments from
// stdin), or a program that copies its stdin (cat, dd, tee), stdin itself
// (-, /dev/stdin, /proc/self/fd/0, the terminal) or an install or content
// write that may read it; any other adb command with -, /dev/stdin… as an
// operand.
func StdinDecides(argv []string) bool {
	for len(argv) > 0 && strings.HasPrefix(argv[0], "wait-for-") {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return false
	}
	switch argv[0] {
	case "exec-in":
		return true
	case "shell", "exec-out":
		args := argv[1:]
		if argv[0] == "shell" {
			i := 0
			for i < len(args) && strings.HasPrefix(args[i], "-") {
				switch args[i] {
				case "-n", "-T", "-t", "-tt", "-x":
					i++
				case "-e":
					i += 2
				default:
					return true // an option adb may read and ADBuddy does not
				}
			}
			args = args[min(i, len(args)):]
		}
		line := strings.Join(args, " ")
		return strings.TrimSpace(line) == "" || lineReadsStdin(line)
	}
	for _, a := range argv[1:] {
		if stdinPath(a) {
			return true
		}
	}
	return false
}

// stdinReaders are programs that, named anywhere on the line, may take code,
// arguments or contents from stdin.
var stdinReaders = map[string]bool{
	// code
	"sh": true, "mksh": true, "bash": true, "ash": true, "dash": true, "zsh": true, "ksh": true,
	"su": true, "eval": true, "source": true, ".": true,
	// arguments and variables
	"xargs": true, "read": true,
	// contents: the programs that copy stdin as is. A filter (sed, awk…)
	// that writes its stdin to a file is like adb push of a local file that
	// changed since it was allowed: the same command on other contents.
	"cat": true, "dd": true, "tee": true,
}

// lineReadsStdin looks for stdinReaders and stdin itself in a shell line,
// with quotes and backslashes removed (s'h' is sh on the phone).
func lineReadsStdin(line string) bool {
	if strings.Contains(line, "$(") || strings.Contains(line, "`") || strings.Contains(line, "<(") {
		return true // a substitution may read stdin: $(cat), $(head -1)…
	}
	plain := strings.NewReplacer(`'`, "", `"`, "", `\`, "").Replace(line)
	words := strings.FieldsFunc(plain, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || strings.ContainsRune(";|&()<>{}=", r)
	})
	install, sizeFromStdin, content, write := false, false, false, false
	for _, w := range words {
		if stdinPath(w) {
			return true
		}
		base := w
		if strings.Contains(w, "/") {
			base = path.Base(w)
		}
		if stdinReaders[base] {
			return true
		}
		switch {
		case strings.HasPrefix(base, "install"):
			install = true // pm install, cmd package install-write…
		case w == "-S":
			sizeFromStdin = true // the APK comes through stdin
		case base == "content":
			content = true
		case w == "write":
			write = true // content write copies stdin to the URI
		}
	}
	return install && sizeFromStdin || content && write
}

// stdinPath is a file operand that is stdin itself (or the terminal adb shell
// -t reads from).
func stdinPath(w string) bool {
	return w == "-" || strings.Contains(w, "/dev/stdin") || strings.Contains(w, "/fd/0") ||
		strings.Contains(w, "/dev/tty") || strings.Contains(w, "/dev/pts")
}
