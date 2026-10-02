package i18n

import "golang.org/x/sys/windows"

// SystemLanguage is the first of the user's preferred UI languages ("es-ES"),
// or "" when Windows does not say.
func SystemLanguage() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(langs) == 0 {
		return ""
	}
	return langs[0]
}
