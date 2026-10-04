package cli

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// runCLI is Main with its output, in English or Spanish as ADBUDDY_LANG says.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Main(args, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

// M6c §3 and §4.2: the human message of an error is in the language of the
// person; --json, the agent and the code of the error never change.
func TestAnErrorIsSpanishForAPersonAndEnglishForMachines(t *testing.T) {
	isolate(t)
	// `undo` without serve running: DAEMON_NOT_RUNNING, a shared error with a message and a fix.
	const code = "DAEMON_NOT_RUNNING"

	i18ntest.Lang(t, i18n.EN)
	exit, _, en := runCLI(t, "undo")
	if exit != 1 || !strings.Contains(en, "Error: "+code+" — the ADBuddy host is not running\nFix: Start it with `adbuddy serve` in another terminal.\n") {
		t.Fatalf("English: exit %d %q", exit, en)
	}
	_, jsonEN, _ := runCLI(t, "undo", "--json")

	i18ntest.Lang(t, i18n.ES)
	exit, _, es := runCLI(t, "undo")
	if exit != 1 || !strings.Contains(es, "Error: "+code+" — el host de ADBuddy no está en marcha\nArreglo: Arráncalo con `adbuddy serve` en otra terminal.\n") {
		t.Fatalf("Spanish: exit %d %q", exit, es)
	}
	i18ntest.NoEnglish(t, es)

	// --json: the same bytes in either language.
	_, jsonES, _ := runCLI(t, "undo", "--json")
	if jsonES != jsonEN || !strings.Contains(jsonES, `"message":"the ADBuddy host is not running"`) || !strings.Contains(jsonES, `"code":"`+code+`"`) {
		t.Fatalf("--json changed with the language:\n%s\n%s", jsonEN, jsonES)
	}

	// The agent's session: English, whatever the person's language.
	t.Setenv("ADBUDDY_SOURCE", "agent")
	_, _, agent := runCLI(t, "undo")
	if agent != en {
		t.Fatalf("the agent must read the English one:\n%q\n%q", agent, en)
	}
}

// An error that has no catalog keys (a cause of the system, a command that is
// not migrated) still comes out, in English.
func TestAnErrorWithoutKeysKeepsItsEnglish(t *testing.T) {
	var out, errOut bytes.Buffer
	WriteFailureIn(&out, &errOut, false, i18n.ES, NewError("SOME_CODE", "a cause of the system", "Try again."))
	if got, want := errOut.String(), "Error: SOME_CODE — a cause of the system\nArreglo: Try again.\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNewErrorTComputesTheEnglishAtOnce(t *testing.T) {
	e := UsageErrorT("error.cause", "needs a number")
	if e.Code != "USAGE" || e.Message != "needs a number" || e.Fix != "Run `adbuddy --help` to see the usage." || ExitCode(e) != ExitUsage {
		t.Fatalf("%+v", e)
	}
	c := ConfigErrorT("X", "error.cause", "", "boom")
	if c.Fix != "" || ExitCode(c) != ExitConfig || c.Message != "boom" {
		t.Fatalf("%+v", c)
	}
	if tr := TransientErrorT("X", "error.cause", "", "boom"); !tr.Transient || ExitCode(tr) != ExitTransient {
		t.Fatalf("%+v", tr)
	}
}

// M6c H0: mainContext (what Main wraps) writes the same JSON whatever the
// language: --json is for programs.
func TestMainContextJSONIgnoresTheLanguage(t *testing.T) {
	isolate(t)
	var got [2]string
	for i, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		i18ntest.Lang(t, lang)
		out, errOut, _ := runCtx(t, t.Context(), "undo", "--json")
		got[i] = out + "|" + errOut
	}
	if got[0] != got[1] || !strings.Contains(got[0], `"code":"DAEMON_NOT_RUNNING"`) {
		t.Fatalf("--json changed with the language:\n%s\n%s", got[0], got[1])
	}
}

// M6c H2: the fix of a code the daemon answers `adbuddy remote on` with is in
// the language of the person; the cause (the daemon's English) stays as it is.
func TestRemoteDeniedFixIsSpanish(t *testing.T) {
	data := isolate(t)
	fakeDaemon(t, data, nil, ipc.Exit{Code: 126, ErrorCode: "REMOTE_DENIED", Error: "the remote mode was not turned on"})
	i18ntest.Lang(t, i18n.ES)
	exit, _, es := runCLI(t, "remote", "on")
	if exit != 126 || !strings.Contains(es, "Error: REMOTE_DENIED — the remote mode was not turned on\nArreglo: Vuelve a ejecutar `adbuddy remote on` y permite la pregunta") {
		t.Fatalf("exit %d %q", exit, es)
	}
}

// A usage error points at the help of the command that failed.
func TestAnErrorPointsAtTheHelpOfItsCommand(t *testing.T) {
	isolate(t)
	i18ntest.Lang(t, i18n.EN)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"tap"}, "Run `adbuddy tap --help`"},
		{[]string{"journal", "--bad"}, "Run `adbuddy journal --help`"},
		{[]string{"serv"}, "Run `adbuddy --help`"},
	} {
		exit, _, errOut := runCLI(t, c.args...)
		if exit != 2 || !strings.Contains(errOut, c.want) {
			t.Fatalf("%v: exit %d %q", c.args, exit, errOut)
		}
	}
	jexit, out, _ := runCLI(t, "tap", "--json")
	if jexit != 2 || !strings.Contains(out, `"fix":"Run `+"`adbuddy tap --help`"+` to see the usage."`) || !strings.Contains(out, `"code":"USAGE"`) {
		t.Fatalf("--json: exit %d %s", jexit, out)
	}
	noFix := regexp.MustCompile(`"fix":"[^"]*",`).ReplaceAllString(out, "")
	if strings.TrimSpace(noFix) != `{"ok":false,"error":{"code":"USAGE","message":"tap needs exactly one of a text, --id or --xy","transient":false}}` {
		t.Fatalf("--json envelope without fix: %s", noFix)
	}
	i18ntest.Lang(t, i18n.ES)
	exit, _, es := runCLI(t, "tap")
	if exit != 2 || !strings.Contains(es, "adbuddy tap --help") || !strings.Contains(es, "cómo se usa") {
		t.Fatalf("es: exit %d %q", exit, es)
	}
}
