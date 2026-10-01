package pty

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain lets the test binary act as the agent: with ADBUDDY_FAKE_AGENT=1
// it echoes each line back in upper case and exits on "exit".
func TestMain(m *testing.M) {
	if os.Getenv("ADBUDDY_FAKE_AGENT") == "1" {
		fmt.Print("\x1b[?2004hready\r\n")
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "exit" {
				os.Exit(7)
			}
			fmt.Printf("got:%s\r\n", strings.ToUpper(line))
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// output collects what the PTY writes.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) read(p PTY) {
	b := make([]byte, 4096)
	for {
		n, err := p.Read(b)
		o.mu.Lock()
		o.buf.Write(b[:n])
		o.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (o *output) waitFor(t *testing.T, s string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		o.mu.Lock()
		got := o.buf.String()
		o.mu.Unlock()
		if strings.Contains(got, s) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	t.Fatalf("%q never came; output:\n%q", s, o.buf.String())
}

func startAgent(t *testing.T) (PTY, *output) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := Start(Options{Path: exe, Dir: t.TempDir(), Env: append(os.Environ(), "ADBUDDY_FAKE_AGENT=1"), Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Kill() })
	out := &output{}
	go out.read(p)
	return p, out
}

// The program sees a terminal, reads what is typed and writes back, and its
// exit code comes out of Wait.
func TestTypeReadAndExit(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	if _, err := p.Write([]byte("hola ñandú\r")); err != nil {
		t.Fatal(err)
	}
	out.waitFor(t, "got:HOLA ÑANDÚ")
	if err := p.Resize(80, 24); err != nil {
		t.Fatal(err)
	}
	p.Write([]byte("exit\r"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, err := p.Wait(ctx)
	if err != nil || code != 7 {
		t.Fatalf("Wait = %d, %v; want 7", code, err)
	}
}

// Kill ends a program that does not end by itself.
func TestKill(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := p.Wait(ctx); ctx.Err() != nil {
		t.Fatalf("still running after Kill: %v", err)
	}
}
