package pty

import (
	"errors"
	"testing"
)

// V07: Contains and Kill at once (Kill closes the job): no race, and once
// the job is closed the tree is not known.
func TestContainsWhileKilled(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	w := p.(*winPTY)
	pid := w.Pid()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_, _ = w.Contains(pid)
		}
	}()
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
	if _, err := w.Contains(pid); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("after Kill: %v, want ErrUnsupported", err)
	}
}
