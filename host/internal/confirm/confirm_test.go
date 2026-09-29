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

func TestAnswers(t *testing.T) {
	cases := map[string]Decision{"y\n": Allow, "sí\n": Allow, "YES\n": Allow, "n\n": Deny, "\n": Deny, "maybe\ny\n": Allow}
	for input, want := range cases {
		pr, pw := io.Pipe()
		out := &syncBuffer{}
		term := NewTerminal(pr, out)
		go func() {
			// wait until the question is shown so the answer is not drained
			for !strings.Contains(out.String(), "Allow?") {
				time.Sleep(5 * time.Millisecond)
			}
			io.WriteString(pw, input)
		}()
		if got := term.Confirm(context.Background(), req()); got != want {
			t.Errorf("%q: got %v want %v", input, got, want)
		}
		pw.Close()
		if !strings.Contains(out.String(), "Restart the phone") || !strings.Contains(out.String(), "adb shell reboot") || !strings.Contains(out.String(), "⚠") {
			t.Errorf("prompt: %q", out.String())
		}
	}
}

func TestTypeAheadIsIgnored(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := NewTerminal(pr, out)
	io.WriteString(pw, "y\n") // typed before any question
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if got := term.Confirm(ctx, req()); got != Timeout {
		t.Fatalf("stale answer was used: %v", got)
	}
}

func TestTimeoutAndCancel(t *testing.T) {
	pr, _ := io.Pipe()
	term := NewTerminal(pr, io.Discard)
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
}

func TestEOFDenies(t *testing.T) {
	term := NewTerminal(strings.NewReader(""), io.Discard)
	if got := term.Confirm(context.Background(), req()); got != Deny {
		t.Fatalf("%v", got)
	}
}

func TestOneQuestionAtATime(t *testing.T) {
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	term := NewTerminal(pr, out)
	results := make(chan Decision, 2)
	for i := 0; i < 2; i++ {
		go func() { results <- term.Confirm(context.Background(), req()) }()
	}
	for answered := 0; answered < 2; answered++ {
		for strings.Count(out.String(), "Allow?") <= answered {
			time.Sleep(5 * time.Millisecond)
		}
		io.WriteString(pw, "y\n")
		if d := <-results; d != Allow {
			t.Fatalf("answer %d: %v", answered, d)
		}
	}
}

func TestDenyAll(t *testing.T) {
	var out bytes.Buffer
	if got := (DenyAll{Out: &out}).Confirm(context.Background(), req()); got != Deny || !strings.Contains(out.String(), "Restart the phone") {
		t.Fatalf("%v %q", got, out.String())
	}
}
