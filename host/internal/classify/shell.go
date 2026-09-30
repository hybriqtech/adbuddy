package classify

import (
	"path"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// maxDepth limits `sh -c` / `su -c` nesting.
const maxDepth = 8

// Android's /system/bin/sh is mksh, so the parser uses the MirBSD Korn dialect.
func newParser() *syntax.Parser {
	return syntax.NewParser(syntax.Variant(syntax.LangMirBSDKorn))
}

// classifyScript parses a shell command line and classifies every simple
// command in it, including those inside $(…), pipelines, lists, subshells,
// loops and here-documents.
func (c *Classifier) classifyScript(src string, sc scope) []Finding {
	if sc.depth > maxDepth {
		return []Finding{unanalysable("shell.depth", "too many nested shells", []string{src}, textParse)}
	}
	file, err := newParser().Parse(strings.NewReader(src), "")
	if err != nil {
		return []Finding{unanalysable("shell.parse", "cannot parse: "+err.Error(), []string{src}, textParse)}
	}
	sc.vars = collectVars(file, sc.tainted)
	vars := sc.vars
	out := vars.integerFindings()
	// arith adds a finding when an arithmetic context may see a value that
	// is not proven to be a number (see arith.go).
	arith := func(safe bool, n syntax.Node) {
		if !safe {
			out = append(out, arithFinding(nodeText(n)))
		}
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Stmt:
			// `cmd &` (at the end or in the middle) and mksh's `cmd |&`.
			if n.Background || n.Coprocess || n.Disown {
				var b strings.Builder
				if syntax.NewPrinter(syntax.SingleLine(true)).Print(&b, n) != nil {
					b.Reset()
					b.WriteString("&")
				}
				out = append(out, backgroundFinding("runs in the background", []string{b.String()}))
			}
		case *syntax.CallExpr:
			out = append(out, c.evalCall(n, sc)...)
		case *syntax.Assign:
			// a[i]=…: the index is arithmetic.
			if n.Index != nil && n.Name != nil {
				arith(vars.arithSafe(n.Index, 0), n.Index)
			}
		case *syntax.ArithmExp:
			arith(vars.arithSafe(n.X, 0), n)
		case *syntax.ArithmCmd:
			arith(vars.arithSafe(n.X, 0), n)
		case *syntax.BinaryTest:
			// [[ a -eq b ]]: both operands are arithmetic.
			if n.Op >= syntax.TsEql && n.Op <= syntax.TsGtr {
				for _, e := range []syntax.TestExpr{n.X, n.Y} {
					w, ok := e.(*syntax.Word)
					arith(ok && vars.operandWord(w, 0), n)
				}
			}
		case *syntax.Redirect:
			if f, bad := checkRedirect(n); bad {
				out = append(out, f)
			}
		case *syntax.DeclClause:
			out = append(out, checkDeclClause(n)...)
		case *syntax.WordIter:
			// The loop variable of `for NAME in …` / `select NAME in …`.
			if n.Name != nil && dangerousVar(n.Name.Value) {
				out = append(out, assignFinding(n.Name.Value, []string{n.Name.Value}))
			}
		case *syntax.ParamExp:
			// ${X=…} and ${X:=…} assign to X as a side effect.
			if n.Exp != nil && n.Param != nil && dangerousVar(n.Param.Value) &&
				(n.Exp.Op == syntax.AssignUnset || n.Exp.Op == syntax.AssignUnsetOrNull) {
				out = append(out, assignFinding(n.Param.Value, []string{n.Param.Value}))
			}
			// ${a[i]} and ${x:off:len} evaluate arithmetic.
			if n.Index != nil && !isAllIndex(n.Index) {
				arith(vars.arithSafe(n.Index, 0), n)
			}
			if n.Slice != nil {
				arith(vars.arithSafe(n.Slice.Offset, 0) && vars.arithSafe(n.Slice.Length, 0), n)
			}
		case *syntax.BinaryArithm:
			if isArithAssign(n.Op) {
				out = append(out, checkArithTarget(n.X)...)
			}
		case *syntax.UnaryArithm:
			if n.Op == syntax.Inc || n.Op == syntax.Dec {
				out = append(out, checkArithTarget(n.X)...)
			}
		case *syntax.LetClause:
			// An unquoted `let PATH=1` is parsed into arithmetic nodes the
			// walker already sees. A quoted or expanded argument (let 'PATH=1',
			// let $x) is kept as a word that mksh re-lexes as arithmetic on the
			// phone, hiding its target: fail closed.
			// Every expression is arithmetic: a quoted one is lexed again,
			// so its targets are checked too (lvalueSafe).
			for _, e := range n.Exprs {
				if _, isWord := e.(*syntax.Word); isWord {
					out = append(out, redFinding("shell.assign", "let with an expression only known on the phone", []string{"let"}, textAssign))
				}
				arith(vars.arithSafe(e, 0), e)
			}
		case *syntax.FuncDecl:
			out = append(out, redFinding("shell.function", "defines a shell function", []string{n.Name.Value}, textScript))
		}
		// No case for *syntax.CoprocClause: `coproc` is bash's keyword and the
		// mksh parser never produces it (there `coproc` is an unknown command,
		// red). mksh's coprocess is `cmd |&`, caught above as Stmt.Coprocess.
		return true
	})
	return out
}

