package adb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

var fakeADB string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeadb")
	if err != nil {
		panic(err)
	}
	fakeADB, err = testutil.Build("github.com/hybriqtech/adbuddy/host/internal/testutil/fakeadb", dir, "adb")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func setup(t *testing.T, rules ...testutil.FakeRule) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	script, err := testutil.WriteScript(dir, rules...)
	if err != nil {
		t.Fatal(err)
	}
	logPath = filepath.Join(dir, "log.jsonl")
	t.Setenv("FAKEADB_SCRIPT", script)
	t.Setenv("FAKEADB_LOG", logPath)
	t.Setenv("ANDROID_SERIAL", "someone-else")
	return logPath
}

func TestRunAddsSerialAndServerPort(t *testing.T) {
	logPath := setup(t, testutil.FakeRule{Match: "^shell getprop$", Stdout: "x\n", Stderr: "w", Exit: 3})
	r := Runner{Path: fakeADB, ServerPort: 5038, Serial: "dev1"}
	var out, errb bytes.Buffer
	cwd := t.TempDir()
	code, err := r.Run(context.Background(), cwd, []string{"shell", "getprop"}, nil, &out, &errb)
	if err != nil || code != 3 || out.String() != "x\n" || errb.String() != "w" {
		t.Fatalf("code %d err %v out %q errb %q", code, err, out.String(), errb.String())
	}
	calls, _ := testutil.ReadLog(logPath)
	if len(calls) != 1 || strings.Join(calls[0].Args, " ") != "-s dev1 shell getprop" || calls[0].Port != "5038" {
		t.Fatalf("calls %+v", calls)
	}
	if got, _ := filepath.EvalSymlinks(calls[0].Cwd); got != mustEval(t, cwd) {
		t.Fatalf("cwd %s want %s", calls[0].Cwd, cwd)
	}
}

