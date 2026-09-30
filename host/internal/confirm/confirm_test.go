package confirm

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a bytes.Buffer safe for the concurrent writes of the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func req() Request {
	return Request{ID: "j_1", Title: "Restart the phone", RawCommand: "adb shell reboot", Detail: "may restart", ExpiresAt: time.Now().Add(time.Minute), Special: true}
}

// testTerminal has a short settle time and predictable codes.
func testTerminal(in io.Reader, out io.Writer, codes ...string) *Terminal {
	t := NewTerminal(in, out)
	t.settle = 30 * time.Millisecond
	var mu sync.Mutex
	t.code = func() string {
		mu.Lock()
		defer mu.Unlock()
		c := codes[0]
		if len(codes) > 1 {
			codes = codes[1:]
		}
		return c
	}
	return t
}

func waitPrompts(t *testing.T, out *syncBuffer, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for strings.Count(out.String(), "Code: ") < n {
		if time.Now().After(deadline) {
			t.Fatalf("prompt %d never shown:\n%s", n, out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond) // past the settle time
}

func TestOnlyTheQuestionsCodeAllows(t *testing.T) {
	cases := map[string]Decision{"42\n": Allow, " 42 \r\n": Allow, "y\n": Deny, "yes\n": Deny, "n\n": Deny, "\n": Deny, "24\n": Deny}
	for input, want := range cases {
		pr, pw := io.Pipe()
		out := &syncBuffer{}
		term := testTerminal(pr, out, "42")
		go func() {
			waitPrompts(t, out, 1)
			io.WriteString(pw, input)
		}()
		if got := term.Confirm(context.Background(), req()); got != want {
			t.Errorf("%q: got %v want %v", input, got, want)
		}
		pw.Close()
		for _, s := range []string{"Restart the phone", "adb shell reboot", "⚠", "type 42"} {
			if !strings.Contains(out.String(), s) {
				t.Errorf("prompt lacks %q: %q", s, out.String())
			}
		}
	}
}

func TestTypeAheadAndFloodAreIgnored(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42")
	go func() {
		for i := 0; i < 40; i++ { // more than the channel holds
			io.WriteString(pw, "42\n")
		}
	}()
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if got := term.Confirm(ctx, req()); got != Timeout {
		t.Fatalf("typed-ahead answers were used: %v", got)
	}
}

func TestLateAnswerCannotApproveTheNextQuestion(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "11", "22")
	ctxA, cancelA := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelA()
	results := make(chan Decision, 1)
	go func() {
		if d := term.Confirm(ctxA, req()); d != Timeout {
			t.Errorf("A: %v", d)
		}
	}()
	waitPrompts(t, out, 1)
	go func() { results <- term.Confirm(context.Background(), req()) }()
	waitPrompts(t, out, 2)     // A expired and B is shown
	io.WriteString(pw, "11\n") // the person answers A, too late
	if d := <-results; d != Deny {
		t.Fatalf("B decided by A's code: %v", d)
	}
}

func TestOneQuestionAtATime(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42", "43")
	results := make(chan Decision, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- term.Confirm(context.Background(), req()) }()
	}
	waitPrompts(t, out, 1)
	time.Sleep(100 * time.Millisecond)
	if n := strings.Count(out.String(), "Code: "); n != 1 {
		t.Fatalf("%d questions shown at once", n)
	}
	io.WriteString(pw, "42\n")
	if d := <-results; d != Allow {
		t.Fatalf("first: %v", d)
	}
	waitPrompts(t, out, 2)
	io.WriteString(pw, "43\n")
	if d := <-results; d != Allow {
		t.Fatalf("second: %v", d)
	}
}

func TestQueuedQuestionExpiresPromptly(t *testing.T) {
	pr, _ := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42")
	go term.Confirm(context.Background(), req()) // holds the turn
	waitPrompts(t, out, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if got := term.Confirm(ctx, req()); got != Timeout || time.Since(start) > time.Second {
		t.Fatalf("%v after %s", got, time.Since(start))
	}
}

func TestTimeoutCancelAndEOF(t *testing.T) {
	pr, _ := io.Pipe()
	term := testTerminal(pr, io.Discard, "42")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if got := term.Confirm(ctx, req()); got != Timeout {
		t.Fatalf("timeout: %v", got)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if got := term.Confirm(ctx2, req()); got != Deny {
		t.Fatalf("cancel: %v", got)
	}
	if got := testTerminal(strings.NewReader(""), io.Discard, "42").Confirm(context.Background(), req()); got != Deny {
		t.Fatalf("eof: %v", got)
	}
}

func TestLongLinesDoNotBreakTheTerminal(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42", "43")
	go func() {
		waitPrompts(t, out, 1)
		io.WriteString(pw, strings.Repeat("x", 100000)+"\n")
		waitPrompts(t, out, 2)
		io.WriteString(pw, "43\n")
	}()
	if got := term.Confirm(context.Background(), req()); got != Deny {
		t.Fatalf("long line: %v", got)
	}
	if got := term.Confirm(context.Background(), req()); got != Allow {
		t.Fatalf("after a long line: %v", got)
	}
}

func TestEverythingShownIsEscaped(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42")
	r := req()
	r.Title = "a\x1b[2Kb"
	r.RawCommand = "adb shell x‮y"
	r.Detail = "first\nsecond \x1b[31mred"
	go func() {
		waitPrompts(t, out, 1)
		io.WriteString(pw, "n\n")
	}()
	term.Confirm(context.Background(), r)
	if s := out.String(); strings.ContainsAny(s, "\x1b‮") {
		t.Fatalf("unescaped output: %q", s)
	}
}

func TestDenyAll(t *testing.T) {
	var out bytes.Buffer
	if got := (DenyAll{Out: &out}).Confirm(context.Background(), req()); got != Nobody || !strings.Contains(out.String(), "Restart the phone") {
		t.Fatalf("%v %q", got, out.String())
	}
}

func TestConsecutiveQuestionsNeverShareACode(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := testTerminal(pr, out, "42", "42", "43")
	go func() {
		waitPrompts(t, out, 1)
		io.WriteString(pw, "n\n")
		waitPrompts(t, out, 2)
		io.WriteString(pw, "42\n") // the first question's code
	}()
	term.Confirm(context.Background(), req())
	if got := term.Confirm(context.Background(), req()); got != Deny {
		t.Fatalf("the previous code approved the next question: %v", got)
	}
}
