package safeio

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// linkDir makes link point to the folder target: a symlink, or a junction on
// Windows without the privilege for symlinks. It is removed with os.Remove,
// which never follows it, before t.TempDir's cleanup.
func linkDir(t *testing.T, link, target string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err != nil && runtime.GOOS == "windows" {
		var out []byte
		if out, err = exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("no links here: mklink /J: %v %s", err, out)
		}
	} else if err != nil {
		t.Skipf("no links here: %v", err)
	}
	t.Cleanup(func() { os.Remove(link) })
}

// OpenRoot takes a real folder, never a link or a junction in its place,
// nor a file.
func TestOpenRootOnlyARealFolder(t *testing.T) {
	base := t.TempDir()
	folder := filepath.Join(base, "real")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	r, err := OpenRoot(folder)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRoot(file); !errors.Is(err, ErrNotRealDir) {
		t.Fatalf("a file: %v, want ErrNotRealDir", err)
	}
	if _, err := OpenRoot(filepath.Join(base, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing folder: %v", err)
	}
	link := filepath.Join(base, "link")
	linkDir(t, link, folder)
	if r, err := OpenRoot(link); !errors.Is(err, ErrNotRealDir) {
		if r != nil {
			r.Close()
		}
		t.Fatalf("a link: %v, want ErrNotRealDir", err)
	}
}

// OpenAppendIn creates and appends, OpenRegularIn reads; neither takes a
// folder, nor a file with a second name (outside the folder, maybe), which
// is left as it was.
func TestOpenInRootOnlyRegularFilesWithOneName(t *testing.T) {
	dir := t.TempDir()
	r, err := OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	f, err := OpenAppendIn(r, "a.jsonl", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("x\n")); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if f, err = OpenRegularIn(r, "a.jsonl"); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(b) != "x\n" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := OpenRegularIn(r, "missing.jsonl"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "d.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRegularIn(r, "d.jsonl"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("read a folder: %v, want ErrNotRegular", err)
	}
	if _, err := OpenAppendIn(r, "d.jsonl", 0o600); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("append to a folder: %v, want ErrNotRegular", err)
	}
	outside := filepath.Join(t.TempDir(), "theirs")
	if err := os.WriteFile(outside, []byte("theirs"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, filepath.Join(dir, "h.jsonl")); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	if _, err := OpenAppendIn(r, "h.jsonl", 0o600); !errors.Is(err, ErrLinked) {
		t.Fatalf("append to a hard link: %v, want ErrLinked", err)
	}
	if _, err := OpenRegularIn(r, "h.jsonl"); !errors.Is(err, ErrLinked) {
		t.Fatalf("read a hard link: %v, want ErrLinked", err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "theirs" {
		t.Fatalf("the file outside changed: %q", b)
	}
}
