package classify

import (
	"path"
	"strings"
)

// trustedDirs hold the phone's own programs: /system/bin/rm is still rm.
var trustedDirs = map[string]bool{
	"/system/bin/": true, "/system/xbin/": true, "/vendor/bin/": true, "/product/bin/": true,
	"/system_ext/bin/": true, "/apex/com.android.runtime/bin/": true,
}

// evalWords classifies one simple command, unwrapping wrappers such as
// nohup, env, toybox or `sh -c`.
func (c *Classifier) evalWords(ws []Word, sc scope) []Finding {
	vals := values(ws)
	if ws[0].Dynamic {
		return []Finding{unanalysable("dynamic", "command name only known on the phone", vals, textDynamic)}
	}
	name := ws[0].Value
	if strings.Contains(name, "/") {
		dir, base := path.Split(path.Clean(name))
		if !trustedDirs[dir] || base == "" {
			return []Finding{unanalysable("exec.file", "runs a file", vals, textScript)}
		}
		name = base
	}
	ws = append([]Word{{Value: name}}, ws[1:]...)

	switch name {
	case "sh", "mksh", "bash", "ash", "dash", "zsh", "ksh":
		return c.evalShell(ws, sc)
	case "su":
		return c.evalSu(ws, sc)
	case ".", "source", "eval":
		return []Finding{unanalysable("shell.script", "runs code from a file or a string", vals, textScript)}
	case "trap":
		if trapRunsCode(ws[1:]) {
			return []Finding{unanalysable("shell.script", "trap runs a string as shell code", vals, textScript)}
		}
	case "alias":
		// An alias makes a name run other shell code from the next line on.
		if len(ws) > 1 {
			return []Finding{unanalysable("shell.script", "alias makes a name run other shell code", vals, textScript)}
		}
	case "find":
		return c.evalFind(ws, sc)
	case "watch":
		return c.evalWatch(ws, sc)
	case "flock":
		return c.evalFlock(ws, sc)
	case "script":
		return c.evalScriptCmd(ws, sc)
	case "fc", "r":
		// fc -e and fc -s run commands from the shell history, after an
		// editor that is itself a command; mksh's r is fc -e -. Only a
		// listing (fc -l) runs nothing.
		if name == "r" || !fcLists(ws[1:]) {
			return []Finding{unanalysable("shell.script", "fc runs commands from the shell history", vals, textScript)}
		}
	case "env", "read", "getopts", "set", "printf", "unset":
		if why := setsHiddenVar(ws); why != "" {
			return []Finding{unanalysable("shell.script", why, vals, textScript)}
		}
	case "toybox", "busybox":
		if len(ws) == 1 {
			return []Finding{{Rule: "toybox.list", Level: Green, Title: Text{EN: "List toybox commands", ES: "Listar los comandos de toybox"}, Words: vals}}
		}
		return c.evalWords(ws[1:], sc)
	case "cmd":
		rest := ws[1:]
		for len(rest) > 0 && !rest[0].Dynamic && rest[0].Value == "-w" { // -w: wait for the service
			rest = rest[1:]
		}
		if len(rest) > 0 && (rest[0].Dynamic || strings.HasPrefix(rest[0].Value, "-")) {
			f := redFinding("cmd.option", "cmd with an option or a service only known on the phone", vals, textUnknown)
			f.Special = true
			return []Finding{f}
		}
		if len(rest) > 0 {
			if alias, ok := cmdAliases[rest[0].Value]; ok {
				return c.evalWords(append(lits(alias), rest[1:]...), sc)
			}
		}
		ws = append([]Word{ws[0]}, rest...)
	}
	switch ws[0].Value {
	case "run-as":
		// A package only known on the phone may be ADBuddy's.
		if len(ws) > 1 && (ws[1].Dynamic || strings.Contains(strings.ToLower(ws[1].Value), ADBuddyPackage)) {
			f := redFinding("run-as.adbuddy", "reads or changes the ADBuddy app's private files", vals, textRunAsADBuddy)
			f.Special = true
			return []Finding{f}
		}
	case "setsid", "daemonize", "start-stop-daemon", "disown":
		// toybox setsid returns at once unless -w, and it still detaches the
		// command from adb's session: judged the same whatever the options.
		return []Finding{backgroundFinding(ws[0].Value+" detaches a process", vals)}
	}
	if inner, handled, reason := unwrap(ws); handled {
		if reason != "" {
			return []Finding{unanalysable("wrapper", reason, vals, textUnknown)}
		}
		if len(inner) == 0 {
			return []Finding{{Rule: "wrapper.noop", Level: Green, Title: render(Text{EN: "Run {cmd}", ES: "Ejecutar {cmd}"}, baseVars(ws)), Words: vals}}
		}
		return c.evalWords(inner, sc)
	}
	out := []Finding{c.evalRule("shell", ws)}
	switch ws[0].Value {
	case "test", "[":
		out = append(out, sc.vars.testFindings(ws)...)
	case "shift", "ulimit":
		out = append(out, sc.vars.argsFindings(ws)...)
	case "integer", "typeset", "export", "readonly", "local", "declare":
		// Here only when run through a wrapper (builtin typeset …) or, for
		// integer, always: the parser reads the others as declarations
		// (checkDeclClause).
		out = append(out, declArgs(ws)...)
	}
	return out
}

