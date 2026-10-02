package classify

import (
	"strings"
	"testing"
)

// sort -o writes a file even grouped (-uo), glued (-ofile) or after a value
// that starts with a dash; uniq's second positional is an output file and
// everything after -- is positional (P1, review A4).

func classifyBoth(t *testing.T, c *Classifier, cmd string, args []string, want Level) {
	t.Helper()
	argv := append([]string{"shell", cmd}, args...)
	if r := c.Classify(argv); r.Level != want {
		t.Errorf("%q: %v (%s), want %v", argv, r.Level, r.Primary.Reason.EN, want)
	}
	line := cmd + " " + strings.Join(args, " ")
	if r := c.Classify(sh(line)); r.Level != want {
		t.Errorf("shell %q: %v (%s), want %v", line, r.Level, r.Primary.Reason.EN, want)
	}
}

func TestSortOutputOptionIsRed(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"-uo f /dev/null", "-mo f x", "-ro/sdcard/x x", "-o f x", "-ofile x", "--output=f x", "--out f x",
		"-u -o f x", "-nro f x", "x -uo f", "x -o f x", "-t -- -o f x", "-k -- -o f x", "- -uo f",
		"+1 -uo f", "-uo", "-t -o f x", "--output f x", "--o f x", "-k", "-t", "-S -o f", "-q x", "--reverse x", "-ro",
	} {
		classifyBoth(t, c, "sort", strings.Fields(args), Red)
	}
	// wrapped
	for _, line := range []string{"nice sort -uo f x", "timeout 5 sort -uo f x", "nice -n 5 sort -mo f x"} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
		if r := c.Classify(append([]string{"shell"}, strings.Fields(line)...)); r.Level != Red {
			t.Errorf("argv %q: %v, want red", line, r.Level)
		}
	}
}

func TestSortReadingFormsStayGreen(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{
		"", "-h", "-rh", "-u", "-n -r", "-t, -k2 x", "-to x", "-k2,2o x", "-- -o", "-nr x", "-u x y", "-S 1 x", "-T /tmp x", "-k2,2 -t, x",
	} {
		classifyBoth(t, c, "sort", strings.Fields(args), Green)
	}
	for _, line := range []string{"nice sort -rh", "timeout 5 sort -u x"} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v (%s), want green", line, r.Level, r.Primary.Reason.EN)
		}
	}
}

func TestUniqPositionalsAndDoubleDash(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range []string{"-- -x /sdcard/y", "-c in out", "in out", "-f 1 in out", "-f1 in out"} {
		classifyBoth(t, c, "uniq", strings.Fields(args), Red)
	}
	for _, args := range []string{"-f1 in", "-f 1 in", "-c in", "", "-- -x", "-s 2 -w 3 in", "-d"} {
		classifyBoth(t, c, "uniq", strings.Fields(args), Green)
	}
	for _, line := range []string{"nice uniq -- -x /sdcard/y", "timeout 5 uniq -c in out"} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
	}
}
