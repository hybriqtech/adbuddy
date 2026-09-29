package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

func entry(id, session, level, status string, inverse ...string) journal.Entry {
	e := journal.Entry{ID: id, Session: session, Source: journal.SourceAgent, Level: level, Status: status, Description: "desc " + id}
	if len(inverse) > 0 {
		e.Inverse = &journal.Inverse{Argv: inverse}
	}
	return e
}

func ids(es []journal.Entry) string {
	var s []string
	for _, e := range es {
		s = append(s, e.ID)
	}
	return strings.Join(s, ",")
}

func itemIDs(items []UndoItem) string {
	var s []string
	for _, it := range items {
		s = append(s, it.ID)
	}
	return strings.Join(s, ",")
}

func TestSelectUndo(t *testing.T) {
	done := "j_u"
	undone := entry("j_0", "s_1", "yellow", "ok", "shell", "x")
	undone.UndoneBy = &done
	entries := []journal.Entry{
		undone,
		entry("j_1", "s_1", "yellow", "ok", "shell", "a"),
		entry("j_2", "s_1", "green", "ok"),
		entry("j_3", "s_1", "yellow", "denied", "shell", "b"),
		entry("j_4", "s_2", "yellow", "ok", "shell", "c"),
		entry("j_5", "s_2", "yellow", "ok"),                    // UI action, no inverse
		entry("j_6", "s_2", "yellow", "running", "shell", "d"), // the host stopped mid-command
		entry("j_7", "s_2", "yellow", "error", "shell", "e"),
	}
	plan, skipped, err := SelectUndo(entries, "last", "")
	if err != nil || ids(plan) != "j_4" || itemIDs(skipped) != "j_7,j_6,j_5" {
		t.Fatalf("last: %s | %s | %v", ids(plan), itemIDs(skipped), err)
	}
	if skipped[0].Error != skipUncertain || skipped[2].Error != skipNoInverse {
		t.Fatalf("skip reasons: %+v", skipped)
	}
	plan, _, err = SelectUndo(entries, "session", "s_1")
	if err != nil || ids(plan) != "j_1" {
		t.Fatalf("session: %s %v", ids(plan), err)
	}
	for target, code := range map[string]string{"j_0": "ALREADY_UNDONE", "j_3": "NOT_APPLIED", "j_5": "NO_INVERSE", "j_6": "UNCERTAIN_STATE", "j_7": "UNCERTAIN_STATE", "j_9": "NOT_FOUND"} {
		if _, _, err := SelectUndo(entries, target, ""); err == nil || err.(*UndoError).Code != code {
			t.Errorf("%s: %v", target, err)
		}
	}
	if _, _, err := SelectUndo(entries[:1], "last", ""); err == nil || err.(*UndoError).Code != "NOTHING_TO_UNDO" {
		t.Fatalf("nothing: %v", err)
	}
}

func TestUndoRunsInversesNewestFirstAndMarksEntries(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.adb.shell["settings get global b"] = "null\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	time.Sleep(2 * time.Millisecond)
	e.exec(t, "shell", "settings", "put", "global", "b", "3")

	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "session"}, s)
	if exit.Code != 0 {
		t.Fatalf("%+v %q", exit, s.errOut.String())
	}
	runs := e.adb.ran()
	tail := runs[len(runs)-2:]
	if tail[0] != "shell settings delete global b" || tail[1] != "shell settings put global a 1" {
		t.Fatalf("runs %q", runs)
	}
	var res UndoResult
	json.Unmarshal(s.data[0], &res)
	if len(res.Undone) != 2 || len(res.Failed) != 0 {
		t.Fatalf("%+v", res)
	}
	all, _, _ := e.store.Load()
	var undos, marked int
	for _, j := range all {
		if j.Source == journal.SourceUndo && j.Status == journal.StatusOK && strings.HasPrefix(j.Description, "Undo: ") {
			undos++
		}
		if j.UndoneBy != nil {
			marked++
		}
	}
	if undos != 2 || marked != 2 {
		t.Fatalf("undos %d marked %d", undos, marked)
	}
	// Nothing left to undo.
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.ErrorCode != "NOTHING_TO_UNDO" {
		t.Fatalf("%+v", exit)
	}
}

func TestUndoFailureIsReported(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	e.adb.exits["shell settings put global a 1"] = 1
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, s)
	if exit.Code != 1 || exit.ErrorCode != "UNDO_FAILED" {
		t.Fatalf("%+v", exit)
	}
	all, _, _ := e.store.Load()
	for _, j := range all {
		if j.UndoneBy != nil {
			t.Fatalf("failed undo must not mark %s", j.ID)
		}
	}
}

