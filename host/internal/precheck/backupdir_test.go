package precheck

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// The backup of an overwritten file goes to the folder Env.MakeBackupDir
// makes (the journal makes it through its own folder), not to a folder made
// by path; if it cannot be made, there is no inverse and the change is 🔴.
func TestPushBackupDirFromTheJournal(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "notes.txt"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	made := filepath.Join(t.TempDir(), "made")
	dev := &fakeDevice{
		answers: map[string]string{kindCmd("/sdcard/notes.txt"): "file\n"},
		pulled:  map[string]string{"/sdcard/notes.txt": "old"},
	}
	env := Env{Cwd: cwd, BackupDir: filepath.Join(t.TempDir(), "by-path"),
		MakeBackupDir: func() (string, error) { return made, os.MkdirAll(made, 0o700) }}
	push := op("adb-push", "local", "notes.txt", "remote", "/sdcard/notes.txt")
	p := Prepare(context.Background(), dev, push, classify.Yellow, env)
	if want := filepath.Join(made, "original"); p.Level != classify.Yellow || len(p.Backups) != 1 || p.Backups[0] != want {
		t.Fatalf("%+v, want the backup in %s", p, want)
	}
	if _, err := os.Stat(env.BackupDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a folder was made by path: %v", err)
	}
	env.MakeBackupDir = func() (string, error) { return "", errors.New("journal/backups is a link") }
	if p := Prepare(context.Background(), dev, push, classify.Yellow, env); p.Level != classify.Red || p.Inverse != nil {
		t.Fatalf("%+v, want 🔴 without an inverse", p)
	}
}
