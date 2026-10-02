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
	// With --json the skipped changes go in the envelope, and nothing else.
	out, errOut, code := run(t, "undo", "--json")
	var env struct {
		OK    bool
		Error *Error
		Data  *daemon.UndoResult
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != ExitFailure || strings.Count(out, "\n") != 1 || env.OK ||
		env.Error == nil || env.Error.Code != "NOTHING_TO_UNDO" || env.Data == nil || len(env.Data.Skipped) != 1 ||
		env.Data.Skipped[0].ID != "j_3" || strings.Contains(errOut, "skipped  j_3") {
		t.Fatalf("exit %d, stdout %q, stderr %q: %v", code, out, errOut, err)
	}
}

// A change recorded on another phone gets a fix that says where to undo it.
func TestUndoOtherDeviceHasAFix(t *testing.T) {
	data := isolate(t)
	fakeDaemon(t, data, nil, ipc.Exit{Code: 1, ErrorCode: "OTHER_DEVICE", Error: "j_1 was recorded on another phone (devB): undo it by hand on that phone"})
	if _, errOut, code := run(t, "undo", "j_1"); code != ExitFailure || !strings.Contains(errOut, "OTHER_DEVICE") || !strings.Contains(errOut, "Connect the phone") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

// A 🔴 inverse left unconfirmed gets a fix, in the language of the PC; the
// daemon's reason stays as it is.
func TestUndoDeniedHasAFix(t *testing.T) {
	const reason = "1 inverse(s) needed your confirmation and did not get it"
	for _, c := range []struct{ lang, fix string }{
		{"en", "Run adbuddy undo again and allow the 🔴 question"},
		{"es", "Vuelve a ejecutar adbuddy undo y permite la pregunta 🔴"},
	} {
		data := isolate(t)
		t.Setenv("ADBUDDY_LANG", c.lang)
		fakeDaemon(t, data, nil, ipc.Exit{Code: daemon.ExitDenied, ErrorCode: "UNDO_DENIED", Error: reason})
		if _, errOut, code := run(t, "undo"); code != ExitFailure || !strings.Contains(errOut, "UNDO_DENIED") || !strings.Contains(errOut, reason) || !strings.Contains(errOut, c.fix) {
			t.Fatalf("%s: exit %d, stderr %q", c.lang, code, errOut)
		}
	}
}

// T3 re-review 2, minor: the fix for WAITING_FOR_ANSWER fits both cases,
// whether the question is already on screen or still waits to be asked.
func TestUndoWaitingForAnswerFixFitsBothCases(t *testing.T) {
	const want = "A 🔴 command is waiting for confirmation (in the ADBuddy app or the serve terminal); retry when it is answered."
	for _, reason := range []string{`waiting to ask about "Restart the phone"`, `waiting for your answer to "Restart the phone"`} {
		data := isolate(t)
		fakeDaemon(t, data, nil, ipc.Exit{Code: 1, ErrorCode: "WAITING_FOR_ANSWER", Error: reason})
		if _, errOut, code := run(t, "undo"); code != ExitTransient || !strings.Contains(errOut, reason) || !strings.Contains(errOut, want) {
			t.Fatalf("exit %d, stderr %q", code, errOut)
		}
		// Final review, M2: it is transient, like the pause.
		out, _, code := run(t, "undo", "--json")
		var env struct{ Error *Error }
		if err := json.Unmarshal([]byte(out), &env); err != nil || code != ExitTransient || env.Error == nil || !env.Error.Transient {
			t.Fatalf("--json: exit %d, stdout %q: %v", code, out, err)
		}
	}
}

// Final review, M2: undo stopped by the pause shows what it undid before,
// like a partial failure, and PAUSED is transient (75) with a fix.
func TestUndoPausedShowsWhatWasUndone(t *testing.T) {
	const fix = "Resume from the ADBuddy app, then run adbuddy undo again for the rest."
	data := isolate(t)
	res := daemon.UndoResult{
		Undone:  []daemon.UndoItem{{ID: "j_1", Description: "Animations 1.0 → 0.5", UndoID: "j_u1"}},
		Skipped: []daemon.UndoItem{},
		Failed:  []daemon.UndoItem{},
	}
	msg := daemon.PausedMessage + "; undid 1 change(s) before stopping"
	fakeDaemon(t, data, res, ipc.Exit{Code: daemon.ExitDenied, ErrorCode: "PAUSED", Error: msg})
	out, errOut, code := run(t, "undo", "--json")
	var env struct {
		OK    bool
		Error *Error
		Data  *daemon.UndoResult
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != ExitTransient || strings.Count(out, "\n") != 1 || env.OK ||
		env.Error == nil || env.Error.Code != "PAUSED" || !env.Error.Transient || env.Error.Fix != fix ||
		env.Data == nil || len(env.Data.Undone) != 1 || env.Data.Undone[0].ID != "j_1" {
		t.Fatalf("exit %d, stdout %q, stderr %q: %v", code, out, errOut, err)
	}
	out, errOut, code = run(t, "undo")
	if code != ExitTransient || !strings.Contains(out, "undone   j_1") || !strings.Contains(errOut, "PAUSED") ||
		!strings.Contains(errOut, msg) || !strings.Contains(errOut, fix) {
		t.Fatalf("human: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

// Paused before undoing anything: no result, still transient with the fix.
func TestUndoPausedBeforeStarting(t *testing.T) {
	data := isolate(t)
	fakeDaemon(t, data, nil, ipc.Exit{Code: daemon.ExitDenied, ErrorCode: "PAUSED", Error: daemon.PausedMessage})
	out, errOut, code := run(t, "undo", "--json")
	var env struct {
		OK    bool
		Error *Error
		Data  json.RawMessage
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || code != ExitTransient || env.OK || env.Error == nil ||
		env.Error.Code != "PAUSED" || !env.Error.Transient || !strings.Contains(env.Error.Fix, "Resume from the ADBuddy app") ||
		(len(env.Data) != 0 && string(env.Data) != "null") {
		t.Fatalf("exit %d, stdout %q, stderr %q: %v", code, out, errOut, err)
	}
}
