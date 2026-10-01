package pty

import (
	"testing"

	"golang.org/x/sys/windows"
)

// quote follows CommandLineToArgvW: each argument comes back as it was.
func TestQuote(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"plain", "plain"},
		{"", `""`},
		{"a b", `"a b"`},
		{`a"b`, `a\"b`},
		{`C:\my dir\`, `"C:\my dir\\"`},
		{`x\"y`, `x\\\"y`},
	} {
		if got := quote(c.in); got != c.want {
			t.Errorf("quote(%q) = %s, want %s", c.in, got, c.want)
		}
		argv, err := windows.DecomposeCommandLine("prog " + quote(c.in))
		if err != nil || len(argv) != 2 || argv[1] != c.in {
			t.Errorf("quote(%q) parses back as %q (%v)", c.in, argv, err)
		}
	}
}