// evalWatch handles watch [-tebx] [-n SEC] PROG ARGS. Without -x, watch
// joins its arguments with spaces and runs them with sh -c, like adb shell.
func (c *Classifier) evalWatch(ws []Word, sc scope) []Finding {
	vals := values(ws)
	bad := func(why string) []Finding { return []Finding{unanalysable("wrapper", why, vals, textUnknown)} }
	rest, direct := ws[1:], false
	for len(rest) > 0 && (rest[0].Dynamic || strings.HasPrefix(rest[0].Value, "-")) {
		v := rest[0]
		switch {
		case v.Dynamic:
			return bad("watch with an argument only known on the phone")
		case v.Value == "-n":
			if len(rest) < 2 || rest[1].Dynamic {
				return bad("watch -n without a static value")
			}
			rest = rest[2:]
		case strings.HasPrefix(v.Value, "-n"):
			rest = rest[1:]
		case len(v.Value) > 1 && strings.Trim(v.Value[1:], "tebx") == "":
			direct = direct || strings.Contains(v.Value, "x")
			rest = rest[1:]
		default:
			return bad("unknown watch option " + v.Value)
		}
	}
	if len(rest) == 0 {
		return []Finding{{Rule: "wrapper.noop", Level: Green, Title: render(Text{EN: "Run {cmd}", ES: "Ejecutar {cmd}"}, baseVars(ws)), Words: vals}}
	}
	if direct {
		return c.evalWords(rest, sc)
	}
	for _, w := range rest {
		if w.Dynamic {
			return []Finding{unanalysable("shell.script", "watch runs a command line only known on the phone", vals, textScript)}
		}
	}
	return c.classifyScript(strings.Join(values(rest), " "), sc.child())
}

// evalFlock handles flock [-sxunoeF] [-w SEC] [-E N] FILE|FD [COMMAND… |
// -c CMD]. A lock file that is not a descriptor is created if missing.
func (c *Classifier) evalFlock(ws []Word, sc scope) []Finding {
	vals := values(ws)
	bad := func(why string) []Finding { return []Finding{unanalysable("wrapper", why, vals, textUnknown)} }
	rest := ws[1:]
	for len(rest) > 0 && (rest[0].Dynamic || strings.HasPrefix(rest[0].Value, "-")) {
		v := rest[0]
		switch {
		case v.Dynamic:
			return bad("flock with an argument only known on the phone")
		case v.Value == "-w" || v.Value == "-E":
			if len(rest) < 2 || rest[1].Dynamic {
				return bad("flock option without a static value")
			}
			rest = rest[2:]
		case len(v.Value) > 1 && strings.Trim(v.Value[1:], "sxunoeF") == "":
			rest = rest[1:]
		default:
			return bad("unknown flock option " + v.Value)
		}
	}
	if len(rest) == 0 {
		return []Finding{{Rule: "wrapper.noop", Level: Green, Title: render(Text{EN: "Run {cmd}", ES: "Ejecutar {cmd}"}, baseVars(ws)), Words: vals}}
	}
	target, rest := rest[0], rest[1:]
	var out []Finding
	if !isDigits(target.Value) && target.Value != "/dev/null" {
		out = append(out, redFinding("shell.redirect", "flock creates the lock file "+target.Value, []string{"flock", target.Value}, textRedirect))
	}
	switch {
	case len(rest) == 0:
		if len(out) == 0 {
			return []Finding{{Rule: "wrapper.noop", Level: Green, Title: render(Text{EN: "Run {cmd}", ES: "Ejecutar {cmd}"}, baseVars(ws)), Words: vals}}
		}
		return out
	case !rest[0].Dynamic && (rest[0].Value == "-c" || rest[0].Value == "--command"):
		if len(rest) != 2 {
			return bad("flock -c takes one command line")
		}
		return append(out, c.evalShell([]Word{{Value: "sh"}, {Value: "-c"}, rest[1]}, sc)...)
	case !rest[0].Dynamic && strings.HasPrefix(rest[0].Value, "-"):
		return bad("unknown flock option " + rest[0].Value)
	}
	return append(out, c.evalWords(rest, sc)...)
}

