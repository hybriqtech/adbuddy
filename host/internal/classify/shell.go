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
func (c *Classifier) classifyScript(src string, depth int) []Finding {
	if depth > maxDepth {
		return []Finding{redFinding("shell.depth", "too many nested shells", []string{src}, textParse)}
	}
	file, err := newParser().Parse(strings.NewReader(src), "")
	if err != nil {
		return []Finding{redFinding("shell.parse", "cannot parse: "+err.Error(), []string{src}, textParse)}
	}
	var out []Finding
	syntax.Walk(file, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.CallExpr:
			out = append(out, c.evalCall(n, depth)...)
		case *syntax.Redirect:
			if f, bad := checkRedirect(n); bad {
				out = append(out, f)
			}
		case *syntax.DeclClause:
			out = append(out, checkDeclClause(n)...)
		case *syntax.WordIter:
			// The loop variable of `for NAME in …` / `select NAME in …`.
			if n.Name != nil && dangerousVar(n.Name.Value) {
				out = append(out, redFinding("shell.assign", "sets "+n.Name.Value, []string{n.Name.Value}, textAssign))
			}
		case *syntax.ParamExp:
			// ${X=…} and ${X:=…} assign to X as a side effect.
			if n.Exp != nil && n.Param != nil && dangerousVar(n.Param.Value) &&
				(n.Exp.Op == syntax.AssignUnset || n.Exp.Op == syntax.AssignUnsetOrNull) {
				out = append(out, redFinding("shell.assign", "sets "+n.Param.Value, []string{n.Param.Value}, textAssign))
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
			for _, e := range n.Exprs {
				if _, isWord := e.(*syntax.Word); isWord {
					out = append(out, redFinding("shell.assign", "let with an expression only known on the phone", []string{"let"}, textAssign))
				}
			}
		case *syntax.FuncDecl:
			out = append(out, redFinding("shell.function", "defines a shell function", []string{n.Name.Value}, textScript))
		case *syntax.CoprocClause:
			out = append(out, redFinding("shell.coproc", "coprocess", []string{"coproc"}, textParse))
		}
		return true
	})
	return out
}

func (c *Classifier) evalCall(ce *syntax.CallExpr, depth int) []Finding {
	var out []Finding
	for _, a := range ce.Assigns {
		if a.Name != nil && dangerousVar(a.Name.Value) {
			out = append(out, redFinding("shell.assign", "sets "+a.Name.Value, []string{a.Name.Value}, textAssign))
		}
	}
	if len(ce.Args) == 0 {
		return out
	}
	ws := make([]Word, len(ce.Args))
	for i, w := range ce.Args {
		ws[i] = wordOf(w)
	}
	return append(out, c.evalWords(ws, depth)...)
}

// dangerousVar reports variables that change which program or library runs.
func dangerousVar(name string) bool {
	switch name {
	case "PATH", "LD_PRELOAD", "LD_LIBRARY_PATH", "LD_AUDIT", "IFS", "ENV", "BASH_ENV", "FPATH",
		"CLASSPATH", "BOOTCLASSPATH", "SYSTEMSERVERCLASSPATH", "ANDROID_ROOT", "ANDROID_DATA", "ANDROID_RUNTIME_ROOT":
		return true
	}
	return false
}

