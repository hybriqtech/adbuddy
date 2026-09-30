package classify

import (
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
