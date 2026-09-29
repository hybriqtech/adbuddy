// Package confirm asks a person to allow or deny a 🔴 command (spec §4.5).
// In M1 the question is asked in the terminal running `adbuddy serve`.
package confirm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Request is what the person sees. Title and Detail come from the rules,
// never from the agent.
type Request struct {
	ID         string
	Title      string
	Detail     string
	RawCommand string
	ExpiresAt  time.Time
	Special    bool // may cut the connection or affect ADBuddy
}

// Decision is the answer.
type Decision int

const (
	Deny Decision = iota
	Allow
	Timeout
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Timeout:
		return "timeout"
	default:
		return "deny"
	}
}

// Confirmer asks for a decision. ctx carries the expiry: when its deadline
// passes the answer is Timeout; when it is cancelled (the command was
// abandoned) the answer is Deny.
type Confirmer interface {
	Confirm(ctx context.Context, r Request) Decision
}

// DenyAll answers Deny: used when nobody can be asked (spec §4.5: "si no hay
// ninguna de las dos opciones, se rechaza").
type DenyAll struct{ Out io.Writer }

func (d DenyAll) Confirm(_ context.Context, r Request) Decision {
	if d.Out != nil {
		fmt.Fprintf(d.Out, "🔴 Denied (no terminal to confirm in): %s\n", r.Title)
	}
	return Deny
}

// Terminal asks one question at a time on a terminal.
type Terminal struct {
	out   io.Writer
	lines chan string
	eof   chan struct{}
	turn  chan struct{}
}

// NewTerminal reads answers from in and writes questions to out.
func NewTerminal(in io.Reader, out io.Writer) *Terminal {
	t := &Terminal{out: out, lines: make(chan string, 16), eof: make(chan struct{}), turn: make(chan struct{}, 1)}
	go func() {
		sc := bufio.NewScanner(in)
		for sc.Scan() {
			t.lines <- sc.Text()
		}
		close(t.eof)
	}()
	return t
}

// Confirm asks and waits for "y" or "n".
func (t *Terminal) Confirm(ctx context.Context, r Request) Decision {
	select {
	case t.turn <- struct{}{}:
		defer func() { <-t.turn }()
	case <-ctx.Done():
		return fromContext(ctx)
	}
	// Anything typed before the question appeared must not answer it.
	for drained := false; !drained; {
		select {
		case <-t.lines:
		default:
			drained = true
		}
	}
	fmt.Fprintf(t.out, "\n🔴 Confirmation needed (expires in %s)\n   %s\n   %s\n", time.Until(r.ExpiresAt).Round(time.Second), r.Title, r.RawCommand)
	if r.Detail != "" {
		fmt.Fprintf(t.out, "   Why: %s\n", strings.ReplaceAll(r.Detail, "\n", "\n        "))
	}
	if r.Special {
		fmt.Fprintf(t.out, "   ⚠ This may cut the connection with the phone or affect ADBuddy.\n")
	}
	for {
		fmt.Fprint(t.out, "Allow? [y/N]: ")
		select {
		case line := <-t.lines:
			switch strings.ToLower(strings.TrimSpace(line)) {
			case "y", "yes", "s", "si", "sí":
				fmt.Fprintln(t.out, "→ allowed")
				return Allow
			case "", "n", "no":
				fmt.Fprintln(t.out, "→ denied")
				return Deny
			default:
				fmt.Fprintln(t.out, "Please answer y or n.")
			}
		case <-t.eof:
			fmt.Fprintln(t.out, "\n→ denied (no more input)")
			return Deny
		case <-ctx.Done():
			d := fromContext(ctx)
			if d == Timeout {
				fmt.Fprintln(t.out, "\n→ expired, denied")
			} else {
				fmt.Fprintln(t.out, "\n→ abandoned by the agent")
			}
			return d
		}
	}
}

func fromContext(ctx context.Context) Decision {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Timeout
	}
	return Deny
}
