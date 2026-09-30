package cli

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/paths"
)

// appPhone is a phone with the app installed, in the process: the reverse
// is dropped after the first check, as adb does when the phone reconnects.
type appPhone struct {
	mu      sync.Mutex
	calls   []string
	nonce   string
	reverse bool
}

func (f *appPhone) Run(_ context.Context, _ string, args []string, stdin io.Reader, stdout, _ io.Writer) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(args, " ")
	f.calls = append(f.calls, joined)
	switch {
	case strings.Contains(joined, "dumpsys package"):
		io.WriteString(stdout, "Packages:\n    versionName=0.2.0\n")
	case strings.HasPrefix(joined, "reverse tcp:"):
		f.reverse = true
	case joined == "reverse --list":
		if f.reverse {
			f.reverse = false // lost at once: the next check finds it gone
			io.WriteString(stdout, "dev1 "+strings.TrimPrefix(f.lastReverse(), "reverse ")+"\n")
		}
	case strings.Contains(joined, "content write"):
		var hs struct{ Nonce string }
		b, _ := io.ReadAll(stdin)
		json.Unmarshal(b, &hs)
		f.nonce = hs.Nonce
	case strings.Contains(joined, "--method ping"):
		port := strings.TrimPrefix(strings.Fields(f.lastReverse())[1], "tcp:")
		io.WriteString(stdout, "Result: Bundle[{ok=true, port="+port+", nonce="+f.nonce+"}]\n")
	}
	return 0, nil
}

func (f *appPhone) lastReverse() string {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if strings.HasPrefix(f.calls[i], "reverse tcp:") {
			return f.calls[i]
		}
	}
	return ""
}

func (f *appPhone) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// The chat port is chosen on first use, between 20000 and 59999, and saved:
// the next serve uses the same one.
func TestChatPortIsChosenOnceAndSaved(t *testing.T) {
	dir := t.TempDir()
	e := &env{dirs: paths.Dirs{Config: dir, Data: dir}, cfg: config.Default()}
	port, err := chatPort(e)
	if err != nil || port < 20000 || port > 59999 {
		t.Fatalf("%d %v", port, err)
	}
	saved, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil || saved.ChatPort != port {
		t.Fatalf("saved %+v %v", saved, err)
	}
	again, err := chatPort(&env{dirs: e.dirs, cfg: saved})
	if err != nil || again != port {
		t.Fatalf("second time %d %v", again, err)
	}
}

// serve checks the tunnel every tunnelCheck and, when adb lost it, opens it
// again and hands the token over again.
func TestTheTunnelIsReopened(t *testing.T) {
	setVar(t, &tunnelCheck, 50*time.Millisecond)
	dir := t.TempDir()
	e := &env{dirs: paths.Dirs{Config: dir, Data: dir}, cfg: config.Default()}
	e.cfg.ChatPort = freeTCPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &appPhone{}
	var errb syncBuffer
	app := &App{Stderr: &errb, Stdout: io.Discard, Getenv: func(string) string { return "" }}
	if _, err := startPhoneLink(ctx, app, e, f, "s_1", "claude", confirm.DenyAll{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.count("reverse tcp:") < 2 || f.count("shell content write") < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("not reopened: %q\n%s", f.calls, errb.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Contains(errb.String(), "Could not") {
		t.Fatalf("stderr %q", errb.String())
	}
	want := "reverse tcp:" + strconv.Itoa(e.cfg.ChatPort) + " tcp:" + strconv.Itoa(e.cfg.ChatPort)
	if f.count(want) < 2 {
		t.Fatalf("calls %q", f.calls)
	}
}

// freeTCPPort returns a loopback port that was free a moment ago.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