func (c *Classifier) evalCall(ce *syntax.CallExpr, sc scope) []Finding {
	var out []Finding
	for _, a := range ce.Assigns {
		if a.Name != nil && dangerousVar(a.Name.Value) {
			out = append(out, assignFinding(a.Name.Value, []string{a.Name.Value}))
		}
	}
	if len(ce.Args) == 0 {
		return out
	}
	ws := make([]Word, len(ce.Args))
	for i, w := range ce.Args {
		ws[i] = wordOf(w)
	}
	return append(out, c.evalWords(ws, sc)...)
}

// codeVars are variables the shell itself runs or expands as code, with no
// command that says so: PS4 (expanded, $(…) included, before each line
// that `set -x` traces), the prompts PS1 to PS3, ENV and BASH_ENV (startup
// files), FPATH (where an unknown command name is autoloaded from as a
// function) and EXECSHELL (the shell that runs a file without #!).
var codeVars = map[string]bool{
	"PS1": true, "PS2": true, "PS3": true, "PS4": true, "ENV": true, "BASH_ENV": true, "FPATH": true, "EXECSHELL": true,
}

// fileVars make an ordinary command run a file the classifier never sees,
// like running the file itself: PATH (which file a command name is), SHELL
// (the shell script, nsenter and chroot start), the bionic linker's
// LD_PRELOAD, LD_LIBRARY_PATH, LD_AUDIT and LD_CONFIG_FILE, and what
// app_process and ART load code from: the class paths and the runtime's
// roots.
var fileVars = map[string]bool{
	"PATH": true, "SHELL": true,
	"LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "LD_AUDIT": true, "LD_CONFIG_FILE": true,
	"CLASSPATH": true, "BOOTCLASSPATH": true, "SYSTEMSERVERCLASSPATH": true, "DEX2OATBOOTCLASSPATH": true,
	"ANDROID_ROOT": true, "ANDROID_DATA": true, "ANDROID_RUNTIME_ROOT": true, "ANDROID_ART_ROOT": true,
	"ANDROID_I18N_ROOT": true, "ANDROID_TZDATA_ROOT": true,
}

// hiddenVar reports a variable whose assignment hides code or a file from
// the classifier: assigning it, in any form, cannot be analysed.
func hiddenVar(name string) bool { return codeVars[name] || fileVars[name] }

// assignFinding is for an assignment to a dangerous variable. A code
// variable or a file variable hides code from the classifier, so it cannot
// be analysed; IFS only changes how words split.
func assignFinding(name string, words []string) Finding {
	switch {
	case codeVars[name]:
		return unanalysable("shell.script", "sets "+name+", which the shell runs or expands as code", words, textScript)
	case fileVars[name]:
		return unanalysable("shell.assign", "sets "+name+", which makes a command run a file ADBuddy does not see", words, textAssign)
	}
	return redFinding("shell.assign", "sets "+name, words, textAssign)
}

// dangerousVar reports variables that change which program, library or
// code runs, or how words split.
func dangerousVar(name string) bool { return hiddenVar(name) || name == "IFS" }

