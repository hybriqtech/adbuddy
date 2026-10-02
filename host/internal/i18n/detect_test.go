package i18n

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestParse(t *testing.T) {
	for v, want := range map[string]Lang{
		"es": ES, "ES": ES, "es_ES": ES, "es_ES.UTF-8": ES, "es-ES": ES, "es_419": ES, "es-419": ES, "es@euro": ES, "es.UTF-8": ES, " es ": ES,
		"en": EN, "en_US.UTF-8": EN, "C": EN, "POSIX": EN, "fr_FR": EN, "estonian": EN, "ess": EN, "et_EE": EN, "": EN, "e": EN,
	} {
		if got := Parse(v); got != want {
			t.Errorf("Parse(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestResolveOrder(t *testing.T) {
	for _, c := range []struct {
		name string
		env  map[string]string
		cfg  string
		want Lang
	}{
		{"nothing", nil, "", EN},
		{"LANG es", map[string]string{"LANG": "es_ES.UTF-8"}, "", ES},
		{"LC_ALL beats LANG", map[string]string{"LC_ALL": "en_US", "LANG": "es_ES"}, "", EN},
		{"LC_ALL empty is skipped", map[string]string{"LC_ALL": "", "LC_MESSAGES": "es", "LANG": "en"}, "", ES},
		{"LC_MESSAGES beats LANG", map[string]string{"LC_MESSAGES": "es_ES", "LANG": "en_US"}, "", ES},
		{"C stops the search", map[string]string{"LC_ALL": "C", "LANG": "es_ES"}, "", EN},
		{"POSIX stops the search", map[string]string{"LANG": "POSIX"}, "", EN},
		{"LANGUAGE is not read", map[string]string{"LANGUAGE": "es"}, "", EN},
		{"config beats LANG", map[string]string{"LANG": "en_US"}, "es", ES},
		{"config en beats LANG es", map[string]string{"LANG": "es_ES"}, "en", EN},
		{"ADBUDDY_LANG beats config", map[string]string{"ADBUDDY_LANG": "en"}, "es", EN},
		{"ADBUDDY_LANG beats LC_ALL", map[string]string{"ADBUDDY_LANG": "es", "LC_ALL": "en_US"}, "", ES},
		{"ADBUDDY_LANG with region", map[string]string{"ADBUDDY_LANG": "es_ES"}, "", ES},
		{"ADBUDDY_LANG unknown is English", map[string]string{"ADBUDDY_LANG": "fr", "LANG": "es"}, "", EN},
	} {
		if got := Resolve(env(c.env), c.cfg); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetectUsesTheSystemOnlyWhenTheEnvironmentIsSilent(t *testing.T) {
	system := func() string { return "es-ES" }
	if got := Detect(env(nil), "", system); got != ES {
		t.Errorf("silent environment: %q", got)
	}
	if got := Detect(env(map[string]string{"LANG": "en_US"}), "", system); got != EN {
		t.Errorf("LANG en must win over the system: %q", got)
	}
	if got := Detect(env(nil), "en", system); got != EN {
		t.Errorf("config en must win over the system: %q", got)
	}
	if got := Detect(env(map[string]string{"ADBUDDY_LANG": "en"}), "", system); got != EN {
		t.Errorf("ADBUDDY_LANG must win over the system: %q", got)
	}
	if got := Detect(env(nil), "", func() string { return "" }); got != EN {
		t.Errorf("a system that says nothing: %q", got)
	}
	if got := Detect(env(nil), "", nil); got != EN {
		t.Errorf("no system: %q", got)
	}
	if got := Detect(env(nil), "", func() string { return "fr-FR" }); got != EN {
		t.Errorf("a system in French: %q", got)
	}
}

func TestSystemLanguageAnswers(t *testing.T) {
	_ = SystemLanguage() // must not panic or hang on any platform
}
