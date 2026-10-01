package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

var (
	fakeOnce sync.Once
	fakeDir  string
	fakeBin  string
	fakeErr  error
)

func TestMain(m *testing.M) {
	// serve starts the agent: never a real one from the tests.
	lookAgent = func(string) (string, error) { return "", exec.ErrNotFound }
	// Nor Claude Code's real configuration: trusting a folder writes it.
	claudeDir, err := os.MkdirTemp("", "cliclaude")
	if err != nil {
		panic(err)
	}
	os.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	code := m.Run()
	os.RemoveAll(claudeDir)
	if fakeDir != "" {
		os.RemoveAll(fakeDir)
	}
	os.Exit(code)
}

// fakeADB builds the fake adb once for the package's tests.
func fakeADB(t *testing.T) string {
	t.Helper()
	fakeOnce.Do(func() {
		if fakeDir, fakeErr = os.MkdirTemp("", "clifake"); fakeErr == nil {
			fakeBin, fakeErr = testutil.Build("github.com/hybriqtech/adbuddy/host/internal/testutil/fakeadb", fakeDir, "adb")
		}
	})
	if fakeErr != nil {
		t.Fatal(fakeErr)
	}
	return fakeBin
}

// shortDir is a data folder short enough for the socket path.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "abc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

// isolate points the data and config folders at fresh temporary ones and
// returns the data folder.
func isolate(t *testing.T) string {
	t.Helper()
	data := shortDir(t)
	t.Setenv("ADBUDDY_DATA_DIR", data)
	t.Setenv("ADBUDDY_CONFIG_DIR", t.TempDir())
	t.Setenv("ADBUDDY_SOURCE", "")
	t.Setenv("ANDROID_SERIAL", "")
	return data
}

// appendEntries writes entries to ws's journal, signed when key is not nil.
func appendEntries(t *testing.T, ws string, key []byte, es ...journal.Entry) {
	t.Helper()
	store := journal.Open(workspace.JournalDir(ws))
	if key != nil {
		store = journal.OpenSigned(workspace.JournalDir(ws), key)
	}
	for _, e := range es {
		if e.TS.IsZero() {
			e.TS = time.Now().UTC()
		}
		if err := store.Append(e); err != nil {
			t.Fatal(err)
		}
	}
}

func runCtx(t *testing.T, ctx context.Context, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = mainContext(ctx, args, strings.NewReader(""), &out, &errb)
	return out.String(), errb.String(), code
}

// fakeDaemon answers every request with data (if not nil) and exit.
func fakeDaemon(t *testing.T, data string, payload any, exit ipc.Exit) {
	t.Helper()
	secret, err := ipc.EnsureSecret(data)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		if payload != nil {
			_ = c.Data(payload)
		}
		return exit
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
