package shim

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
)

func TestIsShim(t *testing.T) {
	for name, want := range map[string]bool{
		"adb": true, "/x/.adbuddy/bin/adb": true, `C:\x\.adbuddy\bin\adb.exe`: true, "ADB.EXE": true,
		"adbuddy": false, "adbuddy.exe": false, "adbd": false,
	} {
		if got := IsShim(name); got != want {
			t.Errorf("IsShim(%q) = %v", name, got)
		}
	}
}

func shortDir(t *testing.T) string {
	d, err := os.MkdirTemp("", "abx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func getenv(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestNotRunning(t *testing.T) {
	var errb bytes.Buffer
	code := Main([]string{"devices"}, strings.NewReader(""), io.Discard, &errb, getenv(map[string]string{"ADBUDDY_DATA_DIR": shortDir(t), "ADBUDDY_CONFIG_DIR": shortDir(t)}))
	if code != 1 || strings.TrimSpace(errb.String()) != NotRunningMessage {
		t.Fatalf("code %d stderr %q", code, errb.String())
	}
}

func TestForwardsRequest(t *testing.T) {
	data := shortDir(t)
	secret, _ := ipc.EnsureSecret(data)
	var got ipc.Request
	srv, err := ipc.Listen(data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		got = req
		in, _ := io.ReadAll(c.Stdin())
		c.Stdout().Write(in)
		io.WriteString(c.Stderr(), "err")
		return ipc.Exit{Code: 126}
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	defer srv.Close()

	var out, errb bytes.Buffer
	env := map[string]string{"ADBUDDY_DATA_DIR": data, "ADBUDDY_CONFIG_DIR": data, "ANDROID_SERIAL": "dev1", "ADBUDDY_SOURCE": "agent", "HOME": "x"}
	code := Main([]string{"shell", "cat"}, strings.NewReader("piped"), &out, &errb, getenv(env))
	if code != 126 || out.String() != "piped" || errb.String() != "err" {
		t.Fatalf("code %d out %q err %q", code, out.String(), errb.String())
	}
	cwd, _ := os.Getwd()
	if strings.Join(got.Argv, " ") != "shell cat" || got.Cwd != cwd || got.Env["ANDROID_SERIAL"] != "dev1" || got.Env["ADBUDDY_SOURCE"] != "agent" || len(got.Env) != 2 {
		t.Fatalf("request %+v", got)
	}
}
