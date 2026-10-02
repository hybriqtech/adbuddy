package classify

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var placeholderRE = regexp.MustCompile(`\{[a-z][a-z0-9_]*\}`)

// placeholders lists the {names} of a template, sorted. The braces of the
// shell in a template ("${ …;}") are not placeholders.
func placeholders(s string) []string {
	found := placeholderRE.FindAllString(s, -1)
	sort.Strings(found)
	return found
}

func samePlaceholders(a, b string) bool {
	return strings.Join(placeholders(a), " ") == strings.Join(placeholders(b), " ")
}

// sources parses the non-test Go files of the package.
func sources(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	return sourcesIn(t, ".")
}

// sourcesIn parses the non-test Go files of dir.
func sourcesIn(t *testing.T, dir string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = f
	}
	return fset, files
}

func unquote(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// reasonCall is one reason("template", "name", value…) in the code.
type reasonCall struct {
	template string
	names    []string // the placeholders the call fills
	where    string
}

func reasonCalls(t *testing.T) (calls []reasonCall, dynamic []string) {
	t.Helper()
	fset, files := sources(t)
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := c.Fun.(*ast.Ident); !ok || id.Name != "reason" || len(c.Args) == 0 {
				return true
			}
			where := fset.Position(c.Pos()).String()
			tpl, ok := unquote(c.Args[0])
			if !ok {
				dynamic = append(dynamic, where)
				return true
			}
			rc := reasonCall{template: tpl, where: where}
			for i := 1; i < len(c.Args); i += 2 {
				if name, ok := unquote(c.Args[i]); ok {
					rc.names = append(rc.names, "{"+name+"}")
				}
			}
			sort.Strings(rc.names)
			calls = append(calls, rc)
			return true
		})
	}
	return calls, dynamic
}

// Every reason the code can give has its Spanish, with the same placeholders,
// and the Spanish table has nothing the code does not use.
func TestReasonsBilingual(t *testing.T) {
	calls, dynamic := reasonCalls(t)
	for _, w := range dynamic {
		t.Errorf("%s: reason() with a template that is not a literal", w)
	}
	if len(calls) < 150 {
		t.Fatalf("only %d reason() calls found: did the scan break?", len(calls))
	}
	used := map[string]bool{}
	for _, c := range calls {
		used[c.template] = true
		es, ok := reasonsES[c.template]
		switch {
		case !ok:
			t.Errorf("%s: %q has no Spanish in reasonsES", c.where, c.template)
			continue
		case es == "" || es == c.template:
			t.Errorf("%s: the Spanish of %q is empty or the same text", c.where, c.template)
		case !samePlaceholders(c.template, es):
			t.Errorf("%s: %q and its Spanish %q have different {placeholders}", c.where, c.template, es)
		}
		if got, want := strings.Join(c.names, " "), strings.Join(placeholders(c.template), " "); got != want {
			t.Errorf("%s: %q is filled with %q but names %q", c.where, c.template, got, want)
		}
	}
	for k, es := range reasonsES {
		if !used[k] {
			t.Errorf("reasonsES has %q and no reason() call uses it", k)
		}
		if es != strings.TrimSpace(es) || strings.HasSuffix(es, ".") {
			t.Errorf("%q: the Spanish must have no spaces around it and no final period: %q", k, es)
		}
	}
}

// The rules (YAML) and the Go texts (Text{EN, ES}) of titles are bilingual too.
func TestTitlesBilingual(t *testing.T) {
	rules, err := builtinRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) < 150 {
		t.Fatalf("%d built-in rules: did the YAML change shape?", len(rules))
	}
	for _, r := range rules {
		switch {
		case r.Title.ES == "":
			t.Errorf("rule %s has no Spanish title", r.ID)
		case !samePlaceholders(r.Title.EN, r.Title.ES):
			t.Errorf("rule %s: the title and its Spanish have different {placeholders}: %q / %q", r.ID, r.Title.EN, r.Title.ES)
		}
	}
	// The texts of the other packages that raise a question (the reasons of
	// the daemon and of the prechecks) are Text literals too.
	found := 0
	for _, dir := range []string{".", "../precheck", "../daemon"} {
		found += textLiteralsAreBilingual(t, dir)
	}
	if found < 30 {
		t.Fatalf("only %d Text literals found: did the scan break?", found)
	}
}

// textLiteralsAreBilingual checks every Text{EN: …, ES: …} (classify.Text{…}
// outside the package) of the Go files of dir and returns how many it found.
func textLiteralsAreBilingual(t *testing.T, dir string) int {
	t.Helper()
	fset, files := sourcesIn(t, dir)
	found := 0
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			switch ty := cl.Type.(type) {
			case *ast.Ident:
				if ty.Name != "Text" {
					return true
				}
			case *ast.SelectorExpr:
				if ty.Sel.Name != "Text" {
					return true
				}
			default:
				return true
			}
			var en, es string
			var hasEN, hasES bool
			for _, el := range cl.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, _ := kv.Key.(*ast.Ident)
				if key == nil {
					continue
				}
				switch key.Name {
				case "EN":
					en, hasEN = literalPart(kv.Value)
				case "ES":
					es, hasES = literalPart(kv.Value)
				}
			}
			if !hasEN {
				return true
			}
			found++
			where := fset.Position(cl.Pos()).String()
			switch {
			case !hasES || es == "":
				t.Errorf("%s: Text{EN: %q} has no Spanish", where, en)
			case !samePlaceholders(en, es):
				t.Errorf("%s: %q and %q have different {placeholders}", where, en, es)
			}
			return true
		})
	}
	return found
}

