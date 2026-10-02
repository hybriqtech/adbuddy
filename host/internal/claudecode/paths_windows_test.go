//go:build windows

package claudecode

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// junction makes link a junction (mklink /J) to target, removed before
// t.TempDir's cleanup so that nothing walks through it. Without mklink the
// test is skipped.
func junction(t *testing.T, link, target string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("a junction cannot be made: %v %s", err, out)
	}
	t.Cleanup(func() { os.Remove(link) }) // the junction only, never what it points to
}

// tailFor follows path as the host does: checked by ValidTranscript first.
func tailFor(t *testing.T, path, cfg string) *Tailer {
	t.Helper()
	tr, err := ValidTranscript(path, cfg)
	if err != nil {
		t.Fatalf("%s refused: %v", path, err)
	}
	return NewTailer(tr, false)
}

// V24: EvalSymlinks does not resolve a junction (Go >= 1.23), so a junction
// under projects/ took the transcript out of it, whether it pointed out of
// projects/ or into it.
func TestValidTranscriptRefusesAJunctionUnderProjects(t *testing.T) {
	cfg := t.TempDir()
	projects := filepath.Join(cfg, "projects")
	p1 := filepath.Join(projects, "p1")
	if err := os.MkdirAll(p1, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(p1, "s.jsonl"), os.O_TRUNC, "")
	out := t.TempDir()
	write(t, filepath.Join(out, "s.jsonl"), os.O_TRUNC, textLine("u9", "m9", "secreto"))
	junction(t, filepath.Join(projects, "J"), out)
	junction(t, filepath.Join(projects, "Jin"), p1)
	for _, p := range []string{
		filepath.Join(projects, "J", "s.jsonl"),     // exists, out of projects/
		filepath.Join(projects, "J", "later.jsonl"), // not yet, out of projects/
		filepath.Join(projects, "Jin", "s.jsonl"),   // exists, back into projects/
		filepath.Join(projects, "Jin", "later.jsonl"),
		filepath.Join(projects, "J", "deeper", "later.jsonl"),
	} {
		if _, err := ValidTranscript(p, cfg); err == nil {
			t.Errorf("%s accepted through a junction", p)
		}
	}
	for _, p := range []string{
		filepath.Join(p1, "s.jsonl"),
		filepath.Join(p1, "later.jsonl"),
		filepath.Join(projects, "new-dir", "later.jsonl"),
	} {
		if _, err := ValidTranscript(p, cfg); err != nil {
			t.Errorf("%s refused: %v", p, err)
		}
	}
}

// V24: the tailer reopened the path at every poll and Lstat only looked at
// its last component: a directory that became a junction after the check
// took the reading out of projects/.
func TestTailerRefusesAJunctionThatAppearsLater(t *testing.T) {
	cfg := t.TempDir()
	p1 := filepath.Join(cfg, "projects", "p1")
	if err := os.MkdirAll(p1, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p1, "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "uno"))
	tl := tailFor(t, path, cfg)
	if ev, err := tl.Poll(); err != nil || len(ev) != 1 {
		t.Fatalf("first poll: %v %v", ev, err)
	}
	out := t.TempDir()
	write(t, filepath.Join(out, "s.jsonl"), os.O_TRUNC, textLine("u1", "m1", "uno")+textLine("u9", "m9", "secreto"))
	if err := os.Rename(p1, p1+"-old"); err != nil {
		t.Fatal(err)
	}
	junction(t, p1, out)
	ev, err := tl.Poll()
	for _, e := range ev {
		if e.Text == "secreto" {
			t.Fatal("the transcript was read through a junction out of projects/")
		}
	}
	if err == nil {
		t.Errorf("no error through a junction: %v", ev)
	}
}

// The restriction of V24: only what lies strictly under projects/ is checked
// for links. A user who moved .claude or projects/ to another disk with a junction
// keeps the chat.
func TestValidTranscriptFollowsALinkedConfigOrProjectsDir(t *testing.T) {
	elsewhere := t.TempDir()
	if err := os.MkdirAll(filepath.Join(elsewhere, "p1"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := t.TempDir()
	junction(t, filepath.Join(cfg, "projects"), elsewhere) // projects/ moved
	path := filepath.Join(cfg, "projects", "p1", "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "uno"))
	if ev, err := tailFor(t, path, cfg).Poll(); err != nil || len(ev) != 1 || ev[0].Text != "uno" {
		t.Errorf("linked projects/: %v %v", ev, err)
	}
	if _, err := ValidTranscript(filepath.Join(cfg, "projects", "p1", "later.jsonl"), cfg); err != nil {
		t.Errorf("linked projects/, not yet written: %v", err)
	}

	moved := t.TempDir() // .claude moved
	link := filepath.Join(t.TempDir(), ".claude")
	junction(t, link, moved)
	if err := os.MkdirAll(filepath.Join(moved, "projects", "p1"), 0o700); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(link, "projects", "p1", "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u2", "m2", "dos"))
	if ev, err := tailFor(t, path, link).Poll(); err != nil || len(ev) != 1 || ev[0].Text != "dos" {
		t.Errorf("linked .claude: %v %v", ev, err)
	}
	if _, err := ValidTranscript(filepath.Join(link, "projects", "new-dir", "later.jsonl"), link); err != nil {
		t.Errorf("linked .claude, not yet written: %v", err)
	}
}
