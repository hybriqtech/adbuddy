package classify

import (
	"path"
	"regexp"
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
// It is an allowlist (H1): no only for an adb command ADBuddy knows does not
// read stdin (inertAdb) with no stdin operand (-, /dev/stdin…), and for a
// line on the phone whose every program is known not to read stdin
// (nonReaders) or to read it only as data it filters to its output
// (dataReaders), none of which writes it to a file. Yes for anything else:
// adb shell or exec-out without a command or with options ADBuddy does not
// know, adb exec-in, abb_exec and abb, a program on the line that is not in those lists (awk,
// sed, a shell, a wrapper, one ADBuddy does not know…), and, wherever on the
// line, a shell, su, eval, source, `.`, xargs, read, dd, tee, a command
// substitution, stdin itself or an install or content write that may read it.
func StdinDecides(argv []string) bool {
	for len(argv) > 0 && strings.HasPrefix(argv[0], "wait-for-") {
		argv = argv[1:]
	}
	if len(argv) == 0 {
		return false
	}
	switch argv[0] {
	case "exec-in", "abb_exec", "abb":
		return true // stdin goes to the command, or to a system service (H2)
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
	if !inertAdb[argv[0]] {
		return true
	}
	for _, a := range argv[1:] {
		if stdinPath(a) {
			return true
		}
	}
	return false
}

// inertAdb are the adb commands that do not hand their stdin to anything on
// the phone: they read files on the PC, or nothing (an operand that is stdin
// itself is looked at apart). pair is not here: it may read the pairing code
// from stdin.
var inertAdb = map[string]bool{
	"devices": true, "version": true, "help": true, "get-state": true, "get-serialno": true, "get-devpath": true,
	"start-server": true, "kill-server": true, "features": true, "host-features": true, "mdns": true, "jdwp": true,
	"pull": true, "bugreport": true, "logcat": true, "reverse": true, "forward": true,
	"install": true, "install-multiple": true, "install-multi-package": true, "uninstall": true, "push": true,
	"sync": true, "sideload": true, "restore": true, "backup": true, "connect": true, "disconnect": true, "reconnect": true,
	"remount": true, "usb": true, "tcpip": true, "reboot": true, "root": true, "unroot": true,
	"disable-verity": true, "enable-verity": true,
}

// stdinReaders are programs that, named anywhere on the line, may take code,
// arguments or contents from stdin: a first look that does not need to know
// where commands start.
var stdinReaders = map[string]bool{
	// code
	"sh": true, "mksh": true, "bash": true, "ash": true, "dash": true, "zsh": true, "ksh": true,
	"su": true, "eval": true, "source": true, ".": true,
	// arguments and variables
	"xargs": true, "read": true,
	// contents: programs that copy stdin to a file
	"dd": true, "tee": true,
}

// nonReaders are the programs on the phone known not to read stdin, or to
// read from it only the answer to a question about the operands they were
// given (rm -i, cp -i, mv -i), which the key covers. The system's services
// (settings, am, pm, wm, input, appops, svc, sm, dumpsys, service) get their
// arguments, not stdin; cmd is not here: it hands stdin to any service, and
// some read it. pm and content stay here for their other subcommands: pm's
// installs (install, install-write…) are not inert (inertCommand), and
// content write is looked at in lineReadsStdin.
var nonReaders = map[string]bool{
	// files
	"rm": true, "rmdir": true, "mv": true, "cp": true, "ln": true, "mkdir": true, "touch": true,
	"chmod": true, "chown": true, "chgrp": true, "ls": true, "stat": true, "du": true, "df": true,
	"realpath": true, "readlink": true, "basename": true, "dirname": true, "pwd": true, "cd": true,
	// output and tests
	"echo": true, "printf": true, "true": true, "false": true, ":": true, "test": true, "[": true,
	"sleep": true, "date": true, "id": true, "whoami": true, "uname": true, "uptime": true, "nproc": true,
	"getprop": true, "getenforce": true, "exit": true, "return": true,
	// the system
	"reboot": true, "setprop": true, "kill": true, "killall": true, "pkill": true, "pidof": true, "pgrep": true,
	"ps": true, "sync": true, "svc": true, "sm": true, "settings": true, "input": true, "wm": true, "am": true,
	"pm": true, "appops": true, "dumpsys": true, "service": true, "content": true, "logcat": true,
	"screencap": true, "screenrecord": true,
}

// dataReaders are the filters that may read stdin, but only as data they
// write to their output: what they do does not depend on it. Each one,
// unless it writes that data to a file (sort -o, or an output redirection on
// the line; see lineReadsStdin): cat copies it, head and tail cut it, wc
// counts it, sort orders it, cut and tr transform it, and grep (also egrep
// and fgrep) selects lines of it (its patterns come from the arguments, or
// from -f with a file; -f - is stdin itself).
var dataReaders = map[string]bool{
	"cat": true, "head": true, "tail": true, "wc": true, "sort": true, "cut": true, "tr": true,
	"grep": true, "egrep": true, "fgrep": true,
}

// keywords keep the next word in the position of a command; the closing ones
// run nothing. for, case, select, function, [[ and (( are not here: what
// follows them is not a command, and ADBuddy does not follow them.
var keywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "do": true, "while": true, "until": true, "!": true, "time": true,
}

var closers = map[string]bool{"fi": true, "done": true}

// systemDirs are where the phone's own programs are; a program named by a
// path elsewhere may be anything.
var systemDirs = []string{"/system/bin/", "/system/xbin/", "/vendor/bin/"}

// assignment is one of a variable of the agent's own, in lower case: one of
// the shell's (PATH, IFS, LD_PRELOAD, ENV…) may change what the next program
// is or does, so it is not followed and the command counts as not inert.
var assignment = regexp.MustCompile(`^[a-z_][a-z0-9_]*=`)

// lineReadsStdin looks at a shell line with quotes and backslashes removed
// (s'h' is sh on the phone): stdinReaders and stdin itself anywhere, then
// every command on it (inertLine).
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
		case w == "-S" || strings.HasPrefix(w, "-S") && strings.Trim(w[2:], "0123456789") == "":
			sizeFromStdin = true // the APK comes through stdin: -S 1234 or -S1234
		case base == "content":
			content = true
		case w == "write":
			write = true // content write copies stdin to the URI
		}
	}
	if install && sizeFromStdin || content && write {
		return true
	}
	return !inertLine(plain)
}

