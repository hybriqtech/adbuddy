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

// fillerRanges are the code points that show as nothing, or as a blank,
// without being a space or a format character (H3): the Hangul fillers, the
// braille blank, and the invisible marks (combining grapheme joiner,
// variation selectors, Mongolian free variation selectors, Khmer inherent
// vowels).
var fillerRanges = [...][2]rune{
	{0x034F, 0x034F},
	{0x115F, 0x1160},
	{0x17B4, 0x17B5},
	{0x180B, 0x180F},
	{0x2800, 0x2800},
	{0x3164, 0x3164},
	{0xFE00, 0xFE0F},
	{0xFFA0, 0xFFA0},
	{0xE0100, 0xE01EF},
}

func filler(r rune) bool {
	for _, rg := range fillerRanges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

// hidden reports whether r could hide or rewrite what a person reads, so
// Printable and Visible escape it: control characters (ANSI sequences start
// with one), DEL, format characters (bidirectional overrides, zero-width
// characters), line and paragraph separators, every space but U+0020, the
// fillers, and the combining marks (thousands of them on one letter cover
// the lines around it). Escaping too much is safe.
func hidden(r rune) bool {
	return unicode.IsControl(r) || r == utf8.RuneError ||
		unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Mn, unicode.Me) ||
		unicode.Is(unicode.Zs, r) && r != ' ' || filler(r)
}

// escape writes r as \uXXXX, or \UXXXXXXXX above U+FFFF (so it cannot be read
// as a shorter escape followed by a digit).
func escape(b *strings.Builder, r rune) {
	if r > 0xFFFF {
		fmt.Fprintf(b, `\U%08x`, r)
	} else {
		fmt.Fprintf(b, `\u%04x`, r)
	}
}

// Visible is Printable plus the right-to-left class: what it escapes
// Printable escapes the same way, and every rune RTL reports comes out as
// \uXXXX (\UXXXXXXXX above U+FFFF). The result is idempotent. Use it for
// what is shown to decide something; Printable stays as it was for
// everything else.
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
		case hidden(r), RTL(r):
			escape(&b, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// minRun is the shortest run of blanks that is shown compacted (␠×N).
const minRun = 4

// maxGroups is how many groups of one blank a compacted run shows in order;
// past it, it shows how many of each kind (see compact).
const maxGroups = 4

// blank reports whether r is something padding is made of: a space of any
// kind, a tab, a newline, a carriage return, a vertical tab or form feed, a
// line or paragraph separator, a format character or a filler.
func blank(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	}
	return unicode.In(r, unicode.Zs, unicode.Zl, unicode.Zp, unicode.Cf) || filler(r)
}

// unit is how one blank shows in a compacted run: ␠ for a space, its escape
// otherwise.
func unit(r rune) string {
	if r == ' ' {
		return "␠"
	}
	return Visible(string(r))
}

// VisibleCommand is Visible for the raw command a person confirms, applied
// once (by DisplayCommand). A backslash the agent wrote comes out as two, so
// it cannot pass for one of Visible's escapes (A4-4); a run of padding comes
// out compacted (compact), so it cannot push the rest of the command out of
// what the phone shows (A4-2, H3). Unlike Visible it is not idempotent (no
// escaping that tells a written escape from a real one can be), so it is
// applied once; Visible on its result changes nothing.
func VisibleCommand(s string) string {
	return compact(s, func(b *strings.Builder, r rune) {
		if r == '\x5c' {
			b.WriteString(strings.Repeat(string('\x5c'), 2))
			return
		}
		b.WriteString(Visible(string(r)))
	})
}

// compact writes s with esc, except for the runs of minRun or more blanks
// (blank), whatever their mix (H3):
//
//   - one kind: its unit and how many (␠×4000, \n×5, \u00a0×9);
//   - up to maxGroups groups of one kind each: each in order, a group of
//     minRun or more compacted (\n␠×4) and a shorter one as esc writes it;
//   - more groups: × and how many in all, and in brackets how many of each
//     kind in the order they first appear (×6000(␠×3000\u00a0×3000)); the
//     order inside the run is not shown.
//
// A ␠ or × the agent wrote, and a digit right after a run, come out escaped:
// every ␠, × and count shown is a compacted run (B1).
func compact(s string, esc func(*strings.Builder, rune)) string {
	var b strings.Builder
	rs := []rune(s)
	afterCount := false
	for i := 0; i < len(rs); {
		n := 0
		for i+n < len(rs) && blank(rs[i+n]) {
			n++
		}
		if n >= minRun {
			afterCount = writeRun(&b, rs[i:i+n], esc)
			i += n
			continue
		}
		r := rs[i]
		switch {
		case r == '␠' || r == '×' || afterCount && r >= '0' && r <= '9':
			escape(&b, r)
		default:
			esc(&b, r)
		}
		afterCount = false
		i++
	}
	return b.String()
}

// writeRun writes one run of blanks as compact says, and reports whether it
// ended with a count.
func writeRun(b *strings.Builder, run []rune, esc func(*strings.Builder, rune)) bool {
	type group struct {
		r rune
		n int
	}
	var groups []group
	for _, r := range run {
		if len(groups) > 0 && groups[len(groups)-1].r == r {
			groups[len(groups)-1].n++
		} else {
			groups = append(groups, group{r, 1})
		}
	}
	if len(groups) <= maxGroups {
		for _, g := range groups {
			if g.n >= minRun {
				b.WriteString(unit(g.r) + "×" + strconv.Itoa(g.n))
				continue
			}
			for range g.n {
				esc(b, g.r)
			}
		}
		return groups[len(groups)-1].n >= minRun
	}
	var kinds []rune
	count := map[rune]int{}
	for _, r := range run {
		if count[r] == 0 {
			kinds = append(kinds, r)
		}
		count[r]++
	}
	b.WriteString("×" + strconv.Itoa(len(run)) + "(")
	for _, k := range kinds {
		b.WriteString(unit(k) + "×" + strconv.Itoa(count[k]))
	}
	b.WriteString(")")
	return false
}