// evalScriptCmd handles util-linux script [-aefq] -c CMD [FILE]: it runs
// CMD with $SHELL -c and writes the session to FILE (./typescript by
// default). Without -c it opens a shell.
func (c *Classifier) evalScriptCmd(ws []Word, sc scope) []Finding {
	vals := values(ws)
	bad := func(why string) []Finding { return []Finding{unanalysable("wrapper", why, vals, textUnknown)} }
	var cmd *Word
	var files []Word
	for i := 1; i < len(ws); i++ {
		v := ws[i]
		switch {
		case v.Dynamic:
			return bad("script with an argument only known on the phone")
		case v.Value == "-c" || v.Value == "--command":
			if i+1 >= len(ws) {
				return bad("script -c without a command")
			}
			i++
			cmd = &ws[i]
		case len(v.Value) > 1 && strings.HasPrefix(v.Value, "-") && strings.Trim(v.Value[1:], "aefq") == "":
		case strings.HasPrefix(v.Value, "-"):
			return bad("unknown script option " + v.Value)
		default:
			files = append(files, v)
		}
	}
	if cmd == nil {
		return bad("script without -c opens a shell")
	}
	if len(files) > 1 {
		return bad("script with more than one file")
	}
	file := "typescript"
	if len(files) == 1 {
		file = files[0].Value
	}
	var out []Finding
	if file != "/dev/null" {
		out = append(out, redFinding("shell.redirect", "script writes the session to "+file, []string{"script", file}, textRedirect))
	}
	return append(out, c.evalShell([]Word{{Value: "sh"}, {Value: "-c"}, *cmd}, sc)...)
}

// backgroundFinding is for anything that keeps running on the phone after
// the command returns: later it could press "Allow" on ADBuddy's confirmation
// (a delayed input tap), so it is always special.
func backgroundFinding(reason string, words []string) Finding {
	f := redFinding("shell.background", reason, words, textBackground)
	f.Special = true
	return f
}

// unanalysable is for what the classifier cannot read: a command name only
// known on the phone, eval or `.`, a shell reading a file or stdin, running a
// file, syntax it cannot parse, too many nested shells or a wrapper it cannot
// read. Any of them could hide ADBuddy's own channel or a delayed tap, so it
// fails closed: special, with no "allow during the session".
func unanalysable(rule, reason string, words []string, title Text) Finding {
	f := redFinding(rule, reason, words, title)
	f.Special = true
	return f
}

// findRuns are find's actions that run a command, ended by ";" (or by "+"
// right after "{}").
var findRuns = map[string]bool{"-exec": true, "-execdir": true, "-ok": true, "-okdir": true}