// inertLine reports whether every command on a shell line is a nonReader or
// a dataReader, and no dataReader's input may end up in a file: then nothing
// on it depends on stdin. Anything it does not follow counts as not inert.
func inertLine(line string) bool {
	commandPos := true // the next word names a command
	redirect := false  // the next word is the target of a redirection
	out, dup := false, false
	filters, writes := false, false
	var name string
	var args []string
	inert := true
	end := func() {
		if name != "" && !inertCommand(name, args) {
			inert = false
		}
		if dataReaders[programName(name)] {
			filters = true
		}
		name, args, commandPos = "", nil, true
	}
	for _, t := range shellTokens(line) {
		switch {
		case t == "<" || t == ">":
			if !redirect {
				out, dup = false, false
			}
			out = out || t == ">" // >, >>, >| and <> (N1) open the file for writing
			redirect = true
		case redirect && (t == "&" || t == "|"):
			dup = dup || t == "&" // >&2, <&0: a file descriptor, not a file
		case len(t) == 1 && strings.ContainsAny(t, ";|&(){}\n"):
			redirect = false
			end()
		case redirect:
			redirect = false
			descriptor := dup && strings.Trim(t, "0123456789") == ""
			if out && !descriptor && t != "/dev/null" {
				writes = true // a file, or a duplication to something not a descriptor
			}
		case commandPos && (assignment.MatchString(t) || keywords[t]):
		case commandPos:
			name, commandPos = t, false
		default:
			args = append(args, t)
		}
	}
	if redirect {
		return false
	}
	end()
	return inert && !(filters && writes)
}

// inertCommand reports whether the command name with args is known not to
// depend on stdin.
func inertCommand(name string, args []string) bool {
	if closers[name] {
		return true
	}
	p := programName(name)
	switch {
	case p == "":
		return false
	case p == "pm":
		for _, a := range args {
			if strings.HasPrefix(a, "install") {
				return false // pm install and install-write may read the APK from stdin (H2)
			}
		}
	case p == "sort":
		for _, a := range args {
			if strings.HasPrefix(a, "-o") || strings.HasPrefix(a, "--output") {
				return false // writes what it read to a file
			}
		}
	}
	return nonReaders[p] || dataReaders[p]
}

// programName is the program a command word names: itself, or the last part
// of a path in the system's directories; "" for any other path.
func programName(w string) string {
	if !strings.Contains(w, "/") {
		return w
	}
	for _, d := range systemDirs {
		if strings.HasPrefix(w, d) && !strings.Contains(w[len(d):], "/") {
			return w[len(d):]
		}
	}
	return ""
}

// shellTokens splits a line (quotes already removed) into words and the
// one-character operators ; | & ( ) { } < > and newline.
func shellTokens(line string) []string {
	var toks []string
	var w strings.Builder
	flush := func() {
		if w.Len() > 0 {
			toks = append(toks, w.String())
			w.Reset()
		}
	}
	for _, r := range line {
		switch {
		case r == ' ' || r == '\t' || r == '\r':
			flush()
		case strings.ContainsRune(";|&(){}<>\n", r):
			flush()
			toks = append(toks, string(r))
		default:
			w.WriteRune(r)
		}
	}
	flush()
	return toks
}

// stdinPath is a file operand that is stdin itself (or the terminal adb shell
// -t reads from).
func stdinPath(w string) bool {
	return w == "-" || strings.Contains(w, "/dev/stdin") || strings.Contains(w, "/fd/0") ||
		strings.Contains(w, "/dev/tty") || strings.Contains(w, "/dev/pts")
}
