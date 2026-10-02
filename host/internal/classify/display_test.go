package classify

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDisplayCommandShowsWhatThePhoneRuns(t *testing.T) {
	cases := map[string][]string{
		"adb shell echo hi; reboot":                         {"shell", "echo hi; reboot"},
		"adb exec-out screencap -p; reboot":                 {"exec-out", "screencap -p; reboot"},
		"adb exec-out sh -c 'rm -rf /sdcard/DCIM'":          {"exec-out", "sh", "-c", "rm -rf /sdcard/DCIM"},
		"adb wait-for-device shell echo hi; reboot":         {"wait-for-device", "shell", "echo hi; reboot"},
		"adb exec-in cat > /sdcard/x; reboot":               {"exec-in", "cat > /sdcard/x; reboot"},
		"adb exec-in cp /dev/stdin '/sdcard/a b'":           {"exec-in", "cp", "/dev/stdin", "/sdcard/a b"},
		"adb wait-for-device exec-out screencap -p; reboot": {"wait-for-device", "exec-out", "screencap -p; reboot"},
		"adb push 'my notes.txt' /sdcard/x":                 {"push", "my notes.txt", "/sdcard/x"},
		"adb -s dev1 shell ls":                              {"-s", "dev1", "shell", "ls"},
		"adb devices":                                       {"devices"},
		"adb shell echo \\u001b[2K":                         {"shell", "echo \x1b[2K"},
	}
	for want, argv := range cases {
		if got := DisplayCommand(argv); got != want {
			t.Errorf("%q: got %q want %q", argv, got, want)
		}
	}
}

// A4-4: a backslash the agent wrote comes out as \\, so it cannot pass for an
// escape of a character it did not write (a real newline is \n).
// A4-2: a run of blanks comes out as ␠×N, so padding cannot push the rest of
// the command out of what the phone shows; a ␠ the agent wrote is escaped.
func TestDisplayCommandEscapesBackslashesAndCompactsBlanks(t *testing.T) {
	pad := strings.Repeat(" ", 4000)
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"shell", `echo a\nb`}, `adb shell echo a\\nb`},
		{[]string{"shell", "echo a\nb"}, `adb shell echo a\nb`},
		{[]string{"shell", `echo \u202e`}, `adb shell echo \\u202e`},
		{[]string{"shell", "echo \u202e"}, `adb shell echo \u202e`},
		{[]string{"shell", "ls" + pad + "; reboot"}, "adb shell ls␠×4000; reboot"},
		{[]string{"shell", "ls   x"}, "adb shell ls   x"},
		{[]string{"shell", "ls    x"}, "adb shell ls␠×4x"},
		{[]string{"shell", "ls\n\n\n\n\nreboot"}, `adb shell ls\n×5reboot`},
		{[]string{"shell", "ls\t\t\t\treboot"}, `adb shell ls\t×4reboot`},
		{[]string{"shell", "echo ␠×9"}, `adb shell echo \u2420\u00d79`}, // B1: the agent's × too
		{[]string{"push", `C:\x\a b.txt`, "/sdcard/x"}, `adb push 'C:\\x\\a b.txt' /sdcard/x`},
	}
	for _, c := range cases {
		if got := DisplayCommand(c.argv); got != c.want {
			t.Errorf("%q: got %q want %q", c.argv, got, c.want)
		}
		// What the surfaces apply again (Visible) changes nothing.
		if got := DisplayCommand(c.argv); Visible(got) != got {
			t.Errorf("%q: Visible changes %q into %q", c.argv, got, Visible(got))
		}
	}
}

// A4-2/A4-3: the agent's words in a title are shortened and their blanks
// compacted, so a long word cannot hide the rest of the title nor make each
// line of the detail endless.
func TestTitlesCompactBlanksAndCapTheAgentsWords(t *testing.T) {
	c := mustNew(t, "")
	pad := strings.Repeat(" ", 4000)
	r := c.Classify(sh(`rm -rf "` + pad + `" /sdcard/DCIM`))
	if !strings.Contains(r.Primary.Title.EN, "/sdcard/DCIM") || utf8.RuneCountInString(r.Primary.Title.EN) > 120 {
		t.Errorf("rm title %q", r.Primary.Title.EN)
	}
	long := strings.Repeat("x", 5000)
	for _, argv := range [][]string{sh("setprop " + long + " 1"), sh("settings put global " + long + " 1")} {
		r = c.Classify(argv)
		for _, f := range r.Findings {
			if n := utf8.RuneCountInString(f.Title.EN); n > 200 {
				t.Errorf("%.40q: title of %d runes", argv, n)
			}
		}
		for _, l := range strings.Split(r.Detail("en"), "\n") {
			if n := utf8.RuneCountInString(l); n > 400 {
				t.Errorf("%.40q: detail line of %d runes", argv, n)
			}
		}
	}
}

