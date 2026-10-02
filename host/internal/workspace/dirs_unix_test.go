//go:build unix

package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// A symbolic link where one of Init's folders goes is refused, before
// anything is created (V26).
func TestInitRefusesASymlinkWhereItsFoldersGo(t *testing.T) {
	for _, rel := range planted {
		t.Run(rel, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}
			checkRefused(t, dir, rel, outside)
		})
	}
}

// The environment itself may be a symbolic link (the user's choice): only
// links inside it are refused.
func TestInitWorksWhenTheEnvironmentIsASymlink(t *testing.T) {
	realDir := t.TempDir()
	env := filepath.Join(t.TempDir(), "env")
	if err := os.Symlink(realDir, env); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	checkInitThroughLink(t, env, realDir)
}
