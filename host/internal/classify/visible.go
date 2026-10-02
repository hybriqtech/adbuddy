package classify

import (
	"fmt"
	"strconv"
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

// minRun is the shortest run of blanks that is shown compacted (␠×N).
const minRun = 4

// blankUnits are the blanks whose runs are compacted, and how one shows.
var blankUnits = map[rune]string{' ': "␠", '\n': `\n`, '\t': `\t`, '\r': `\r`}

// VisibleCommand is Visible for the raw command a person confirms, applied
// once (by DisplayCommand). A backslash the agent wrote comes out as two, so
// it cannot pass for one of Visible's escapes (A4-4); a run of minRun or more
// spaces, newlines, tabs or carriage returns comes out as ␠×N (or the escape
// and ×N), so padding cannot push the rest of the command out of what the
// phone shows (A4-2); and a ␠ the agent wrote comes out escaped. Unlike
// Visible it is not idempotent (no escaping that tells a written escape from
// a real one can be), so it is applied once; Visible on its result changes
// nothing.
func VisibleCommand(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); {
		r := rs[i]
		n := runLen(rs, i)
		if unit, ok := blankUnits[r]; ok && n >= minRun {
			b.WriteString(unit + "×" + strconv.Itoa(n))
			i += n
			continue
		}
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '␠':
			b.WriteString(`\u2420`)
		default:
			b.WriteString(Visible(string(r)))
		}
		i++
	}
	return b.String()
}

// compactSpaces shortens text already escaped (Printable) for a title: a run
// of minRun or more spaces comes out as ␠×N, and a ␠ in it escaped.
func compactSpaces(s string) string {
	if !strings.Contains(s, "    ") && !strings.ContainsRune(s, '␠') {
		return s
	}
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); {
		n := runLen(rs, i)
		switch {
		case rs[i] == ' ' && n >= minRun:
			b.WriteString("␠×" + strconv.Itoa(n))
			i += n
			continue
		case rs[i] == '␠':
			b.WriteString(`\u2420`)
		default:
			b.WriteRune(rs[i])
		}
		i++
	}
	return b.String()
}

// runLen is how many times rs[i] repeats from i.
func runLen(rs []rune, i int) int {
	n := 1
	for i+n < len(rs) && rs[i+n] == rs[i] {
		n++
	}
	return n
}
