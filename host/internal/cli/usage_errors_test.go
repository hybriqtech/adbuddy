package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
	"github.com/spf13/cobra"
)

// cobraTexts are strings that only cobra and pflag write; none may reach a person.
var cobraTexts = []string{"unknown command", "Did you mean", "arg(s)", "flag needs", "accepts ", "unknown flag", "unknown shorthand", "invalid argument", "bad flag syntax"}

func noCobra(t *testing.T, out string) {
	t.Helper()
	for _, s := range cobraTexts {
		if strings.Contains(out, s) {
			t.Errorf("cobra text %q in %q", s, out)
		}
	}
}

func TestUsageErrorsUnknownCommand(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.EN)
	exit, _, en := runCLI(t, "serv")
	if exit != 2 || !strings.Contains(en, "Error: USAGE — unknown command \"serv\": did you mean \"serve\"?") || !strings.Contains(en, "Run `adbuddy --help`") {
		t.Fatalf("EN: exit %d %q", exit, en)
	}
	i18ntest.Lang(t, i18n.ES)
	exit, _, es := runCLI(t, "serv")
	if exit != 2 || !strings.Contains(es, `el comando "serv" no existe: ¿querías decir "serve"?`) {
		t.Fatalf("ES: exit %d %q", exit, es)
	}
	noCobra(t, es)
	i18ntest.NoEnglish(t, es)
	if strings.Contains(es, "env") || strings.Contains(es, "send") {
		t.Errorf("bad suggestion: %q", es)
	}
	exit, out, _ := runCLI(t, "serv", "--json")
	if exit != 2 {
		t.Fatalf("json exit %d", exit)
	}
	var env struct {
		Error struct{ Code, Message, Fix string }
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err, out)
	}
	if env.Error.Code != "USAGE" || env.Error.Message != `unknown command "serv": did you mean "serve"?` || env.Error.Fix == "" {
		t.Fatalf("json: %+v", env.Error)
	}
	// No candidate: no suggestion.
	i18ntest.Lang(t, i18n.EN)
	_, _, en = runCLI(t, "zzzzz")
	if !strings.Contains(en, `unknown command "zzzzz"`) || strings.Contains(en, "did you mean") {
		t.Fatalf("no candidate: %q", en)
	}
}

func TestUsageErrorsSubcommandNoArgs(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.ES)
	exit, _, es := runCLI(t, "doctor", "serv")
	if exit != 2 || !strings.Contains(es, `doctor no admite argumentos (recibió "serv")`) {
		t.Fatalf("exit %d %q", exit, es)
	}
	noCobra(t, es)
	if strings.Contains(es, "querías decir") || strings.Contains(es, "serve") {
		t.Errorf("suggestion for another command's argument: %q", es)
	}
}

func TestUsageErrorsArgCounts(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.ES)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"connect"}, "connect admite 1 argumento, recibió 0"},
		{[]string{"connect", "a", "b", "c"}, "connect admite 1 argumento, recibió 3"},
		{[]string{"new", "a", "b"}, "new admite como máximo 1 argumento, recibió 2"},
	} {
		exit, _, es := runCLI(t, tc.args...)
		if exit != 2 || !strings.Contains(es, tc.want) {
			t.Errorf("%v: exit %d %q", tc.args, exit, es)
		}
		noCobra(t, es)
	}
	i18ntest.Lang(t, i18n.EN)
	_, _, en := runCLI(t, "new", "a", "b")
	if !strings.Contains(en, "Error: USAGE — new takes at most 1 argument, got 2") {
		t.Errorf("EN: %q", en)
	}
}

