package daemon

import (
	"context"
	"encoding/json"
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
	code := 0
	j := journal.Entry{
		ID: id, TS: e.d.o.Now().UTC(), Session: "s_1", Serial: "dev1", Source: journal.SourceAgent,
		Argv: argv, Level: "yellow", Rule: "r", Description: "forged " + id, Status: journal.StatusOK, ExitCode: &code,
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
