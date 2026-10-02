// Package i18ntest has the helpers of the tests that run in Spanish.
package i18ntest

import (
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// Lang makes ADBUDDY_LANG name lang for the test (the highest source of
// i18n.Resolve) and returns it.
func Lang(t testing.TB, lang i18n.Lang) i18n.Lang {
	t.Helper()
	t.Setenv("ADBUDDY_LANG", string(lang))
	return lang
}

// Without removes keys from lang's catalog for the test, to see what the
// fallback to English shows. The test must not run in parallel.
func Without(t testing.TB, lang i18n.Lang, keys ...string) {
	t.Helper()
	t.Cleanup(i18n.WithoutKeys(lang, keys...))
}
