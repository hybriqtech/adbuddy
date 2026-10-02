package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

// chunks is a reader that returns one chunk per Read, like a terminal.
type chunks struct {
	parts []string
	reads int
}

func (c *chunks) Read(p []byte) (int, error) {
	if len(c.parts) == 0 {
		return 0, io.EOF
	}
	c.reads++
	n := copy(p, c.parts[0])
	c.parts = c.parts[1:]
	return n, nil
}

// Ctrl+] ends what is typed: what came before it goes, the rest does not,
// and nothing more is read from the terminal.
func TestDetachReader(t *testing.T) {
	for _, c := range []struct {
		parts []string
		want  string
		reads int
	}{
		{[]string{"\x1dafter", "more"}, "", 1},
		{[]string{"ab\x1dcd", "more"}, "ab", 1},
		{[]string{"first", "x\x1dy", "more"}, "firstx", 2},
		{[]string{"no key", " here"}, "no key here", 2},
	} {
		src := &chunks{parts: c.parts}
		d := &detachReader{r: src}
		got, err := io.ReadAll(d)
		if err != nil || string(got) != c.want {
			t.Errorf("%q: got %q, %v; want %q", c.parts, got, err, c.want)
		}
		if src.reads != c.reads {
			t.Errorf("%q: %d reads from the terminal, want %d", c.parts, src.reads, c.reads)
		}
		if detached := c.want != "no key here"; d.detached() != detached {
			t.Errorf("%q: detached %v", c.parts, d.detached())
		}
	}
}

// Leaving the terminal resets the modes the agent may have set before the
// console is restored, once.
func TestLeaveTerminal(t *testing.T) {
	var out bytes.Buffer
	var order []string
	leave := leaveTerminal(&out, func() { order = append(order, "in:"+out.String()) }, func() { order = append(order, "out") })
	leave()
	leave()
	want := "\x1b[0m\x1b[?25h\x1b[?2004l\x1b[?1004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l"
	if out.String() != want {
		t.Fatalf("wrote %q, want %q", out.String(), want)
	}
	if len(order) != 2 || order[0] != "in:"+want || order[1] != "out" {
		t.Fatalf("restores %q", order)
	}
}

// The messages printed when the terminal leaves raw mode start with "\r\n"
// (the cursor is at the end of the agent's last line): in both languages.
func TestAttachClosingMessagesStartWithCarriageReturn(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		for _, key := range []string{"attach.detached_adbuddy_attach_connects_again", "attach.adbuddy_serve_closed_the_connection"} {
			if got := lang.T(key); !strings.HasPrefix(got, "\r\n") || strings.Count(got, "\n") != 1 {
				t.Errorf("%s %s: %q", lang, key, got)
			}
		}
	}
	if got, want := i18n.EN.T("attach.detached_adbuddy_attach_connects_again"), "\r\nDetached. `adbuddy attach` connects again."; got != want {
		t.Errorf("%q, want %q", got, want)
	}
}
