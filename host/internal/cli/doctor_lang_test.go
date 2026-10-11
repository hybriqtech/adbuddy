package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// M6c §9.3: `adbuddy doctor` in Spanish. Each scenario runs in both languages
// against the same fake adb: the Spanish report has nothing of the English
// catalog and no phrase that stayed in the code, the commands of the Run lines
// are the same, and the JSON (what an agent reads) does not depend on the language.
func TestDoctorInSpanish(t *testing.T) {
	noServer := testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n"}
	cases := []struct {
		name  string
		rules []testutil.FakeRule
		want  []string // in Spanish
		cmds  []string // in both languages
	}{
		{"healthy phone, no serve", doctorRules(), []string{
			"✓ configuración:", "✓ servidor adb: puerto", "⚠ serve: no está en marcha: el agente no puede usar el móvil",
			"    Ejecuta: adbuddy serve", "    Qué hacer: Déjalo abierto en su propia terminal", "✓ móvil:", "✓ identidad: es el móvil que configuraste",
			"✓ app del móvil:", "Veredicto: degradado"}, []string{"adbuddy serve"}},
		{"no phone", doctorRules(testutil.FakeRule{Match: `^devices$`, Stdout: "List of devices attached\n"}), []string{
			"✗ móvil: el móvil que configuraste no está conectado", "    Qué hacer: Activa la depuración inalámbrica",
			"· identidad: no comprobado: el móvil no está conectado", "Veredicto: roto — arregla primero las líneas ✗"}, nil},
		{"old adb", doctorRules(testutil.FakeRule{Match: `^version$`, Stdout: "Version 35.0.2-1\n"}), []string{
			"✗ adb:", "versión 35.0.2", "    Ejecuta: adbuddy setup", "· servidor adb: no comprobado: el adb de ADBuddy no funciona"}, []string{"adbuddy setup"}},
		{"wrong phone", doctorRules(testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "ZZZZ9999\n"}), []string{
			"✗ identidad: el móvil en ", "no es el que configuraste", "ejecuta `adbuddy connect <ip:puerto>`"}, []string{"adbuddy connect <ip:port>", "adbuddy connect <ip:puerto>"}},
		{"app without its settings", doctorRules(testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,com.other,10124\n"}), []string{
			"⚠ app del móvil: 0.2.0 instalada, pero sin: la exención de batería", "    Qué hacer: Acepta la pantalla de consentimiento"}, []string{"adbuddy setup"}},
		{"persist port", doctorRules(testutil.FakeRule{Match: `^shell getprop service\.adb\.tcp\.port$`, Stdout: "0\n"}, testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n"}), []string{
			"⚠ puerto de red: cerrado por ahora, pero persist.adb.tcp.port vale 5555", "    Qué hacer: ADBuddy no la fijó"}, nil},
		{"adb server down", doctorRules(noServer), []string{
			"✗ servidor adb: el servidor adb de ADBuddy (puerto ", "no responde: ", "    Ejecuta: adbuddy doctor --fix", "· móvil: no comprobado: el servidor adb está caído"}, []string{"adbuddy doctor --fix"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := doctorEnv(t, tc.rules)
			run := func(lang i18n.Lang, args ...string) string {
				i18ntest.Lang(t, lang)
				out, _, _ := runWithInput(t, "", append([]string{"doctor"}, args...)...)
				return strings.ReplaceAll(out, e.ws, "<ws>")
			}
			en, es := run(i18n.EN), run(i18n.ES)
			for _, want := range tc.want {
				if !strings.Contains(es, want) {
					t.Errorf("the Spanish report lacks %q:\n%s", want, es)
				}
			}
			i18ntest.NoEnglish(t, es)
			// Same on purpose: the checks that only hold a version, a path or a serial (✓ adb: 37.0.1 (…)).
			for _, l := range i18ntest.Untranslated(en, es, "✓ adb:", "adb-AAAA", "<ws>") {
				t.Errorf("a line that is the same in both languages (text outside the catalog?): %q", l)
			}
			for _, cmd := range tc.cmds {
				if !strings.Contains(en+es, cmd) {
					t.Errorf("the command %q is in neither report", cmd)
				}
			}
			if run(i18n.EN, "--json") != run(i18n.ES, "--json") {
				t.Errorf("the JSON of doctor depends on the language")
			}
		})
	}
}

// --fix reports in the language of the run and writes the note of what it
// could not do inside the line of the failing check.
func TestDoctorFixInSpanish(t *testing.T) {
	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n", Times: 1}))
	i18ntest.Lang(t, i18n.ES)
	out, _, _ := runWithInput(t, "", "doctor", "--fix")
	for _, want := range []string{"↻ se reinició el servidor adb de ADBuddy (puerto ", "✓ servidor adb:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	i18ntest.NoEnglish(t, out)

	doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^devices$`, Exit: 1, Stderr: "cannot connect to daemon\n"}))
	out, _, _ = runWithInput(t, "", "doctor", "--fix")
	if !strings.Contains(out, "--fix lo reinició y sigue sin responder") {
		t.Errorf("the note of --fix is not in Spanish:\n%s", out)
	}
	if strings.Contains(out, "    Ejecuta: adbuddy doctor --fix") {
		t.Errorf("--fix is offered again:\n%s", out)
	}
}

// The agent reads English whatever the language of the PC, and the message of a
// broken config is the one of the error in the language of the run.
func TestDoctorAgentAndBrokenConfigInSpanish(t *testing.T) {
	e := doctorEnv(t, doctorRules(testutil.FakeRule{Match: `^shell dumpsys deviceidle whitelist$`, Stdout: "user,com.other,10124\n"}))
	t.Setenv("ADBUDDY_SOURCE", "agent")
	i18ntest.Lang(t, i18n.EN)
	en, _, _ := runWithInput(t, "", "doctor")
	i18ntest.Lang(t, i18n.ES)
	if es, _, _ := runWithInput(t, "", "doctor"); es != en {
		t.Errorf("the agent's report changes with the language:\n%s\n---\n%s", en, es)
	}
	t.Setenv("ADBUDDY_SOURCE", "")

	if err := os.WriteFile(filepath.Join(e.cfgDir, "config.toml"), []byte("chat_port = 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, _ := runWithInput(t, "", "doctor")
	i18ntest.NoEnglish(t, out)
	for _, want := range []string{"✗ configuración: CONFIG_INVALID: ", "    Qué hacer: "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestDoctorPhoneHowMentionsSetup(t *testing.T) {
	for _, l := range []i18n.Lang{i18n.EN, i18n.ES} {
		app := &App{Lang: l, Getenv: func(string) string { return "" }}
		for _, k := range []string{app.T("doctor.phone.how_wifi"), app.T("doctor.phone.how_tailscale")} {
			if !strings.Contains(k, "adbuddy setup") {
				t.Errorf("%v: %q", l, k)
			}
		}
	}
}