// evalFind classifies find with its rule and, as commands of their own, the
// ones it runs with -exec, -execdir, -ok or -okdir, where {} is a file name
// only known on the phone. A word of find's own only known on the phone may
// split into one of those actions: that cannot be analysed.
func (c *Classifier) evalFind(ws []Word, sc scope) []Finding {
	vals := values(ws)
	var runs [][]Word
	ownDynamic := false
	for i := 1; i < len(ws); i++ {
		w := ws[i]
		if w.Dynamic {
			ownDynamic = true
			continue
		}
		if !findRuns[w.Value] {
			continue
		}
		var cmd []Word
		for i++; i < len(ws); i++ {
			v := ws[i]
			if !v.Dynamic && (v.Value == ";" || (v.Value == "+" && len(cmd) > 0 && cmd[len(cmd)-1].Value == "{}")) {
				break
			}
			if strings.Contains(v.Value, "{}") {
				v.Dynamic = true
			}
			cmd = append(cmd, v)
		}
		runs = append(runs, cmd)
	}
	var out []Finding
	if ownDynamic {
		out = append(out, unanalysable("wrapper", "find with an argument only known on the phone may run a command", vals, textDynamic))
	} else {
		out = append(out, c.evalRule("shell", ws))
	}
	for _, cmd := range runs {
		if len(cmd) == 0 {
			out = append(out, unanalysable("wrapper", "find runs a command it does not name", vals, textUnknown))
			continue
		}
		out = append(out, c.evalWords(cmd, sc)...)
	}
	return out
}

// cmdAliases maps `cmd <service>` to the equivalent command.
var cmdAliases = map[string][]string{
	"package":    {"pm"},
	"appops":     {"appops"},
	"settings":   {"settings"},
	"deviceidle": {"dumpsys", "deviceidle"},
	"activity":   {"am"},
}

// unwrap strips commands that run another command. handled=false means ws is
// not a wrapper; a non-empty reason means it cannot be analysed. A dynamic
// word the wrapper itself consumes (an option, its value, a duration, a
// package, an assignment) is refused: on the phone an unquoted $(…) splits
// into several words and one of them could become the command.
func unwrap(ws []Word) (inner []Word, handled bool, reason string) {
	inner, start, handled, reason := unwrapWords(ws)
	if !handled || reason != "" {
		return inner, handled, reason
	}
	for _, w := range ws[1:start] {
		if w.Dynamic {
			return nil, true, ws[0].Value + " with an argument only known on the phone"
		}
	}
	return inner, true, ""
}

