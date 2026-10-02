package i18n

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// sameInBoth are the keys whose text is the same in English and Spanish on
// purpose (a proper name, a command, a symbol). Each one says why.
var sameInBoth = map[string]string{
	"error.line":                "the label of an error is Error in both languages",
	"err.setup.usage_adb":       "a flag name and the message of the system: nothing to translate",
	"err.setup.usage_workspace": "a flag name and the message of the system: nothing to translate",
	"appsetup.summary.consent":  "a version and the state word of the JSON result: nothing to translate",
	"doctor.name.adb":           "adb is the name of the tool",
	"doctor.name.serve":         "serve is the name of a command",
	"version.adbuddy":           "a name, a version and a platform: nothing to translate",
}

// keyTables are the variables that hold catalog keys by a code (a map from an
// error code to the key of its fix): their string values count as used keys,
// and a call may take its key from them (`NewErrorT(code, msg, fixKeys[code])`).
// Each one says where it is.
var keyTables = map[string]string{
	"remoteFixKeys": "cli/remotecmd.go: the key of the fix of each code the daemon answers to `adbuddy remote`",
	"undoFixKeys":   "cli/undo.go: the key of the fix of each code the daemon answers to `adbuddy undo`",
}

// dynamicPrefixes are the key prefixes that a call builds at run time
// (`T("doctor.name."+id)`): every catalog key with one of them counts as used,
// and the call is accepted without a literal key. Each one says where it is built.
var dynamicPrefixes = map[string]string{
	"doctor.name.": "cli/doctor.go: checkName builds the key of the name of a check from its identifier",
	"doctor.list.": "cli/doctor.go: listMsg picks the pattern of a list by its length (1 to 3)",
}

// verbs lists the fmt verbs of a catalog pattern as "index:verb", and what is
// wrong with it. Every verb needs an explicit index (%[1]s), so that Spanish
// may reorder them and a pattern may use fewer arguments than its call gives.
func verbs(pattern string) (found []string, problems []string) {
	next := 1
	explicit, implicit := 0, 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '%' {
			continue
		}
		at := i
		i++
		if i >= len(pattern) {
			problems = append(problems, "a lone % at the end")
			break
		}
		if pattern[i] == '%' {
			continue
		}
		idx := 0
		if pattern[i] == '[' {
			end := strings.IndexByte(pattern[i:], ']')
			if end < 0 {
				problems = append(problems, "an unclosed [")
				break
			}
			n, err := strconv.Atoi(pattern[i+1 : i+end])
			if err != nil || n < 1 {
				problems = append(problems, "a bad index "+pattern[i:i+end+1])
				break
			}
			idx = n
			i += end + 1
			explicit++
		} else {
			idx = next
			implicit++
		}
		for i < len(pattern) && strings.IndexByte("+-# 0123456789.", pattern[i]) >= 0 {
			i++
		}
		if i >= len(pattern) || strings.IndexByte("sdvqxXtf", pattern[i]) < 0 {
			problems = append(problems, fmt.Sprintf("an unsupported verb at %q", pattern[at:min(i+1, len(pattern))]))
			break
		}
		found = append(found, fmt.Sprintf("%d:%c", idx, pattern[i]))
		next = idx + 1
	}
	if implicit > 0 {
		problems = append(problems, "every verb needs an explicit index (%[1]s, %[2]s)")
	}
	sort.Strings(found)
	return found, problems
}

