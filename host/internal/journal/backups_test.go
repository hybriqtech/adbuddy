package journal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
)

// The backup folder of an entry is made inside the journal, through it: a
// link in place of journal or of journal/backups is refused and nothing is
// made where it points.
func TestMakeBackupDir(t *testing.T) {
	now := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	s := fixedStore(t, now)
	got, err := s.MakeBackupDir("j_1")
	if err != nil || got != s.BackupDir("j_1") {
		t.Fatalf("%q %v, want %q", got, err, s.BackupDir("j_1"))
	}
	if st, err := os.Lstat(got); err != nil || !st.IsDir() {
		t.Fatalf("not made: %v %v", st, err)
	}

	s = fixedStore(t, now)
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	linkDir(t, filepath.Join(s.Dir(), "backups"), outside)
	if _, err := s.MakeBackupDir("j_1"); err == nil {
		t.Fatal("made through a link in place of journal/backups")
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Fatalf("made where the link points: %v", names)
	}

	base := t.TempDir()
	linkDir(t, filepath.Join(base, "journal"), outside)
	if _, err := Open(filepath.Join(base, "journal")).MakeBackupDir("j_1"); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("journal is a link: %v, want ErrNotRealDir", err)
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Fatalf("made where the link points: %v", names)
	}
}
