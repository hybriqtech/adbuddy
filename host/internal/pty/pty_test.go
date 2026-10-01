package pty

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	xterm "golang.org/x/term"
)

// TestMain lets the test binary act as the agent: with ADBUDDY_FAKE_AGENT=1
// it reports its terminal's size, echoes each line back in upper case and
// exits on "exit". Other lines: "size" reports the size again, "env" checks
// its directory against ADBUDDY_WANT_DIR and prints ADBUDDY_VAR, "spawn"
// starts a child that only waits and prints its pid. With
// ADBUDDY_FAKE_CHILD=1 it is that child.
func TestMain(m *testing.M) {
	if os.Getenv("ADBUDDY_FAKE_CHILD") == "1" {
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	if os.Getenv("ADBUDDY_FAKE_AGENT") == "1" {
		fakeAgent()
	}
	os.Exit(m.Run())
}

func fakeAgent() {
	size := func() {
		c, r, err := xterm.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			fmt.Printf("size:%v\r\n", err)
			return
		}
		fmt.Printf("size:%dx%d\r\n", c, r)
	}
	fmt.Print("\x1b[?2004hready\r\n")
	size()
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		switch line := strings.TrimSpace(sc.Text()); line {
		case "exit":
			os.Exit(7)
		case "size":
			size()
		case "env":
			wd, _ := os.Getwd()
			a, errA := os.Stat(wd)
			b, errB := os.Stat(os.Getenv("ADBUDDY_WANT_DIR"))
			fmt.Printf("dir:%v\r\nvar:%s\r\n", errA == nil && errB == nil && os.SameFile(a, b), os.Getenv("ADBUDDY_VAR"))
		case "spawn":
			exe, _ := os.Executable()
			c := exec.Command(exe)
			c.Env = append(os.Environ(), "ADBUDDY_FAKE_CHILD=1")
			ownConsole(c)
			if err := c.Start(); err != nil {
				fmt.Printf("child:%v\r\n", err)
				continue
			}
			fmt.Printf("child:%d;\r\n", c.Process.Pid)
		default:
			fmt.Printf("got:%s\r\n", strings.ToUpper(line))
		}
	}
	os.Exit(0)
}

// output collects what the PTY writes.
type output struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	err  error
	done chan struct{} // closed when Read returns an error
}

func (o *output) read(p PTY) {
	defer close(o.done)
	b := make([]byte, 4096)
	for {
		n, err := p.Read(b)
		o.mu.Lock()
		o.buf.Write(b[:n])
		o.err = err
		o.mu.Unlock()
		if err != nil {
			return
		}
	}
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func (o *output) has(s string) bool { return strings.Contains(o.String(), s) }

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
	return startIn(t, Options{Path: exe, Dir: t.TempDir(), Env: append(os.Environ(), "ADBUDDY_FAKE_AGENT=1"), Cols: 100, Rows: 30})
}

func startIn(t *testing.T, o Options) (PTY, *output) {
	t.Helper()
	p, err := Start(o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Kill() })
	out := &output{done: make(chan struct{})}
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
	// Wait keeps answering after Kill released everything.
	if _, err := p.Wait(ctx); err != nil {
		t.Fatalf("Wait after Kill: %v", err)
	}
}

// Hangup ends a program that keeps the default SIGHUP action (Unix); on
// Windows it does nothing and the program keeps running.
func TestHangup(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	if err := p.Hangup(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := p.Wait(ctx)
	if runtime.GOOS == "windows" {
		if ctx.Err() == nil {
			t.Fatalf("ended after a Hangup that does nothing: %v", err)
		}
		return
	}
	if ctx.Err() != nil {
		t.Fatalf("still running after Hangup: %v", err)
	}
}

// Read ends by itself when the program ends, after its last output: no Kill
// is needed (on Windows conpty keeps the pipe open until the pseudo console
// is closed).
func TestReadEndsWhenTheProgramEnds(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	p.Write([]byte("bye\r"))
	out.waitFor(t, "got:BYE")
	p.Write([]byte("exit\r"))
	select {
	case <-out.done:
	case <-time.After(10 * time.Second):
		t.Fatal("Read still blocked 10 s after the program asked to exit")
	}
	out.mu.Lock()
	err := out.err
	out.mu.Unlock()
	if err != io.EOF {
		t.Fatalf("Read ended with %v, want io.EOF", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if code, err := p.Wait(ctx); err != nil || code != 7 {
		t.Fatalf("Wait = %d, %v; want 7", code, err)
	}
	// Once it ended, a hangup sends nothing and Kill keeps the exit code.
	if err := p.Hangup(); err != nil {
		t.Fatalf("Hangup after the end: %v", err)
	}
	if err := p.Kill(); err != nil {
		t.Fatalf("Kill after the end: %v", err)
	}
	if code, err := p.Wait(ctx); err != nil || code != 7 {
		t.Fatalf("Wait after Kill = %d, %v; want 7", code, err)
	}
}

// The program sees the size it was started with, and the new one after a
// Resize.
func TestSize(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "size:100x30")
	if err := p.Resize(80, 24); err != nil {
		t.Fatal(err)
	}
	// The resize and the typing reach the program by different ways: ask
	// until the new size shows.
	deadline := time.Now().Add(10 * time.Second)
	for !out.has("size:80x24") {
		if time.Now().After(deadline) {
			t.Fatalf("size never became 80x24; output:\n%q", out.String())
		}
		p.Write([]byte("size\r"))
		time.Sleep(100 * time.Millisecond)
	}
}

// Dir and Env reach the program.
func TestDirAndEnv(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := append(os.Environ(), "ADBUDDY_FAKE_AGENT=1", "ADBUDDY_WANT_DIR="+dir, "ADBUDDY_VAR=nandu-42")
	p, out := startIn(t, Options{Path: exe, Dir: dir, Env: env, Cols: 100, Rows: 30})
	out.waitFor(t, "ready")
	p.Write([]byte("env\r"))
	out.waitFor(t, "dir:true")
	out.waitFor(t, "var:nandu-42")
}

// Kill also ends the processes the program started.
func TestKillEndsTheChildren(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	p.Write([]byte("spawn\r"))
	re := regexp.MustCompile(`child:(\d+);`)
	var m []string
	deadline := time.Now().Add(10 * time.Second)
	for m = re.FindStringSubmatch(out.String()); m == nil; m = re.FindStringSubmatch(out.String()) {
		if time.Now().After(deadline) {
			t.Fatalf("no child pid; output:\n%q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	pid, _ := strconv.Atoi(m[1])
	t.Cleanup(func() {
		if pr, err := os.FindProcess(pid); err == nil && alive(pid) {
			pr.Kill()
		}
	})
	if !alive(pid) {
		t.Fatalf("child %d not running", pid)
	}
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d still running 10 s after Kill", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
