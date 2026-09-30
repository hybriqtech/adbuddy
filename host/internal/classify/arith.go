package classify

import (
	"regexp"

	"mvdan.cc/sh/v3/syntax"
)

// mksh evaluates arithmetic in $((…)) and in the integer operands of test
// and [. A variable used there is not read as a number: its value is
// evaluated again as an expression, and an array index in that expression is
// expanded in full, $(…) included, so x='a[$(reboot)]'; echo $((x)) runs
// reboot. The evaluator only admits operands it can prove to be numbers
// (see numericVars); the other places where mksh evaluates arithmetic
// ((( )), let, [[ ]], ${x:a:b}, array indexes, typeset -i) are not admitted
// at all.

var numberRE = regexp.MustCompile(`^[+-]?([0-9]+|0[xX][0-9A-Fa-f]+|[0-9]+#[0-9A-Za-z]+)$`)

var textArith = Text{
	EN: "Evaluate shell arithmetic on a value that could hide code: {cmd}",
	ES: "Evaluar aritmética de la shell con un valor que podría esconder código: {cmd}",
}

func arithFinding(what string) Finding {
	return unanalysable("shell.script", "arithmetic on a value not proven to be a number", []string{what}, textArith)
}

// arithOK reports an arithmetic expansion that can only see numbers.
func arithOK(a *syntax.ArithmExp, numeric map[string]bool) bool {
	return !a.Bracket && !a.Unsigned && arithExprOK(a.X, numeric)
}