// unwrapWords returns the inner command and start, the index in ws where it
// begins (len(ws) when there is none): ws[1:start] are the wrapper's own
// words. inner may hold words that are not in ws (xargs adds its own).
func unwrapWords(ws []Word) (inner []Word, start int, handled bool, reason string) {
	rest := ws[1:]
	at := func(r []Word) int { return len(ws) - len(r) }
	switch ws[0].Value {
	case "exec", "nohup", "time":
		if len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			return nil, 0, true, ws[0].Value + " with options"
		}
		return rest, at(rest), true, ""
	case "nice":
		if len(rest) > 1 && rest[0].Value == "-n" {
			rest = rest[2:]
		} else if len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			rest = rest[1:]
		}
		return rest, at(rest), true, ""
	case "timeout":
		for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			switch rest[0].Value {
			case "-s", "-k":
				if len(rest) < 2 {
					return nil, 0, true, "timeout option without value"
				}
				rest = rest[2:]
			case "-i", "-v", "--preserve-status", "--foreground":
				rest = rest[1:]
			default:
				return nil, 0, true, "unknown timeout option " + rest[0].Value
			}
		}
		if len(rest) == 0 {
			return nil, 0, true, "timeout without command"
		}
		rest = rest[1:] // skip the duration
		return rest, at(rest), true, ""
	case "env":
		for len(rest) > 0 {
			v := rest[0].Value
			switch {
			case v == "-i" || v == "-0":
				rest = rest[1:]
			case v == "-u":
				if len(rest) < 2 {
					return nil, 0, true, "env -u without name"
				}
				rest = rest[2:]
			case strings.HasPrefix(v, "-"):
				return nil, 0, true, "unknown env option " + v
			case strings.Contains(v, "="):
				name := v[:strings.Index(v, "=")]
				if dangerousVar(name) {
					return nil, 0, true, "sets " + name
				}
				rest = rest[1:]
			default:
				return rest, at(rest), true, ""
			}
		}
		return nil, len(ws), true, ""
	case "command":
		if len(rest) > 0 && (rest[0].Value == "-v" || rest[0].Value == "-V") {
			return nil, len(ws), true, ""
		}
		if len(rest) > 0 && rest[0].Value == "-p" {
			rest = rest[1:]
		}
		return rest, at(rest), true, ""
	case "builtin":
		// builtin [--] name args…: runs a shell builtin, like command.
		if len(rest) > 0 && rest[0].Value == "--" {
			rest = rest[1:]
		} else if len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			return nil, 0, true, "builtin with options"
		}
		return rest, at(rest), true, ""
	case "run-as":
		// run-as <package> [--user <id>] <command…>
		if len(rest) < 2 {
			return nil, 0, true, "run-as without command opens a shell"
		}
		rest = rest[1:]
		if rest[0].Value == "--user" {
			if len(rest) < 3 {
				return nil, 0, true, "run-as without command opens a shell"
			}
			rest = rest[2:]
		}
		return rest, at(rest), true, ""
	case "xargs":
		repl := ""
		for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			switch v := rest[0].Value; v {
			case "-0", "-r", "-t", "-x", "-p":
				rest = rest[1:]
			case "-n", "-s", "-P", "-L", "-d", "-E":
				if len(rest) < 2 {
					return nil, 0, true, "xargs option without value"
				}
				rest = rest[2:]
			case "-I":
				if len(rest) < 2 {
					return nil, 0, true, "xargs option without value"
				}
				repl = rest[1].Value
				rest = rest[2:]
			default:
				return nil, 0, true, "unknown xargs option " + v
			}
		}
		start := at(rest)
		if len(rest) == 0 {
			rest = []Word{{Value: "echo"}} // xargs without a command runs echo
		}
		inner := make([]Word, len(rest))
		copy(inner, rest)
		for i := range inner {
			if repl != "" && strings.Contains(inner[i].Value, repl) {
				inner[i].Dynamic = true
			}
		}
		// xargs appends arguments read from stdin: unknown in advance.
		return append(inner, Word{Value: "<stdin>", Dynamic: true}), start, true, ""
	case "logwrapper":
		// logwrapper [-a] [-d] [-k] PROG ARGS
		for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			if !flagsIn(rest[0].Value, "adk") {
				return nil, 0, true, "unknown logwrapper option " + rest[0].Value
			}
			rest = rest[1:]
		}
		return rest, at(rest), true, ""
	case "taskset":
		// taskset [-a] MASK COMMAND…; with -p it changes another process.
		for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			if !flagsIn(rest[0].Value, "a") {
				return nil, 0, true, "taskset " + rest[0].Value + " is not a command wrapper"
			}
			rest = rest[1:]
		}
		if len(rest) > 0 {
			rest = rest[1:] // the mask
		}
		return rest, at(rest), true, ""
	case "chrt":
		// chrt [-Rofrbi] PRIORITY COMMAND…; -p changes another process and
		// -m only prints the limits.
		for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			if !flagsIn(rest[0].Value, "Rofrbi") {
				return nil, 0, true, "chrt " + rest[0].Value + " is not a command wrapper"
			}
			rest = rest[1:]
		}
		if len(rest) > 0 {
			rest = rest[1:] // the priority
		}
		return rest, at(rest), true, ""
	case "ionice":
		// ionice [-t] [-c CLASS] [-n LEVEL] [COMMAND…]; -p changes another
		// process.
		for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			switch v := rest[0].Value; {
			case v == "-t":
				rest = rest[1:]
			case v == "-c" || v == "-n":
				if len(rest) < 2 {
					return nil, 0, true, "ionice option without value"
				}
				rest = rest[2:]
			case strings.HasPrefix(v, "-c") || strings.HasPrefix(v, "-n"):
				rest = rest[1:]
			default:
				return nil, 0, true, "ionice " + v + " is not a command wrapper"
			}
		}
		return rest, at(rest), true, ""
	case "runcon":
		// runcon CONTEXT COMMAND…
		if len(rest) < 2 || strings.HasPrefix(rest[0].Value, "-") {
			return nil, 0, true, "runcon without a context and a command"
		}
		rest = rest[1:]
		return rest, at(rest), true, ""
	case "nsenter", "unshare":
		// nsenter [-t PID] [-FimnpuU] COMMAND…; unshare [-fimnpruU]
		// COMMAND…. Without a command they open $SHELL. Entering another
		// process's mount namespace may make the command another file.
		flags := "fimnpruU"
		if ws[0].Value == "nsenter" {
			flags = "FinpuU"
		}
		for len(rest) > 0 && strings.HasPrefix(rest[0].Value, "-") {
			v := rest[0].Value
			switch {
			case ws[0].Value == "nsenter" && v == "-t":
				if len(rest) < 2 {
					return nil, 0, true, "nsenter -t without a process"
				}
				rest = rest[2:]
			case ws[0].Value == "nsenter" && strings.HasPrefix(v, "-t") && isDigits(v[2:]):
				rest = rest[1:]
			case flagsIn(v, flags):
				rest = rest[1:]
			default:
				return nil, 0, true, ws[0].Value + " " + v + " may run the command from another file system or is unknown"
			}
		}
		if len(rest) == 0 {
			return nil, 0, true, ws[0].Value + " without a command opens a shell"
		}
		return rest, at(rest), true, ""
	case "chroot":
		// chroot NEWROOT [COMMAND…]: every program then comes from
		// NEWROOT, files the classifier never sees (and without a command
		// it opens $SHELL).
		return nil, 0, true, "chroot runs programs from another root directory"
	}
	return nil, 0, false, ""
}

