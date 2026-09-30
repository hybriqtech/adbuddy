package classify

import (
	"regexp"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// mksh evaluates arithmetic in $((…)), ((…)), let, the integer operands of
// test, [ and [[, ${x:a:b}, array indexes and assignments to integer
// variables. A variable used there is not read as a number: its value is
// evaluated again as an expression, and an array index in that expression is
// expanded in full, $(…) included, so x='a[$(reboot)]'; echo $((x)) runs
// reboot. The classifier only lets through operands it can prove to be
// numbers; anything else cannot be analysed (see arithFinding).

// scope is what the analysis of one script carries into the shells it
// starts (sh -c, su -c, find -exec sh -c…).
type scope struct {
	depth int
	// tainted are variables an enclosing shell may have set, in its
	// environment, to something that is not a number.
	tainted map[string]bool
	vars    *varTable // the current script's variables
}

// child is the scope of a shell started by the current script: every
// variable the script set to something not proven numeric, or declared
// justified (typeset -L/-R/-Z, with or without a value), may reach it.
// testOperatorWord trusts a variable the nested shell does not set only
// because this marks everything else: a new mark in varInfo that makes a
// variable not numeric must be added here too.
func (s scope) child() scope {
	t := map[string]bool{}
	for k := range s.tainted {
		t[k] = true
	}
	if s.vars != nil {
		for name, vi := range s.vars.info {
			if (vi.assigned && (vi.nonNumeric || vi.unknown)) || vi.justified {
				t[name] = true
			}
		}
	}
	return scope{depth: s.depth + 1, tainted: t}
}

// alwaysNumeric are shell variables whose value is always a number.
var alwaysNumeric = map[string]bool{"RANDOM": true, "SECONDS": true, "LINENO": true}

// mkshIntegerVars have mksh's integer attribute from the start: assigning
// one evaluates the value as arithmetic, like a typeset -i variable. The
// list errs on the side of more checks.
var mkshIntegerVars = []string{
	"COLUMNS", "LINES", "OPTIND", "RANDOM", "SECONDS", "TMOUT", "LINENO", "HISTSIZE",
	"PPID", "PGRP", "USER_ID", "KSHUID", "KSHGID", "KSHEGID", "BASHPID",
}

// varInfo is what one script does to one variable.
type varInfo struct {
	assigned   bool
	nonNumeric bool // some assignment is not provably a number
	unknown    bool // set from input or in a way the classifier cannot read
	integer    bool // typeset -i, integer, or one of mkshIntegerVars
	// justified: typeset -L, -R or -Z cut or pad the value, so a number
	// such as 16#ab may be read back as the name ab.
	justified bool
	values    []*syntax.Word
}

// varTable holds every assignment in a script, whatever its order: a
// variable is proven numeric only if all of them are numbers.
type varTable struct {
	info    map[string]*varInfo
	tainted map[string]bool
}

func (t *varTable) get(name string) *varInfo {
	vi := t.info[name]
	if vi == nil {
		vi = &varInfo{}
		t.info[name] = vi
	}
	return vi
}

func (t *varTable) word(name string, w *syntax.Word) {
	vi := t.get(name)
	vi.assigned = true
	vi.values = append(vi.values, w)
	if !numericValue(w) {
		vi.nonNumeric = true
	}
}

func (t *varTable) setUnknown(name string) {
	vi := t.get(name)
	vi.assigned, vi.unknown = true, true
}

// numeric reports whether a variable is proven to hold a number: one of the
// shell's own numeric variables, or a variable the script assigns, whose
// every assignment is a number, an arithmetic result or a checked integer
// assignment, and that no enclosing shell may have set to something else.
// A variable the script never assigns comes from outside: not proven.
func (t *varTable) numeric(name string) bool {
	if alwaysNumeric[name] {
		return true
	}
	if t == nil || t.tainted[name] {
		return false
	}
	vi := t.info[name]
	if vi == nil || !vi.assigned || vi.unknown || vi.justified {
		return false
	}
	return vi.integer || !vi.nonNumeric
}

// declBuiltins declare variables; with an option that contains i they make
// them integers.
var declBuiltins = map[string]bool{"typeset": true, "export": true, "local": true, "readonly": true, "declare": true, "integer": true}

// readsInto are commands that set the variables they name from input or
// from their arguments.
var readsInto = map[string]bool{"read": true, "getopts": true, "set": true, "printf": true}

// collectVars records every assignment in a script (see varTable).
func collectVars(f *syntax.File, tainted map[string]bool) *varTable {
	t := &varTable{info: map[string]*varInfo{}, tainted: tainted}
	for _, name := range mkshIntegerVars {
		t.get(name).integer = true
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.Assign:
			switch {
			case n.Name == nil:
			case n.Naked && n.Value == nil && n.Array == nil:
				// typeset x: declares, assigns nothing.
			case n.Array != nil:
				for _, e := range n.Array.Elems {
					t.word(n.Name.Value, e.Value)
				}
			default:
				t.word(n.Name.Value, n.Value)
			}
		case *syntax.DeclClause:
			integer, justified := declaresInteger(n.Variant.Value, n.Args), declaresJustified(n.Args)
			for _, a := range n.Args {
				if a.Name != nil {
					vi := t.get(a.Name.Value)
					vi.integer = vi.integer || integer
					vi.justified = vi.justified || justified
				}
			}
		case *syntax.CallExpr:
			t.call(n)
		case *syntax.ForClause:
			if wi, ok := n.Loop.(*syntax.WordIter); ok && n.Select {
				t.setUnknown(wi.Name.Value)
				t.setUnknown("REPLY")
			}
		case *syntax.WordIter:
			if !n.InPos.IsValid() {
				t.setUnknown(n.Name.Value) // for NAME; do: the positional parameters
				break
			}
			for _, w := range n.Items {
				t.word(n.Name.Value, w)
			}
		case *syntax.BinaryArithm:
			if isArithAssign(n.Op) {
				t.arithTarget(n.X)
			}
		case *syntax.UnaryArithm:
			if n.Op == syntax.Inc || n.Op == syntax.Dec {
				t.arithTarget(n.X)
			}
		case *syntax.ParamExp:
			if n.Exp != nil && n.Param != nil && (n.Exp.Op == syntax.AssignUnset || n.Exp.Op == syntax.AssignUnsetOrNull) {
				t.word(n.Param.Value, n.Exp.Word)
			}
		}
		return true
	})
	return t
}

