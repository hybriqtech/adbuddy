package cli

import (
	"io"
	"os"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/paths"
)

// T is the message key (catalog/en.toml and es.toml) in the language this run
// speaks. See speaks for when that is English whatever the person's language.
func (a *App) T(key string, args ...any) string { return a.speaks().T(key, args...) }

// TN is T for a message that depends on a count (key.one and key.other).
func (a *App) TN(key string, n int, args ...any) string { return a.speaks().TN(key, n, args...) }

// speaks is the language of what this run writes for people: App.Lang, except
// in English when the output is for a program: --json, or a command run by the
// agent (ADBUDDY_SOURCE=agent), whose reader is a model that works from the
// English text. An App built by hand has no Lang, which is English.
func (a *App) speaks() i18n.Lang {
	if a.JSON || (a.Getenv != nil && a.Getenv("ADBUDDY_SOURCE") == "agent") {
		return i18n.EN
	}
	return a.Lang.Norm()
}

// detectLang is the language of this run: ADBUDDY_LANG, the "language" of
// config.toml (read leniently: a broken file does not stop --help), the POSIX
// variables, and then, in Windows, the interface language of the user.
func detectLang(getenv func(string) string) i18n.Lang {
	cfg := ""
	if dirs, err := paths.Default(getenv); err == nil {
		cfg = config.ReadLanguage(dirs.ConfigFile())
	}
	return i18n.Detect(getenv, cfg, i18n.SystemLanguage)
}

// withLang is getenv, but ADBUDDY_LANG answers lang: whoever asks the App
// for the language again (config.Config.Lang in serve, which hands it to the
// daemon and to `remote on`) gets the one this run already decided, the
// Windows interface language included, so no two parts disagree.
func withLang(getenv func(string) string, lang i18n.Lang) func(string) string {
	return func(k string) string {
		if k == "ADBUDDY_LANG" {
			return string(lang)
		}
		return getenv(k)
	}
}

func asFile(w io.Writer) *os.File {
	f, _ := w.(*os.File)
	return f
}
