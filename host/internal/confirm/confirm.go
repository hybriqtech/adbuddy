// Package confirm asks a person to allow or deny a 🔴 command (spec §4.5).
// In M1 the question is asked in the terminal running `adbuddy serve`.
package confirm

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// Request is what the person sees. Title and Detail come from the rules,
// never from the agent.
type Request struct {
	ID         string
	Title      string
	Detail     string
	RawCommand string
	ExpiresAt  time.Time
	Special    bool   // may cut the connection or affect ADBuddy
	Key        string // the rule and the exact arguments: what "allow during the session" covers
}

// Decision is the answer.
type Decision int

const (
	Deny Decision = iota
	Allow
	Timeout
	Nobody // nobody could be asked (DenyAll): denied
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Timeout:
		return "timeout"
	case Nobody:
		return "nobody"
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

// DenyAll answers Nobody: used when nobody can be asked (spec §4.5: "si no hay
// ninguna de las dos opciones, se rechaza").
type DenyAll struct{ Out io.Writer }

func (d DenyAll) Confirm(_ context.Context, r Request) Decision {
	if d.Out != nil {
		fmt.Fprintf(d.Out, "🔴 Denied (nobody to confirm): %s\n", classify.Printable(r.Title))
	}
	return Nobody
}

// settleTime: lines that arrive earlier than this after a question is shown
// were typed before it (or for a previous one) and never answer it.
const settleTime = time.Second

// maxLine bounds what is kept of one line of input; the rest is discarded.
const maxLine = 256

type line struct {
	text string
	at   time.Time
}

// Terminal asks one question at a time on a terminal. To allow a command the
// person types the short code shown with that question: a "y" typed ahead, or
// an answer meant for another question, can never approve it.
type Terminal struct {
	out    io.Writer
	lines  chan line
	eof    chan struct{}
	turn   chan struct{}
	settle time.Duration
	code   func() string
	last   string // the previous question's code, never reused next
}

// NewTerminal reads answers from in and writes questions to out.
func NewTerminal(in io.Reader, out io.Writer) *Terminal {
	t := &Terminal{out: out, lines: make(chan line, 16), eof: make(chan struct{}), turn: make(chan struct{}, 1),
		settle: settleTime, code: randomCode}
	go t.read(in)
	return t
}

func (t *Terminal) read(in io.Reader) {
	br := bufio.NewReaderSize(in, maxLine)
	for {
		frag, err := br.ReadSlice('\n')
		text := string(frag)
		for errors.Is(err, bufio.ErrBufferFull) { // an over-long line: keep the start only
			_, err = br.ReadSlice('\n')
		}
		if len(text) > 0 {
			t.lines <- line{text: text, at: time.Now()}
		}
		if err != nil {
			close(t.eof)
			return
		}
	}
}

// randomCode returns a two-digit code, 10-99.
func randomCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(90))
	if err != nil {
		return "73" // never reached: crypto/rand does not fail on supported systems
	}
	return fmt.Sprint(n.Int64() + 10)
}

// Confirm shows the question and waits for its code (allow) or anything else
// (deny).
func (t *Terminal) Confirm(ctx context.Context, r Request) Decision {
	select {
	case t.turn <- struct{}{}:
		defer func() { <-t.turn }()
	case <-ctx.Done():
		return fromContext(ctx)
	}
	code := t.code()
	for i := 0; code == t.last; i++ {
		if i == 8 { // a broken source of codes: step to the next one
			n, _ := strconv.Atoi(code)
			code = fmt.Sprint((n-10+1+90)%90 + 10)
			break
		}
		code = t.code()
	}
	t.last = code
	fmt.Fprintf(t.out, "\n🔴 Confirmation needed (expires in %s)\n   %s\n   %s\n",
		time.Until(r.ExpiresAt).Round(time.Second), classify.Printable(r.Title), classify.Printable(r.RawCommand))
	if r.Detail != "" {
		var parts []string
		for _, l := range strings.Split(r.Detail, "\n") {
			parts = append(parts, classify.Printable(l))
		}
		fmt.Fprintf(t.out, "   Why: %s\n", strings.Join(parts, "\n        "))
	}
	if r.Special {
		fmt.Fprintf(t.out, "   ⚠ This may cut the connection with the phone or affect ADBuddy.\n")
	}
	fmt.Fprintf(t.out, "   To allow it type %s and press Enter; anything else denies it.\nCode: ", code)
	shown := time.Now()
	for {
		select {
		case l := <-t.lines:
			if l.at.Before(shown.Add(t.settle)) {
				continue // typed before the question, or too fast to have read it
			}
			if ctx.Err() != nil {
				return t.expired(ctx)
			}
			if strings.TrimSpace(l.text) == code {
				fmt.Fprintln(t.out, "→ allowed")
				return Allow
			}
			fmt.Fprintln(t.out, "→ denied")
			return Deny
		case <-t.eof:
			// Nobody can answer any more (a pipe that ended): not the user's no.
			fmt.Fprintln(t.out, "\n→ denied (no more input: nobody to confirm)")
			return Nobody
		case <-ctx.Done():
			return t.expired(ctx)
		}
	}
}

func (t *Terminal) expired(ctx context.Context) Decision {
	d := fromContext(ctx)
	if d == Timeout {
		fmt.Fprintln(t.out, "\n→ expired, denied")
	} else {
		fmt.Fprintln(t.out, "\n→ abandoned by the agent")
	}
	return d
}

func fromContext(ctx context.Context) Decision {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Timeout
	}
	return Deny
}