// checkDeclClause classifies a declare clause (typeset, export, local,
// readonly, declare or nameref). It fails closed: red for the nameref
// variant, for a `-n` option in any combination (which turns a name into a
// reference to another variable), for any argument whose variable name is not
// a static literal (quoted, computed or a $(…) that could expand to an
// assignment), for an assignment to a dangerous variable, and for a code or
// file variable declared with an attribute that changes its value (see
// valueAttrs). The names it cannot read, like an assignment to a code
// variable, are special: they may hide code (see codeVars).
func checkDeclClause(d *syntax.DeclClause) []Finding {
	var out []Finding
	// A name the classifier cannot read may be a code variable (see
	// codeVars): it cannot be analysed.
	unknownName := func(reason string, words ...string) {
		out = append(out, unanalysable("shell.script", reason, words, textScript))
	}
	if d.Variant.Value == "nameref" {
		unknownName("nameref makes a variable a reference to another", d.Variant.Value)
	}
	changes := changesValue(d.Args)
	for _, a := range d.Args {
		switch {
		case a.Name == nil:
			// A naked argument with no parsed name: an option, or a value
			// whose name part was quoted or computed on the phone.
			w := wordOf(a.Value)
			switch {
			case w.Dynamic:
				unknownName("declare argument only known on the phone", d.Variant.Value)
			case strings.HasPrefix(w.Value, "-"):
				if strings.ContainsRune(w.Value[1:], 'n') {
					unknownName("typeset -n makes a variable a reference to another", w.Value)
				}
			case strings.HasPrefix(w.Value, "+"):
				// +x, +n… only remove attributes; harmless.
			default:
				// e.g. typeset "PATH"=/x: the name was quoted, so it is not
				// a static literal the classifier can trust.
				unknownName("declare with a quoted or computed variable name", w.Value)
			}
		case (a.Value != nil || a.Array != nil) && dangerousVar(a.Name.Value):
			out = append(out, assignFinding(a.Name.Value, []string{d.Variant.Value, a.Name.Value}))
		case changes != "" && hiddenVar(a.Name.Value):
			// typeset -R4 PATH sets PATH to its last 4 characters (xbin, a
			// relative path) without assigning it.
			out = append(out, assignFinding(a.Name.Value, []string{d.Variant.Value, changes, a.Name.Value}))
		}
	}
	return out
}

// valueAttrs are mksh's typeset attributes that change a variable's value
// when they are set or removed: justify and pad (L, R, Z), integer and
// unsigned integer (i, U: the value is evaluated as arithmetic), and lower
// or upper case (l, u). The others (r, x, t, f, p) keep it; n is a
// reference, checked apart.
const valueAttrs = "LRZiUlu"

// changesValue returns the first option of a declaration that changes the
// value of the variables it names, or "".
func changesValue(args []*syntax.Assign) string {
	for _, a := range args {
		if a.Name != nil || a.Value == nil {
			continue
		}
		if v := wordOf(a.Value); !v.Dynamic && (strings.HasPrefix(v.Value, "-") || strings.HasPrefix(v.Value, "+")) &&
			strings.ContainsAny(v.Value[1:], valueAttrs) {
			return v.Value
		}
	}
	return ""
}

// isArithAssign reports the assignment operators inside arithmetic (=, +=, …).
func isArithAssign(op syntax.BinAritOperator) bool {
	switch op {
	case syntax.Assgn, syntax.AddAssgn, syntax.SubAssgn, syntax.MulAssgn, syntax.QuoAssgn,
		syntax.RemAssgn, syntax.AndAssgn, syntax.OrAssgn, syntax.XorAssgn, syntax.ShlAssgn,
		syntax.ShrAssgn, syntax.AndBoolAssgn, syntax.OrBoolAssgn, syntax.XorBoolAssgn, syntax.PowAssgn:
		return true
	}
	return false
}

// checkArithTarget flags an arithmetic assignment (=, +=, ++, --) whose target
// is a dangerous variable, or whose target is not a plain static name (so it
// could resolve to one on the phone). A normal counter such as i=i+1 or i++ is
// left alone.
func checkArithTarget(x syntax.ArithmExpr) []Finding {
	w, ok := x.(*syntax.Word)
	if !ok {
		return []Finding{redFinding("shell.assign", "arithmetic assignment to a name only known on the phone", []string{"$((…))"}, textAssign)}
	}
	got := wordOf(w)
	if got.Dynamic {
		return []Finding{redFinding("shell.assign", "arithmetic assignment to a name only known on the phone", []string{"$((…))"}, textAssign)}
	}
	if dangerousVar(got.Value) {
		return []Finding{assignFinding(got.Value, []string{got.Value})}
	}
	return nil
}

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

