package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestServeRefusesTheAgentSession(t *testing.T) {
	t.Setenv("ADBUDDY_SOURCE", "agent")
	_, errOut, code := run(t, "serve", "--adb", "adb", "--confirm", "terminal")
	if code != ExitFailure || !strings.Contains(errOut, "AGENT_SESSION") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestServeNeedsAnAdb(t *testing.T) {
	t.Setenv("ADBUDDY_SOURCE", "")
	t.Setenv("ADBUDDY_CONFIG_DIR", t.TempDir())
	t.Setenv("ADBUDDY_DATA_DIR", t.TempDir())
	_, errOut, code := run(t, "serve")
	if code != ExitConfig || !strings.Contains(errOut, "ADB_NOT_CONFIGURED") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestUndoAndRunWithoutHost(t *testing.T) {
	t.Setenv("ADBUDDY_CONFIG_DIR", t.TempDir())
	t.Setenv("ADBUDDY_DATA_DIR", t.TempDir())
	for _, args := range [][]string{{"undo"}, {"run", "claude"}} {
		_, errOut, code := run(t, args...)
		if code != ExitFailure || !strings.Contains(errOut, "DAEMON_NOT_RUNNING") || !strings.Contains(errOut, "adbuddy serve") {
			t.Fatalf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
}

func TestJournalEmptyJSON(t *testing.T) {
	t.Setenv("ADBUDDY_CONFIG_DIR", t.TempDir())
	t.Setenv("ADBUDDY_DATA_DIR", t.TempDir())
	out, _, code := run(t, "journal", "--json", "--workspace", t.TempDir())
	if code != 0 || strings.TrimSpace(out) != `{"ok":true,"data":{"entries":[],"skippedLines":0}}` {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
}

func TestUndoneLogPath(t *testing.T) {
	data := t.TempDir()
	a := undoneLogPath(data, filepath.Join(t.TempDir(), "journal"))
	b := undoneLogPath(data, filepath.Join(t.TempDir(), "journal"))
	if filepath.Dir(a) != data || !strings.HasPrefix(filepath.Base(a), "undone-") || !strings.HasSuffix(a, ".log") || len(filepath.Base(a)) != len("undone-")+16+len(".log") {
		t.Fatalf("undone log %q", a)
	}
	if a == b {
		t.Fatalf("two journals share %q", a)
	}
}
