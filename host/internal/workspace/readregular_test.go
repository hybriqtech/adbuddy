package workspace

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A settings file larger than 1 MiB is an error, not read whole: Init reads
// what the agent can write only up to a bound (V14).
func TestSettingsLargerThanAMiBAreAnError(t *testing.T) {
	for _, rel := range []string{filepath.Join(".claude", "settings.json"), filepath.Join(".gemini", "settings.json")} {
		t.Run(rel, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err != nil {
				t.Fatal(err)
			}
			big := `{"pad":"` + strings.Repeat("a", 2<<20) + `"}`
			p := filepath.Join(dir, rel)
			if err := os.WriteFile(p, []byte(big), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err == nil || !strings.Contains(err.Error(), "1 MiB") {
				t.Fatalf("error %v, want one about the size", err)
			}
			if read(t, p) != big {
				t.Fatal("the settings file was changed")
			}
		})
	}
}

func TestReadRegular(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "small")
	if err := os.WriteFile(small, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := readRegular(small); err != nil || string(b) != "hello" {
		t.Fatalf("%q %v", b, err)
	}
	big := filepath.Join(dir, "big")
	if err := os.WriteFile(big, bytes.Repeat([]byte("x"), maxRead+10), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := readRegular(big); !errors.Is(err, errTooLarge) || len(b) != maxRead {
		t.Fatalf("len %d, %v: want the first MiB and errTooLarge", len(b), err)
	}
	if _, err := readRegular(dir); !errors.Is(err, errNotRegular) {
		t.Fatalf("a folder: %v, want errNotRegular", err)
	}
	if _, err := readRegular(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing file: %v", err)
	}
}

// A settings file that is a link to a regular JSON file elsewhere is an error,
// and the file on the other side is left as it was (V14).
func TestSettingsThatAreALinkAreAnError(t *testing.T) {
	dir := t.TempDir()
	exe := fakeExe(t, "v1")
	if _, err := Init(Options{Dir: dir, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "settings.json")
	const theirs = `{"permissions":{"allow":["Bash(ls:*)"]}}`
	if err := os.WriteFile(target, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, ".claude", "settings.json")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if _, err := Init(Options{Dir: dir, Exe: exe}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("error %v, want one that says it is not a regular file", err)
	}
	if read(t, target) != theirs {
		t.Fatal("the file on the other side of the link was changed")
	}
	if st, err := os.Lstat(p); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced: %v %v", st, err)
	}
}

// The marker is looked for in the first MiB of a larger file too.
func TestHasUserMarkerInALargeFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(p, append([]byte(UserMarker), bytes.Repeat([]byte("x"), maxRead)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasUserMarker(p) {
		t.Fatal("the marker in the first MiB was not found")
	}
}