// declaresInteger reports a declaration whose options contain i
// (typeset -i, local -xi, typeset -i10…) or the integer alias.
func declaresInteger(variant string, args []*syntax.Assign) bool {
	if variant == "integer" {
		return true
	}
	for _, a := range args {
		if a.Name != nil || a.Value == nil {
			continue
		}
		if v := wordOf(a.Value); !v.Dynamic && strings.HasPrefix(v.Value, "-") && strings.Contains(v.Value, "i") {
			return true
		}
	}
	return false
}

// declaresJustified reports a declaration with -L, -R or -Z, with or
// without a width and alone or combined (typeset -R2, typeset -iZ3…).
func declaresJustified(args []*syntax.Assign) bool {
	for _, a := range args {
		if a.Name != nil || a.Value == nil {
			continue
		}
		if v := wordOf(a.Value); !v.Dynamic && (strings.HasPrefix(v.Value, "-") || strings.HasPrefix(v.Value, "+")) &&
			strings.ContainsAny(v.Value[1:], "LRZ") {
			return true
		}
	}
	return false
}

// call records what a simple command assigns: the NAME=value words of env,
// integer or a declaration run through a wrapper (builtin typeset …), and
// the names that read, getopts, set -A or printf -v set.
func (t *varTable) call(ce *syntax.CallExpr) {
	var static []string
	decl, reads := false, false
	for _, w := range ce.Args {
		v := wordOf(w)
		if v.Dynamic {
			continue
		}
		static = append(static, v.Value)
		switch {
		case v.Value == "env" || declBuiltins[v.Value]:
			decl = true
		case readsInto[v.Value]:
			reads = true
		}
	}
	if !decl && !reads {
		return
	}
	integer, justified := false, false
	for _, v := range static {
		switch v {
		case "integer":
			integer = true
		case "read":
			t.setUnknown("REPLY") // read with no name sets REPLY
		case "getopts":
			t.setUnknown("OPTARG")
		}
	}
	if decl {
		var opts []*syntax.Assign
		for _, w := range ce.Args {
			opts = append(opts, &syntax.Assign{Naked: true, Value: w})
		}
		integer = integer || declaresInteger("", opts)
		justified = declaresJustified(opts)
	}
	mark := func(name string) {
		vi := t.get(name)
		vi.integer = vi.integer || integer
		vi.justified = vi.justified || justified
	}
	for _, w := range ce.Args[1:] {
		if name, val, ok := splitAssignWord(w); ok && decl {
			t.word(name, val)
			mark(name)
			continue
		}
		v := wordOf(w)
		if v.Dynamic {
			continue
		}
		if reads {
			if name, _, _ := strings.Cut(v.Value, "?"); isName(name) {
				t.setUnknown(name)
			}
		}
		if decl && isName(v.Value) {
			mark(v.Value)
		}
	}
}

