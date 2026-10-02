package journal

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/safeio"
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

// A journal folder that is a link (a junction on Windows), planted after
// Init, is refused: no record and no backup folder land where it points,
// and Load says so instead of reading the other side.
func TestJournalThatIsALinkIsRefused(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "journal")
	linkDir(t, dir, outside)
	s := Open(dir)
	if err := s.Append(Entry{ID: "j_1"}); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("Append: %v, want ErrNotRealDir", err)
	}
	if err := s.Writable(); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("Writable: %v, want ErrNotRealDir", err)
	}
	if _, _, err := s.Load(); !errors.Is(err, safeio.ErrNotRealDir) {
		t.Fatalf("Load: %v, want ErrNotRealDir", err)
	}
	if names, _ := os.ReadDir(outside); len(names) != 0 {
		t.Fatalf("written through the link: %v", names)
	}
}

// This month's file with a second name (a hard link to a file outside the
// journal) is not appended to, and the file outside is left as it was.
func TestMonthFileWithAnotherNameIsRefused(t *testing.T) {
	s := fixedStore(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	if err := s.Append(Entry{ID: "j_1"}); err != nil {
		t.Fatal(err)
	}
	month := filepath.Join(s.Dir(), "2026-10.jsonl")
	outside := filepath.Join(t.TempDir(), "theirs.txt")
	if err := os.WriteFile(outside, []byte("theirs\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(month); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(outside, month); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	if err := s.Append(Entry{ID: "j_2"}); !errors.Is(err, safeio.ErrLinked) {
		t.Fatalf("Append: %v, want ErrLinked", err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "theirs\n" {
		t.Fatalf("the file outside changed: %q", b)
	}
}

// A month "file" that is not a regular file is an error of Load that names
// it, never lines skipped: undo and adbuddy journal fail where the person
// sees it.
func TestLoadNamesAMonthFileThatIsNotRegular(t *testing.T) {
	s := fixedStore(t, time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC))
	if err := s.Append(Entry{ID: "j_1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.Dir(), "2026-09.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Load(); !errors.Is(err, safeio.ErrNotRegular) || !strings.Contains(err.Error(), "2026-09.jsonl") {
		t.Fatalf("Load: %v, want ErrNotRegular naming 2026-09.jsonl", err)
	}
}

// A journal that does not exist yet loads as empty, as before.
func TestLoadWithoutAJournal(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "journal"))
	if entries, skipped, err := s.Load(); err != nil || len(entries) != 0 || skipped != 0 {
		t.Fatalf("%v %d %v", entries, skipped, err)
	}
}