func mustEval(t *testing.T, p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunMissingBinary(t *testing.T) {
	r := Runner{Path: filepath.Join(t.TempDir(), "nope")}
	if _, err := r.Run(context.Background(), "", []string{"devices"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseDevices(t *testing.T) {
	out := "* daemon started successfully\r\nList of devices attached\r\nadb-X-y._adb-tls-connect._tcp\tdevice\r\n1.2.3.4:5555\toffline\r\nemu\tunauthorized\r\n\r\n"
	ds := ParseDevices(out)
	if len(ds) != 3 || ds[0].Serial != "adb-X-y._adb-tls-connect._tcp" || ds[0].State != "device" || ds[1].State != "offline" {
		t.Fatalf("%+v", ds)
	}
	// mDNS renames a clash to "name (2)": the serial has a space, and the
	// state comes after the tab.
	ds = ParseDevices("List of devices attached\nadb-X-y (2)._adb-tls-connect._tcp\tdevice\n")
	if len(ds) != 1 || ds[0].Serial != "adb-X-y (2)._adb-tls-connect._tcp" || ds[0].State != "device" {
		t.Fatalf("serial with a space: %+v", ds)
	}
}

func TestWaitSerialWithASpace(t *testing.T) {
	setup(t, testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\nadb-X-y (2)._adb-tls-connect._tcp\tdevice\n"})
	r := Runner{Path: fakeADB}
	if s, err := r.WaitSerial(context.Background(), "adb-X-y (2)._adb-tls-connect._tcp", time.Second); err != nil || s != "adb-X-y (2)._adb-tls-connect._tcp" {
		t.Fatalf("%q %v", s, err)
	}
}

func TestWaitSerial(t *testing.T) {
	setup(t, testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\nA\tdevice\nB\toffline\n"})
	r := Runner{Path: fakeADB}
	if s, err := r.WaitSerial(context.Background(), "", time.Second); err != nil || s != "A" {
		t.Fatalf("%q %v", s, err)
	}
	if s, err := r.WaitSerial(context.Background(), "A", time.Second); err != nil || s != "A" {
		t.Fatalf("%q %v", s, err)
	}
	if _, err := r.WaitSerial(context.Background(), "B", 10*time.Millisecond); err != ErrNoDevice {
		t.Fatalf("offline device: %v", err)
	}
	setup(t, testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\nA\tdevice\nC\tdevice\n"})
	if _, err := r.WaitSerial(context.Background(), "", time.Second); err != ErrManyDevices {
		t.Fatalf("two devices: %v", err)
	}
}

func TestShellAndPull(t *testing.T) {
	logPath := setup(t,
		testutil.FakeRule{Match: "^shell settings get global k$", Stdout: "1\n"},
		testutil.FakeRule{Match: "^pull ", Exit: 1, Stderr: "adb: error: remote object does not exist\n"},
	)
	r := Runner{Path: fakeADB, Serial: "d"}
	out, code, err := r.Shell(context.Background(), "settings get global k")
	if err != nil || code != 0 || out != "1\n" {
		t.Fatalf("%q %d %v", out, code, err)
	}
	if err := r.Pull(context.Background(), "/sdcard/x", filepath.Join(t.TempDir(), "x")); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("pull: %v", err)
	}
	calls, _ := testutil.ReadLog(logPath)
	if len(calls) != 2 || calls[0].Joined() != "shell settings get global k" {
		t.Fatalf("%+v", calls)
	}
}

func TestRunIsolatesTheAdbServer(t *testing.T) {
	logPath := setup(t)
	t.Setenv("ANDROID_SERIAL", "someone-else")
	t.Setenv("ANDROID_ADB_SERVER_PORT", "9999")
	t.Setenv("ADB_SERVER_SOCKET", "tcp:localhost:5037")
	t.Setenv("ANDROID_ADB_SERVER_ADDRESS", "192.0.2.9")
	r := Runner{Path: fakeADB, ServerPort: 5038, Serial: "dev1"}
	if _, err := r.Run(context.Background(), "", []string{"devices"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	r = Runner{Path: fakeADB, ServerPort: 0}
	if _, err := r.Run(context.Background(), "", []string{"devices"}, nil, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	calls, _ := testutil.ReadLog(logPath)
	if len(calls) != 2 {
		t.Fatalf("calls %+v", calls)
	}
	if c := calls[0]; c.Port != "5038" || c.Serial != "" || c.Socket != "" || c.Address != "" {
		t.Fatalf("first call leaks the environment: %+v", c)
	}
	if c := calls[1]; c.Port != "" || c.Serial != "" || c.Socket != "" || c.Address != "" {
		t.Fatalf("port 0 call leaks the environment: %+v", c)
	}
}

// A caller's stdin that never reaches EOF (a pipe nobody closes) must not
// keep Run waiting once adb has exited.
func TestRunReturnsWhenStdinNeverCloses(t *testing.T) {
	setup(t, testutil.FakeRule{Match: "^devices$", Stdout: "List of devices attached\n"})
	pr, pw := io.Pipe()
	t.Cleanup(func() { pw.Close() })
	r := Runner{Path: fakeADB, ServerPort: 5038}
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	var out bytes.Buffer
	go func() {
		code, err := r.Run(context.Background(), "", []string{"devices"}, pr, &out, io.Discard)
		done <- result{code, err}
	}()
	select {
	case res := <-done:
		if res.code != 0 || res.err != nil || !strings.Contains(out.String(), "List of devices") {
			t.Fatalf("code %d err %v out %q", res.code, res.err, out.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run still waiting for stdin 2 s after adb exited")
	}
}

func TestRunForwardsStdin(t *testing.T) {
	setup(t, testutil.FakeRule{Match: "^shell cat$", EchoStdin: true, Exit: 4})
	r := Runner{Path: fakeADB, ServerPort: 5038, Serial: "dev1"}
	var out bytes.Buffer
	code, err := r.Run(context.Background(), "", []string{"shell", "cat"}, strings.NewReader("hello\n"), &out, io.Discard)
	if code != 4 || err != nil || out.String() != "hello\n" {
		t.Fatalf("code %d err %v out %q", code, err, out.String())
	}
}