// arithTarget records an arithmetic assignment (x=…, x+=…, x++): its
// result is a number. A target written $x is only known on the phone.
func (t *varTable) arithTarget(x syntax.ArithmExpr) {
	w, ok := x.(*syntax.Word)
	if !ok || len(w.Parts) != 1 {
		return
	}
	switch p := w.Parts[0].(type) {
	case *syntax.Lit:
		if isName(p.Value) {
			t.get(p.Value).assigned = true
		}
	case *syntax.ParamExp:
		if !p.Dollar.IsValid() && p.Param != nil { // a[i]=…
			t.get(p.Param.Value).assigned = true
		}
	}
}

// splitAssignWord splits a NAME=value argument (as env or integer take it).
func splitAssignWord(w *syntax.Word) (string, *syntax.Word, bool) {
	if len(w.Parts) == 0 {
		return "", nil, false
	}
	lit, ok := w.Parts[0].(*syntax.Lit)
	if !ok {
		return "", nil, false
	}
	name, rest, found := strings.Cut(lit.Value, "=")
	name = strings.TrimSuffix(name, "+")
	if !found || !isName(name) {
		return "", nil, false
	}
	var parts []syntax.WordPart
	if rest != "" {
		parts = append(parts, &syntax.Lit{Value: rest})
	}
	parts = append(parts, w.Parts[1:]...)
	if len(parts) == 0 {
		return name, nil, true
	}
	return name, &syntax.Word{Parts: parts}, true
}

var (
	nameRE    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	numberRE  = regexp.MustCompile(`^[+-]?([0-9]+|0[xX][0-9A-Fa-f]+|[0-9]+#[0-9A-Za-z@_]+)$`)
	digitsRE  = regexp.MustCompile(`^[+-]?[0-9]*$`)
	maxReLex  = 8
	intTestOp = map[string]bool{"-eq": true, "-ne": true, "-lt": true, "-le": true, "-gt": true, "-ge": true}
)

func isName(s string) bool { return nameRE.MatchString(s) }

// numericValue reports an assigned value that is always a number: empty, a
// number literal, or only arithmetic results, the shell's numeric
// parameters and digits.
func numericValue(w *syntax.Word) bool {
	if w == nil {
		return true
	}
	if v := wordOf(w); !v.Dynamic {
		return v.Value == "" || numberRE.MatchString(v.Value)
	}
	return numericParts(w.Parts)
}

