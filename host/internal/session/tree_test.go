package session

import (
	"errors"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/pty"
)

// treeFake is a fake agent that can tell its process tree.
type treeFake struct {
	*fakePTY
	contains func(pid int) (bool, error)
}

func (f treeFake) Contains(pid int) (bool, error) { return f.contains(pid) }

// V07: InAgentTree asks the agent's PTY. Without an agent nothing is in its
// tree; a PTY that cannot tell, or has no record of its tree, is unknown; any
// other error counts as in the tree.
func TestInAgentTree(t *testing.T) {
	var contains func(int) (bool, error) // read by Start, on this goroutine
	m := New(Options{
		Dir:  t.TempDir(),
		Look: func(name string) (string, error) { return "/bin/" + name, nil },
		Env:  func(string) []string { return nil },
		Start: func(pty.Options) (pty.PTY, error) {
			if contains == nil {
				return newFake(), nil
			}
			return treeFake{newFake(), contains}, nil
		},
	})
	t.Cleanup(m.Stop)
	if in, known := m.InAgentTree(42); in || !known {
		t.Fatalf("no agent: %v %v, want not in it, known", in, known)
	}
	for _, c := range []struct {
		name      string
		contains  func(int) (bool, error)
		in, known bool
	}{
		{"cannot tell", nil, false, false},
		{"in", func(pid int) (bool, error) { return pid == 42, nil }, true, true},
		{"out", func(int) (bool, error) { return false, nil }, false, true},
		{"no record", func(int) (bool, error) { return false, errors.ErrUnsupported }, false, false},
		{"error", func(int) (bool, error) { return false, errors.New("injected") }, true, true},
	} {
		contains = c.contains
		if err := m.Start("claude", false); err != nil {
			t.Fatal(err)
		}
		if in, known := m.InAgentTree(42); in != c.in || known != c.known {
			t.Errorf("%s: %v %v, want %v %v", c.name, in, known, c.in, c.known)
		}
	}
}