var letters = regexp.MustCompile(`[\p{L}]{2,}`)

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// parityProblems compares two catalogs: same keys, no empty text, the same
// verbs with the same index in each pair, plural forms in pairs that use the
// count, and no text copied untranslated.
func parityProblems(en, es map[string]string) []string {
	var out []string
	add := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	for k, e := range en {
		s, ok := es[k]
		if !ok {
			add("%s: missing in es.toml", k)
			continue
		}
		if strings.TrimSpace(e) == "" {
			add("%s: empty in en.toml", k)
		}
		if strings.TrimSpace(s) == "" {
			add("%s: empty in es.toml", k)
		}
		ve, pe := verbs(e)
		vs, ps := verbs(s)
		for _, p := range pe {
			add("%s (en): %s", k, p)
		}
		for _, p := range ps {
			add("%s (es): %s", k, p)
		}
		if strings.Join(ve, " ") != strings.Join(vs, " ") {
			add("%s: the verbs differ: en %v, es %v", k, ve, vs)
		}
		if e == s && letters.MatchString(e) && sameInBoth[k] == "" {
			add("%s: the same text in en and es (untranslated?); list it in sameInBoth with a reason if it is right", k)
		}
	}
	for k := range es {
		if _, ok := en[k]; !ok {
			add("%s: in es.toml but not in en.toml (English is the source)", k)
		}
	}
	for k := range en {
		if base, ok := strings.CutSuffix(k, ".one"); ok {
			if _, ok := en[base+".other"]; !ok {
				add("%s: a .one without its .other", k)
			}
		}
		if base, ok := strings.CutSuffix(k, ".other"); ok {
			if _, ok := en[base+".one"]; !ok {
				add("%s: a .other without its .one", k)
			}
		}
		if strings.HasSuffix(k, ".one") || strings.HasSuffix(k, ".other") {
			for _, l := range []map[string]string{en, es} {
				if v, _ := verbs(l[k]); !contains(v, "1:d") {
					add("%s: a plural form must show the count with %%[1]d", k)
					break
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func readCatalog(t *testing.T, lang string) map[string]string {
	t.Helper()
	data, err := catalogFS.ReadFile("catalog/" + lang + ".toml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := parseCatalog(data)
	if err != nil {
		t.Fatalf("%s.toml: %v", lang, err)
	}
	return m
}

func TestCatalogParity(t *testing.T) {
	for _, p := range parityProblems(readCatalog(t, "en"), readCatalog(t, "es")) {
		t.Error(p)
	}
}

func TestParityAnalyzer(t *testing.T) {
	good := map[string]string{"a": "Hello %[1]s", "b.one": "%[1]d file", "b.other": "%[1]d files", "c": "100%% of %[1]s in %[2]s"}
	goodES := map[string]string{"a": "Hola %[1]s", "b.one": "%[1]d fichero", "b.other": "%[1]d ficheros", "c": "100%% de %[1]s en %[2]s"}
	if p := parityProblems(good, goodES); len(p) != 0 {
		t.Fatalf("a good pair has problems: %v", p)
	}
	bad := func(en, es map[string]string, want string) {
		t.Helper()
		for _, p := range parityProblems(en, es) {
			if strings.Contains(p, want) {
				return
			}
		}
		t.Errorf("no problem containing %q for %v / %v: %v", want, en, es, parityProblems(en, es))
	}
	bad(map[string]string{"a": "x"}, map[string]string{}, "missing in es.toml")
	bad(map[string]string{}, map[string]string{"a": "x"}, "not in en.toml")
	bad(map[string]string{"a": "Hello"}, map[string]string{"a": " "}, "empty in es.toml")
	bad(map[string]string{"a": "Hello %[1]s"}, map[string]string{"a": "Hola %[1]d"}, "verbs differ")
	bad(map[string]string{"a": "Hello %[1]s"}, map[string]string{"a": "Hola"}, "verbs differ")
	bad(map[string]string{"a": "%[1]s of %[2]s"}, map[string]string{"a": "%[2]s de %[1]d"}, "verbs differ")
	bad(map[string]string{"a": "%s of %s"}, map[string]string{"a": "%s de %s"}, "explicit index")
	bad(map[string]string{"a": "Hello %s"}, map[string]string{"a": "Hola %s"}, "explicit index")
	bad(map[string]string{"a": "100%!"}, map[string]string{"a": "100%!"}, "unsupported verb")
	bad(map[string]string{"a": "Same words"}, map[string]string{"a": "Same words"}, "untranslated")
	bad(map[string]string{"a.one": "%[1]d x"}, map[string]string{"a.one": "%[1]d y"}, "without its .other")
	bad(map[string]string{"a.one": "one", "a.other": "%[1]d x"}, map[string]string{"a.one": "uno", "a.other": "%[1]d y"}, "show the count")
	// The same verbs in another order are fine: Spanish may reorder.
	if p := parityProblems(map[string]string{"a": "%[1]s of %[2]s"}, map[string]string{"a": "%[2]s de %[1]s"}); len(p) != 0 {
		t.Errorf("reordering is allowed: %v", p)
	}
}

// ---- every key the code uses exists, and every key is used ----

type use struct {
	key   string
	args  int // the arguments the call passes after the key; -1 when it spreads a slice
	where string
}

// callSpec says where a function takes catalog keys: the positions of its key
// arguments and where the arguments of the pattern start (-1: unknown).
type callSpec struct {
	keys     []int
	argStart int
	method   bool // only as a method call (lang.T, app.T): a bare T(...) is something else
}

var callSpecs = map[string]callSpec{
	"T":               {keys: []int{0}, argStart: 1, method: true},
	"TN":              {keys: []int{0}, argStart: 1, method: true}, // the count is the first argument of the pattern
	"NewErrorT":       {keys: []int{1, 2}, argStart: 3},
	"ConfigErrorT":    {keys: []int{1, 2}, argStart: 3},
	"TransientErrorT": {keys: []int{1, 2}, argStart: 3},
	"UsageErrorT":     {keys: []int{0}, argStart: 1},
	"errorT":          {keys: []int{3, 4}, argStart: -1},           // cli/errors.go: the shared builder
	"M":               {keys: []int{0}, argStart: 1, method: true}, // i18n.M: a message as an argument of another
	"tell":            {keys: []int{1}, argStart: 2},               // cli/say.go: tell(w, key, args...)
	"say":             {keys: []int{0}, argStart: 1},               // the say and warn closures of setup (since H3a): say(key, args...)
	"warn":            {keys: []int{0}, argStart: 1},
	"failf":           {keys: []int{0}, argStart: 1}, // cli/doctor.go: failf(key, args...), an error that is a message
}

// keyUses finds the catalog keys a Go source asks for (see callSpecs). A key
// that is a parameter of the enclosing function, or a field of its receiver or
// of a parameter, is a forwarder (func (a *App) T(key string, …) { …T(key, …) })
// and is skipped. Any other key that is not a literal, and does not start
// with a dynamic prefix, is reported in dynamic.
func keyUses(fset *token.FileSet, f *ast.File) (uses []use, dynamic []string) {
	for _, d := range f.Decls {
		params := map[string]bool{}
		if fd, ok := d.(*ast.FuncDecl); ok {
			lists := []*ast.FieldList{fd.Type.Params}
			if fd.Recv != nil {
				lists = append(lists, fd.Recv)
			}
			for _, l := range lists {
				for _, fl := range l.List {
					for _, n := range fl.Names {
						params[n.Name] = true
					}
				}
			}
		}
		ast.Inspect(d, func(n ast.Node) bool { // the parameters of the closures count too: say := func(key string, …)
			if fl, ok := n.(*ast.FuncLit); ok && fl.Type.Params != nil {
				for _, f := range fl.Type.Params.List {
					for _, name := range f.Names {
						params[name.Name] = true
					}
				}
			}
			return true
		})
		ast.Inspect(d, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, method := "", false
			switch fn := c.Fun.(type) {
			case *ast.SelectorExpr:
				name, method = fn.Sel.Name, true
			case *ast.Ident:
				name = fn.Name
			}
			spec, ok := callSpecs[name]
			if !ok || (spec.method && !method) {
				return true
			}
			pos := fset.Position(c.Pos()).String()
			args := -1
			if spec.argStart >= 0 && c.Ellipsis == token.NoPos {
				args = len(c.Args) - spec.argStart
			}
			for _, i := range spec.keys {
				if i >= len(c.Args) || (spec.argStart >= 0 && len(c.Args) <= spec.argStart-1) {
					continue
				}
				e := c.Args[i]
				if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if k, err := strconv.Unquote(lit.Value); err == nil && k != "" {
						uses = append(uses, use{k, args, pos})
					}
					continue
				}
				if forwarded(e, params) {
					continue
				}
				if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.ADD {
					if lit, ok := b.X.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if p, _ := strconv.Unquote(lit.Value); dynamicPrefixes[p] != "" {
							continue
						}
					}
				}
				dynamic = append(dynamic, pos)
			}
			return true
		})
	}
	return uses, dynamic
}

// forwarded: e is a parameter of the enclosing function, or a field of one, or
// an element of a key table.
func forwarded(e ast.Expr, params map[string]bool) bool {
	if ix, ok := e.(*ast.IndexExpr); ok {
		if id, ok := ix.X.(*ast.Ident); ok && keyTables[id.Name] != "" {
			return true
		}
	}
	for {
		switch v := e.(type) {
		case *ast.Ident:
			return params[v.Name]
		case *ast.SelectorExpr:
			e = v.X
		default:
			return false
		}
	}
}

// tableUses are the keys of the key tables declared in f: the string values
// of `var <table> = map[string]string{…}`.
func tableUses(fset *token.FileSet, f *ast.File) (uses []use) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, s := range gd.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || keyTables[vs.Names[0].Name] == "" || len(vs.Values) != 1 {
				continue
			}
			if cl, ok := vs.Values[0].(*ast.CompositeLit); ok {
				for _, el := range cl.Elts {
					if kv, ok := el.(*ast.KeyValueExpr); ok {
						if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							if k, err := strconv.Unquote(lit.Value); err == nil {
								uses = append(uses, use{k, -1, fset.Position(lit.Pos()).String()})
							}
						}
					}
				}
			}
		}
	}
	return uses
}

