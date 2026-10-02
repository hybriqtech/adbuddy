package classify

import (
	"path"
	"strings"
)

// checkResult is what an argument check found: a minimum level, why, the
// change it recognised (if any) and extra title placeholders.
type checkResult struct {
	level  Level
	reason Text
	op     *Op
	vars   map[string]string
}

func ok() checkResult           { return checkResult{level: Green} }
func red(why Text) checkResult  { return checkResult{level: Red, reason: why} }
func change(op *Op) checkResult { return checkResult{level: Yellow, op: op, vars: op.Args} }
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

// getoptSpec describes the options of a command that parses its arguments
// with getopt, so that a check reads them the way the command does.
//
//   - A word -xyz is a cluster read letter by letter. A letter in values takes
//     the rest of the cluster as its value, or the next word if the cluster ends
//     there. A letter in flags takes none. Any other letter is unknown.
//   - Long options are matched exactly (no abbreviations): longFlags take no
//     value, longValues take one (--name=value or the next word).
//   - A lone - and a word that does not start with - are positionals. Options
//     are read after a positional too.
//   - strict: -- ends the options (the rest are positionals), a value taken
//     from the next word that starts with - or is missing is not trusted
//     (-t -- -o f: -- is the value of -t, and -o is then an option), and the
//     command is not asked to guess. Otherwise -- is an unknown long option.
type getoptSpec struct {
	flags, values         string
	longFlags, longValues map[string]bool
	strict                bool
}

// getoptResult is what a getoptSpec found: the letters that were seen (with the
// value they took, "" if none), the positional words and the first word it
// could not read with certainty (bad, "" if none).
type getoptResult struct {
	seen map[byte]string
	long map[string]bool
	pos  []string
	bad  string
}

func (s getoptSpec) parse(ws []Word) getoptResult {
	r := getoptResult{seen: map[byte]string{}, long: map[string]bool{}}
	fail := func(v string) getoptResult { r.bad = v; return r }
	for i := 0; i < len(ws); i++ {
		v := ws[i].Value
		// next returns the value taken from the next word.
		next := func() (string, bool) {
			if !s.strict {
				i++
				if i < len(ws) {
					return ws[i].Value, true
				}
				return "", true
			}
			if i+1 >= len(ws) || strings.HasPrefix(ws[i+1].Value, "-") {
				return "", false
			}
			i++
			return ws[i].Value, true
		}
		switch {
		case s.strict && v == "--":
			for _, w := range ws[i+1:] {
				r.pos = append(r.pos, w.Value)
			}
			return r
		case v == "-" || !strings.HasPrefix(v, "-"):
			r.pos = append(r.pos, v)
		case strings.HasPrefix(v, "--"):
			name, _, hasValue := strings.Cut(v, "=")
			switch {
			case s.longFlags[name] && !hasValue:
			case s.longValues[name]:
				if !hasValue {
					if _, ok := next(); !ok {
						return fail(v)
					}
				}
			default:
				return fail(v)
			}
			r.long[name] = true
		default:
			for j := 1; j < len(v); j++ {
				c := v[j]
				switch {
				case strings.IndexByte(s.flags, c) >= 0:
					r.seen[c] = ""
				case strings.IndexByte(s.values, c) >= 0:
					val := v[j+1:]
					if val == "" {
						var ok bool
						if val, ok = next(); !ok {
							return fail(v)
						}
					}
					r.seen[c] = val
					j = len(v) // the value took the rest of the cluster
				default:
					return fail(v)
				}
			}
		}
	}
	return r
}

// logcat options that only read. logcat uses getopt_long, which also accepts
// unambiguous abbreviations of long options (--cl = --clear), so only these
// exact spellings are 🟢; any other option is 🔴.
var logcatOptions = getoptSpec{
	flags:      "dsLgSDB", // no value
	values:     "vbtTem",  // take a value
	longFlags:  map[string]bool{"--dividers": true, "--last": true, "--statistics": true, "--binary": true},
	longValues: map[string]bool{"--buffer": true, "--format": true, "--max-count": true, "--pid": true, "--regex": true, "--uid": true},
}

// logcat is 🟢 only with read-only options: clearing, resizing, pruning,
// rotating or writing files is 🔴.
func checkLogcat(ws []Word) checkResult {
	if r := logcatOptions.parse(ws[1:]); r.bad != "" {
		return red(reason("logcat option {v} may change the logs or write files", "v", r.bad))
	}
	return ok()
}

// checkListOnly allows only `<cmd> --list`.
func checkListOnly(ws []Word) checkResult {
	if len(ws) == 2 && ws[1].Value == "--list" {
		return ok()
	}
	return red(reason("creates or removes a port redirection"))
}

// adb install / install-multiple [-r] [-t] [-l] [--streaming|--no-streaming|--no-incremental] APK…
func checkInstall(ws []Word) checkResult {
	pos, user, good := positional(ws, 1, "-r", "-t", "-l", "--streaming", "--no-streaming", "--no-incremental")
	if !good || user != "" || len(pos) == 0 {
		return red(reason("install options that ADBuddy cannot undo"))
	}
	if ws[0].Value == "install" && len(pos) != 1 {
		return red(reason("install with several files"))
	}
	return change(newOp("adb-install", "apk", pos[0]))
}

