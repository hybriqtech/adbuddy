package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// Final review of M1b, I1: the journal and undo know the phone by its stable
// identity (its mDNS name), not by the serial adb lists it with this session,
// which changes when mDNS renames it or adb connects to its address.

const (
	stableID  = "adb-AAA-bbb._adb-tls-connect._tcp"
	renamedID = "adb-AAA-bbb (2)._adb-tls-connect._tcp"
	otherID   = "adb-CCC-ddd._adb-tls-connect._tcp"
	addressID = "192.0.2.10:41000"
)

func TestUndoFindsTheSamePhoneUnderAnotherSerial(t *testing.T) {
	for _, active := range []string{renamedID, addressID} {
		t.Run(active, func(t *testing.T) {
			e := newEnv(t)
			e.adb.shell["settings get global a"] = "1\n"
			e.d.o.Serial, e.d.o.Identity = stableID, stableID
			if _, exit := e.exec(t, "shell", "settings", "put", "global", "a", "2"); exit.Code != 0 {
				t.Fatalf("change: %+v", exit)
			}
			// Another session: same phone, another serial.
			e.d.o.Serial = active
			before := len(e.adb.ran())
			s := &stream{}
			if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, s); exit.Code != 0 {
				t.Fatalf("undo: %+v stderr %q", exit, s.errOut.String())
			}
			if len(e.adb.ran()) == before {
				t.Fatal("the inverse did not run")
			}
			all, _, _ := e.store.Load()
			for _, j := range all {
				if j.Serial != stableID {
					t.Errorf("%s (%s): serial %q, want the identity", j.ID, j.Source, j.Serial)
				}
			}
		})
	}
}

func TestSelectUndoComparesIdentities(t *testing.T) {
	mine := entry("j_1", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.a")
	mine.Serial = stableID
	renamed := entry("j_2", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.b") // recorded before identities, as (2)
	renamed.Serial = renamedID
	other := entry("j_3", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.c")
	other.Serial = otherID
	otherRenamed := entry("j_4", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.d")
	otherRenamed.Serial = "adb-CCC-ddd (3)._adb-tls-connect._tcp"
	entries := []journal.Entry{mine, renamed, other, otherRenamed}

	plan, skipped, err := SelectUndo(entries, "session", "s_1", stableID)
	if err != nil || ids(plan) != "j_2,j_1" || itemIDs(skipped) != "j_4,j_3" {
		t.Fatalf("session: %s | %s | %v", ids(plan), itemIDs(skipped), err)
	}
	for _, id := range []string{"j_3", "j_4"} {
		if _, _, err := SelectUndo(entries, id, "", stableID); err == nil || err.(*UndoError).Code != "OTHER_DEVICE" {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if plan, _, err := SelectUndo(entries, "j_2", "", stableID); err != nil || ids(plan) != "j_2" {
		t.Fatalf("renamed: %s %v", ids(plan), err)
	}
	// An address says nothing about which phone it was: only an exact match counts.
	addr := entry("j_5", "s_1", "yellow", "ok", "shell", "pm", "enable", "com.e")
	addr.Serial = addressID
	if _, _, err := SelectUndo([]journal.Entry{addr}, "j_5", "", stableID); err == nil || err.(*UndoError).Code != "OTHER_DEVICE" {
		t.Fatalf("address: %v", err)
	}
}

func TestTheIdentityIsAnAcceptedTarget(t *testing.T) {
	e := newEnv(t)
	e.d.o.Serial, e.d.o.Identity = addressID, stableID
	for _, env := range []map[string]string{{"ANDROID_SERIAL": stableID}, {"ANDROID_SERIAL": addressID}} {
		s := &stream{}
		if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "ls"}, Env: env}, s); exit.Code != 0 {
			t.Fatalf("%v: %+v %q", env, exit, s.errOut.String())
		}
	}
	for _, target := range []string{stableID, addressID} {
		if _, exit := e.exec(t, "-s", target, "shell", "ls"); exit.Code != 0 {
			t.Fatalf("-s %s: %+v", target, exit)
		}
	}
	// The real adb gets the active serial from the runner, never the -s given.
	for _, r := range e.adb.ran() {
		if r != "shell ls" {
			t.Fatalf("ran %q", e.adb.ran())
		}
	}
	for _, env := range []map[string]string{{"ANDROID_SERIAL": otherID}, {"ANDROID_SERIAL": renamedID}} {
		if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindExec, Argv: []string{"shell", "ls"}, Env: env}, &stream{}); exit.Code != ExitDenied {
			t.Fatalf("%v: %+v", env, exit)
		}
	}
}

func TestPingAnswersTheIdentity(t *testing.T) {
	e := newEnv(t)
	e.d.o.Serial, e.d.o.Identity = addressID, stableID
	s := &stream{}
	if exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindPing}, s); exit.Code != 0 || len(s.data) != 1 {
		t.Fatalf("%+v", exit)
	}
	var p ipc.Ping
	json.Unmarshal(s.data[0], &p)
	if p.Serial != addressID || p.Identity != stableID || p.Phone() != stableID {
		t.Fatalf("%+v", p)
	}
	// Without an identity, the phone is its serial (and an older host's ping).
	if p := (ipc.Ping{Serial: "dev1"}); p.Phone() != "dev1" {
		t.Fatalf("%+v", p)
	}
}