func numericParts(parts []syntax.WordPart) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if !digitsRE.MatchString(p.Value) {
				return false
			}
		case *syntax.ArithmExp:
		case *syntax.DblQuoted:
			if p.Dollar || !numericParts(p.Parts) {
				return false
			}
		case *syntax.ParamExp:
			if !simpleParam(p) || !(shellNumericParam(p.Param.Value) || alwaysNumeric[p.Param.Value]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func shellNumericParam(name string) bool { return name == "#" || name == "?" || name == "$" }

// simpleParam is $x or ${x}: no operator, index or modifier.
func simpleParam(p *syntax.ParamExp) bool {
	return p.Param != nil && !p.Excl && !p.Length && !p.Width && p.Flags == nil && p.NestedParam == nil &&
		p.Index == nil && len(p.Modifiers) == 0 && p.Slice == nil && p.Repl == nil && p.Names == 0 && p.Exp == nil
}

// arithSafe reports whether evaluating x can only see numbers.
func (t *varTable) arithSafe(x syntax.ArithmExpr, depth int) bool {
	switch x := x.(type) {
	case nil:
		return true
	case *syntax.BinaryArithm:
		if isArithAssign(x.Op) {
			// The target is written; with +=, -=… it is also read.
			return t.lvalueSafe(x.X, depth) && (x.Op == syntax.Assgn || t.arithSafe(x.X, depth)) && t.arithSafe(x.Y, depth)
		}
		return t.arithSafe(x.X, depth) && t.arithSafe(x.Y, depth)
	case *syntax.UnaryArithm:
		if x.Op == syntax.Inc || x.Op == syntax.Dec {
			return t.lvalueSafe(x.X, depth) && t.arithSafe(x.X, depth)
		}
		return t.arithSafe(x.X, depth)
	case *syntax.ParenArithm:
		return t.arithSafe(x.X, depth)
	case *syntax.Word:
		return t.operandWord(x, depth)
	}
	return false
}

// lvalueSafe: a plain name is safe to write, unless it hides code or a file
// (a quoted let 'PATH=1' only shows its target here); anything else is
// evaluated.
func (t *varTable) lvalueSafe(x syntax.ArithmExpr, depth int) bool {
	if w, ok := x.(*syntax.Word); ok && len(w.Parts) == 1 {
		if l, ok := w.Parts[0].(*syntax.Lit); ok && isName(l.Value) {
			return !hiddenVar(l.Value)
		}
		if p, ok := w.Parts[0].(*syntax.ParamExp); ok && !p.Dollar.IsValid() && p.Param != nil && p.Index != nil {
			return !isAllIndex(p.Index) && t.arithSafe(p.Index, depth) // a[i]=…
		}
	}
	return t.arithSafe(x, depth)
}

// operandWord reports whether a word evaluated as arithmetic can only see
// numbers. mksh expands the word and evaluates the text, so quoted parts
// are lexed again as an expression.
func (t *varTable) operandWord(w *syntax.Word, depth int) bool {
	if len(w.Parts) == 1 {
		switch p := w.Parts[0].(type) {
		case *syntax.Lit:
			switch {
			case numberRE.MatchString(p.Value):
				return true
			case isName(p.Value):
				return t.numeric(p.Value)
			}
		case *syntax.ParamExp:
			return t.paramNumeric(p, depth)
		case *syntax.ArithmExp:
			return t.arithSafe(p.X, depth)
		}
	}
	var b strings.Builder
	if !t.arithText(&b, w.Parts, depth) {
		return false
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		return true // an empty expression is 0
	}
	if depth >= maxReLex {
		return false
	}
	x, ok := parseArith(text)
	if !ok {
		return false
	}
	if xw, ok := x.(*syntax.Word); ok && len(xw.Parts) == 1 {
		if l, ok := xw.Parts[0].(*syntax.Lit); ok && l.Value == text {
			return false // nothing left to lex: not a number or a name
		}
	}
	return t.arithSafe(x, depth+1)
}

// arithText writes the text mksh evaluates for these parts, with (0) for
// each expansion proven numeric; false if any other expansion is there.
func (t *varTable) arithText(b *strings.Builder, parts []syntax.WordPart, depth int) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			if p.Dollar {
				return false
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar || !t.arithText(b, p.Parts, depth) {
				return false
			}
		case *syntax.ParamExp:
			if !t.paramNumeric(p, depth) {
				return false
			}
			b.WriteString("(0)")
		case *syntax.ArithmExp:
			if !t.arithSafe(p.X, depth) {
				return false
			}
			b.WriteString("(0)")
		default:
			return false
		}
	}
	return true
}

// paramNumeric reports an expansion that is proven to be a number.
func (t *varTable) paramNumeric(p *syntax.ParamExp, depth int) bool {
	if p.Param == nil || p.Excl || p.Width || p.Flags != nil || p.NestedParam != nil || len(p.Modifiers) > 0 ||
		p.Slice != nil || p.Repl != nil || p.Names != 0 || p.Exp != nil {
		return false
	}
	if p.Index != nil && (isAllIndex(p.Index) || !t.arithSafe(p.Index, depth)) {
		return false
	}
	if p.Length {
		return true // ${#x}
	}
	if p.Index == nil && shellNumericParam(p.Param.Value) {
		return true
	}
	return t.numeric(p.Param.Value)
}

