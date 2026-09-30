package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

// Final review, M1: without --workspace, `journal` and `run` use the
// environment of the `serve` that is running, not the default one.

// serveIn fakes a running serve whose environment is ws, and moves the home
// folder (and so the default environment) to a temporary one.
func serveIn(t *testing.T, ws string) (data string) {
	t.Helper()
	data = isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	fakeDaemon(t, data, ipc.Ping{Version: Version, Session: "s_1", Serial: "dev1", Journal: workspace.JournalDir(ws)}, ipc.Exit{})
	return data
}

func TestJournalUsesTheEnvironmentOfServe(t *testing.T) {
	ws := t.TempDir()
	data := serveIn(t, ws)
	secret, err := ipc.ReadSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	appendEntries(t, ws, []byte(secret), journal.Entry{ID: "j_serve", Session: "s_1", Level: "green", Status: journal.StatusOK})
	out, errOut, code := run(t, "journal", "--json")
	var env struct {
		Data struct {
			Entries []journalEntry
		}
	}
	if code != 0 || json.Unmarshal([]byte(out), &env) != nil || len(env.Data.Entries) != 1 || env.Data.Entries[0].ID != "j_serve" || !env.Data.Entries[0].Verified {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	// An explicit --workspace still wins.
	if out, _, code := run(t, "journal", "--json", "--workspace", t.TempDir()); code != 0 || strings.Contains(out, "j_serve") {
		t.Fatalf("--workspace: %d %q", code, out)
	}
}

func TestRunUsesTheEnvironmentOfServe(t *testing.T) {
	ws := t.TempDir()
	serveIn(t, ws)
	t.Setenv("PATH", t.TempDir()) // no agent can be found: run stops after preparing the environment
	_, errOut, code := run(t, "run", "adbuddy-test-no-such-agent")
	if code != ExitFailure || !strings.Contains(errOut, "AGENT_NOT_FOUND") || !strings.Contains(errOut, " in "+ws+" ") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(ws, "AGENTS.md")); err != nil {
		t.Fatalf("serve's environment not prepared: %v", err)
	}
}

func TestRunRefusesAnotherEnvironment(t *testing.T) {
	ws := t.TempDir()
	serveIn(t, ws)
	t.Setenv("PATH", t.TempDir())
	other := t.TempDir()
	_, errOut, code := run(t, "run", "adbuddy-test-no-such-agent", "--workspace", other)
	if code != ExitFailure || !strings.Contains(errOut, "WORKSPACE_MISMATCH") || !strings.Contains(errOut, ws) {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(other, "AGENTS.md")); err == nil {
		t.Fatal("prepared an environment serve does not watch")
	}
	// The same environment, written another way, is fine.
	same := ws + string(filepath.Separator)
	if _, errOut, _ := run(t, "run", "adbuddy-test-no-such-agent", "--workspace", same); strings.Contains(errOut, "WORKSPACE_MISMATCH") {
		t.Fatalf("same environment refused: %q", errOut)
	}
}

// Final review of M1b, I1: the agent names the phone by its identity, the
// name the journal records, not by the serial adb lists it with this session.
func TestRunExportsTheIdentity(t *testing.T) {
	ws := t.TempDir()
	data := isolate(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	fakeDaemon(t, data, ipc.Ping{Version: Version, Session: "s_1", Serial: "192.0.2.10:41000", Identity: "adb-AAA-bbb._adb-tls-connect._tcp", Journal: workspace.JournalDir(ws)}, ipc.Exit{})
	log := filepath.Join(t.TempDir(), "agent.log")
	t.Setenv("FAKEADB_SCRIPT", "")
	t.Setenv("FAKEADB_LOG", log)
	// The fake adb stands in for the agent: it logs its ANDROID_SERIAL.
	if _, errOut, code := run(t, "run", fakeADB(t)); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	calls, err := testutil.ReadLog(log)
	if err != nil || len(calls) != 1 || calls[0].Serial != "adb-AAA-bbb._adb-tls-connect._tcp" {
		t.Fatalf("agent calls %+v (%v)", calls, err)
	}
}