func TestSelectUndoTrustsOnlySignedRecordsAndFinishedUndos(t *testing.T) {
	forged := entry("j_f", "s_1", "yellow", "ok", "shell", "settings", "put", "global", "adb_enabled", "0")
	forged.Tampered = true
	crashedUndo := journal.Entry{ID: "j_u1", Source: journal.SourceUndo, Status: journal.StatusRunning, Undoes: "j_2"}
	finishedUndo := journal.Entry{ID: "j_u2", Source: journal.SourceUndo, Status: journal.StatusOK, Undoes: "j_3"}
	entries := []journal.Entry{
		entry("j_1", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.a"),
		entry("j_2", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.b"),
		entry("j_3", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.c"),
		crashedUndo, finishedUndo, forged,
	}
	plan, skipped, err := SelectUndo(entries, "session", "s_1")
	if err != nil || ids(plan) != "j_1" || itemIDs(skipped) != "j_f,j_2" {
		t.Fatalf("plan %s skipped %s err %v", ids(plan), itemIDs(skipped), err)
	}
	for target, code := range map[string]string{"j_f": "UNTRUSTED_ENTRY", "j_2": "UNCERTAIN_STATE", "j_3": "ALREADY_UNDONE", "j_u2": "NO_INVERSE"} {
		if _, _, err := SelectUndo(entries, target, ""); err == nil || err.(*UndoError).Code != code {
			t.Errorf("%s: %v", target, err)
		}
	}
}

func TestValidInverse(t *testing.T) {
	backup := filepath.Join("j", "backups", "j_1")
	good := [][]string{
		{"shell", "settings", "put", "global", "window_animation_scale", "1.0"},
		{"shell", "settings", "--user", "0", "put", "secure", "k", "'a b; $(reboot)'"},
		{"shell", "settings", "delete", "global", "k"},
		{"shell", "pm", "enable", "--user", "0", "com.facebook.katana"},
		{"shell", "cmd", "package", "install-existing", "--user", "0", "com.sys"},
		{"shell", "appops", "set", "com.x", "RUN_IN_BACKGROUND", "ignore"},
		{"shell", "dumpsys", "deviceidle", "whitelist", "-com.c"},
		{"shell", "mv", "'/sdcard/new name'", "/sdcard/a"},
		{"shell", "rmdir", "/sdcard/New"},
		{"shell", "rm", "-f", "/sdcard/t.png"},
		{"uninstall", "com.new"},
		{"push", filepath.Join(backup, "original"), "/sdcard/Download/notes.txt"},
	}
	for _, argv := range good {
		if err := validInverse(argv, backup); err != nil {
			t.Errorf("%q: %v", argv, err)
		}
	}
	bad := [][]string{
		{"shell", "rm -rf /sdcard"},
		{"shell", "rm", "-rf", "/sdcard"},
		{"shell", "settings", "put", "global", "k", "v;reboot"},
		{"shell", "pm", "clear", "com.x"},
		{"shell", "mv", "a", "b"},
		{"push", "C:/Users/u/.ssh/id_rsa", "/sdcard/x"},
		{"push", filepath.Join(backup, "original"), "relative"},
		{"-s", "other", "shell", "pm", "enable", "com.x"},
		{"tcpip", "5555"},
		{"uninstall", "-k", "com.x"},
		{},
	}
	for _, argv := range bad {
		if err := validInverse(argv, backup); err == nil {
			t.Errorf("accepted %q", argv)
		}
	}
}

func TestUndoRefusesAForgedJournalLine(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	// The agent appends a line of its own to the journal in its folder.
	file := filepath.Join(e.store.Dir(), time.Now().UTC().Format("2006-01")+".jsonl")
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(`{"id":"j_zz","ts":"2099-01-01T00:00:00Z","status":"ok","level":"yellow","source":"agent","inverse":{"argv":["shell","settings","put","global","adb_enabled","0"]}}` + "\n")
	f.Close()
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, s)
	if exit.Code != 0 {
		t.Fatalf("%+v %q", exit, s.errOut.String())
	}
	for _, r := range e.adb.ran() {
		if strings.Contains(r, "adb_enabled") {
			t.Fatalf("ran the forged inverse: %q", r)
		}
	}
	var res UndoResult
	json.Unmarshal(s.data[0], &res)
	if itemIDs(res.Undone) == "" || itemIDs(res.Skipped) != "j_zz" || res.Skipped[0].Error != skipUntrusted {
		t.Fatalf("%+v", res)
	}
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "j_zz"}, &stream{}); exit.ErrorCode != "UNTRUSTED_ENTRY" {
		t.Fatalf("%+v", exit)
	}
}

func TestConcurrentUndosRunTheInverseOnce(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
		}()
	}
	wg.Wait()
	n := 0
	for _, r := range e.adb.ran() {
		if r == "shell settings put global a 1" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("inverse ran %d times", n)
	}
}

func TestUndoStopsWhenItCannotRecordTheUndo(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.adb.shell["settings get global b"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	time.Sleep(2 * time.Millisecond)
	e.exec(t, "shell", "settings", "put", "global", "b", "2")
	// Break the journal right after the first inverse runs.
	moved := e.store.Dir() + ".moved"
	e.d.o.ADB = &installADB{fakeADB: e.adb, after: func() {
		os.Rename(e.store.Dir(), moved)
		os.WriteFile(e.store.Dir(), []byte("x"), 0o600)
	}}
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "session"}, s)
	if exit.Code != 1 || exit.ErrorCode != "JOURNAL_UNWRITABLE" {
		t.Fatalf("%+v", exit)
	}
	n := 0
	for _, r := range e.adb.ran() {
		if strings.HasPrefix(r, "shell settings put global") && strings.HasSuffix(r, " 1") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("ran %d inverses after the journal broke", n)
	}
}