// checkDeclClause classifies a declare clause (typeset, export, local,
// readonly, declare or nameref). It fails closed: red for the nameref
// variant, for a `-n` option in any combination (which turns a name into a
// reference to another variable), for any argument whose variable name is not
// a static literal (quoted, computed or a $(…) that could expand to an
// assignment), and for an assignment to a dangerous variable.
func checkDeclClause(d *syntax.DeclClause) []Finding {
	var out []Finding
	red := func(reason string, words ...string) {
		out = append(out, redFinding("shell.assign", reason, words, textAssign))
	}
	if d.Variant.Value == "nameref" {
		red("nameref makes a variable a reference to another", d.Variant.Value)
	}
	for _, a := range d.Args {
		switch {
		case a.Name == nil:
			// A naked argument with no parsed name: an option, or a value
			// whose name part was quoted or computed on the phone.
			w := wordOf(a.Value)
			switch {
			case w.Dynamic:
				red("declare argument only known on the phone", d.Variant.Value)
			case strings.HasPrefix(w.Value, "-"):
				if strings.ContainsRune(w.Value[1:], 'n') {
					red("typeset -n makes a variable a reference to another", w.Value)
				}
			case strings.HasPrefix(w.Value, "+"):
				// +x, +n… only remove attributes; harmless.
			default:
				// e.g. typeset "PATH"=/x: the name was quoted, so it is not
				// a static literal the classifier can trust.
				red("declare with a quoted or computed variable name", w.Value)
			}
		case (a.Value != nil || a.Array != nil) && dangerousVar(a.Name.Value):
			red("sets "+a.Name.Value, d.Variant.Value, a.Name.Value)
		}
	}
	return out
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
		return []Finding{redFinding("shell.assign", "sets "+got.Value, []string{got.Value}, textAssign)}
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
func (c *Classifier) evalWords(ws []Word, depth int) []Finding {
	vals := values(ws)
	if ws[0].Dynamic {
		return []Finding{redFinding("dynamic", "command name only known on the phone", vals, textDynamic)}
	}
	name := ws[0].Value
	if strings.Contains(name, "/") {
		dir, base := path.Split(path.Clean(name))
		if !trustedDirs[dir] || base == "" {
			return []Finding{redFinding("exec.file", "runs a file", vals, textScript)}
		}
		name = base
	}
	ws = append([]Word{{Value: name}}, ws[1:]...)

	switch name {
	case "sh", "mksh", "bash", "ash", "dash", "zsh", "ksh":
		return c.evalShell(ws, depth)
	case "su":
		return c.evalSu(ws, depth)
	case ".", "source", "eval":
		return []Finding{redFinding("shell.script", "runs code from a file or a string", vals, textScript)}
	case "toybox", "busybox":
		if len(ws) == 1 {
			return []Finding{{Rule: "toybox.list", Level: Green, Title: Text{EN: "List toybox commands", ES: "Listar los comandos de toybox"}, Words: vals}}
		}
		return c.evalWords(ws[1:], depth)
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
				return c.evalWords(append(lits(alias), rest[1:]...), depth)
			}
		}
		ws = append([]Word{ws[0]}, rest...)
	}
	if ws[0].Value == "run-as" && len(ws) > 1 && strings.Contains(strings.ToLower(ws[1].Value), ADBuddyPackage) {
		f := redFinding("run-as.adbuddy", "reads or changes the ADBuddy app's private files", vals, textUnknown)
		f.Special = true
		return []Finding{f}
	}
	if inner, handled, reason := unwrap(ws); handled {
		if reason != "" {
			return []Finding{redFinding("wrapper", reason, vals, textUnknown)}
		}
		if len(inner) == 0 {
			return []Finding{{Rule: "wrapper.noop", Level: Green, Title: render(Text{EN: "Run {cmd}", ES: "Ejecutar {cmd}"}, baseVars(ws)), Words: vals}}
		}
		return c.evalWords(inner, depth)
	}
	return []Finding{c.evalRule("shell", ws)}
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
	}
	return nil, 0, false, ""
}

// evalShell handles `sh [-opts] -c 'script' [args…]`; any other use runs a
// file or reads the script from stdin, which cannot be analysed.
func (c *Classifier) evalShell(ws []Word, depth int) []Finding {
	vals := values(ws)
	rest := ws[1:]
	for len(rest) > 0 && (strings.HasPrefix(rest[0].Value, "-") || strings.HasPrefix(rest[0].Value, "+")) && !rest[0].Dynamic {
		opts := rest[0].Value[1:]
		rest = rest[1:]
		if strings.ContainsAny(opts, "is") || opts == "" || opts == "-" {
			return []Finding{redFinding("shell.script", "shell reading commands from stdin", vals, textScript)}
		}
		if strings.Contains(opts, "c") {
			if len(rest) == 0 || rest[0].Dynamic {
				return []Finding{redFinding("shell.script", "sh -c with a script only known on the phone", vals, textScript)}
			}
			return c.classifyScript(rest[0].Value, depth+1)
		}
		if strings.Trim(opts, "eufxvnaC") != "" {
			return []Finding{redFinding("shell.script", "unknown shell option", vals, textScript)}
		}
	}
	if len(rest) == 0 {
		return []Finding{redFinding("shell.script", "shell reading commands from stdin", vals, textScript)}
	}
	return []Finding{redFinding("shell.script", "runs a script file", vals, textScript)}
}

// evalSu handles `su [user] -c 'script'` and `su [user] command…`.
func (c *Classifier) evalSu(ws []Word, depth int) []Finding {
	vals := values(ws)
	rest := ws[1:]
	if len(rest) > 0 && !rest[0].Dynamic && isSuUser(rest[0].Value) {
		rest = rest[1:]
	}
	switch {
	case len(rest) == 0:
		return []Finding{redFinding("shell.script", "su without command opens a root shell", vals, textScript)}
	case rest[0].Value == "-c":
		if len(rest) < 2 || rest[1].Dynamic {
			return []Finding{redFinding("shell.script", "su -c with a script only known on the phone", vals, textScript)}
		}
		return c.classifyScript(rest[1].Value, depth+1)
	case strings.HasPrefix(rest[0].Value, "-"):
		return []Finding{redFinding("shell.script", "unknown su option", vals, textScript)}
	}
	return c.evalWords(rest, depth)
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
				return Word{Value: lit.Value}
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
	return Word{Value: b.String(), Dynamic: dyn}
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
