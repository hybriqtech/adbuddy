//go:build !windows

package i18n

// SystemLanguage is "" outside Windows: macOS and Linux terminals set LANG
// from the system's language, and ADBuddy does not run `defaults`.
func SystemLanguage() string { return "" }
