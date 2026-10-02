package classify

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// rtlRanges is the one table of the code points of right-to-left scripts
// (Hebrew, Arabic, Syriac, Thaana, N'Ko, Samaritan, Mandaic, presentation
// forms, Adlam, Rumi…) and of the Arabic-Indic number signs. It is
// conservative on purpose: it takes whole blocks, unassigned gaps included,
// so a code point Unicode assigns later to R, AL or AN is already covered
// (escaping too much is safe, escaping too little is not).
var rtlRanges = [...][2]rune{
	{0x0590, 0x08FF},
	{0xFB1D, 0xFDFF},
	{0xFE70, 0xFEFF},
	{0x10800, 0x10FFF},
	{0x1E800, 0x1EFFF},
}

// RTL reports whether r belongs to a right-to-left script or number class.
// What is shown to decide a confirmation must not contain such a rune: the
// order a person sees would not be the order that runs.
func RTL(r rune) bool {
	for _, rg := range rtlRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// Visible is Printable plus the right-to-left class: what it escapes
// Printable escapes the same way, and every rune RTL reports comes out as
// \uXXXX (\UXXXXXXXX above U+FFFF, so it cannot be read as a shorter escape
// followed by a digit). The result is idempotent. Use it for what is shown
// to decide something; Printable stays as it was for everything else.
func Visible(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case unicode.IsControl(r), unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp), r == utf8.RuneError, RTL(r):
			if r > 0xFFFF {
				fmt.Fprintf(&b, `\U%08x`, r)
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
