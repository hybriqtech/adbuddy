package confirm

import (
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// Shown returns a copy of r with what a person reads to decide made visible:
// Title and RawCommand go through classify.Visible, and Detail line by line,
// so its real line breaks stay. Letters and digits of right-to-left scripts
// come out as \uXXXX, so the order on the screen is the order that runs. If
// any of them was escaped, a last line of Detail says so with note (the
// caller passes it in the language of the PC; the wording is not decided
// here).
//
// Every Confirmer shows r.Shown(...) and nothing else of the agent's text:
// the terminal and the bridge to the app do, and so must any other. The
// daemon does not escape: its title and the journal keep the original.
func (r Request) Shown(note string) Request {
	escaped := hasRTL(r.Title) || hasRTL(r.Detail) || hasRTL(r.RawCommand)
	out := r
	out.Title = classify.Visible(r.Title)
	out.RawCommand = classify.Visible(r.RawCommand)
	lines := strings.Split(r.Detail, "\n")
	for i, l := range lines {
		lines[i] = classify.Visible(l)
	}
	out.Detail = strings.Join(lines, "\n")
	if escaped {
		if out.Detail != "" {
			out.Detail += "\n"
		}
		out.Detail += note
	}
	return out
}

func hasRTL(s string) bool { return strings.IndexFunc(s, classify.RTL) >= 0 }
