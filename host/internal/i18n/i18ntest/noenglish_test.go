package i18ntest

import (
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

func TestProblems(t *testing.T) {
	pairs := []i18n.Pair{
		{Key: "a.hello", EN: "Hello %[1]s, welcome back", ES: "Hola %[1]s, bienvenido"},
		{Key: "a.short", EN: "Done", ES: "Hecho"},
		{Key: "a.same", EN: "Same text for everyone", ES: "Same text for everyone"},
		{Key: "a.cmd", EN: "Run `adbuddy setup` now", ES: "Ejecuta `adbuddy setup` ahora"},
	}
	has := func(out string, except []string, want ...string) {
		t.Helper()
		got := strings.Join(Problems(out, pairs, except...), "\n")
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%q: %q lacks %q", out, got, w)
			}
		}
		if len(want) == 0 && got != "" {
			t.Errorf("%q: unexpected %q", out, got)
		}
	}
	has("Hola Ana, bienvenido\nHecho", nil)
	has("line\nHello Ana, welcome back\n", nil, "a.hello")
	has("Hello Ana, welcome back", []string{"a.hello"})
	has("Done", nil)                   // too short to look for
	has("Same text for everyone", nil) // the same in both languages
	has("Run `adbuddy setup` now", nil, "a.cmd")
	has("a.hello", nil, "the name of a catalog key")
	has("doctor.check.adb.missing\n", nil, "the name of a catalog key")
	has("see adbuddy.exe and v1.2.3 for details", nil) // not keys
}

func TestUntranslated(t *testing.T) {
	en := "Getting adb…\nRun: adbuddy setup\nPhone: Pixel 9, Android 17.\nNote: not translated yet\n"
	es := "Obteniendo adb…\nRun: adbuddy setup\nMóvil: Pixel 9, Android 17.\nNote: not translated yet\n"
	got := Untranslated(en, es)
	if len(got) != 2 || got[0] != "Run: adbuddy setup" || got[1] != "Note: not translated yet" {
		t.Fatalf("got %q", got)
	}
	if got := Untranslated(en, es, "Run: adbuddy"); len(got) != 1 || got[0] != "Note: not translated yet" {
		t.Fatalf("with an allowed line: %q", got)
	}
}
