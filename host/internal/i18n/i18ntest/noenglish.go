package i18ntest

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

var verbRE = regexp.MustCompile(`%\[\d+\][sdvqxXtf]`)
var keyLine = regexp.MustCompile(`(?m)^\s*[a-z][a-z0-9]*(\.[a-z0-9_]+)+\s*$`)

// NoEnglish fails the test when out, written in Spanish, still has a message
// of the catalog in English (a text that was not looked up in the language),
// or shows the name of a key. except lists the keys whose English may appear
// on purpose. See Problems for what is looked for.
func NoEnglish(t testing.TB, out string, except ...string) {
	t.Helper()
	for _, p := range Problems(out, i18n.Pairs(), except...) {
		t.Error(p)
	}
}

// Problems is NoEnglish without the test: the English messages of pairs found
// in out (a message with fewer than 12 characters of fixed text is not looked
// for: it would match by chance), and the names of keys shown as text.
func Problems(out string, pairs []i18n.Pair, except ...string) []string {
	skip := map[string]bool{}
	for _, k := range except {
		skip[k] = true
	}
	var found []string
	for _, p := range pairs {
		if p.EN == p.ES || skip[p.Key] {
			continue
		}
		if len(strings.TrimSpace(verbRE.ReplaceAllString(p.EN, ""))) < 12 {
			continue
		}
		parts := verbRE.Split(p.EN, -1)
		for i, s := range parts {
			parts[i] = regexp.QuoteMeta(s)
		}
		if regexp.MustCompile(strings.Join(parts, `[^\n]*?`)).MatchString(out) {
			found = append(found, "the output has the English of "+p.Key+": "+p.EN)
		}
	}
	if k := keyLine.FindString(out); k != "" {
		found = append(found, "the output shows the name of a catalog key: "+strings.TrimSpace(k))
	}
	return found
}

var phraseRE = regexp.MustCompile(`\p{L}{2,}\s+\p{L}{2,}`)

// Untranslated are the lines that a command writes the same in English and in
// Spanish and that have a phrase in them (two words): text that was never
// moved to the catalog and that NoEnglish cannot see, because it is not in it.
// allow lists substrings of lines that may be the same on purpose (an order, a
// path, a serial, a name). Run the same scenario twice, once in each language.
func Untranslated(en, es string, allow ...string) []string {
	inEN := map[string]bool{}
	for _, l := range strings.Split(en, "\n") {
		inEN[strings.TrimSpace(l)] = true
	}
	var found []string
lines:
	for _, l := range strings.Split(es, "\n") {
		l = strings.TrimSpace(l)
		if !inEN[l] || !phraseRE.MatchString(l) {
			continue
		}
		for _, a := range allow {
			if strings.Contains(l, a) {
				continue lines
			}
		}
		found = append(found, l)
	}
	return found
}