// adb push LOCAL /absolute/REMOTE, a single file without options.
func checkPush(ws []Word) checkResult {
	pos, user, good := positional(ws, 1)
	if !good || user != "" || len(pos) != 2 {
		return red(reason("push with options or several files"))
	}
	if !strings.HasPrefix(pos[1], "/") {
		return red(reason("push to a relative path"))
	}
	return change(newOp("adb-push", "local", pos[0], "remote", pos[1]))
}

// protectedTargets are the environment's own files: `adb pull` must not
// overwrite them (they steer the agent and its permissions).
var protectedTargets = map[string]bool{
	"journal": true, "inbox": true, "outbox": true,
}

// configExts: at the top of the environment, files with these extensions
// are context or configuration for some agent (AGENTS.md, CLAUDE.local.md,
// opencode.json, device.json…).
var configExts = []string{".md", ".json", ".jsonc", ".toml", ".yaml", ".yml"}

// programExts: in any folder, programs a shell there could run instead of
// the shim or at the agent's request (cmd.exe looks in the current folder
// first, and the agent may go into any of them: adb.bat, tools/run.vbs…).
var programExts = []string{
	".exe", ".bat", ".cmd", ".com", ".ps1", ".sh", ".vbs", ".vbe", ".js", ".jse", ".wsf", ".wsh", ".msc", ".cpl",
}

// protectedBases: in any folder, these names with any extension (adb,
// adb.exe, adbuddy.bat…) could shadow the shim.
var protectedBases = map[string]bool{"adb": true, "adbuddy": true}

// SafeLocalTarget accepts a relative path without ".." that does not touch
// hidden files or folders (.claude, .adbuddy…), programs or names that could
// shadow the shim, or, in its first part, the environment's own files. The
// daemon applies it to the path relative to the environment's root.
func SafeLocalTarget(p string) bool {
	p = strings.ReplaceAll(p, `\`, "/")
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return false
	}
	// ".." is checked before cleaning: "out/../x" cleans to "x", but if out
	// is a link Linux and macOS resolve it in the parent of its target.
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return false
		}
	}
	for i, part := range strings.Split(path.Clean(p), "/") {
		if (strings.HasPrefix(part, ".") && part != ".") || strings.Contains(part, "~") {
			return false // hidden, or a Windows short name such as CLAUDE~1.MD
		}
		// Windows ignores trailing dots and spaces: "CLAUDE.md. " is CLAUDE.md.
		name := strings.ToLower(strings.TrimRight(part, ". "))
		base, _, _ := strings.Cut(name, ".")
		if protectedBases[base] || hasExt(name, programExts) {
			return false
		}
		if i == 0 && (protectedTargets[name] || hasExt(name, configExts)) {
			return false
		}
	}
	return true
}

func hasExt(name string, exts []string) bool {
	for _, ext := range exts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// remoteBase is the name a pulled file or folder gets on the PC. A remote
// ending in "." (or the root) may copy the folder's contents straight into
// the target, so it has no safe name.
func remoteBase(remote string) (string, bool) {
	r := strings.ReplaceAll(remote, `\`, "/")
	base := path.Base(strings.TrimRight(r, "/"))
	if base == "." || base == ".." || base == "/" || base == "" || strings.HasSuffix(strings.TrimRight(r, "/"), "/.") {
		return "", false
	}
	return base, true
}

// adb pull REMOTE [LOCAL] and adb bugreport [PATH] only read the phone, but
// they write on the PC without the agent's own permission prompt: 🟢 only
// inside the environment and away from its control files. The daemon checks
// the same targets against the agent's working directory (Result.PCWrites).
func checkPullTarget(ws []Word) checkResult {
	if _, why := pcTargets(ws); !why.Empty() {
		return red(why)
	}
	return ok()
}

// pcTargets lists where pull or bugreport writes, relative to the working
// directory, and why that is unsafe (empty if it looks safe).
func pcTargets(ws []Word) (targets []string, why Text) {
	var pos []string
	for _, w := range ws[1:] {
		switch v := w.Value; {
		case v == "-a":
		case strings.HasPrefix(v, "-"):
			return nil, reason("unknown option {v}", "v", v)
		default:
			pos = append(pos, v)
		}
	}
	switch {
	case ws[0].Value == "bugreport" && len(pos) == 1:
		// adb adds .zip to a path without it; an existing folder gets the
		// report inside.
		targets = []string{pos[0], pos[0] + ".zip"}
	case ws[0].Value == "bugreport" && len(pos) == 0:
		// bugreport-<device>-<date>.zip in the working directory.
	case ws[0].Value == "pull" && (len(pos) == 1 || len(pos) == 2):
		base, named := remoteBase(pos[0])
		if !named {
			return nil, reason("pulls a folder's contents without a folder name")
		}
		targets = []string{base}
		if len(pos) == 2 {
			// If LOCAL is an existing folder the file lands inside it.
			targets = []string{pos[1], path.Join(strings.ReplaceAll(pos[1], `\`, "/"), base)}
		}
	default:
		return nil, reason("several sources or targets")
	}
	for _, t := range targets {
		if !SafeLocalTarget(t) {
			return targets, reason("writes outside the environment or over its own files: {t}", "t", t)
		}
	}
	return targets, Text{}
}
