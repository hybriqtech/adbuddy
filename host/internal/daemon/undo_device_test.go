package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// Final review, Round3: every record keeps the phone it was made on, and undo
// never applies the inverse of one phone's change to another.

func TestRecordsKeepTheActivePhone(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	e.exec(t, "-s", "other", "shell", "ls") // blocked
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.Code != 0 {
		t.Fatalf("undo: %+v", exit)
	}
	all, _, _ := e.store.Load()
	if len(all) != 3 {
		t.Fatalf("entries %+v", all)
	}
	for _, j := range all {
		if j.Serial != "dev1" {
			t.Errorf("%s (%s, %s): serial %q", j.ID, j.Source, j.Status, j.Serial)
		}
	}
}

func TestSelectUndoSkipsChangesOfAnotherPhone(t *testing.T) {
	mine := entry("j_1", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.a")
	mine.Serial = "dev1"
	old := entry("j_2", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.b") // before the serial was recorded
	other := entry("j_3", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.c")
	other.Serial = "devB"
	entries := []journal.Entry{mine, old, other}

	plan, skipped, err := SelectUndo(entries, "last", "", "dev1")
	if err != nil || ids(plan) != "j_2" || itemIDs(skipped) != "j_3" {
		t.Fatalf("last: %s | %s | %v", ids(plan), itemIDs(skipped), err)
	}
	if want := "recorded on another phone (devB): undo it by hand on that phone"; skipped[0].Error != want {
		t.Fatalf("reason %q", skipped[0].Error)
	}
	plan, _, err = SelectUndo(entries, "session", "s_1", "dev1")
	if err != nil || ids(plan) != "j_2,j_1" {
		t.Fatalf("session: %s %v", ids(plan), err)
	}
	if _, _, err := SelectUndo(entries, "j_3", "", "dev1"); err == nil || err.(*UndoError).Code != "OTHER_DEVICE" {
		t.Fatalf("explicit id: %v", err)
	}
	if plan, _, err := SelectUndo(entries, "j_3", "", "devB"); err != nil || ids(plan) != "j_3" {
		t.Fatalf("on its own phone: %s %v", ids(plan), err)
	}
}

func TestUndoOnAnotherPhoneRunsNothing(t *testing.T) {
	e := newEnv(t)
	e.adb.shell["settings get global a"] = "1\n"
	e.exec(t, "shell", "settings", "put", "global", "a", "2")
	e.d.o.Serial = "dev2"
	before := len(e.adb.ran())
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, s)
	if exit.ErrorCode != "NOTHING_TO_UNDO" || len(e.adb.ran()) != before {
		t.Fatalf("%+v runs %q", exit, e.adb.ran())
	}
	var res UndoResult
	json.Unmarshal(s.data[0], &res)
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Error, "another phone (dev1)") {
		t.Fatalf("%+v", res)
	}
}

// Final review, M5: undo checks that the source of a move is still free
// before moving the file back over it.

func kindCmd(p string) string {
	q := shq.Quote(p)
	return "if [ -d " + q + " ]; then echo dir; elif [ -e " + q + " ] || [ -L " + q + " ]; then echo file; else echo none; fi"
}

func TestUndoMoveSkipsWhenTheSourceExistsAgain(t *testing.T) {
	e := newEnv(t)
	e.adb.shell[kindCmd("/sdcard/b")] = "none\n"
	if _, exit := e.exec(t, "shell", "mv", "/sdcard/a", "/sdcard/b"); exit.Code != 0 {
		t.Fatalf("mv: %+v", exit)
	}
	if j := e.last(t); j.Inverse == nil || strings.Join(j.Inverse.Argv, " ") != "shell mv /sdcard/b /sdcard/a" {
		t.Fatalf("entry %+v", j)
	}
	undoRuns := func() int {
		n := 0
		for _, r := range e.adb.ran() {
			if r == "shell mv /sdcard/b /sdcard/a" {
				n++
			}
		}
		return n
	}

	// A new /sdcard/a appeared after the move.
	e.adb.shell[kindCmd("/sdcard/a")] = "file\n"
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, s)
	if exit.Code == 0 || exit.ErrorCode != "UNCERTAIN_STATE" || undoRuns() != 0 {
		t.Fatalf("source exists: %+v runs %q", exit, e.adb.ran())
	}
	var res UndoResult
	json.Unmarshal(s.data[0], &res)
	if len(res.Skipped) != 1 || res.Skipped[0].Error != "/sdcard/a exists again: undo it by hand" {
		t.Fatalf("%+v", res)
	}

	// The phone cannot be read: nothing runs, and it may be retried.
	delete(e.adb.shell, kindCmd("/sdcard/a"))
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.ErrorCode != "UNDO_FAILED" || undoRuns() != 0 {
		t.Fatalf("unreadable: %+v runs %q", exit, e.adb.ran())
	}

	// Free again: the move is undone.
	e.adb.shell[kindCmd("/sdcard/a")] = "none\n"
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, &stream{}); exit.Code != 0 || undoRuns() != 1 {
		t.Fatalf("free: %+v runs %q", exit, e.adb.ran())
	}
}
