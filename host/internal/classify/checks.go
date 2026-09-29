package classify

import (
	"path"
	"strings"
)

// checkResult is what an argument check found: a minimum level, why, the
// change it recognised (if any) and extra title placeholders.
type checkResult struct {
	level  Level
	reason string
	op     *Op
	vars   map[string]string
}

func ok() checkResult               { return checkResult{level: Green} }
func red(reason string) checkResult { return checkResult{level: Red, reason: reason} }
func change(op *Op) checkResult     { return checkResult{level: Yellow, op: op, vars: op.Args} }
func newOp(kind string, kv ...string) *Op {
	args := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		args[kv[i]] = kv[i+1]
	}
	return &Op{Kind: kind, Args: args}
}

// checkFunc inspects a command's words.
type checkFunc func(ws []Word) checkResult

// checks are referenced by name from the rules. Each file registers its own.
var checks = map[string]checkFunc{}

func register(m map[string]checkFunc) {
	for name, f := range m {
		checks[name] = f
	}
}

func init() {
	register(map[string]checkFunc{
		"logcat":      checkLogcat,
		"list-only":   checkListOnly,
		"install":     checkInstall,
		"push":        checkPush,
		"pull-target": checkPullTarget,
	})
}

// positional splits words after the subcommand into positional arguments and
// an optional --user value. Any other option makes ok false.
func positional(ws []Word, skip int, allowed ...string) (pos []string, user string, valid bool) {
	for i := skip; i < len(ws); i++ {
		v := ws[i].Value
		switch {
		case v == "--user":
			if i+1 >= len(ws) {
				return nil, "", false
			}
			user = ws[i+1].Value
			i++
		case strings.HasPrefix(v, "-"):
			if !contains(allowed, v) {
				return nil, "", false
			}
		default:
			pos = append(pos, v)
		}
	}
	return pos, user, true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// indexOf returns the index of the first word equal to s, or -1.
func indexOf(ws []Word, s string) int {
	for i, w := range ws {
		if w.Value == s {
			return i
		}
	}
	return -1
}

// logcat options that only read. logcat uses getopt_long, which also accepts
// unambiguous abbreviations of long options (--cl = --clear), so only these
// exact spellings are 🟢; any other option is 🔴.
var (
	logcatShortFlags  = "dsLgSDB" // no value
	logcatShortValues = "vbtTem"  // take a value
	logcatLongFlags   = map[string]bool{"--dividers": true, "--last": true, "--statistics": true, "--binary": true}
	logcatLongValues  = map[string]bool{"--buffer": true, "--format": true, "--max-count": true, "--pid": true, "--regex": true, "--uid": true}
)

// logcat is 🟢 only with read-only options: clearing, resizing, pruning,
// rotating or writing files is 🔴.
func checkLogcat(ws []Word) checkResult {
	for i := 1; i < len(ws); i++ {
		v := ws[i].Value
		switch {
		case !strings.HasPrefix(v, "-"):
			continue // a filter spec such as ActivityManager:I or *:S
		case strings.HasPrefix(v, "--"):
			name, _, hasValue := strings.Cut(v, "=")
			switch {
			case logcatLongFlags[name] && !hasValue:
			case logcatLongValues[name]:
				if !hasValue {
					i++
				}
			default:
				return red("logcat option " + v + " may change the logs or write files")
			}
		default:
			// A cluster of short options, e.g. -d, -dv time, -vtime.
			for j := 1; j < len(v); j++ {
				c := v[j]
				if strings.IndexByte(logcatShortFlags, c) >= 0 {
					continue
				}
				if strings.IndexByte(logcatShortValues, c) >= 0 {
					if j == len(v)-1 {
						i++ // the value is the next word
					}
					break
				}
				return red("logcat option " + v + " may change the logs or write files")
			}
		}
	}
	return ok()
}

// checkListOnly allows only `<cmd> --list`.
func checkListOnly(ws []Word) checkResult {
	if len(ws) == 2 && ws[1].Value == "--list" {
		return ok()
	}
	return red("creates or removes a port redirection")
}

// adb install / install-multiple [-r] [-t] [-l] [--streaming|--no-streaming|--no-incremental] APK…
func checkInstall(ws []Word) checkResult {
	pos, user, good := positional(ws, 1, "-r", "-t", "-l", "--streaming", "--no-streaming", "--no-incremental")
	if !good || user != "" || len(pos) == 0 {
		return red("install options that ADBuddy cannot undo")
	}
	if ws[0].Value == "install" && len(pos) != 1 {
		return red("install with several files")
	}
	return change(newOp("adb-install", "apk", pos[0]))
}

// adb push LOCAL /absolute/REMOTE, a single file without options.
func checkPush(ws []Word) checkResult {
	pos, user, good := positional(ws, 1)
	if !good || user != "" || len(pos) != 2 {
		return red("push with options or several files")
	}
	if !strings.HasPrefix(pos[1], "/") {
		return red("push to a relative path")
	}
	return change(newOp("adb-push", "local", pos[0], "remote", pos[1]))
}

// protectedTargets are the environment's own files: `adb pull` must not
// overwrite them (they steer the agent and its permissions).
var protectedTargets = map[string]bool{
	"agents.md": true, "claude.md": true, "notes.md": true, "device.json": true, "journal": true,
}

// safeLocalTarget accepts a relative path inside the working directory that
// does not touch hidden files or folders (.claude, .adbuddy…) or the
// environment's own files.
func safeLocalTarget(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return false
	}
	for i, part := range strings.Split(path.Clean(p), "/") {
		if part == ".." || (strings.HasPrefix(part, ".") && part != ".") {
			return false
		}
		if i == 0 && protectedTargets[strings.ToLower(part)] {
			return false
		}
	}
	return true
}

// adb pull REMOTE [LOCAL] and adb bugreport [PATH] only read the phone, but
// they write on the PC without the agent's own permission prompt: 🟢 only
// inside the environment and away from its control files.
func checkPullTarget(ws []Word) checkResult {
	var pos []string
	for _, w := range ws[1:] {
		switch v := w.Value; {
		case v == "-a":
		case strings.HasPrefix(v, "-"):
			return red("unknown option " + v)
		default:
			pos = append(pos, v)
		}
	}
	var targets []string
	switch {
	case ws[0].Value == "bugreport" && len(pos) <= 1:
		targets = pos
	case ws[0].Value == "pull" && len(pos) == 1:
		targets = []string{path.Base(strings.TrimRight(strings.ReplaceAll(pos[0], `\`, "/"), "/"))}
	case ws[0].Value == "pull" && len(pos) == 2:
		// If LOCAL is an existing folder the file lands inside it.
		targets = []string{pos[1], path.Join(strings.ReplaceAll(pos[1], `\`, "/"), path.Base(strings.TrimRight(pos[0], "/")))}
	default:
		return red("several sources or targets")
	}
	for _, t := range targets {
		if !safeLocalTarget(t) {
			return red("writes outside the environment or over its own files: " + t)
		}
	}
	return ok()
}
