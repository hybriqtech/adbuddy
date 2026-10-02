package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/i18n/i18ntest"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestTestMainPinsEnglish(t *testing.T) {
	// The tests that existed before M6c read English text: the TestMain of this
	// package fixes ADBUDDY_LANG=en, which beats config.language, LANG and the
	// language of Windows, so the machine the tests run on never matters.
	if got := os.Getenv("ADBUDDY_LANG"); got != "en" {
		t.Fatalf("ADBUDDY_LANG = %q: TestMain must set it to en", got)
	}
	i18ntest.Lang(t, i18n.ES)
	if got := os.Getenv("ADBUDDY_LANG"); got != "es" {
		t.Fatalf("i18ntest.Lang did not set it: %q", got)
	}
}

func TestSpeaks(t *testing.T) {
	for _, c := range []struct {
		name string
		app  App
		want i18n.Lang
	}{
		{"built by hand: English", App{}, i18n.EN},
		{"es", App{Lang: i18n.ES, Getenv: envOf(nil)}, i18n.ES},
		{"es with --json is English", App{Lang: i18n.ES, JSON: true, Getenv: envOf(nil)}, i18n.EN},
		{"es for the agent is English", App{Lang: i18n.ES, Getenv: envOf(map[string]string{"ADBUDDY_SOURCE": "agent"})}, i18n.EN},
		{"es for the user stays es", App{Lang: i18n.ES, Getenv: envOf(map[string]string{"ADBUDDY_SOURCE": "user"})}, i18n.ES},
		{"no Getenv does not panic", App{Lang: i18n.ES}, i18n.ES},
	} {
		if got := c.app.speaks(); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetectLang(t *testing.T) {
	dir := t.TempDir()
	write := func(content string) {
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := func(extra map[string]string) func(string) string {
		m := map[string]string{"ADBUDDY_CONFIG_DIR": dir, "ADBUDDY_DATA_DIR": dir}
		for k, v := range extra {
			m[k] = v
		}
		return envOf(m)
	}
	write("language = \"es\"\n")
	if got := detectLang(env(nil)); got != i18n.ES {
		t.Errorf("config.language es: %q", got)
	}
	if got := detectLang(env(map[string]string{"ADBUDDY_LANG": "en"})); got != i18n.EN {
		t.Errorf("ADBUDDY_LANG must win over config.language: %q", got)
	}
	write("language = \"en\"\n")
	if got := detectLang(env(map[string]string{"LANG": "es_ES.UTF-8"})); got != i18n.EN {
		t.Errorf("config.language en must win over LANG: %q", got)
	}
	write("this is [not toml")
	if got := detectLang(env(map[string]string{"LANG": "es_ES.UTF-8"})); got != i18n.ES {
		t.Errorf("a broken config.toml is ignored, LANG decides: %q", got)
	}
	write("typo = 1\nlanguage = \"es\"\n")
	if got := detectLang(env(nil)); got != i18n.ES {
		t.Errorf("other keys do not stop language from being read: %q", got)
	}
	if err := os.Remove(filepath.Join(dir, "config.toml")); err != nil {
		t.Fatal(err)
	}
	if got := detectLang(env(map[string]string{"LC_ALL": "C", "LANG": "es_ES"})); got != i18n.EN {
		t.Errorf("no config, LC_ALL=C: %q", got)
	}
}

func TestWithLangAgreesWithConfigLang(t *testing.T) {
	// serve asks config.Config.Lang(app.Getenv): it must get the language the
	// run decided, also when it came from the system and not from the variables.
	getenv := withLang(envOf(map[string]string{"LANG": "en_US"}), i18n.ES)
	if got := config.Default().Lang(getenv); got != "es" {
		t.Errorf("Config.Lang = %q, want es", got)
	}
	if got := getenv("LANG"); got != "en_US" {
		t.Errorf("the other variables pass through: %q", got)
	}
}

func TestUTF8ConsoleDoesNothingWithoutAConsole(t *testing.T) {
	utf8Console()()
	utf8Console(nil)()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	utf8Console(f, nil)()
}