// flagsIn reports a short option cluster (-ab…) made only of these letters.
func flagsIn(v, letters string) bool {
	return len(v) > 1 && v[0] == '-' && strings.Trim(v[1:], letters) == ""
}

// evalShell handles `sh [-opts] -c 'script' [args…]`; any other use runs a
// file or reads the script from stdin, which cannot be analysed.
func (c *Classifier) evalShell(ws []Word, sc scope) []Finding {
	vals := values(ws)
	rest := ws[1:]
	for len(rest) > 0 && (strings.HasPrefix(rest[0].Value, "-") || strings.HasPrefix(rest[0].Value, "+")) && !rest[0].Dynamic {
		opts := rest[0].Value[1:]
		rest = rest[1:]
		if strings.ContainsAny(opts, "is") || opts == "" || opts == "-" {
			return []Finding{unanalysable("shell.script", "shell reading commands from stdin", vals, textScript)}
		}
		if strings.Contains(opts, "c") {
			if len(rest) == 0 || rest[0].Dynamic {
				return []Finding{unanalysable("shell.script", "sh -c with a script only known on the phone", vals, textScript)}
			}
			return c.classifyScript(rest[0].Value, sc.child())
		}
		if strings.Trim(opts, "eufxvnaC") != "" {
			return []Finding{unanalysable("shell.script", "unknown shell option", vals, textScript)}
		}
	}
	if len(rest) == 0 {
		return []Finding{unanalysable("shell.script", "shell reading commands from stdin", vals, textScript)}
	}
	return []Finding{unanalysable("shell.script", "runs a script file", vals, textScript)}
}

// evalSu handles `su [user] -c 'script'` and `su [user] command…`.
func (c *Classifier) evalSu(ws []Word, sc scope) []Finding {
	vals := values(ws)
	rest := ws[1:]
	if len(rest) > 0 && !rest[0].Dynamic && isSuUser(rest[0].Value) {
		rest = rest[1:]
	}
	switch {
	case len(rest) == 0:
		return []Finding{unanalysable("shell.script", "su without command opens a root shell", vals, textScript)}
	case rest[0].Value == "-c":
		if len(rest) < 2 || rest[1].Dynamic {
			return []Finding{unanalysable("shell.script", "su -c with a script only known on the phone", vals, textScript)}
		}
		return c.classifyScript(rest[1].Value, sc.child())
	case strings.HasPrefix(rest[0].Value, "-"):
		return []Finding{unanalysable("shell.script", "unknown su option", vals, textScript)}
	}
	return c.evalWords(rest, sc)
}

func isSuUser(s string) bool {
	if s == "root" || s == "shell" || s == "system" {
		return true
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != ',' {
			return false
		}
	}
	return s != ""
}
