package journal

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixedStore(t *testing.T, now time.Time) *Store {
	s := Open(filepath.Join(t.TempDir(), "journal"))
	s.now = func() time.Time { return now }
	return s
}

func intp(i int) *int { return &i }

func TestAppendWritesMonthlyJSONL(t *testing.T) {
	now := time.Date(2026, 9, 29, 18, 3, 11, 0, time.UTC)
	s := fixedStore(t, now)
	e := Entry{ID: "j_1", Session: "s_1", Source: SourceAgent, Argv: []string{"shell", "pm", "disable-user", "--user", "0", "com.facebook.katana"},
		Level: "yellow", Rule: "pm.disable-user", Description: "Desactivar la app com.facebook.katana", Status: StatusOK, ExitCode: intp(0),
		Inverse: &Inverse{Argv: []string{"shell", "pm", "enable", "--user", "0", "com.facebook.katana"}}}
	if err := s.Append(e); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir(), "2026-09.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	for _, want := range []string{`"id":"j_1"`, `"ts":"2026-09-29T18:03:11Z"`, `"source":"agent"`, `"status":"ok"`, `"exitCode":0`,
		`"inverse":{"argv":["shell","pm","enable","--user","0","com.facebook.katana"]}`, `"undoneBy":null`, `"backups":null`} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %s in %s", want, line)
		}
	}
	if !strings.HasSuffix(line, "}\n") || strings.Count(line, "\n") != 1 {
		t.Fatalf("not one line: %q", line)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(s.Dir(), "2026-09.jsonl"))
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v", st.Mode().Perm())
		}
	}
}

func TestLoadKeepsLastRecordPerID(t *testing.T) {
	s := fixedStore(t, time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC))
	t1 := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Second)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.Append(Entry{ID: "j_a", TS: t1, Status: StatusRunning}))
	must(s.Append(Entry{ID: "j_b", TS: t2, Status: StatusOK}))
	must(s.Append(Entry{ID: "j_a", TS: t1, Status: StatusOK, ExitCode: intp(0)}))
	undo := "j_c"
	must(s.Append(Entry{ID: "j_a", TS: t1, Status: StatusOK, UndoneBy: &undo}))
	// a torn line from a crash
	f, _ := os.OpenFile(filepath.Join(s.Dir(), "2026-09.jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(`{"id":"j_x","st`)
	f.Close()

	entries, skipped, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 1 || len(entries) != 2 {
		t.Fatalf("skipped %d, entries %+v", skipped, entries)
	}
	if entries[0].ID != "j_a" || entries[0].UndoneBy == nil || *entries[0].UndoneBy != "j_c" || entries[1].ID != "j_b" {
		t.Fatalf("entries %+v", entries)
	}
	e, ok, err := s.Get("j_b")
	if err != nil || !ok || e.Status != StatusOK {
		t.Fatalf("get: %+v %v %v", e, ok, err)
	}
}

func TestLoadAcrossMonthsSortedByTime(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "journal"))
	s.now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }
	if err := s.Append(Entry{ID: "j_new", TS: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC) }
	if err := s.Append(Entry{ID: "j_old", TS: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	entries, _, err := s.Load()
	if err != nil || len(entries) != 2 || entries[0].ID != "j_old" {
		t.Fatalf("%+v %v", entries, err)
	}
}

func TestWritableFailsWhenDirIsAFile(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "journal")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Open(blocker)
	if err := s.Writable(); err == nil {
		t.Fatal("Writable succeeded on a file")
	}
	if err := s.Append(Entry{ID: "j_1"}); err == nil {
		t.Fatal("Append succeeded on a file")
	}
}

func TestConcurrentAppends(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "journal"))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Append(Entry{ID: NewID("j_", time.Now()), Description: strings.Repeat("x", 500)}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	entries, skipped, err := s.Load()
	if err != nil || skipped != 0 || len(entries) != 50 {
		t.Fatalf("%d entries, %d skipped, %v", len(entries), skipped, err)
	}
}

func TestNewID(t *testing.T) {
	a := NewID("j_", time.UnixMilli(1000))
	b := NewID("j_", time.UnixMilli(2000))
	if len(a) != 28 || !strings.HasPrefix(a, "j_") || a >= b {
		t.Fatalf("%s %s", a, b)
	}
	if NewID("j_", time.UnixMilli(1000)) == a {
		t.Fatal("ids must differ")
	}
}
