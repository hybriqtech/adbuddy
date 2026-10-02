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
	os.WriteFile(small, []byte("hello"), 0o600)
	if b, err := readRegular(small); err != nil || string(b) != "hello" {
		t.Fatalf("%q %v", b, err)
	}
	big := filepath.Join(dir, "big")
	os.WriteFile(big, bytes.Repeat([]byte("x"), maxRead+10), 0o600)
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

// The marker is looked for in the first MiB of a larger file too.
func TestHasUserMarkerInALargeFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "SKILL.md")
	os.WriteFile(p, append([]byte(UserMarker), bytes.Repeat([]byte("x"), maxRead)...), 0o600)
	if !hasUserMarker(p) {
		t.Fatal("the marker in the first MiB was not found")
	}
}