// arithExprOK admits number literals, parentheses, operators that assign
// nothing, variables proven numeric, $? and $#.
func arithExprOK(x syntax.ArithmExpr, numeric map[string]bool) bool {
	switch x := x.(type) {
	case *syntax.BinaryArithm:
		return !isArithAssign(x.Op) && arithExprOK(x.X, numeric) && arithExprOK(x.Y, numeric)
	case *syntax.UnaryArithm:
		return x.Op != syntax.Inc && x.Op != syntax.Dec && arithExprOK(x.X, numeric)
	case *syntax.ParenArithm:
		return arithExprOK(x.X, numeric)
	case *syntax.Word:
		if len(x.Parts) != 1 {
			return false
		}
		switch p := x.Parts[0].(type) {
		case *syntax.Lit:
			return numberRE.MatchString(p.Value) || numeric[p.Value]
		case *syntax.ParamExp:
			return numericParam(p, numeric)
		}
	}
	return false
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

// simpleParam is $x or ${x}: no operator, index or modifier.
func simpleParam(p *syntax.ParamExp) bool {
	return p.Param != nil && !p.Excl && !p.Length && !p.Width && p.Flags == nil && p.NestedParam == nil &&
		p.Index == nil && len(p.Modifiers) == 0 && p.Slice == nil && p.Repl == nil && p.Names == 0 && p.Exp == nil
}

// numericParam is $x or ${x} of a variable proven numeric, $? or $#.
func numericParam(p *syntax.ParamExp, numeric map[string]bool) bool {
	return simpleParam(p) && (numeric[p.Param.Value] || p.Param.Value == "?" || p.Param.Value == "#")
}

// The words of test and [ (mksh's builtin), as the evaluator reads them.
const (
	testLiteral = iota // a static word
	testNumber         // only known on the phone, proven to be a number
	testData           // only known on the phone, in double quotes: one word
	testUnread         // anything else: it may split into operators
)

// testKind classifies one word of test or [.
func testKind(w Word, numeric map[string]bool) int {
	if !w.Dynamic {
		return testLiteral
	}
	if w.node == nil || len(w.node.Parts) != 1 {
		return testUnread
	}
	part := w.node.Parts[0]
	quoted := false
	if dq, ok := part.(*syntax.DblQuoted); ok {
		if dq.Dollar {
			return testUnread
		}
		quoted = true
		if len(dq.Parts) == 1 {
			part = dq.Parts[0]
		}
	}
	switch p := part.(type) {
	case *syntax.ParamExp:
		if numericParam(p, numeric) {
			return testNumber
		}
	case *syntax.ArithmExp:
		if arithOK(p, numeric) {
			return testNumber
		}
	}
	if quoted {
		return testData
	}
	return testUnread
}

var (
	testIntOps = map[string]bool{"-eq": true, "-ne": true, "-lt": true, "-le": true, "-gt": true, "-ge": true}
	// testUnaryOps are the unary file and string operators. -o (a shell
	// option) and -v (a variable, whose array index mksh would evaluate) are
	// left out on purpose.
	testUnaryOps = map[string]bool{
		"-b": true, "-c": true, "-d": true, "-e": true, "-f": true, "-g": true, "-G": true, "-h": true, "-k": true,
		"-L": true, "-n": true, "-O": true, "-p": true, "-r": true, "-s": true, "-S": true, "-t": true, "-u": true,
		"-w": true, "-x": true, "-z": true,
	}
	testBinaryOps = map[string]bool{"=": true, "==": true, "!=": true, "-nt": true, "-ot": true, "-ef": true}
	identRE       = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
)

// testFindings checks the words of test or [ against a closed grammar, in
// which every operator is a static word:
//
//	expr    := term { (-a | -o) term }
//	term    := [ ! ] primary
//	primary := ( expr ) | UNARY word | word BINARY word | number INT number | word
//
// A word is static, a number, or data in double quotes. On the phone a word
// of data may itself be -eq or another integer operator, and mksh then
// evaluates its neighbours as arithmetic: a static neighbour such as -n
// reads the variable n. So a word of data with a word on each side needs
// neighbours that cannot reach a variable the script sets to something that
// is not a number (see inert).
func (e *evaluator) testFindings(ws []Word) []Finding {
	bad := func(why string) []Finding {
		return []Finding{unanalysable("shell.script", why, values(ws), textArith)}
	}
	args := ws[1:]
	if ws[0].Value == "[" {
		if len(args) == 0 || args[len(args)-1].Dynamic || args[len(args)-1].Value != "]" {
			return bad("[ without a closing ]")
		}
		args = args[:len(args)-1]
	}
	kinds := make([]int, len(args))
	for i, w := range args {
		kinds[i] = testKind(w, e.numeric)
		if kinds[i] == testUnread {
			return bad("a word of test that may split into operators on the phone")
		}
		if kinds[i] == testLiteral && (w.Value == "-v" || w.Value == "-o" && !testJoins(args, i)) {
			return bad("test " + w.Value + " reads a shell variable or option")
		}
	}
	p := &testParser{args: args, kinds: kinds}
	if !p.expr() || p.pos != len(args) {
		return bad("a test expression ADBuddy cannot read")
	}
	for i := range args {
		if kinds[i] != testData || i == 0 || i == len(args)-1 {
			continue
		}
		for _, j := range []int{i - 1, i + 1} {
			if !e.inert(args[j], kinds[j]) {
				return bad("a word of test only known on the phone could be an integer operator")
			}
		}
	}
	return nil
}

// testJoins reports whether the static -o at i joins two terms (there is a
// word on each side), as opposed to the unary -o OPTION.
func testJoins(args []Word, i int) bool {
	return i > 0 && i < len(args)-1
}

// inert reports a word mksh can evaluate as arithmetic without running
// anything: a number, or a static word that names no variable the script
// sets to something that is not a number.
func (e *evaluator) inert(w Word, kind int) bool {
	switch kind {
	case testNumber:
		return true
	case testLiteral:
		for _, name := range identRE.FindAllString(w.Value, -1) {
			if e.unproven[name] {
				return false
			}
		}
		return true
	}
	return false
}

// testParser reads the words of test with the grammar of testFindings.
type testParser struct {
	args  []Word
	kinds []int
	pos   int
}

func (p *testParser) lit(v string) bool {
	return p.pos < len(p.args) && p.kinds[p.pos] == testLiteral && p.args[p.pos].Value == v
}

func (p *testParser) litIn(set map[string]bool) bool {
	return p.pos < len(p.args) && p.kinds[p.pos] == testLiteral && set[p.args[p.pos].Value]
}

func (p *testParser) expr() bool {
	if !p.term() {
		return false
	}
	for p.lit("-a") || p.lit("-o") {
		p.pos++
		if !p.term() {
			return false
		}
	}
	return true
}

func (p *testParser) term() bool {
	if p.lit("!") {
		p.pos++
	}
	return p.primary()
}

func (p *testParser) primary() bool {
	switch {
	case p.pos >= len(p.args):
		return false
	case p.lit("("):
		p.pos++
		if !p.expr() || !p.lit(")") {
			return false
		}
		p.pos++
		return true
	case p.litIn(testUnaryOps) && p.pos+1 < len(p.args) && !p.binaryAt(p.pos+1):
		p.pos += 2
		return true
	case p.binaryAt(p.pos + 1):
		op := p.args[p.pos+1].Value
		if p.pos+2 >= len(p.args) {
			return false
		}
		if testIntOps[op] && !(p.number(p.pos) && p.number(p.pos+2)) {
			return false
		}
		p.pos += 3
		return true
	}
	p.pos++ // a word alone: true if it is not empty
	return true
}

// binaryAt reports a static binary operator at i.
func (p *testParser) binaryAt(i int) bool {
	return i < len(p.args) && p.kinds[i] == testLiteral && (testBinaryOps[p.args[i].Value] || testIntOps[p.args[i].Value])
}

// number reports a number literal or a word proven to be a number.
func (p *testParser) number(i int) bool {
	return p.kinds[i] == testNumber || (p.kinds[i] == testLiteral && numberRE.MatchString(p.args[i].Value))
}
