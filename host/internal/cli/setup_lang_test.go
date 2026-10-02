package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// M6c §9.3: `adbuddy setup` in Spanish. The same scenario runs in both
// languages: nothing of the English catalog may show in Spanish, no line with a
// phrase may be the same in both (text that never moved to the catalog), and
// the commands the person is told to run stay as they are. (The JSON being the
// same in both languages is TestSetupJSONIgnoresTheLanguage.)
func TestSetupInSpanishHasNothingInEnglish(t *testing.T) {
	scenario := func(lang i18n.Lang) (stdout, stderr string) {
		e := newSetupEnv(t, phoneRules())
		i18ntest.Lang(t, lang)
		out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
		if code != 0 {
			t.Fatalf("%s: exit %d\nstdout %s\nstderr %s", lang, code, out, errOut)
		}
		// The paths of the run are not part of what is compared.
		return strings.ReplaceAll(out, e.ws, "<ws>"), strings.ReplaceAll(errOut, e.ws, "<ws>")
	}
	outEN, errEN := scenario(i18n.EN)
	outES, errES := scenario(i18n.ES)

	i18ntest.NoEnglish(t, errES)
	i18ntest.NoEnglish(t, outES)
	// Same on purpose: the URL of the guide, the commands in backticks, the QR art and the serial are not text.
	for _, l := range i18ntest.Untranslated(errEN+outEN, errES+outES, "http", "`adbuddy", "adb-AAAA", "█", "▄", "▀") {
		t.Errorf("a line that is the same in both languages (text outside the catalog?): %q", l)
	}
	for _, want := range []string{"Depuración inalámbrica", "emparejamiento", "Listo:"} {
		if !strings.Contains(errES+outES, want) {
			t.Errorf("the Spanish run lacks %q:\n%s\n%s", want, errES, outES)
		}
	}
	// The commands the person is told to run are the same.
	for _, cmd := range []string{"adbuddy serve", "adbuddy run"} {
		if strings.Contains(errEN+outEN, cmd) != strings.Contains(errES+outES, cmd) {
			t.Errorf("%q is in one language and not in the other", cmd)
		}
	}
}

// With a terminal to ask on, the agent to use is asked in Spanish, in the
// Spanish prompt of the catalog, and the answer is taken as in English.
func TestSetupAsksTheAgentInSpanish(t *testing.T) {
	e := newSetupEnv(t, phoneRules())
	bin := filepath.Join(filepath.Dir(e.ws), "bin")
	if err := os.WriteFile(filepath.Join(bin, testutil.Exe("codex")), []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	setVar(t, &isTerminal, func(any) bool { return true })
	i18ntest.Lang(t, i18n.ES)
	_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\ncodex\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	for _, want := range []string{"Agente que se usa (claude, codex) [claude]: ", "codex"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("the Spanish run lacks %q:\n%s", want, errOut)
		}
	}
	i18ntest.NoEnglish(t, errOut)
}

// --json is for programs: the same bytes in either language.
func TestSetupJSONIgnoresTheLanguage(t *testing.T) {
	run := func(lang i18n.Lang) string {
		e := newSetupEnv(t, phoneRules())
		i18ntest.Lang(t, lang)
		out, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws, "--json")
		if code != 0 {
			t.Fatalf("%s: exit %d\n%s\n%s", lang, code, out, errOut)
		}
		// What changes from run to run (the time, the paths of the run) is not compared.
		var v struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal([]byte(out), &v); err != nil {
			t.Fatal(err)
		}
		delete(v.Data, "workspace")
		delete(v.Data["device"].(map[string]any), "collectedAt")
		b, _ := json.Marshal(v.Data)
		return string(b)
	}
	if en, es := run(i18n.EN), run(i18n.ES); en != es {
		t.Errorf("the JSON of setup depends on the language:\n%s\n%s", en, es)
	}
}

// The state of the consent is a word for a person: Spanish in Spanish; the
// value in the JSON (appResult.Consent) is not touched.
func TestAppSummaryConsentWordFollowsTheLanguage(t *testing.T) {
	for consent, es := range map[string]string{"denied": "denegado", "timeout": "sin respuesta a tiempo", "skipped": "omitido"} {
		r := appResult{Installed: true, Version: "0.1.0", Consent: consent}
		if got, want := fmtAppMsg(r).In(i18n.EN), "app 0.1.0 ("+consent+")"; got != want {
			t.Errorf("%s: %q, want %q", consent, got, want)
		}
		if got, want := fmtAppMsg(r).In(i18n.ES), "app 0.1.0 ("+es+")"; got != want {
			t.Errorf("%s: %q, want %q", consent, got, want)
		}
	}
}
