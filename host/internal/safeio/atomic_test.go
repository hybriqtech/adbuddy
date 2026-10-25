package safeio

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func tempsLeft(t *testing.T, dir, prefix string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestWriteFileAtomicReplacesWithPerm(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("new"), AtomicOpts{Pattern: ".t-*", Perm: 0o600}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Fatalf("content %q", b)
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}
	if left := tempsLeft(t, dir, ".t-"); len(left) != 0 {
		t.Fatalf("temporaries left: %v", left)
	}
}

func TestWriteFileAtomicCreatesNew(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFileAtomic(p, []byte("x"), AtomicOpts{Pattern: ".t-*"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Fatalf("content %q", b)
	}
}

func TestWriteFileAtomicKeepMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix modes")
	}
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("new"), AtomicOpts{Pattern: ".t-*", KeepMode: true}); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	if err := WriteFileAtomic(filepath.Join(filepath.Dir(p), "missing"), nil, AtomicOpts{Pattern: ".t-*", KeepMode: true}); err == nil {
		t.Fatal("KeepMode on a missing file must fail")
	}
}

func TestWriteFileAtomicUsesPattern(t *testing.T) {
	// os.CreateTemp refuses a pattern with a separator: the error shows the
	// pattern reaches it (the callers' tests look at the real prefixes).
	p := filepath.Join(t.TempDir(), "f")
	if err := WriteFileAtomic(p, nil, AtomicOpts{Pattern: "a" + string(filepath.Separator) + "b-*"}); err == nil {
		t.Fatal("pattern with a separator must be refused")
	}
}

func TestWriteFileAtomicNoTempAfterRenameFailure(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.MkdirAll(filepath.Join(p, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("d"), AtomicOpts{Pattern: ".mine-*", Perm: 0o600}); err == nil {
		t.Fatal("rename over a folder must fail")
	}
	if left := tempsLeft(t, dir, ".mine-"); len(left) != 0 {
		t.Fatalf("temporaries left: %v", left)
	}
}
