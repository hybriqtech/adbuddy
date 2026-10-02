package classify

import (
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
		{[]string{"shell", "echo ␠×9"}, `adb shell echo \u2420×9`},
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