// isAllIndex is the [@] or [*] of ${a[@]}: every element, not arithmetic.
func isAllIndex(x syntax.ArithmExpr) bool {
	w, ok := x.(*syntax.Word)
	if !ok || len(w.Parts) != 1 {
		return false
	}
	l, ok := w.Parts[0].(*syntax.Lit)
	return ok && (l.Value == "@" || l.Value == "*")
}

// parseArith parses text as one arithmetic expression, the way mksh
// evaluates a variable's value; ok is false if it is anything else.
func parseArith(text string) (syntax.ArithmExpr, bool) {
	// A # outside a base number (16#ff) would start a comment the parser
	// skips.
	if strings.Contains(text, "\n") || (strings.Contains(text, "#") && !baseOnly(text)) {
		return nil, false
	}
	src := "((" + text + "))"
	f, err := newParser().Parse(strings.NewReader(src), "")
	if err != nil || len(f.Stmts) != 1 || len(f.Last) != 0 {
		return nil, false
	}
	st := f.Stmts[0]
	cmd, ok := st.Cmd.(*syntax.ArithmCmd)
	if !ok || len(st.Comments) != 0 || len(st.Redirs) != 0 || st.Negated || st.Background || st.Coprocess || st.Disown ||
		int(st.End().Offset()) != len(src) {
		return nil, false
	}
	return cmd.X, true
}

// baseOnly reports that every # in text is part of a base number (16#ff).
var baseRE = regexp.MustCompile(`[0-9]+#[0-9A-Za-z@_]+`)

func baseOnly(text string) bool {
	return !strings.Contains(baseRE.ReplaceAllString(text, "0"), "#")
}