// bs is s with each ~ written as a backslash: the escapes read better so.
func bs(s string) string { return strings.ReplaceAll(s, "~", string(rune(92))) }

// H3: a space that is not U+0020 and the invisible fillers are escaped, and a
// run of mixed blanks (spaces, NBSP, tabs, newlines…) is compacted too: padding
// made of them cannot hide the end of the command nor of the title.
func TestVisibleEscapesOtherSpacesAndFillers(t *testing.T) {
	for _, r := range []rune{
		0x00a0, 0x2000, 0x3000, 0x202f, 0x1680, // spaces that are not U+0020
		0x2800, 0x3164, 0x115f, 0x1160, 0xffa0, // blank-looking letters and symbols
		0x034f, 0xfe0f, 0x180b, 0x17b4, // invisible marks
		0x0301, 0x20dd, // combining marks: thousands of them could cover other lines
	} {
		in := "a" + string(r) + "b"
		want := "a" + bs(fmt.Sprintf("~u%04x", r)) + "b"
		if got := Visible(in); got != want {
			t.Errorf("Visible(%q) = %q, want %q", in, got, want)
		}
		if got := Printable(in); got != want {
			t.Errorf("Printable(%q) = %q, want %q", in, got, want)
		}
	}
	if got, want := Visible("a"+string(rune(0xe0100))+"b"), bs("a~U000e0100b"); got != want {
		t.Errorf("Visible: %q, want %q", got, want)
	}
}

func TestDisplayCommandCompactsMixedBlanks(t *testing.T) {
	nbsp, braille := string(rune(0x00a0)), string(rune(0x2800))
	alt := strings.Repeat(" "+nbsp, 3000)
	cases := []struct {
		argv []string
		want string
	}{
		// Few kinds in a row: each one, in order.
		{[]string{"shell", "ls\n    reboot"}, bs("adb shell ls~n␠×4reboot")},
		{[]string{"shell", "ls \t\t\t\t reboot"}, bs("adb shell ls ~t×4 reboot")},
		{[]string{"shell", "ls" + strings.Repeat(nbsp, 4) + "x"}, bs("adb shell ls~u00a0×4x")},
		{[]string{"shell", "ls  " + nbsp + " x"}, bs("adb shell ls  ~u00a0 x")},
		// Many: how many of each kind, in a mark of its own.
		{[]string{"shell", "rm -rf '" + alt + "' /sdcard/DCIM"}, bs("adb shell rm -rf '×6000(␠×3000~u00a0×3000)' /sdcard/DCIM")},
		{[]string{"shell", "ls" + strings.Repeat("\n \t", 100) + "x"}, bs("adb shell ls×300(~n×100␠×100~t×100)x")},
		{[]string{"shell", "ls" + strings.Repeat(" "+braille, 50) + "x"}, bs("adb shell ls×100(␠×50~u2800×50)x")},
		// B1: a × the agent wrote cannot pass for a count; nor a digit after a run.
		{[]string{"shell", "echo \n×5"}, bs("adb shell echo ~n~u00d75")},
		{[]string{"shell", "echo    5"}, bs("adb shell echo␠×4~u0035")},
		{[]string{"shell", "echo    x5"}, bs("adb shell echo␠×4x5")},
	}
	for _, c := range cases {
		got := DisplayCommand(c.argv)
		if got != c.want {
			t.Errorf("%.60q: got %.200q want %.200q", c.argv, got, c.want)
		}
		if Visible(got) != got {
			t.Errorf("%.60q: Visible changes %.200q", c.argv, got)
		}
	}
}

// The title of a recursive delete padded with spaces and NBSP still shows
// what it deletes.
func TestTitlesCompactMixedBlanks(t *testing.T) {
	c := mustNew(t, "")
	nbsp, braille, hangul := string(rune(0x00a0)), string(rune(0x2800)), string(rune(0x3164))
	for _, pad := range []string{
		strings.Repeat(" "+nbsp, 3000), strings.Repeat(nbsp, 3000), strings.Repeat("\t ", 3000),
		strings.Repeat(" "+braille+hangul, 2000),
	} {
		r := c.Classify(sh(`rm -rf "` + pad + `" /sdcard/DCIM`))
		title := Visible(r.Primary.Title.EN)
		if !strings.Contains(title, "/sdcard/DCIM") || utf8.RuneCountInString(title) > 160 {
			t.Errorf("rm title %.200q", title)
		}
	}
}
