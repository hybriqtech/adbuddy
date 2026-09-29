package classify

import (
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
		"logcat":    checkLogcat,
		"list-only": checkListOnly,
		"install":   checkInstall,
		"push":      checkPush,
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

// logcat is 🟢 unless it clears, resizes, prunes or writes files.
func checkLogcat(ws []Word) checkResult {
	for _, w := range ws[1:] {
		v := w.Value
		switch {
		case v == "--clear" || v == "--buffer-size" || strings.HasPrefix(v, "--buffer-size=") ||
			v == "--prune" || strings.HasPrefix(v, "--prune=") || v == "--file" || strings.HasPrefix(v, "--file="):
			return red("logcat " + v + " changes the logs or writes files")
		case strings.HasPrefix(v, "-") && !strings.HasPrefix(v, "--") && strings.ContainsAny(v[1:], "cGPf"):
			return red("logcat " + v + " changes the logs or writes files")
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
