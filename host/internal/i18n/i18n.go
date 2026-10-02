// Package i18n is ADBuddy's message catalog for the host: English and Spanish,
// embedded, with no dependency beyond the TOML parser the host already uses
// and no global language. Every caller holds its own Lang, so two tests (or
// two goroutines) in different languages never step on each other.
package i18n

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/BurntSushi/toml"
)

// Lang is a language of the interface: "en" or "es". The zero value is
// English, as is anything that is not Spanish.
type Lang string

const (
	EN Lang = "en"
	ES Lang = "es"
)

// Norm returns EN or ES.
func (l Lang) Norm() Lang {
	if l == ES {
		return ES
	}
	return EN
}

//go:embed catalog/en.toml catalog/es.toml
var catalogFS embed.FS

// bundle holds the messages of every language.
type bundle struct{ m map[Lang]map[string]string }

func parseCatalog(data []byte) (map[string]string, error) {
	out := map[string]string{}
	if _, err := toml.Decode(string(data), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func newBundle(en, es []byte) (*bundle, error) {
	e, err := parseCatalog(en)
	if err != nil {
		return nil, fmt.Errorf("en.toml: %w", err)
	}
	s, err := parseCatalog(es)
	if err != nil {
		return nil, fmt.Errorf("es.toml: %w", err)
	}
	return &bundle{m: map[Lang]map[string]string{EN: e, ES: s}}, nil
}

func loadEmbedded() *bundle {
	en, err := catalogFS.ReadFile("catalog/en.toml")
	if err != nil {
		panic(err)
	}
	es, err := catalogFS.ReadFile("catalog/es.toml")
	if err != nil {
		panic(err)
	}
	b, err := newBundle(en, es)
	if err != nil {
		panic("i18n: " + err.Error()) // a broken catalog is a programming error; TestCatalogParity catches it first
	}
	return b
}

// std is the embedded catalog. It is replaced only by WithoutKeys (tests).
var std atomic.Pointer[bundle]

func init() { std.Store(loadEmbedded()) }

// lookup finds key in lang, then in English.
func (b *bundle) lookup(l Lang, key string) (string, bool) {
	if s, ok := b.m[l.Norm()][key]; ok && s != "" {
		return s, true
	}
	if s, ok := b.m[EN][key]; ok && s != "" {
		return s, true
	}
	return "", false
}

// T returns the message key in l, formatted with args. Every verb of a
// catalog pattern has an explicit index (%[1]s, %[2]d), so that Spanish may
// reorder them and a pattern may use fewer arguments than the call gives
// (an error and its fix share the arguments of NewErrorT). A key missing in l
// is answered in English; one missing in English is answered with the key
// itself, which TestKeysExist makes impossible in a build that passes its
// tests. The patterns come from the catalog, never from args.
func (l Lang) T(key string, args ...any) string {
	s, ok := std.Load().lookup(l, key)
	if !ok {
		return key
	}
	args = l.resolve(args)
	if len(args) > 0 && !strings.Contains(strings.ReplaceAll(s, "%%", ""), "%") {
		// No verbs: fmt would append %!(EXTRA …) for the arguments the pattern does not use.
		return strings.ReplaceAll(s, "%%", "%")
	}
	return fmt.Sprintf(s, args...)
}

// Msg is a message that is not written yet: a key and its arguments. As an
// argument of T it is written in the language of that T, so a sentence can
// hold a fragment (a reason, a hint) that is a message of its own and comes
// out in the same language. In English, with fmt, it is its English.
type Msg struct {
	Key  string
	Args []any
}

// M builds a Msg.
func M(key string, args ...any) Msg { return Msg{key, args} }

// In writes m in l.
func (m Msg) In(l Lang) string { return l.T(m.Key, m.Args...) }

// String is the English of m.
func (m Msg) String() string { return m.In(EN) }

// resolve writes the Msg among args in l; args that are not a Msg stay as they are.
func (l Lang) resolve(args []any) []any {
	var out []any
	for i, a := range args {
		if m, ok := a.(Msg); ok {
			if out == nil {
				out = append([]any(nil), args...)
			}
			out[i] = m.In(l)
		}
	}
	if out == nil {
		return args
	}
	return out
}

// TN is T for a message that depends on a count: it uses key+".one" when n is
// 1 and key+".other" otherwise, and n is the first argument (%[1]d).
func (l Lang) TN(key string, n int, args ...any) string {
	suffix := ".other"
	if n == 1 {
		suffix = ".one"
	}
	return l.T(key+suffix, append([]any{n}, args...)...)
}

// Pair is a message in both languages, for the tests that compare an output
// with the catalog.
type Pair struct{ Key, EN, ES string }

// Pairs lists every message of the catalog, sorted by key.
func Pairs() []Pair {
	b := std.Load()
	out := make([]Pair, 0, len(b.m[EN]))
	for k, en := range b.m[EN] {
		out = append(out, Pair{k, en, b.m[ES][k]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Has reports whether key exists in English, the source of every catalog.
func Has(key string) bool {
	_, ok := std.Load().m[EN][key]
	return ok
}

// WithoutKeys removes keys from lang's catalog until the returned function
// restores it: a test of the fallback to English. Not for concurrent tests.
func WithoutKeys(lang Lang, keys ...string) (restore func()) {
	old := std.Load()
	nb := &bundle{m: map[Lang]map[string]string{}}
	for l, m := range old.m {
		c := make(map[string]string, len(m))
		for k, v := range m {
			c[k] = v
		}
		nb.m[l] = c
	}
	for _, k := range keys {
		delete(nb.m[lang.Norm()], k)
	}
	std.Store(nb)
	return func() { std.Store(old) }
}

// Parse reads a language name as written in ADBUDDY_LANG, config.toml or the
// POSIX locale variables ("es", "es_ES.UTF-8", "es-419", "es@euro"): Spanish
// when it starts with "es" followed by nothing or one of _ - . @; English for
// anything else, "C" and "POSIX" included.
func Parse(v string) Lang {
	v = strings.ToLower(strings.TrimSpace(v))
	if rest, ok := strings.CutPrefix(v, "es"); ok {
		if rest == "" || strings.ContainsRune("_-.@", rune(rest[0])) {
			return ES
		}
	}
	return EN
}