// declArgs checks a declaration the parser did not read as one: mksh's
// integer (typeset -i, whose values are evaluated as arithmetic, see
// varTable.integerFindings) and typeset, export, readonly, local or
// declare run through builtin or command. Like checkDeclClause, it cannot
// analyse a name only known on the phone, an array element (a[i] evaluates
// i, $(…) included), -n (a reference to another variable) or a variable
// that hides code or a file.
func declArgs(ws []Word) []Finding {
	cmd := ws[0].Value
	var out []Finding
	for _, w := range ws[1:] {
		if w.node != nil {
			if name, _, ok := splitAssignWord(w.node); ok {
				if dangerousVar(name) {
					out = append(out, assignFinding(name, []string{cmd, name}))
				}
				continue
			}
		}
		switch {
		case w.Dynamic:
			out = append(out, unanalysable("shell.script", cmd+" with a name only known on the phone", []string{cmd, w.Value}, textScript))
		case strings.HasPrefix(w.Value, "-") && strings.ContainsRune(w.Value[1:], 'n'):
			out = append(out, unanalysable("shell.script", cmd+" -n makes a variable a reference to another", []string{cmd, w.Value}, textScript))
		case strings.Contains(w.Value, "["):
			out = append(out, unanalysable("shell.script", cmd+" of an array element, whose index is evaluated as arithmetic", []string{cmd, w.Value}, textScript))
		case dangerousVar(w.Value):
			out = append(out, assignFinding(w.Value, []string{cmd, w.Value}))
		}
	}
	return out
}

// fcLists reports fc -l (list the history) without -e or -s.
func fcLists(args []Word) bool {
	lists := false
	for _, a := range args {
		if a.Dynamic {
			return false
		}
		if !strings.HasPrefix(a.Value, "-") || a.Value == "-" || a.Value == "--" {
			break
		}
		if strings.ContainsAny(a.Value[1:], "es") {
			return false
		}
		if strings.Contains(a.Value[1:], "l") {
			lists = true
		}
	}
	return lists
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

// trapRunsCode reports whether `trap <args>` sets an action: a string the
// shell runs later as code, like eval. `trap`, `trap -p`, `trap -l`, a reset
// (`trap - EXIT`) and an empty action (which ignores the signals) run
// nothing.
func trapRunsCode(args []Word) bool {
	if len(args) > 0 && !args[0].Dynamic && args[0].Value == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return false
	}
	a := args[0]
	if a.Dynamic {
		return true
	}
	switch a.Value {
	case "-", "-p", "-l", "":
		return false
	}
	return true
}

