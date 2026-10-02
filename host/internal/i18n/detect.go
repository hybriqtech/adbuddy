package i18n

// Resolve is the language from what the environment says, and nothing
// outside it (no system call), in this order; the first source that has a
// value decides, and anything that is not Spanish decides English:
//
//  1. ADBUDDY_LANG
//  2. configLang, the "language" of config.toml
//  3. LC_ALL, LC_MESSAGES, LANG (the first one not empty, as POSIX says)
//
// With none of them it is English. LANGUAGE is not read.
func Resolve(getenv func(string) string, configLang string) Lang {
	l, _ := resolve(getenv, configLang)
	return l
}

func resolve(getenv func(string) string, configLang string) (Lang, bool) {
	if v := getenv("ADBUDDY_LANG"); v != "" {
		return Parse(v), true
	}
	if configLang != "" {
		return Parse(configLang), true
	}
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			return Parse(v), true
		}
	}
	return EN, false
}

// Detect is Resolve, and when the environment says nothing, the language of
// the system's interface as system gives it (SystemLanguage; "" for none).
// Only the command line's entry point passes a real SystemLanguage: tests
// pass nil, so the machine they run on never changes their answers.
func Detect(getenv func(string) string, configLang string, system func() string) Lang {
	if l, ok := resolve(getenv, configLang); ok {
		return l
	}
	if system != nil {
		if v := system(); v != "" {
			return Parse(v)
		}
	}
	return EN
}