func TestDeletedJournalLinesCannotRepeatAnUndo(t *testing.T) {
	e := newEnv(t)
	e.d.o.UndoneLog = filepath.Join(t.TempDir(), "data", "undone.log")
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	// The agent deletes every journal line after the change itself.
	file := filepath.Join(e.store.Dir(), time.Now().UTC().Format("2006-01")+".jsonl")
	lines := strings.SplitAfter(string(mustRead(t, file)), "\n")
	os.WriteFile(file, []byte(lines[0]+lines[1]), 0o600) // running + ok of the change
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	if exit.ErrorCode != "NOTHING_TO_UNDO" {
		t.Fatalf("undo ran again: %+v", exit)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The undone log gets "running <id>" before the inverse runs. If the host
// dies before "<id>" follows, deleting the journal lines does not let the
// agent repeat that inverse: the change stays uncertain. "failed <id>" means
// the inverse failed and the change can be undone again.
func TestUndoRunningMarkKeepsAnInterruptedUndoUncertain(t *testing.T) {
	e := newEnv(t)
	log := filepath.Join(t.TempDir(), "data", "undone.log")
	e.d.o.UndoneLog = log
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	id := e.last(t).ID
	var seen string
	e.d.o.ADB = &installADB{fakeADB: e.adb, after: func() {
		b, _ := os.ReadFile(log)
		seen = string(b)
	}}
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	if seen != "running "+id+"\n" {
		t.Fatalf("log while the inverse ran: %q", seen)
	}
	if got := string(mustRead(t, log)); got != "running "+id+"\n"+id+"\n" {
		t.Fatalf("log after the undo: %q", got)
	}
	inverses := func() int {
		n := 0
		for _, r := range e.adb.ran() {
			if r == "shell settings put global a 1" {
				n++
			}
		}
		return n
	}

	// The host died after the inverse: only the mark is left, and the agent
	// deletes every journal line after the change itself.
	os.WriteFile(log, []byte("running "+id+"\n"), 0o600)
	file := filepath.Join(e.store.Dir(), time.Now().UTC().Format("2006-01")+".jsonl")
	lines := strings.SplitAfter(string(mustRead(t, file)), "\n")
	os.WriteFile(file, []byte(lines[0]+lines[1]), 0o600)
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, s)
	var res UndoResult
	if len(s.data) > 0 {
		json.Unmarshal(s.data[len(s.data)-1], &res)
	}
	if exit.ErrorCode != "NOTHING_TO_UNDO" || itemIDs(res.Skipped) != id || res.Skipped[0].Error != skipUncertain {
		t.Fatalf("%+v %+v", exit, res)
	}
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: id}, &stream{}); exit.ErrorCode != "UNCERTAIN_STATE" {
		t.Fatalf("%+v", exit)
	}
	if n := inverses(); n != 1 {
		t.Fatalf("inverse ran %d times", n)
	}

	// The inverse failed: the change can be undone again.
	os.WriteFile(log, []byte("running "+id+"\nfailed "+id+"\n"), 0o600)
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.Code != 0 {
		t.Fatalf("%+v", exit)
	}
	if n := inverses(); n != 2 {
		t.Fatalf("inverse ran %d times", n)
	}
}

// A failed inverse is logged as failed, so it can be retried.
func TestUndoLogsAFailedInverse(t *testing.T) {
	e := newEnv(t)
	log := filepath.Join(t.TempDir(), "data", "undone.log")
	e.d.o.UndoneLog = log
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	id := e.last(t).ID
	e.adb.exits["shell settings put global a 1"] = 1
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.ErrorCode != "UNDO_FAILED" {
		t.Fatalf("%+v", exit)
	}
	if got := string(mustRead(t, log)); got != "running "+id+"\nfailed "+id+"\n" {
		t.Fatalf("log: %q", got)
	}
	delete(e.adb.exits, "shell settings put global a 1")
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.Code != 0 {
		t.Fatalf("retry: %+v", exit)
	}
}

// Without the running mark on disk the inverse does not run.
func TestUndoNeedsTheRunningMark(t *testing.T) {
	e := newEnv(t)
	log := filepath.Join(t.TempDir(), "undone.log")
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chmod(log, 0o400)
	t.Cleanup(func() { os.Chmod(log, 0o600) })
	e.d.o.UndoneLog = log
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	before := len(e.adb.ran())
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{})
	if exit.ErrorCode != "JOURNAL_UNWRITABLE" || len(e.adb.ran()) != before {
		t.Fatalf("%+v, ran %q", exit, e.adb.ran()[before:])
	}
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.ErrorCode != "JOURNAL_UNWRITABLE" {
		t.Fatalf("the change was marked although nothing ran: %+v", exit)
	}
}
