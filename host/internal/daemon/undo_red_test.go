package daemon

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// forge writes a journal entry signed with the installation key, as an agent
// that read ipc.secret could: the signature proves nothing about the inverse.
func (e *env) forge(t *testing.T, id string, argv []string, inverse ...string) journal.Entry {
	t.Helper()
	return e.forgeDesc(t, id, "forged "+id, argv, inverse...)
}

func (e *env) forgeDesc(t *testing.T, id, desc string, argv []string, inverse ...string) journal.Entry {
	t.Helper()
	code := 0
	j := journal.Entry{
		ID: id, TS: e.d.o.Now().UTC(), Session: "s_1", Serial: "dev1", Source: journal.SourceAgent,
		Argv: argv, Level: "yellow", Rule: "r", Description: desc, Status: journal.StatusOK, ExitCode: &code,
		Inverse: &journal.Inverse{Argv: inverse},
	}
	if err := e.store.Append(j); err != nil {
		t.Fatal(err)
	}
	return j
}

func (e *env) undoLast(t *testing.T) (UndoResult, ipc.Exit, *stream) {
	t.Helper()
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "last"}, s)
	var res UndoResult
	if len(s.data) > 0 {
		json.Unmarshal(s.data[0], &res)
	}
	return res, exit, s
}

func (e *env) ranContains(sub string) bool {
	for _, r := range e.adb.ran() {
		if strings.Contains(r, sub) {
			return true
		}
	}
	return false
}

var putArgv = []string{"shell", "settings", "put", "global", "a", "2"}

func TestUndoAsksBeforeARedInverse(t *testing.T) {
	// `settings delete` is 🔴: a signed record cannot make it run unasked.
	delInv := []string{"shell", "settings", "delete", "global", "a"}
	for _, tc := range []struct {
		name     string
		decision confirm.Decision
		runs     bool
		skip     string
	}{
		{"denied", confirm.Deny, false, "denied"},
		{"nobody", confirm.Nobody, false, "nobody could confirm"},
		{"expired", confirm.Timeout, false, "expired"},
		{"allowed", confirm.Allow, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.conf.decision = tc.decision
			e.forge(t, "j_f1", putArgv, delInv...)
			res, exit, s := e.undoLast(t)
			if len(e.conf.asked) != 1 || !strings.Contains(e.conf.asked[0].RawCommand, "settings delete global a") {
				t.Fatalf("asked %+v", e.conf.asked)
			}
			if e.ranContains("settings delete global a") != tc.runs {
				t.Fatalf("ran %q, want run=%v", e.adb.ran(), tc.runs)
			}
			if tc.runs {
				if exit.Code != 0 || len(res.Undone) != 1 {
					t.Fatalf("%+v %+v %q", exit, res, s.errOut.String())
				}
				return
			}
			if len(res.Undone) != 0 || len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Error, tc.skip) || exit.Code != ExitDenied {
				t.Fatalf("%+v %+v", exit, res)
			}
			// Denied: the change stays pending, nothing marked as undone.
			all, _, _ := e.store.Load()
			for _, j := range all {
				if j.UndoneBy != nil || j.Source == journal.SourceUndo {
					t.Fatalf("recorded %+v", j)
				}
			}
		})
	}
}

func TestUndoRefusesAnInverseOfAnotherKind(t *testing.T) {
	e := newEnv(t) // Allow: asking must not even be reached
	e.forge(t, "j_f1", putArgv, "uninstall", "com.victim.app")
	res, exit, _ := e.undoLast(t)
	if e.ranContains("uninstall") || len(e.conf.asked) != 0 {
		t.Fatalf("ran %q asked %d", e.adb.ran(), len(e.conf.asked))
	}
	if len(res.Failed) != 1 || !strings.Contains(res.Failed[0].Error, "refused") || exit.Code == 0 {
		t.Fatalf("%+v %+v", exit, res)
	}
}

func TestUndoYellowInverseStillRunsUnasked(t *testing.T) {
	e := newEnv(t)
	e.forge(t, "j_f1", putArgv, "shell", "settings", "put", "global", "a", "1")
	res, exit, _ := e.undoLast(t)
	if exit.Code != 0 || len(res.Undone) != 1 || len(e.conf.asked) != 0 || !e.ranContains("settings put global a 1") {
		t.Fatalf("%+v %+v asked %d ran %q", exit, res, len(e.conf.asked), e.adb.ran())
	}
}

func TestInverseFitsTheKind(t *testing.T) {
	for _, tc := range []struct {
		kind string
		inv  []string
		ok   bool
	}{
		{"settings-put", []string{"shell", "settings", "delete", "global", "k"}, true},
		{"settings-put", []string{"shell", "settings", "--user", "0", "put", "secure", "k", "v"}, true},
		{"settings-put", []string{"uninstall", "com.x"}, false},
		{"settings-put", []string{"shell", "rm", "-f", "/sdcard/x"}, false},
		{"adb-install", []string{"uninstall", "com.x"}, true},
		{"adb-install", []string{"shell", "rm", "-f", "/sdcard/x"}, false},
		{"adb-push", []string{"shell", "rm", "-f", "/sdcard/x"}, true},
		{"adb-push", []string{"push", "/b/original", "/sdcard/x"}, true},
		{"new-file", []string{"shell", "rm", "-f", "/sdcard/x"}, true},
		{"mkdir", []string{"shell", "rmdir", "/sdcard/d"}, true},
		{"mkdir", []string{"shell", "rm", "-f", "/sdcard/d"}, false},
		{"mv", []string{"shell", "mv", "/sdcard/b", "/sdcard/a"}, true},
		{"pm-disable", []string{"shell", "pm", "enable", "com.x"}, true},
		{"pm-uninstall-user", []string{"shell", "cmd", "package", "install-existing", "--user", "0", "com.x"}, true},
		{"appops-set", []string{"shell", "appops", "set", "com.x", "OP", "ignore"}, true},
		{"deviceidle", []string{"shell", "dumpsys", "deviceidle", "whitelist", "-com.c"}, true},
		{"deviceidle", []string{"shell", "pm", "enable", "com.x"}, false},
		{"", []string{"uninstall", "com.x"}, false},
	} {
		if got := inverseFitsKind(tc.kind, tc.inv); got != tc.ok {
			t.Errorf("%q %q: got %v", tc.kind, tc.inv, got)
		}
	}
}

