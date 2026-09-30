package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
)

var adbuddyBin, fakeADB string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "e2e")
	if err != nil {
		panic(err)
	}
	adbuddyBin, err = testutil.Build("github.com/hybriqtech/adbuddy/host/cmd/adbuddy", dir, "adbuddy")
	if err == nil {
		fakeADB, err = testutil.Build("github.com/hybriqtech/adbuddy/host/internal/testutil/fakeadb", dir, "fakeadb")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
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

type harness struct {
	t        *testing.T
	env      []string
	data     string
	ws       string
	logPath  string
	serveIn  io.WriteCloser
	serveErr *syncBuffer
	serveOut *syncBuffer
}

func start(t *testing.T) *harness {
	t.Helper()
	return startWith(t, "")
}

// startWith starts serve with extra fake adb rules first and, when config is
// not empty, that config.toml.
func startWith(t *testing.T, config string, extra ...testutil.FakeRule) *harness {
	t.Helper()
	data, err := os.MkdirTemp("", "e2d")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(data) })
	tmp := t.TempDir()
	h := &harness{t: t, data: data, ws: filepath.Join(tmp, "ws"), logPath: filepath.Join(tmp, "adb.log"), serveErr: &syncBuffer{}, serveOut: &syncBuffer{}}
	rules := append(extra,
		testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\ndev1\tdevice\n"},
		testutil.FakeRule{Match: "^shell getprop ro.product.model$", Stdout: "Pixel 9 Pro XL\n"},
		testutil.FakeRule{Match: "^shell settings get global window_animation_scale$", Stdout: "1.0\n"},
		testutil.FakeRule{Match: "^shell cat$", EchoStdin: true},
	)
	script, err := testutil.WriteScript(tmp, rules...)
	if err != nil {
		t.Fatal(err)
	}
	if config != "" {
		os.MkdirAll(filepath.Join(tmp, "cfg"), 0o700)
		if err := os.WriteFile(filepath.Join(tmp, "cfg", "config.toml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Run from inside an agent session, the inherited ADBUDDY_SOURCE=agent
	// would make serve refuse to start and ANDROID_SERIAL would leak in.
	var inherited []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if !strings.EqualFold(k, "ADBUDDY_SOURCE") && !strings.EqualFold(k, "ANDROID_SERIAL") {
			inherited = append(inherited, kv)
		}
	}
	h.env = append(inherited, "ADBUDDY_DATA_DIR="+data, "ADBUDDY_CONFIG_DIR="+filepath.Join(tmp, "cfg"),
		"FAKEADB_SCRIPT="+script, "FAKEADB_LOG="+h.logPath, "LANG=en_US.UTF-8")

	cmd := exec.Command(adbuddyBin, "serve", "--adb", fakeADB, "--serial", "dev1", "--workspace", h.ws, "--confirm", "terminal")
	cmd.Env = h.env
	cmd.Stdout, cmd.Stderr = h.serveOut, h.serveErr
	if h.serveIn, err = cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := ipc.PingDaemon(context.Background(), data); err == nil {
			return h
		}
		if time.Now().After(deadline) {
			t.Fatalf("serve did not start:\n%s", h.serveErr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// adb runs the workspace shim.
func (h *harness) adb(stdin string, args ...string) (stdout, stderr string, code int) {
	h.t.Helper()
	return h.adbIn("", stdin, args...)
}

// adbIn runs the workspace shim from the working directory dir ("": the test's).
func (h *harness) adbIn(dir, stdin string, args ...string) (stdout, stderr string, code int) {
	h.t.Helper()
	cmd := exec.Command(filepath.Join(workspace.BinDir(h.ws), testutil.Exe("adb")), args...)
	cmd.Dir = dir
	cmd.Env = append(h.env, "ADBUDDY_SOURCE=agent")
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code = 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		h.t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func (h *harness) adbuddy(args ...string) (stdout, stderr string, code int) {
	h.t.Helper()
	cmd := exec.Command(adbuddyBin, args...)
	cmd.Env = h.env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		h.t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func (h *harness) adbCalls() []string {
	calls, err := testutil.ReadLog(h.logPath)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []string
	for _, c := range calls {
		if c.Port != "5038" {
			h.t.Fatalf("adb called without ADBuddy's server port: %+v", c)
		}
		out = append(out, c.Joined())
	}
	return out
}

func TestEndToEnd(t *testing.T) {
	h := start(t)

	// 🟢 runs and streams output.
	out, errOut, code := h.adb("", "shell", "getprop", "ro.product.model")
	if code != 0 || out != "Pixel 9 Pro XL\n" {
		t.Fatalf("green: %d %q %q", code, out, errOut)
	}
	// stdin reaches the real adb.
	if out, _, code := h.adb("from stdin", "shell", "cat"); code != 0 || out != "from stdin" {
		t.Fatalf("stdin: %d %q", code, out)
	}
	// 🟡 runs after its precheck and gets an inverse.
	if _, errOut, code := h.adb("", "shell", "settings", "put", "global", "window_animation_scale", "0.5"); code != 0 {
		t.Fatalf("yellow: %d %q", code, errOut)
	}
	// 🔴 waits for the terminal running serve; answering "n" denies it.
	done := make(chan struct{})
	var redOut string
	var redCode int
	go func() {
		_, redOut, redCode = h.adb("", "shell", "rm", "-rf", "/sdcard/DCIM")
		close(done)
	}()
	waitFor(t, func() bool { return strings.Contains(h.serveErr.String(), "Code: ") }, h.serveErr)
	time.Sleep(1200 * time.Millisecond) // answers in the first second are ignored as typed ahead
	io.WriteString(h.serveIn, "n\n")
	<-done
	if redCode != 126 || !strings.Contains(redOut, "Denied by user: Delete files: -rf /sdcard/DCIM") {
		t.Fatalf("red: %d %q", redCode, redOut)
	}

	calls := h.adbCalls()
	want := []string{"devices", "shell getprop service.adb.tcp.port", "shell dumpsys package tech.hybriq.adbuddy", "shell getprop ro.product.model", "shell cat", "shell settings get global window_animation_scale", "shell settings put global window_animation_scale 0.5"}
	if strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Fatalf("adb calls:\n%q\nwant\n%q", calls, want)
	}

	// The journal shows everything, with the inverse of the change.
	out, _, code = h.adbuddy("journal", "--json", "--workspace", h.ws)
	var env struct {
		OK   bool
		Data struct {
			Entries []struct {
				ID, Level, Status, Description string
				Inverse                        *struct{ Argv []string }
				Verified                       bool
			}
		}
	}
	if code != 0 || json.Unmarshal([]byte(out), &env) != nil || !env.OK || len(env.Data.Entries) != 4 {
		t.Fatalf("journal: %d %q", code, out)
	}
	// Every entry was written and signed by serve.
	for _, e := range env.Data.Entries {
		if !e.Verified {
			t.Fatalf("entry %s not verified: %q", e.ID, out)
		}
	}
	yellow := env.Data.Entries[2]
	if yellow.Level != "yellow" || strings.Join(yellow.Inverse.Argv, " ") != "shell settings put global window_animation_scale 1.0" {
		t.Fatalf("yellow entry %+v", yellow)
	}
	if red := env.Data.Entries[3]; red.Level != "red" || red.Status != "denied" {
		t.Fatalf("red entry %+v", red)
	}

	// Undo runs the inverse.
	out, errOut, code = h.adbuddy("undo")
	if code != 0 || !strings.Contains(out, "undone") {
		t.Fatalf("undo: %d %q %q", code, out, errOut)
	}
	calls = h.adbCalls()
	if last := calls[len(calls)-1]; last != "shell settings put global window_animation_scale 1.0" {
		t.Fatalf("undo ran %q", last)
	}
	if _, errOut, code := h.adbuddy("undo"); code != 1 || !strings.Contains(errOut, "NOTHING_TO_UNDO") {
		t.Fatalf("second undo: %d %q", code, errOut)
	}
	// The live view in serve's stdout shows the entries.
	if !strings.Contains(h.serveOut.String(), "Delete files: -rf /sdcard/DCIM") {
		t.Fatalf("live view: %q", h.serveOut.String())
	}
}

// Final review, I1: serve tells the daemon where the environment is, and
// pull only writes there without asking.
func TestPullOnlyInsideTheEnvironment(t *testing.T) {
	h := start(t)
	h.adbIn(h.ws, "", "pull", "/sdcard/a.jpg")
	if calls := h.adbCalls(); calls[len(calls)-1] != "pull /sdcard/a.jpg" {
		t.Fatalf("pull inside the environment did not run: %q\n%s", calls, h.serveErr.String())
	}
	done := make(chan struct{})
	var out string
	var code int
	go func() {
		_, out, code = h.adbIn(t.TempDir(), "", "pull", "/sdcard/b.jpg")
		close(done)
	}()
	waitFor(t, func() bool { return strings.Contains(h.serveErr.String(), "Code: ") }, h.serveErr)
	if !strings.Contains(h.serveErr.String(), "pull writes outside the agent environment") {
		t.Fatalf("confirmation without the reason:\n%s", h.serveErr.String())
	}
	time.Sleep(1200 * time.Millisecond) // answers in the first second are ignored as typed ahead
	io.WriteString(h.serveIn, "n\n")
	<-done
	if code != 126 || !strings.Contains(out, "Denied by user") {
		t.Fatalf("pull outside: %d %q", code, out)
	}
	if calls := h.adbCalls(); calls[len(calls)-1] != "pull /sdcard/a.jpg" {
		t.Fatalf("pull outside ran: %q", calls)
	}
}

func TestShimWithoutHost(t *testing.T) {
	data, _ := os.MkdirTemp("", "e2n")
	defer os.RemoveAll(data)
	dir := t.TempDir()
	shim := filepath.Join(dir, testutil.Exe("adb"))
	b, _ := os.ReadFile(adbuddyBin)
	os.WriteFile(shim, b, 0o700)
	cmd := exec.Command(shim, "devices")
	cmd.Env = append(os.Environ(), "ADBUDDY_DATA_DIR="+data, "ADBUDDY_CONFIG_DIR="+data)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 1 || strings.TrimSpace(errb.String()) != "ADBuddy host not running (run: adbuddy serve)" {
		t.Fatalf("%v %q", err, errb.String())
	}
}

func waitFor(t *testing.T, cond func() bool, log *syncBuffer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out; serve stderr:\n%s", log.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}
