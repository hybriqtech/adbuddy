package cli

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The text that a person reads comes from the catalog (app.T, NewErrorT…), not
// from a literal in the code. This test finds the literals that are left in
// internal/cli, by rule:
//
//	cobra   the Short or Long of a cobra.Command
//	flag    the help of a flag
//	error   the message or the fix of NewError, ConfigError, TransientError, UsageError
//	print   the text of fmt.Fprint, Fprintf or Fprintln, and of the say and warn helpers of setup
//	check   the Detail or Guidance of a doctor check
//
// Examples are command lines and are not text. What is left is counted per
// file and rule in testdata/textlint.txt and can only go down: a new literal
// fails the test, and so does a count that is lower than the file says (lower
// it with -update-lint, and read the diff). exemptions are the files whose
// text is not for a person (it is for the agent, or it is a log), each with
// its reason.
var updateLint = flag.Bool("update-lint", false, "rewrite testdata/textlint.txt")

// exemptions: file -> rule -> why that text stays in English and in the code.
var exemptions = map[string]map[string]string{
	"helpers.go": {
		"error": "the commands of the agent (screen, tap, type, foreground, screenshot) answer in English always (M6c spec section 3)",
	},
	"send.go": {
		"error": "adbuddy send is for the agent: its errors are English always (M6c spec section 3)",
	},
}

var wordRE = regexp.MustCompile(`\p{L}{2,}`)

// hasText reports whether e is, or contains, a literal with a word in it: a
// string, a concatenation, or the format of a fmt.Sprintf.
func hasText(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return false
		}
		s, err := strconv.Unquote(v.Value)
		return err == nil && wordRE.MatchString(s)
	case *ast.BinaryExpr:
		return v.Op == token.ADD && (hasText(v.X) || hasText(v.Y))
	case *ast.ParenExpr:
		return hasText(v.X)
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" && len(v.Args) > 0 {
			return hasText(v.Args[0])
		}
	}
	return false
}

// hasSpace reports whether a literal of e (a string or a concatenation) has a space.
func hasSpace(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		return v.Kind == token.STRING && strings.Contains(v.Value, " ")
	case *ast.BinaryExpr:
		return v.Op == token.ADD && (hasSpace(v.X) || hasSpace(v.Y))
	case *ast.ParenExpr:
		return hasSpace(v.X)
	}
	return false
}

var flagFuncs = map[string]bool{
	"String": true, "StringVar": true, "StringP": true, "StringVarP": true, "Bool": true, "BoolVar": true, "BoolP": true, "BoolVarP": true,
	"Int": true, "IntVar": true, "IntP": true, "IntVarP": true, "Duration": true, "DurationVar": true, "DurationP": true,
	"StringSlice": true, "StringSliceVar": true, "StringArray": true, "StringArrayVar": true, "Float64": true, "Float64Var": true,
}

type lintHit struct {
	file, rule string
	line       int
}

// lintFile finds the literals of one file.
func lintFile(fset *token.FileSet, name string, f *ast.File) []lintHit {
	var hits []lintHit
	add := func(rule string, p token.Pos) { hits = append(hits, lintHit{name, rule, fset.Position(p).Line}) }
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.KeyValueExpr:
			if id, ok := v.Key.(*ast.Ident); ok && hasText(v.Value) {
				switch id.Name {
				case "Short", "Long":
					add("cobra", v.Pos())
				case "Detail", "Guidance":
					add("check", v.Pos())
				}
			}
		case *ast.CallExpr:
			var recv ast.Expr
			fname := ""
			switch fn := v.Fun.(type) {
			case *ast.Ident:
				fname = fn.Name
			case *ast.SelectorExpr:
				fname, recv = fn.Sel.Name, fn.X
			}
			switch {
			case fname == "NewError" || fname == "ConfigError" || fname == "TransientError":
				if len(v.Args) == 3 && (hasText(v.Args[1]) || hasText(v.Args[2])) {
					add("error", v.Pos())
				}
			case fname == "UsageError":
				if len(v.Args) == 1 && hasText(v.Args[0]) {
					add("error", v.Pos())
				}
			case (fname == "Fprint" || fname == "Fprintf" || fname == "Fprintln") && recv != nil:
				if len(v.Args) >= 2 && hasText(v.Args[1]) {
					add("print", v.Pos())
				}
			case (fname == "say" || fname == "warn") && recv == nil:
				// A key (setup.getting_tools) has no space: only a phrase is text.
				if len(v.Args) >= 1 && hasText(v.Args[0]) && hasSpace(v.Args[0]) {
					add("print", v.Pos())
				}
			case flagFuncs[fname] && recv != nil && len(v.Args) > 0:
				if c, ok := recv.(*ast.CallExpr); ok {
					if s, ok := c.Fun.(*ast.SelectorExpr); ok && (s.Sel.Name == "Flags" || s.Sel.Name == "PersistentFlags") && hasText(v.Args[len(v.Args)-1]) {
						add("flag", v.Pos())
					}
				}
			}
		}
		return true
	})
	return hits
}