// testFindings checks the arguments of test or [ (mksh's builtin): the
// operands of -eq, -ne, -lt, -le, -gt and -ge are arithmetic, and a word
// that splits on the phone could become one of those operators with its
// operands. A word only known on the phone and not proven to be a number,
// with an operand on each side, may be one of those operators too
// (op=-eq; [ "$x" "$op" 1 ]): its neighbours must then be numbers, unless
// they are static operators that mksh cannot read as an operand.
func (t *varTable) testFindings(ws []Word) []Finding {
	// The operands are ws[first:last]: not the command name, nor the
	// closing ] of [.
	first, last := 1, len(ws)
	if ws[0].Value == "[" && last > 1 && !ws[last-1].Dynamic && ws[last-1].Value == "]" {
		last--
	}
	var bad []string
	seen := map[int]bool{}
	flag := func(j int) {
		if !seen[j] {
			seen[j] = true
			bad = append(bad, ws[j].Value)
		}
	}
	for i := 1; i < len(ws); i++ {
		w := ws[i]
		if w.node != nil && !t.splitSafe(w.node) {
			flag(i)
			continue
		}
		switch {
		case !w.Dynamic && intTestOp[w.Value]:
			for _, j := range []int{i - 1, i + 1} {
				if j >= 1 && j < len(ws) && !t.wordArith(ws[j]) {
					flag(j)
				}
			}
		case w.Dynamic && i > first && i+1 < last && !t.wordArith(w):
			for _, j := range []int{i - 1, i + 1} {
				if !t.testOperatorWord(ws[j]) && !t.wordArith(ws[j]) {
					flag(j)
				}
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return []Finding{arithFinding(ws[0].Value + " " + strings.Join(bad, " "))}
}

// testUnaryOps are the unary file and string operators of test and [.
var testUnaryOps = map[string]bool{
	"-b": true, "-c": true, "-d": true, "-e": true, "-f": true, "-g": true, "-G": true, "-h": true, "-k": true,
	"-L": true, "-n": true, "-O": true, "-p": true, "-r": true, "-s": true, "-S": true, "-t": true, "-u": true,
	"-w": true, "-x": true, "-z": true,
}

// testOperatorWord reports a static operator of test or [ that mksh cannot
// evaluate as an arithmetic operand: =, !=, !, ( and ) are syntax errors
// there. -a, -o and the unary operators (-f, -n…) are not: mksh prefers a
// binary operator to a unary one ([ -f -eq 1 ] compares -f), and as an
// operand -f reads the variable f. They only count when the script and its
// enclosing shells never set that variable (the value then comes from
// adbd's environment). That assumption holds only because scope.child()
// taints every variable an enclosing shell leaves not numeric or justified:
// a new mark in varInfo must be checked there.
func (t *varTable) testOperatorWord(w Word) bool {
	if w.Dynamic {
		return false
	}
	switch {
	case w.Value == "=" || w.Value == "!=" || w.Value == "!" || w.Value == "(" || w.Value == ")":
		return true
	case w.Value == "-a" || w.Value == "-o" || testUnaryOps[w.Value]:
		name := w.Value[1:]
		if t == nil {
			return true
		}
		vi := t.info[name]
		return !t.tainted[name] && (vi == nil || !vi.assigned)
	}
	return false
}

// argsFindings checks commands whose arguments mksh evaluates as
// arithmetic: shift and ulimit.
func (t *varTable) argsFindings(ws []Word) []Finding {
	var bad []string
	for _, w := range ws[1:] {
		if !w.Dynamic && strings.HasPrefix(w.Value, "-") && ws[0].Value == "ulimit" {
			continue
		}
		if !t.wordArith(w) {
			bad = append(bad, w.Value)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return []Finding{arithFinding(ws[0].Value + " " + strings.Join(bad, " "))}
}

// wordArith is operandWord for a command argument. A dynamic word without a
// syntax node was added by xargs or find, which run the external program
// (toybox), where these operands are plain numbers.
func (t *varTable) wordArith(w Word) bool {
	switch {
	case w.node != nil:
		return t.operandWord(w.node, 0)
	case w.Dynamic:
		return true
	}
	return t.operandWord(&syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: w.Value}}}, 0)
}

// splitSafe reports a word that stays one word on the phone, or splits
// only into numbers.
func (t *varTable) splitSafe(w *syntax.Word) bool {
	if !wordOf(w).Dynamic {
		return true // a static word, including a lone [ or ]
	}
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.ContainsAny(p.Value, "*?[{") {
				return false
			}
		case *syntax.SglQuoted:
		case *syntax.DblQuoted:
			for _, q := range p.Parts {
				if pe, ok := q.(*syntax.ParamExp); ok && pe.Param != nil && (pe.Param.Value == "@" || (pe.Index != nil && isAllIndex(pe.Index))) {
					return false // "$@" is one word per parameter
				}
			}
		case *syntax.ParamExp:
			if !t.paramNumeric(p, 0) {
				return false
			}
		case *syntax.ArithmExp:
		default:
			return false
		}
	}
	return true
}

// integerFindings checks the assignments to integer variables: mksh
// evaluates each value as arithmetic.
func (t *varTable) integerFindings() []Finding {
	names := make([]string, 0, len(t.info))
	for name := range t.info {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []Finding
	for _, name := range names {
		vi := t.info[name]
		if !vi.integer || !vi.assigned {
			continue
		}
		if vi.unknown {
			out = append(out, arithFinding(name+" (an integer variable set from input)"))
			continue
		}
		for _, w := range vi.values {
			if w != nil && !t.operandWord(w, 0) {
				out = append(out, arithFinding(name+"="+nodeText(w)))
			}
		}
	}
	return out
}

var textArith = Text{
	EN: "Evaluate shell arithmetic on a value that could hide code: {cmd}",
	ES: "Evaluar aritmética de la shell con un valor que podría esconder código: {cmd}",
}

func arithFinding(what string) Finding {
	return unanalysable("shell.script", "arithmetic on a value not proven to be a number", []string{what}, textArith)
}

// nodeText prints a syntax node on one line, for titles.
func nodeText(n syntax.Node) string {
	// The printer prints expressions only inside their command or word.
	switch x := n.(type) {
	case *syntax.BinaryArithm, *syntax.UnaryArithm, *syntax.ParenArithm:
		n = &syntax.Word{Parts: []syntax.WordPart{&syntax.ArithmExp{X: x.(syntax.ArithmExpr)}}}
	case *syntax.BinaryTest:
		n = &syntax.TestClause{X: x}
	}
	var b strings.Builder
	if syntax.NewPrinter(syntax.SingleLine(true)).Print(&b, n) != nil {
		return "…"
	}
	return b.String()
}