// What the person reads comes from the classifier: the journal is forgeable.
func TestUndoQuestionTextComesFromTheClassifier(t *testing.T) {
	e := newEnv(t)
	e.forgeDesc(t, "j_f1", "Restore screen brightness", putArgv, "shell", "settings", "delete", "global", "a")
	e.undoLast(t)
	if len(e.conf.asked) != 1 {
		t.Fatalf("asked %d", len(e.conf.asked))
	}
	r := e.conf.asked[0]
	if strings.Contains(r.Title, "brightness") || strings.Contains(r.Detail, "brightness") || r.Title == "" || r.Key != "" || r.ID == "j_f1" || r.ID == "" {
		t.Fatalf("%+v", r)
	}
	e = newEnv(t)
	e.forgeDesc(t, "j_f2", "Restore screen brightness", putArgv, "shell", "settings", "put", "global", "a", "1")
	e.forgeDesc(t, "j_f3", "Restore screen brightness", []string{"install", "/x/a.apk"}, "uninstall", "com.whatsapp")
	e.undoLast(t)
	if len(e.conf.asked) != 1 || strings.Contains(e.conf.asked[0].Title+e.conf.asked[0].Detail, "brightness") {
		t.Fatalf("%+v", e.conf.asked)
	}
}

func TestUndoPassesSpecialToTheQuestion(t *testing.T) {
	e := newEnv(t)
	e.forge(t, "j_f1", putArgv, "shell", "settings", "put", "global", "adb_wifi_enabled", "0")
	e.undoLast(t)
	if len(e.conf.asked) != 1 || !e.conf.asked[0].Special {
		t.Fatalf("%+v", e.conf.asked)
	}
}

type pauseOnAskConfirmer struct{ pause func() }

func (c pauseOnAskConfirmer) Confirm(context.Context, confirm.Request) confirm.Decision {
	c.pause()
	return confirm.Allow
}

func TestUndoPausedWhileAskedRunsNothing(t *testing.T) {
	e := newEnv(t)
	paused := false
	e.d.o.Paused = func() bool { return paused }
	e.d.o.Confirmer = pauseOnAskConfirmer{func() { paused = true }}
	e.forge(t, "j_f1", putArgv, "shell", "settings", "delete", "global", "a")
	_, exit, _ := e.undoLast(t)
	if exit.ErrorCode != "PAUSED" || e.ranContains("settings delete") {
		t.Fatalf("%+v ran %q", exit, e.adb.ran())
	}
}

// Another question that took the door between the checks: not a denial.
func TestUndoBusyIsNotADenial(t *testing.T) {
	e := newEnv(t)
	calls := 0
	e.d.o.Paused = func() bool {
		calls++
		if calls == 3 { // the loop's check, with the undo already admitted
			e.d.ask(&question{title: "Other question"})
		}
		return false
	}
	e.forge(t, "j_f1", putArgv, "shell", "settings", "delete", "global", "a")
	_, exit, _ := e.undoLast(t)
	if exit.ErrorCode != "WAITING_FOR_ANSWER" || !strings.Contains(exit.Error, "another question is open") || len(e.conf.asked) != 0 || e.ranContains("settings delete") {
		t.Fatalf("%+v asked %d ran %q", exit, len(e.conf.asked), e.adb.ran())
	}
}

type flightADB struct {
	*fakeADB
	d      *Daemon
	flying []bool
}

func (a *flightADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	a.d.askMu.Lock()
	a.flying = append(a.flying, len(a.d.inFlight) > 0)
	a.d.askMu.Unlock()
	return a.fakeADB.Run(ctx, dir, args, stdin, stdout, stderr)
}

// A 🔴 denied in the middle of an undo does not take the undo out of the
// changes in flight for the entries that follow.
func TestUndoSessionStaysInFlightAfterADeniedInverse(t *testing.T) {
	e := newEnv(t)
	e.conf.decision = confirm.Deny
	fa := &flightADB{fakeADB: e.adb, d: e.d}
	e.d.o.ADB = fa
	e.forge(t, "j_f1", putArgv, "shell", "settings", "put", "global", "b", "1") // 🟡, runs second
	e.forge(t, "j_f2", putArgv, "shell", "settings", "delete", "global", "a")   // 🔴, asked first
	s := &stream{}
	exit := e.d.Handle(context.Background(), ipc.Request{Kind: ipc.KindUndo, Target: "session"}, s)
	var res UndoResult
	json.Unmarshal(s.data[0], &res)
	if exit.Code != 0 || len(res.Undone) != 1 || len(res.Skipped) != 1 || len(fa.flying) != 1 || !fa.flying[0] {
		t.Fatalf("%+v %+v flying %v", exit, res, fa.flying)
	}
	if len(e.d.inFlight) != 0 {
		t.Fatal("left in flight")
	}
}