// lintCounts is the literals of the Go files of dir per file and rule, without
// the exempt ones.
func lintCounts(t *testing.T, dir string) map[string]int {
	t.Helper()
	fset := token.NewFileSet()
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, path := range names {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		base := filepath.Base(path)
		for _, h := range lintFile(fset, base, f) {
			if exemptions[base][h.rule] != "" {
				continue
			}
			out[base+" "+h.rule]++
		}
	}
	return out
}

func formatLint(c map[string]int) string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Text literals left in internal/cli, per file and rule (textlint_test.go). It only goes down.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %d\n", k, c[k])
	}
	return b.String()
}

func readLint(t *testing.T) map[string]int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "textlint.txt"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 3 {
			t.Fatalf("textlint.txt: %q", line)
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			t.Fatalf("textlint.txt: %q", line)
		}
		out[parts[0]+" "+parts[1]] = n
	}
	return out
}

func TestNoTextLeftInTheCode(t *testing.T) {
	got := lintCounts(t, ".")
	if *updateLint {
		if err := os.WriteFile(filepath.Join("testdata", "textlint.txt"), []byte(formatLint(got)), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want := readLint(t)
	seen := map[string]bool{}
	var keys []string
	for k := range got {
		seen[k] = true
		keys = append(keys, k)
	}
	for k := range want {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch g, w := got[k], want[k]; {
		case g > w:
			t.Errorf("%s: %d literals and %d recorded: text for a person goes in catalog/en.toml and es.toml and is used with app.T or NewErrorT (see host/README.md)", k, g, w)
		case g < w:
			t.Errorf("%s: %d literals left and %d recorded: well done, lower it (go test ./internal/cli -run TestNoTextLeftInTheCode -update-lint)", k, g, w)
		}
	}
}

func TestTextLintSeesWhatItShould(t *testing.T) {
	src := `package p
func f(app *App) {
	_ = &cobra.Command{Short: "Do the thing", Long: app.T("x.long"), Example: "  adbuddy x"}
	cmd.Flags().BoolVar(&fix, "fix", false, "also restart the server")
	cmd.Flags().BoolVar(&fix, "ok", false, app.T("x.flag.ok"))
	_ = NewError("C", "went wrong", "")
	_ = NewError("C", err.Error(), "Check the file.")
	_ = NewError("C", err.Error(), "")
	_ = NewErrorT("C", "x.msg", "x.fix")
	_ = UsageError("needs a number")
	_ = UsageError(err.Error())
	fmt.Fprintln(app.Stderr, "Now doing it")
	fmt.Fprintf(app.Stdout, "%s · %d\n", a, b)
	fmt.Fprintf(w, "done in %s\n", d)
	say("Looking for your phone…\n")
	say("%s", prompt)
	warn(app.T("x.warn"))
	fmt.Fprintln(app.Stderr, app.T("x.say"))
	_ = check{Name: "adb", Detail: "not installed", Guidance: app.T("x.g")}
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, h := range lintFile(fset, "p.go", f) {
		got[h.rule]++
	}
	want := map[string]int{"cobra": 1, "flag": 1, "error": 3, "print": 3, "check": 1}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %d, want %d", k, got[k], v)
		}
	}
}