// literalPart is the literal text of a string expression: a literal, or a
// concatenation of literals and values, whose values are left out.
func literalPart(e ast.Expr) (string, bool) {
	if s, ok := unquote(e); ok {
		return s, true
	}
	if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.ADD {
		x, okx := literalPart(b.X)
		y, oky := literalPart(b.Y)
		if okx || oky {
			return x + y, true
		}
	}
	return "", false
}

func TestReasonAndJoinReason(t *testing.T) {
	r := reason("unknown sed option {v}", "v", "-x")
	if r.EN != "unknown sed option -x" || r.ES != "opción de sed desconocida: -x" {
		t.Fatalf("got %+v", r)
	}
	if r := reason("a template that is not in the table"); r.EN != "a template that is not in the table" || r.ES != "" {
		t.Fatalf("an unknown template must have no Spanish (Text.In answers English): %+v", r)
	}
	if got := reason("a template that is not in the table").In("es"); got != "a template that is not in the table" {
		t.Fatalf("In(es) must fall back to English, never empty: %q", got)
	}
	if !(Text{}).Empty() || r.Empty() {
		t.Fatal("Empty")
	}
	j := Join(reason("restarts adbd"), reason("changes how adbd runs"))
	if j.EN != "restarts adbd; changes how adbd runs" || j.ES != "reinicia adbd; cambia cómo se ejecuta adbd" {
		t.Fatalf("join: %+v", j)
	}
	if j := Join(Text{}, reason("restarts adbd")); j.EN != "restarts adbd" {
		t.Fatalf("join with an empty first: %+v", j)
	}
	if j := Join(reason("restarts adbd"), Text{}); j.EN != "restarts adbd" {
		t.Fatalf("join with an empty second: %+v", j)
	}
}

// corpusResults classifies every case of every corpus, the generated
// differential cases included: the largest body of commands the classifier
// has been checked against.
func corpusResults(t *testing.T) []Result {
	t.Helper()
	c := mustNew(t, "")
	var out []Result
	for _, name := range []string{"habitual.jsonl", "m2-levels.jsonl", "raised.jsonl"} {
		for _, k := range loadCorpus(t, name) {
			if r, ok := safeClassify(t, c, k.argv()); ok {
				out = append(out, r)
			}
		}
	}
	for _, k := range differentialCases(t) {
		if r, ok := safeClassify(t, c, sh(withSentinel(k.Sh))); ok {
			out = append(out, r)
		}
	}
	return out
}

// Spec M6c §6.6 and §9.2: no title and no reason of any 🔴 of the corpora
// falls back to English, and the Spanish detail of a question has none of the
// English reasons in it.
func TestCorpusIsFullyInSpanish(t *testing.T) {
	var names []string
	for _, rs := range []map[string]string{reasonsES} {
		for k := range rs {
			for _, p := range placeholders(k) {
				names = append(names, p)
			}
		}
	}
	reds, reasons := 0, 0
	for _, r := range corpusResults(t) {
		if r.Level != Red {
			continue
		}
		reds++
		for _, f := range r.Findings {
			if f.Level != Red {
				continue
			}
			if f.Title.ES == "" || f.Title.ES == f.Title.EN {
				t.Errorf("%s: the title %q has no Spanish of its own (%q)", f.Rule, f.Title.EN, f.Title.ES)
			}
			if f.Reason.Empty() {
				continue
			}
			reasons++
			if f.Reason.ES == "" || f.Reason.ES == f.Reason.EN {
				t.Errorf("%s: the reason %q has no Spanish of its own (%q)", f.Rule, f.Reason.EN, f.Reason.ES)
			}
			for _, p := range names {
				if strings.Contains(f.Reason.EN, p) || strings.Contains(f.Reason.ES, p) {
					// A command that really has the text {value} would do this; none of the corpus does.
					t.Errorf("%s: the placeholder %s was not filled: %q / %q", f.Rule, p, f.Reason.EN, f.Reason.ES)
				}
			}
		}
		es, en := r.Detail("es"), r.Detail("en")
		if strings.Count(es, "\n") != strings.Count(en, "\n") {
			t.Errorf("the detail has %d lines in Spanish and %d in English:\n%s\n--\n%s", strings.Count(es, "\n")+1, strings.Count(en, "\n")+1, es, en)
		}
		for _, f := range r.Findings {
			if f.Level != Red || f.Reason.Empty() {
				continue
			}
			if !strings.Contains(es, Printable(f.Reason.ES)) {
				t.Errorf("the Spanish detail lacks the Spanish reason %q:\n%s", f.Reason.ES, es)
			}
			if f.Reason.EN != f.Reason.ES && strings.Contains(es, Printable(f.Reason.EN)) {
				t.Errorf("the Spanish detail has the English reason %q:\n%s", f.Reason.EN, es)
			}
		}
	}
	if reds < 1000 || reasons < 500 {
		t.Fatalf("the corpora gave %d 🔴 results and %d reasons: did they shrink?", reds, reasons)
	}
}
