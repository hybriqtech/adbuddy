package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

// With a partial result, --json prints one envelope with the error and the
// data, and nothing else.
func TestUndoJSONPartialResult(t *testing.T) {
	data := isolate(t)
	res := daemon.UndoResult{
		Undone:  []daemon.UndoItem{{ID: "j_1", Description: "Animations 1.0 → 0.5", UndoID: "j_u1"}},
		Skipped: []daemon.UndoItem{},
		Failed:  []daemon.UndoItem{{ID: "j_2", Description: "Disable x", Error: "exit 1"}},
	}
	fakeDaemon(t, data, res, ipc.Exit{Code: 1, ErrorCode: "UNDO_FAILED", Error: "1 of 2 inverses failed"})
	out, errOut, code := run(t, "undo", "--json")
	if code != ExitFailure || strings.Count(out, "\n") != 1 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	var env struct {
		OK    bool
		Error *Error
		Data  *daemon.UndoResult
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.OK || env.Error == nil || env.Error.Code != "UNDO_FAILED" ||
		env.Data == nil || len(env.Data.Undone) != 1 || env.Data.Undone[0].ID != "j_1" || len(env.Data.Failed) != 1 {
		t.Fatalf("stdout %q: %v", out, err)
	}
	// Human mode is unchanged.
	out, errOut, code = run(t, "undo")
	if code != ExitFailure || !strings.Contains(out, "undone   j_1") || !strings.Contains(out, "failed   j_2") || !strings.Contains(errOut, "UNDO_FAILED") {
		t.Fatalf("human: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestUndoErrorWithoutCode(t *testing.T) {
	data := isolate(t)
	fakeDaemon(t, data, nil, ipc.Exit{Code: 1, Error: "unauthorized"})
	if _, errOut, code := run(t, "undo"); code != ExitFailure || !strings.Contains(errOut, "DAEMON_ERROR — unauthorized") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

// NOTHING_TO_UNDO still tells why the last changes were skipped, escaped.
func TestUndoNothingToUndoShowsSkipped(t *testing.T) {
	data := isolate(t)
	res := daemon.UndoResult{Undone: []daemon.UndoItem{}, Failed: []daemon.UndoItem{},
		Skipped: []daemon.UndoItem{{ID: "j_3", Description: "Disable x", Error: "uncertain\x1b[2K"}}}
	fakeDaemon(t, data, res, ipc.Exit{Code: 1, ErrorCode: "NOTHING_TO_UNDO", Error: "nothing to undo"})
	_, errOut, code := run(t, "undo")
	skipped, failure := strings.Index(errOut, "skipped  j_3"), strings.Index(errOut, "NOTHING_TO_UNDO")
	if code != ExitFailure || skipped < 0 || failure < skipped || strings.Contains(errOut, "\x1b") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}
