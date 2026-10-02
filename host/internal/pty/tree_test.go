package pty

import (
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

// V07: the PTY tells the program and what it starts (in its job on
// Windows, in its session on Unix) from any other process.
func TestContainsTheProgramAndItsChildren(t *testing.T) {
	p, out := startAgent(t)
	out.waitFor(t, "ready")
	p.Write([]byte("spawn\r"))
	re := regexp.MustCompile(`child:(\d+);`)
	deadline := time.Now().Add(30 * time.Second)
	var m []string
	for m = re.FindStringSubmatch(out.String()); m == nil; m = re.FindStringSubmatch(out.String()) {
		if time.Now().After(deadline) {
			t.Fatalf("no child pid; output:\n%q", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	child, _ := strconv.Atoi(m[1])
	t.Cleanup(func() {
		if pr, err := os.FindProcess(child); err == nil && alive(child) {
			pr.Kill()
		}
	})
	tree, ok := p.(interface{ Contains(pid int) (bool, error) })
	if !ok {
		t.Fatal("the PTY cannot tell its process tree")
	}
	for _, c := range []struct {
		what string
		pid  int
		want bool
	}{
		{"the program", p.Pid(), true},
		{"its child", child, true},
		{"this test", os.Getpid(), false},
	} {
		if in, err := tree.Contains(c.pid); err != nil || in != c.want {
			t.Errorf("%s (%d): %v %v, want %v", c.what, c.pid, in, err, c.want)
		}
	}
}
