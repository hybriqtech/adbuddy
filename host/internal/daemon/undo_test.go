package daemon

import (
	"context"
	"encoding/json"
	"strings"
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
	if err != nil || ids(plan) != "j_4" || ids(skipped) != "j_7,j_6,j_5" {
		t.Fatalf("last: %s | %s | %v", ids(plan), ids(skipped), err)
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