// The plural forms no real command uses (exact 2, at most 2) are checked directly.
func TestUsageErrorsPluralForms(t *testing.T) {
	root := NewRoot(&App{})
	cmd, _, err := root.Find([]string{"connect"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		in, en, es string
	}{
		{"accepts 2 arg(s), received 0", "connect takes 2 arguments, got 0", "connect admite 2 argumentos, recibió 0"},
		{"accepts 1 arg(s), received 3", "connect takes 1 argument, got 3", "connect admite 1 argumento, recibió 3"},
		{"accepts at most 2 arg(s), received 5", "connect takes at most 2 arguments, got 5", "connect admite como máximo 2 argumentos, recibió 5"},
		{"accepts at most 1 arg(s), received 4", "connect takes at most 1 argument, got 4", "connect admite como máximo 1 argumento, recibió 4"},
	} {
		e := translateArgsError(root, cmd, errors.New(tc.in))
		if e == nil {
			t.Fatalf("%q not translated", tc.in)
		}
		if got := msgIn(e, i18n.EN); got != tc.en {
			t.Errorf("EN %q: %q, want %q", tc.in, got, tc.en)
		}
		if got := msgIn(e, i18n.ES); got != tc.es {
			t.Errorf("ES %q: %q, want %q", tc.in, got, tc.es)
		}
	}
}

func TestUsageErrorsQuotedArgumentIsNotEscapedTwice(t *testing.T) {
	root := NewRoot(&App{})
	e := translateArgsError(root, root, errors.New(`unknown command "serv\"x" for "adbuddy"`))
	if e == nil {
		t.Fatal("not translated")
	}
	if got, want := e.Message, `unknown command "serv\"x": did you mean "serve"?`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	e = translateArgsError(root, root, errors.New(`unknown command "doc\"x" for "adbuddy journal"`))
	if got, want := e.Message, `journal takes no arguments (got "doc\"x")`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUsageErrorsFlags(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.ES)
	for _, tc := range []struct {
		args []string
		want string
		fix  string
	}{
		{[]string{"journal", "--bad"}, "opción desconocida: --bad", "adbuddy journal --help"},
		{[]string{"journal", "-z"}, "opción desconocida: -z", "adbuddy journal --help"},
		{[]string{"journal", "--limit"}, "--limit necesita un valor", ""},
		{[]string{"journal", "--limit", "abc"}, `valor no válido "abc" para --limit`, ""},
		{[]string{"screenshot", "--scale", "x"}, `valor no válido "x" para --scale`, ""},
		{[]string{"screenshot", "-o"}, "-o necesita un valor", ""},
		{[]string{"--nope"}, "opción desconocida: --nope", ""},
	} {
		exit, _, es := runCLI(t, tc.args...)
		if exit != 2 || !strings.Contains(es, tc.want) || !strings.Contains(es, "Error: USAGE") {
			t.Errorf("%v: exit %d %q", tc.args, exit, es)
		}
		if tc.fix != "" && !strings.Contains(es, tc.fix) {
			t.Errorf("%v: fix %q not in %q", tc.args, tc.fix, es)
		}
		noCobra(t, es)
	}
	// Bad syntax is a usage error (exit 2), not INTERNAL.
	i18ntest.Lang(t, i18n.EN)
	exit, _, en := runCLI(t, "journal", "---x")
	if exit != 2 || !strings.Contains(en, "Error: USAGE — malformed option: ---x") {
		t.Errorf("syntax: exit %d %q", exit, en)
	}
}

func TestSuggestCommand(t *testing.T) {
	root := NewRoot(&App{})
	for name, want := range map[string]string{
		"serv": "serve", "jour": "journal", "jurnal": "journal", "zzz": "", "ver": "version",
		"s": "", "set": "setup", "hook": "", "help": "", "completion": "", "comp": "",
		"Serv": "serve",
	} {
		if got := suggestCommand(root, name); got != want {
			t.Errorf("suggestCommand(%q) = %q, want %q", name, got, want)
		}
	}
	synth := func(names ...string) *cobra.Command {
		r := &cobra.Command{Use: "x"}
		for _, n := range names {
			r.AddCommand(&cobra.Command{Use: n, Run: func(*cobra.Command, []string) {}})
		}
		return r
	}
	for _, tc := range []struct {
		names []string
		in    string
		want  string
	}{
		{[]string{"abcd", "abc", "abce"}, "ab", "abc"}, // prefix: the shortest
		{[]string{"abd", "abc", "abe"}, "ab", "abc"},   // prefix, same length: alphabetical
		{[]string{"aab", "aaa"}, "aac", "aaa"},         // edit distance 1 for both: alphabetical
		{[]string{"zzzz", "aaaa"}, "aabb", "aaaa"},     // closest by distance (2) wins
		{[]string{"aaaa"}, "bbbb", ""},                 // too far
		{[]string{"aaaa"}, "a", ""},                    // one letter: no prefix; distance 3 is too far
	} {
		if got := suggestCommand(synth(tc.names...), tc.in); got != tc.want {
			t.Errorf("%v %q: %q, want %q", tc.names, tc.in, got, tc.want)
		}
	}
}

func TestTranslatorsLeaveUnknownErrorsAlone(t *testing.T) {
	if translateFlagError(errors.New("algo raro")) != nil {
		t.Error("translateFlagError translated an unknown error")
	}
	if translateArgsError(NewRoot(&App{}), nil, errors.New("algo raro")) != nil {
		t.Error("translateArgsError translated an unknown error")
	}
}

func TestUsageErrorsHelpTopic(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.EN)
	_, _, en := runCLI(t, "help", "serv")
	if !strings.Contains(en, `unknown help topic "serv": did you mean "serve"?`) {
		t.Errorf("help serv: %q", en)
	}
	_, _, en = runCLI(t, "help", "zzzzz")
	if !strings.Contains(en, `unknown help topic "zzzzz"`) || strings.Contains(en, "did you mean") {
		t.Errorf("help zzzzz: %q", en)
	}
	_, _, en = runCLI(t, "tap")
	if !strings.Contains(en, "Error: USAGE — tap needs exactly one of a text, --id or --xy") {
		t.Errorf("tap: %q", en)
	}
}

func msgIn(e *Error, lang i18n.Lang) string {
	m, _ := e.in(lang)
	return m
}
