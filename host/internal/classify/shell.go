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

// scope is what the analysis of one script carries into the shells it
// starts (sh -c, su -c, find -exec sh -c…). Nothing else crosses: a script
// never exports a variable (see vars.go), so a nested shell starts from the
// phone's own environment.
type scope struct {
	depth int
}

func (s scope) child() scope { return scope{depth: s.depth + 1} }

// evaluator classifies one script. It only understands the shell it lists:
// every other statement, word part, expansion, builtin or variable name is
// 🔴 special (spec M2.5 §4). What mksh does with each admitted form is
// recorded against a real mksh in testdata/oracle.jsonl.
type evaluator struct {
	c        *Classifier
	sc       scope
	numeric  map[string]bool // variables proven to hold a number (see numericVars)
	unproven map[string]bool // variables the script sets that are not proven numeric
	out      []Finding
}

var textUnsupported = Text{
	EN: "Run shell code ADBuddy does not analyse: {cmd}",
	ES: "Ejecutar código de shell que ADBuddy no analiza: {cmd}",
}

// unsupported records shell the evaluator does not list.
func (e *evaluator) unsupported(reason string, n syntax.Node) {
	e.out = append(e.out, unanalysable("shell.unsupported", reason, []string{nodeText(n)}, textUnsupported))
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
	e := &evaluator{c: c, sc: sc}
	e.numeric, e.unproven = numericVars(file)
	e.stmts(file.Stmts)
	return e.out
}

func (e *evaluator) stmts(list []*syntax.Stmt) {
	for _, s := range list {
		e.stmt(s)
	}
}

func (e *evaluator) stmt(s *syntax.Stmt) {
	// `cmd &` (at the end or in the middle) and mksh's `cmd |&`.
	if s.Background || s.Coprocess || s.Disown {
		e.out = append(e.out, backgroundFinding("runs in the background", []string{nodeText(s)}))
	}
	for _, r := range s.Redirs {
		e.redirect(r)
	}
	switch cmd := s.Cmd.(type) {
	case nil: // only redirections
	case *syntax.CallExpr:
		e.call(cmd)
	case *syntax.BinaryCmd:
		switch cmd.Op {
		case syntax.AndStmt, syntax.OrStmt, syntax.Pipe:
			e.stmt(cmd.X)
			e.stmt(cmd.Y)
		default:
			e.unsupported("shell operator "+cmd.Op.String(), s)
		}
	case *syntax.IfClause:
		for c := cmd; c != nil; c = c.Else {
			e.stmts(c.Cond)
			e.stmts(c.Then)
		}
	case *syntax.WhileClause:
		e.stmts(cmd.Cond)
		e.stmts(cmd.Do)
	case *syntax.ForClause:
		e.forClause(cmd, s)
	case *syntax.Subshell:
		e.stmts(cmd.Stmts)
	case *syntax.Block:
		e.stmts(cmd.Stmts)
	case *syntax.TimeClause:
		if cmd.PosixFormat {
			e.unsupported("time with options", s)
		} else if cmd.Stmt != nil {
			e.stmt(cmd.Stmt)
		}
	default:
		// case, select, functions, [[ ]], (( )), let, typeset and the other
		// declarations, and whatever the parser learns next.
		e.unsupported("shell construct", s)
	}
}

// forClause admits `for name in words; do …; done` with a name of the
// script's own (see ownName).
func (e *evaluator) forClause(f *syntax.ForClause, s *syntax.Stmt) {
	wi, ok := f.Loop.(*syntax.WordIter)
	if !ok || f.Select || !wi.InPos.IsValid() {
		// select, a C-style loop, or `for x; do`, which reads the positional
		// parameters.
		e.unsupported("shell construct", s)
		return
	}
	if !ownName(wi.Name.Value) {
		e.out = append(e.out, nameFinding(wi.Name.Value))
	}
	for _, w := range wi.Items {
		e.word(w)
	}
	e.stmts(f.Do)
}

// call classifies a simple command: its assignments and its words.
func (e *evaluator) call(ce *syntax.CallExpr) {
	if len(ce.Args) == 0 {
		for _, a := range ce.Assigns {
			e.assign(a)
		}
		return
	}
	for _, a := range ce.Assigns {
		e.envAssign(a)
	}
	ws := make([]Word, len(ce.Args))
	for i, w := range ce.Args {
		ws[i] = e.word(w)
	}
	e.out = append(e.out, e.c.evalWords(ws, e)...)
}

// redirect admits output to /dev/null, duplicates between numbered
// descriptors, input and here-documents. Output to any other file is a plain
// write: 🔴, not special.
func (e *evaluator) redirect(r *syntax.Redirect) {
	if r.N != nil && !isDigits(r.N.Value) {
		e.unsupported("redirection of a named descriptor", r)
		return
	}
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.AppClob, syntax.RdrInOut,
		syntax.RdrAll, syntax.RdrAllClob, syntax.AppAll, syntax.AppAllClob:
		w := e.word(r.Word)
		if !w.Dynamic && w.Value == "/dev/null" {
			return
		}
		e.out = append(e.out, redFinding("shell.redirect", "writes to "+w.Value, []string{r.Op.String(), w.Value}, textRedirect))
	case syntax.DplOut, syntax.DplIn:
		w := e.word(r.Word)
		if !w.Dynamic && (w.Value == "-" || isDigits(w.Value)) {
			return
		}
		e.out = append(e.out, redFinding("shell.redirect", "writes to "+w.Value, []string{r.Op.String(), w.Value}, textRedirect))
	case syntax.RdrIn, syntax.WordHdoc:
		e.word(r.Word)
	case syntax.Hdoc, syntax.DashHdoc:
		// The body expands like text in double quotes (with a quoted
		// delimiter it is one literal).
		if r.Hdoc != nil {
			e.parts(r.Hdoc.Parts, true)
		}
	default:
		e.unsupported("redirection "+r.Op.String(), r)
	}
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

// nodeText prints a syntax node on one line, for titles.
func nodeText(n syntax.Node) string {
	// The printer prints expressions only inside their command or word.
	switch x := n.(type) {
	case *syntax.BinaryArithm, *syntax.UnaryArithm, *syntax.ParenArithm:
		n = &syntax.Word{Parts: []syntax.WordPart{&syntax.ArithmExp{X: x.(syntax.ArithmExpr)}}}
	case *syntax.Redirect:
		n = &syntax.Stmt{Redirs: []*syntax.Redirect{x}}
	case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst, *syntax.ExtGlob, *syntax.SglQuoted, *syntax.DblQuoted:
		n = &syntax.Word{Parts: []syntax.WordPart{x.(syntax.WordPart)}}
	}
	var b strings.Builder
	if syntax.NewPrinter(syntax.SingleLine(true)).Print(&b, n) != nil || b.Len() == 0 {
		return "…"
	}
	return b.String()
}