// setsHiddenVar reports why env, read, getopts, set -A or printf -v assigns
// a variable that hides code or a file (see hiddenVar), or a variable whose
// name is only known on the phone, which may be one; "" if none does. For
// read, getopts, set -A and unset, a name with [ is an array element, whose
// index mksh evaluates as arithmetic, $(…) included: it cannot be analysed
// either.
func setsHiddenVar(ws []Word) string {
	why := func(cmd, name string) string {
		if codeVars[name] {
			return cmd + " sets " + name + ", which the shell runs or expands as code"
		}
		return cmd + " sets " + name + ", which makes a command run a file ADBuddy does not see"
	}
	switch ws[0].Value {
	case "env":
		for i := 1; i < len(ws); i++ {
			v := ws[i].Value
			switch {
			case v == "-u":
				i++
			case strings.HasPrefix(v, "-"):
			case strings.Contains(v, "="):
				if name := v[:strings.Index(v, "=")]; hiddenVar(name) {
					return why("env", name)
				}
			default:
				return "" // the command
			}
		}
	case "read", "getopts":
		// read [options] name?prompt…; getopts optstring name [args…]. Any
		// word may be the name: fail closed.
		for _, w := range ws[1:] {
			if w.Dynamic {
				return ws[0].Value + " into a variable only known on the phone"
			}
			name, _, _ := strings.Cut(w.Value, "?")
			if hiddenVar(name) {
				return why(ws[0].Value, name)
			}
			if strings.Contains(name, "[") {
				return ws[0].Value + " into an array element, whose index is evaluated as arithmetic"
			}
		}
	case "unset":
		// unset [-fv] name…: unsetting a[i] evaluates i.
		for _, w := range ws[1:] {
			switch {
			case w.Dynamic:
				return "unset of a variable only known on the phone"
			case strings.Contains(w.Value, "["):
				return "unset of an array element, whose index is evaluated as arithmetic"
			}
		}
	case "set":
		// set -A NAME / +A NAME assigns to an array variable.
		for i := 1; i+1 < len(ws); i++ {
			v := ws[i].Value
			if v == "--" || v == "-" || len(v) < 2 || (v[0] != '-' && v[0] != '+') {
				return ""
			}
			if strings.Contains(v[1:], "A") {
				if n := ws[i+1]; n.Dynamic {
					return "set -A into a variable only known on the phone"
				} else if hiddenVar(n.Value) {
					return why("set -A", n.Value)
				} else if strings.Contains(n.Value, "[") {
					return "set -A into an array element, whose index is evaluated as arithmetic"
				}
				return ""
			}
		}
	case "printf":
		// printf -v NAME stores the output in NAME. An option only known on
		// the phone may be -v.
		for i := 1; i < len(ws); i++ {
			w := ws[i]
			switch {
			case w.Dynamic:
				return "printf with an option only known on the phone may set a variable"
			case w.Value == "-v" && i+1 < len(ws):
				if n := ws[i+1]; n.Dynamic {
					return "printf -v into a variable only known on the phone"
				} else if hiddenVar(n.Value) {
					return why("printf -v", n.Value)
				}
				return ""
			case w.Value == "--" || !strings.HasPrefix(w.Value, "-"):
				return "" // the format
			}
		}
	}
	return ""
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

// checkRedirect flags output redirections to anything but /dev/null.
func checkRedirect(r *syntax.Redirect) (Finding, bool) {
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.AppClob, syntax.RdrInOut,
		syntax.RdrAll, syntax.RdrAllClob, syntax.AppAll, syntax.AppAllClob:
		w := wordOf(r.Word)
		if !w.Dynamic && w.Value == "/dev/null" {
			return Finding{}, false
		}
		return redFinding("shell.redirect", "writes to "+w.Value, []string{r.Op.String(), w.Value}, textRedirect), true
	case syntax.DplOut, syntax.DplIn:
		w := wordOf(r.Word)
		if !w.Dynamic && (w.Value == "-" || isDigits(w.Value)) {
			return Finding{}, false
		}
		return redFinding("shell.redirect", "writes to "+w.Value, []string{r.Op.String(), w.Value}, textRedirect), true
	}
	return Finding{}, false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// wordOf computes the literal value of a word. Anything whose value is only
// known when the phone's shell runs it — expansions, substitutions, globs,
// braces, a leading ~ or $'…' — makes the word Dynamic. Braces and glob
// classes may span quoted parts: in {5,'reboot'} the parser sees "{5,",
// 'reboot' and "}", and the phone still expands it into two words.
func wordOf(w *syntax.Word) Word {
	// A word that is exactly [ ] { or } is a command name or a plain literal,
	// not a brace expansion or glob class: keep it static so `[ … ]` stays a
	// recognised command.
	if len(w.Parts) == 1 {
		if lit, ok := w.Parts[0].(*syntax.Lit); ok {
			switch lit.Value {
			case "[", "]", "{", "}":
				return Word{Value: lit.Value, node: w}
			}
		}
	}
	var b strings.Builder
	var st globState
	dyn := false
	for i, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			v, d := unescapeUnquoted(p.Value, i == 0, &st)
			b.WriteString(v)
			dyn = dyn || d
		case *syntax.SglQuoted:
			if p.Dollar {
				dyn = true
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				dyn = true
			}
			for _, q := range p.Parts {
				if l, ok := q.(*syntax.Lit); ok {
					b.WriteString(unescapeDouble(l.Value))
				} else {
					dyn = true
				}
			}
		default:
			dyn = true
		}
	}
	return Word{Value: b.String(), Dynamic: dyn, node: w}
}

// globState carries, across the parts of a word, whether an unquoted { or [
// has opened a brace expansion or a glob class.
type globState struct{ brace, bracket bool }

// unescapeUnquoted removes backslashes from an unquoted literal. It reports
// Dynamic for globs, a leading ~, an unquoted { or [ (which may open a brace
// expansion or a glob class), and a } or ] that closes one seen earlier in
// this literal or in an earlier literal of the same word (escaped closers
// count too, which only errs on the safe side).
func unescapeUnquoted(s string, first bool, st *globState) (string, bool) {
	var b strings.Builder
	dyn := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch == '}' && st.brace) || (ch == ']' && st.bracket) {
			dyn = true // brace expansion or glob class
		}
		switch {
		case ch == '\\' && i+1 < len(s):
			i++
			if (s[i] == '}' && st.brace) || (s[i] == ']' && st.bracket) {
				dyn = true
			}
			if s[i] != '\n' {
				b.WriteByte(s[i])
			}
			continue
		case ch == '*' || ch == '?':
			dyn = true
		case ch == '[':
			// An unquoted [ or { may open a glob class or a brace expansion,
			// whether or not a closer follows: treat the word as dynamic.
			st.bracket = true
			dyn = true
		case ch == '{':
			st.brace = true
			dyn = true
		case ch == '~' && i == 0 && first:
			dyn = true
		}
		b.WriteByte(ch)
	}
	return b.String(), dyn
}

func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '$', '`', '"', '\\':
				b.WriteByte(s[i+1])
				i++
				continue
			case '\n':
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