func TestKeyUsesAnalyzer(t *testing.T) {
	src := `package p
func f(app *App, lang Lang) {
	_ = app.T("a.one")
	_ = lang.TN("a.n", 3, "x")
	_ = NewErrorT("C", "e.msg", "e.fix", 1, 2)
	_ = NewErrorT("C", "e.msg2", "", 1)
	_ = ConfigErrorT("C", "e.cfg", "e.cfgfix")
	_ = TransientErrorT("C", "e.tr", "e.trfix", 1)
	_ = UsageErrorT("e.use", 1)
	_ = i18n.M("m.msg", 1)
	tell(w, "t.say", 1, 2)
	_ = app.T("a.spread", xs...)
	_ = app.T("dyn." + id)
	_ = app.T(key)
	_ = other.T(key, args...)
	T("not.a.method")
}

var fixKeys = map[string]string{"A": "t.a", "B": "t.b"}

func g(code string) error { return NewErrorT(code, "e.table", fixKeys[code], 1) }

func (e *Error) in(lang Lang, key string) string {
	_ = lang.T(e.key, e.args...)
	_ = lang.T(key, e.args...)
	_ = lang.T(key)
	return errorT(1, false, "C", key, "e.shared", args)
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	old, oldTables := dynamicPrefixes, keyTables
	dynamicPrefixes = map[string]string{"dyn.": "test"}
	keyTables = map[string]string{"fixKeys": "test"}
	defer func() { dynamicPrefixes, keyTables = old, oldTables }()
	uses, dyn := keyUses(fset, f)
	uses = append(uses, tableUses(fset, f)...)
	got := map[string]int{}
	for _, u := range uses {
		got[u.key] = u.args
	}
	want := map[string]int{"a.one": 0, "a.n": 2, "e.msg": 2, "e.fix": 2, "e.msg2": 1, "e.cfg": 0, "e.cfgfix": 0, "e.tr": 1, "e.trfix": 1,
		"e.use": 1, "a.spread": -1, "e.shared": -1, "e.table": 1, "t.a": -1, "t.b": -1, "m.msg": 1, "t.say": 2}
	if len(got) != len(want) {
		t.Fatalf("uses %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: args %d, want %d", k, got[k], v)
		}
	}
	if len(dyn) != 2 { // app.T(key) and other.T(key, args...) in f, where key is not a parameter
		t.Errorf("dynamic keys %v, want two", dyn)
	}
}

// hostRoot is host/, two folders above this package.
func hostRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("host/go.mod not found at %s", root)
	}
	return root
}

func maxIndex(pattern string) int {
	found, _ := verbs(pattern)
	max := 0
	for _, v := range found {
		if n, _ := strconv.Atoi(v[:strings.IndexByte(v, ':')]); n > max {
			max = n
		}
	}
	return max
}

func TestKeysExist(t *testing.T) {
	en := readCatalog(t, "en")
	used := map[string]bool{}
	// The keys of one call (NewErrorT has two: the message and its fix) share
	// its arguments: the call gives exactly as many as the most demanding key
	// uses, and no key uses more than the call gives.
	type group struct {
		args int
		max  int
	}
	groups := map[string]*group{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(hostRoot(t), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "i18n" { // the package builds its own keys
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		uses, dynamic := keyUses(fset, f)
		uses = append(uses, tableUses(fset, f)...)
		for _, p := range dynamic {
			t.Errorf("%s: T, TN or NewErrorT with a key that is not a literal: declare its prefix in dynamicPrefixes", p)
		}
		for _, u := range uses {
			// A key used through TN is written key.one and key.other.
			pattern, ok := en[u.key]
			if !ok {
				if pattern, ok = en[u.key+".other"]; !ok {
					t.Errorf("%s: the key %q is not in catalog/en.toml", u.where, u.key)
					continue
				}
			}
			used[u.key] = true
			if u.args >= 0 {
				g := groups[u.where]
				if g == nil {
					g = &group{args: u.args}
					groups[u.where] = g
				}
				if n := maxIndex(pattern); n > u.args {
					t.Errorf("%s: %q takes %d argument(s) and the call gives %d", u.where, u.key, n, u.args)
				} else if n > g.max {
					g.max = n
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for where, g := range groups {
		if g.max != g.args {
			t.Errorf("%s: the call gives %d argument(s) and its keys use %d", where, g.args, g.max)
		}
	}
	for k := range en {
		base := strings.TrimSuffix(strings.TrimSuffix(k, ".one"), ".other")
		if used[k] || used[base] {
			continue
		}
		dyn := false
		for p := range dynamicPrefixes {
			dyn = dyn || strings.HasPrefix(k, p)
		}
		if !dyn {
			t.Errorf("the key %q is in the catalog and nothing uses it", k)
		}
	}
}
