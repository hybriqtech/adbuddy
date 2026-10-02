package cli

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
)

// M6c §9.3: `adbuddy setup` in Spanish. The same scenario runs in both
// languages: nothing of the English catalog may show in Spanish, no line with a
// phrase may be the same in both (text that never moved to the catalog), the
// commands and the values stay as they are, and the JSON is the same.
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
