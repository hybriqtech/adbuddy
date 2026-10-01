package pathguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mk(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestWithin(t *testing.T) {
	root := mk(t)
	for _, c := range []struct {
		p      string
		rel    string
		inside bool
	}{
		{root, ".", true},
		{filepath.Join(root, "a", "b"), filepath.Join("a", "b"), true},
		{filepath.Dir(root), "", false},
		{filepath.Join(filepath.Dir(root), "otro"), "", false},
		{root + "x", "", false}, // a sibling with the same prefix
		{filepath.Join(root, "..", "x"), "", false},
	} {
		rel, ok := Within(root, c.p)
		if ok != c.inside || (ok && rel != c.rel) {
			t.Errorf("Within(%q): %q %v, want %q %v", c.p, rel, ok, c.rel, c.inside)
		}
	}
}

func TestWithinIgnoresCaseOnlyOnWindows(t *testing.T) {
	root := mk(t)
	_, ok := Within(root, filepath.Join(strings.ToUpper(root), "a"))
	if ok != (runtime.GOOS == "windows") {
		t.Fatalf("another case inside: %v on %s", ok, runtime.GOOS)
	}
}

func TestResolveLinksOfWhatDoesNotExist(t *testing.T) {
	root := mk(t)
	want := filepath.Join(root, "no", "hay", "f.txt")
	got, err := ResolveLinks(want)
	if err != nil || got != want {
		t.Fatalf("%q %v", got, err)
	}
}

func TestResolveLinksFollowsASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root, target := mk(t), mk(t)
	os.WriteFile(filepath.Join(target, "f"), []byte("x"), 0o600)
	if err := os.Symlink(target, filepath.Join(root, "l")); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveLinks(filepath.Join(root, "l", "f"))
	if err != nil || got != filepath.Join(target, "f") {
		t.Fatalf("%q %v", got, err)
	}
	if _, inside := Within(root, got); inside {
		t.Fatal("inside")
	}
	// A link that points nowhere is an error.
	if err := os.Symlink(filepath.Join(target, "nada"), filepath.Join(root, "roto")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveLinks(filepath.Join(root, "roto")); err == nil {
		t.Fatal("dangling link resolved")
	}
	if !ReparseBelow(root, "l") {
		t.Fatal("symlink not seen by ReparseBelow")
	}
}

func TestReparseBelow(t *testing.T) {
	root := mk(t)
	os.MkdirAll(filepath.Join(root, "a", "b"), 0o700)
	for _, rel := range []string{".", filepath.Join("a", "b"), filepath.Join("a", "no", "existe")} {
		if ReparseBelow(root, rel) {
			t.Errorf("%q is plain", rel)
		}
	}
}

func TestJunctionIsSeenOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows")
	}
	root, target := mk(t), mk(t)
	os.WriteFile(filepath.Join(target, "f"), []byte("x"), 0o600)
	j := filepath.Join(root, "j")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", j, target).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	if !ReparseBelow(root, filepath.Join("j", "f")) || !ReparseBelow(root, "j") {
		t.Fatal("junction not seen")
	}
}
