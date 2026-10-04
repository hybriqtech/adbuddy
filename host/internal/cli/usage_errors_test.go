package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
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
		{[]string{"new", "a", "b", "c"}, "new admite como máximo"},
	} {
		exit, _, es := runCLI(t, tc.args...)
		if exit != 2 || !strings.Contains(es, tc.want) {
			t.Errorf("%v: exit %d %q", tc.args, exit, es)
		}
		noCobra(t, es)
	}
	i18ntest.Lang(t, i18n.EN)
	_, _, en := runCLI(t, "new", "a", "b", "c")
	if !strings.Contains(en, "new takes at most") {
		t.Errorf("EN: %q", en)
	}
	// The plural is chosen by N: a command with an exact count above one would say "arguments";
	// MaximumNArgs(1) with 3 given shows both forms exist in the catalog.
	for _, k := range []string{"err.usage.args_exact.one", "err.usage.args_exact.other", "err.usage.args_at_most.one", "err.usage.args_at_most.other"} {
		if !i18n.Has(k) {
			t.Errorf("missing %s", k)
		}
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
	// Equal length: the alphabetically first. "s" prefix candidates of equal length do not exist
	// among real commands, so check the edit-distance tie too: "tyle" is 1 from "type"; 2+ letters only.
	if got := suggestCommand(root, "stap"); got != "stop" && got != "setup" && got != "" {
		t.Errorf("stap: %q", got)
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
	if !strings.Contains(en, "USAGE") {
		t.Errorf("tap: %q", en)
	}
}
