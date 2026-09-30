package classify

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// wordOf computes the literal value of a word. Anything whose value is only
// known when the phone's shell runs it — expansions, substitutions, globs,
// braces, a leading ~ or $'…' — makes the word Dynamic. Braces and glob
// classes may span quoted parts: in {5,'reboot'} the parser sees "{5,",
// 'reboot' and "}", and the phone still expands it into two words.
func wordOf(w *syntax.Word) Word {
	// A word that is exactly [ ] { or } is a command name or a plain literal,
	// not a brace expansion or glob class: keep it static so `[ … ]` stays a
	// recognised command.
	if len(w.Parts) == 1 {
		if lit, ok := w.Parts[0].(*syntax.Lit); ok {
			switch lit.Value {
			case "[", "]", "{", "}":
				return Word{Value: lit.Value, node: w}
			}
		}
	}
	var b strings.Builder
	var st globState
	dyn := false
	for i, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			v, d := unescapeUnquoted(p.Value, i == 0, &st)
			b.WriteString(v)
			dyn = dyn || d
		case *syntax.SglQuoted:
			if p.Dollar {
				dyn = true
			}
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			if p.Dollar {
				dyn = true
			}
			for _, q := range p.Parts {
				if l, ok := q.(*syntax.Lit); ok {
					b.WriteString(unescapeDouble(l.Value))
				} else {
					dyn = true
				}
			}
		default:
			dyn = true
		}
	}
	return Word{Value: b.String(), Dynamic: dyn, node: w}
}

// globState carries, across the parts of a word, whether an unquoted { or [
// has opened a brace expansion or a glob class.
type globState struct{ brace, bracket bool }

// unescapeUnquoted removes backslashes from an unquoted literal. It reports
// Dynamic for globs, a leading ~, an unquoted { or [ (which may open a brace
// expansion or a glob class), and a } or ] that closes one seen earlier in
// this literal or in an earlier literal of the same word (escaped closers
// count too, which only errs on the safe side).
func unescapeUnquoted(s string, first bool, st *globState) (string, bool) {
	var b strings.Builder
	dyn := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch == '}' && st.brace) || (ch == ']' && st.bracket) {
			dyn = true // brace expansion or glob class
		}
		switch {
		case ch == '\\' && i+1 < len(s):
			i++
			if (s[i] == '}' && st.brace) || (s[i] == ']' && st.bracket) {
				dyn = true
			}
			if s[i] != '\n' {
				b.WriteByte(s[i])
			}
			continue
		case ch == '*' || ch == '?':
			dyn = true
		case ch == '[':
			// An unquoted [ or { may open a glob class or a brace expansion,
			// whether or not a closer follows: treat the word as dynamic.
			st.bracket = true
			dyn = true
		case ch == '{':
			st.brace = true
			dyn = true
		case ch == '~' && i == 0 && first:
			dyn = true
		}
		b.WriteByte(ch)
	}
	return b.String(), dyn
}

func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '$', '`', '"', '\\':
				b.WriteByte(s[i+1])
				i++
				continue
			case '\n':
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
