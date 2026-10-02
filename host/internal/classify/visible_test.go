package classify

import (
	"strings"
	"testing"
)

func TestRTLBoundaries(t *testing.T) {
	out := []rune{0x058F, 0x0900, 0xFB1C, 0xFE6F, 0x107FF, 0x11000, 0x1E7FF, 0x1F000, 'a', 'é', '日', 'Ж', '😀'}
	in := []rune{0x0590, 0x08FF, 0xFB1D, 0xFDFF, 0xFE70, 0x10800, 0x10FFF, 0x1E800, 0x1EFFF, 'א', 'ب', '٣'}
	for _, r := range out {
		if RTL(r) {
			t.Errorf("RTL(U+%04X) = true, want false", r)
		}
	}
	for _, r := range in {
		if !RTL(r) {
			t.Errorf("RTL(U+%04X) = false, want true", r)
		}
	}
}

func TestVisible(t *testing.T) {
	cases := map[string]string{
		"plain ascii":      "plain ascii",
		"é ñ 日本語 Ж 😀":      "é ñ 日本語 Ж 😀",
		"\u05d0":           `\u05d0`,
		"\u0628":           `\u0628`,
		"\u0663":           `\u0663`,
		"\U0001e900":       `\U0001e900`,
		"\U000e0001":       `\U000e0001`, // a Cf above U+FFFF
		"\U0001d173":       `\U0001d173`, // a Cf above U+FFFF
		"a\nb\tc\rd":       `a\nb\tc\rd`,
		"x\u202ey":         `x\u202ey`,
		"mv \u05d0 \u05d1": `mv \u05d0 \u05d1`,
		"\x1b[2K":          `\u001b[2K`,
		"\U0001e900" + "0": `\U0001e900` + "0",
		"\xff":             `\ufffd`,
	}
	for in, want := range cases {
		got := Visible(in)
		if got != want {
			t.Errorf("Visible(%q) = %q, want %q", in, got, want)
		}
		if Visible(got) != got {
			t.Errorf("Visible not idempotent on %q: %q", in, Visible(got))
		}
	}
}

func TestPrintableStillLetsRTLThrough(t *testing.T) {
	if got := Printable("א"); got != "א" {
		t.Fatalf("Printable changed: %q", got)
	}
}

func TestVisibleAgreesWithPrintableOutsideRTL(t *testing.T) {
	for r := rune(0); r < 0x3000; r++ {
		if RTL(r) {
			continue
		}
		if Visible(string(r)) != Printable(string(r)) {
			t.Fatalf("U+%04X: Visible %q, Printable %q", r, Visible(string(r)), Printable(string(r)))
		}
	}
}

func TestDisplayCommandHasNoRTL(t *testing.T) {
	got := DisplayCommand([]string{"shell", "mv", "א", "ب"})
	if strings.IndexFunc(got, RTL) >= 0 {
		t.Fatalf("RTL in %q", got)
	}
	if !strings.Contains(got, string(rune(92))+"u05d0") {
		t.Fatalf("not escaped: %q", got)
	}
	got = DisplayCommand([]string{"push", "א.txt", "/sdcard/ב"})
	if strings.IndexFunc(got, RTL) >= 0 {
		t.Fatalf("RTL in %q", got)
	}
}
