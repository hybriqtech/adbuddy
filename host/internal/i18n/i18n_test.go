package i18n

import "testing"

func testBundle(t *testing.T, en, es string) func() {
	t.Helper()
	b, err := newBundle([]byte(en), []byte(es))
	if err != nil {
		t.Fatal(err)
	}
	old := std.Load()
	std.Store(b)
	return func() { std.Store(old) }
}

func TestTAndFallback(t *testing.T) {
	defer testBundle(t, `
"a.hello" = "Hello %[1]s"
"a.only_en" = "Only English"
"a.order" = "%[2]s then %[1]s"
`, `
"a.hello" = "Hola %[1]s"
"a.order" = "%[2]s y luego %[1]s"
`)()
	cases := []struct {
		lang Lang
		key  string
		args []any
		want string
	}{
		{ES, "a.hello", []any{"Ana"}, "Hola Ana"},
		{EN, "a.hello", []any{"Ana"}, "Hello Ana"},
		{"", "a.hello", []any{"Ana"}, "Hello Ana"},   // the zero Lang is English
		{"fr", "a.hello", []any{"Ana"}, "Hello Ana"}, // anything else too
		{ES, "a.only_en", nil, "Only English"},       // missing in es: English, never empty
		{ES, "a.order", []any{"uno", "dos"}, "dos y luego uno"},
		{ES, "a.nowhere", nil, "a.nowhere"}, // missing everywhere: the key
	}
	for _, c := range cases {
		if got := c.lang.T(c.key, c.args...); got != c.want {
			t.Errorf("%q.T(%s) = %q, want %q", c.lang, c.key, got, c.want)
		}
	}
}

func TestTNeverUsesTheArgumentAsPattern(t *testing.T) {
	defer testBundle(t, `"a.say" = "said: %[1]s"`, `"a.say" = "dijo: %[1]s"`)()
	if got := ES.T("a.say", "%d %s %[3]q"); got != "dijo: %d %s %[3]q" {
		t.Fatalf("got %q", got)
	}
}

func TestTN(t *testing.T) {
	defer testBundle(t, `
"u.done.one" = "Undid %[1]d change."
"u.done.other" = "Undid %[1]d changes."
"u.of.one" = "%[1]d change in %[2]s"
"u.of.other" = "%[1]d changes in %[2]s"
`, `
"u.done.one" = "Se deshizo %[1]d cambio."
"u.done.other" = "Se deshicieron %[1]d cambios."
"u.of.one" = "%[1]d cambio en %[2]s"
"u.of.other" = "%[1]d cambios en %[2]s"
`)()
	for _, c := range []struct {
		lang Lang
		n    int
		want string
	}{{EN, 1, "Undid 1 change."}, {EN, 0, "Undid 0 changes."}, {EN, 2, "Undid 2 changes."}, {ES, 1, "Se deshizo 1 cambio."}, {ES, 3, "Se deshicieron 3 cambios."}} {
		if got := c.lang.TN("u.done", c.n); got != c.want {
			t.Errorf("%q.TN(%d) = %q, want %q", c.lang, c.n, got, c.want)
		}
	}
	if got := ES.TN("u.of", 2, "el diario"); got != "2 cambios en el diario" {
		t.Errorf("TN with an argument: %q", got)
	}
}

func TestWithoutKeysFallsBackToEnglishAndRestores(t *testing.T) {
	defer testBundle(t, `"k.x" = "English"`, `"k.x" = "Español"`)()
	restore := WithoutKeys(ES, "k.x")
	if got := ES.T("k.x"); got != "English" {
		t.Errorf("without the key: %q", got)
	}
	if got := EN.T("k.x"); got != "English" {
		t.Errorf("English changed: %q", got)
	}
	restore()
	if got := ES.T("k.x"); got != "Español" {
		t.Errorf("after restore: %q", got)
	}
}

func TestEmbeddedCatalogsParse(t *testing.T) {
	if std.Load() == nil || len(std.Load().m) != 2 {
		t.Fatal("the embedded catalogs did not load")
	}
	if Has("no.such.key") {
		t.Fatal("Has invented a key")
	}
}

func TestAPatternWithoutVerbsIgnoresTheArguments(t *testing.T) {
	defer testBundle(t, `
"a.fix" = "Run `+"`adbuddy setup`"+`."
"a.pct" = "100%% done"
"a.two" = "only the second: %[2]s"
`, `
"a.fix" = "Ejecuta `+"`adbuddy setup`"+`."
"a.pct" = "100%% hecho"
"a.two" = "solo el segundo: %[2]s"
`)()
	if got := ES.T("a.fix", "x", 2); got != "Ejecuta `adbuddy setup`." {
		t.Errorf("an error and its fix share the arguments: %q", got)
	}
	if got := ES.T("a.pct"); got != "100% hecho" {
		t.Errorf("%%%% without arguments: %q", got)
	}
	if got := ES.T("a.pct", 1); got != "100% hecho" {
		t.Errorf("%%%% with arguments: %q", got)
	}
	if got := ES.T("a.two", "uno", "dos"); got != "solo el segundo: dos" {
		t.Errorf("an unused argument: %q", got)
	}
}

func TestAMsgAsAnArgumentIsWrittenInTheLanguageOfTheSentence(t *testing.T) {
	defer testBundle(t, `
"s.warn" = "Warning: the phone at %[1]s %[2]s."
"s.why" = "is not the one you set up"
"s.nested" = "cannot be checked: %[1]s"
"s.leaf" = "no phone is set up in %[1]s"
`, `
"s.warn" = "Aviso: el móvil en %[1]s %[2]s."
"s.why" = "no es el que configuraste"
"s.nested" = "no se puede comprobar: %[1]s"
"s.leaf" = "no hay ningún móvil configurado en %[1]s"
`)()
	if got := ES.T("s.warn", "192.0.2.2", M("s.why")); got != "Aviso: el móvil en 192.0.2.2 no es el que configuraste." {
		t.Errorf("es: %q", got)
	}
	if got := EN.T("s.warn", "192.0.2.2", M("s.why")); got != "Warning: the phone at 192.0.2.2 is not the one you set up." {
		t.Errorf("en: %q", got)
	}
	if got := ES.T("s.nested", M("s.nested", M("s.leaf", "config.toml"))); got != "no se puede comprobar: no se puede comprobar: no hay ningún móvil configurado en config.toml" {
		t.Errorf("a Msg inside a Msg: %q", got)
	}
	if got := M("s.why").String(); got != "is not the one you set up" {
		t.Errorf("String is the English: %q", got)
	}
	args := []any{"a", M("s.why")}
	_ = ES.T("s.warn", args...)
	if _, still := args[1].(Msg); !still {
		t.Errorf("T must not change the caller's arguments")
	}
}
