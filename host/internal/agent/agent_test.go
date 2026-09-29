package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnv(t *testing.T) {
	sep := string(os.PathListSeparator)
	in := []string{
		"PATH=/usr/bin", "HOME=/home/u", "ANTHROPIC_API_KEY=k", "CLAUDECODE=1", "CLAUDE_CODE_SESSION_ID=x",
		"CLAUDE_EFFORT=xhigh", "CLAUDE_PID=9", "CLAUDECODEX=1", "claudecode_entrypoint=cli", "AI_AGENT=claude-code", "ANDROID_SERIAL=old", "ANDROID_ADB_SERVER_PORT=5037", "CLAUDE_CONFIG_DIR=/c", "ADBUDDY_SOURCE=user",
	}
	got := strings.Join(Env(in, "/ws/.adbuddy/bin", "dev1"), "\n")
	for _, want := range []string{"PATH=/ws/.adbuddy/bin" + sep + "/usr/bin", "HOME=/home/u", "ANTHROPIC_API_KEY=k", "CLAUDE_CONFIG_DIR=/c", "ANDROID_SERIAL=dev1", "ADBUDDY_SOURCE=agent"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	for _, bad := range []string{"CLAUDECODE", "claudecode", "AI_AGENT", "CLAUDE_CODE_SESSION_ID", "CLAUDE_EFFORT", "CLAUDE_PID", "ANDROID_SERIAL=old", "5037", "ADBUDDY_SOURCE=user"} {
		if strings.Contains(got, bad) {
			t.Errorf("leaked %s", bad)
		}
	}
}

func TestEnvWindowsPathSpelling(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	got := Env([]string{`Path=C:\Windows`}, `C:\ws\bin`, "d")
	if got[0] != `Path=C:\ws\bin;C:\Windows` {
		t.Fatalf("%q", got)
	}
}

func TestRunReturnsExitCodeAndUsesEnvironment(t *testing.T) {
	// Build a tiny agent that checks where adb resolves and exits with 3.
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	os.WriteFile(src, []byte(`package main
import ("fmt";"os";"os/exec")
func main() {
	if os.Getenv("FAKEAGENT_KILL") == "1" {
		p, _ := os.FindProcess(os.Getpid())
		p.Kill()
	}
	p, _ := exec.LookPath("adb")
	wd, _ := os.Getwd()
	fmt.Fprintf(os.Stdout, "%s|%s|%s", p, os.Getenv("ANDROID_SERIAL"), wd)
	os.Exit(3)
}`), 0o600)
	name := "fakeagent"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	if out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	ws := t.TempDir()
	binDir := filepath.Join(ws, ".adbuddy", "bin")
	os.MkdirAll(binDir, 0o700)
	shim := filepath.Join(binDir, "adb")
	if runtime.GOOS == "windows" {
		shim += ".exe"
	}
	os.WriteFile(shim, []byte("x"), 0o700)

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	outFile, _ := os.Create(filepath.Join(t.TempDir(), "out"))
	code, err := Run(context.Background(), Options{Agent: "fakeagent", Dir: ws, BinDir: binDir, Serial: "dev1", Environ: os.Environ(), Stdout: outFile, Stderr: outFile})
	outFile.Close()
	if err != nil || code != 3 {
		t.Fatalf("code %d err %v", code, err)
	}
	b, _ := os.ReadFile(outFile.Name())
	parts := strings.Split(string(b), "|")
	if len(parts) != 3 || !strings.EqualFold(parts[0], shim) || parts[1] != "dev1" {
		t.Fatalf("agent saw %q", b)
	}
	if got, _ := filepath.EvalSymlinks(parts[2]); !strings.EqualFold(got, mustEval(t, ws)) {
		t.Fatalf("cwd %s want %s", parts[2], ws)
	}
	if _, err := Run(context.Background(), Options{Agent: "no-such-agent-xyz", BinDir: binDir, Environ: os.Environ()}); err != ErrNotFound {
		t.Fatalf("missing agent: %v", err)
	}
	// Killed by a signal: ExitCode() is -1 on Unix, which must not become 255.
	t.Run("killed by a signal", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("on Windows Kill ends the process with exit code 1, not -1: the signal path is only exercised on Unix")
		}
		code, err := Run(context.Background(), Options{Agent: "fakeagent", Dir: ws, BinDir: binDir, Serial: "dev1", Environ: append(os.Environ(), "FAKEAGENT_KILL=1")})
		if err != nil || code != 1 {
			t.Fatalf("killed agent: code %d err %v", code, err)
		}
	})
	// Without an absolute bin folder the current directory would come first in PATH.
	for _, bad := range []string{"", "rel/bin"} {
		if _, err := Run(context.Background(), Options{Agent: "fakeagent", Dir: ws, BinDir: bad, Serial: "dev1", Environ: os.Environ()}); err == nil || err == ErrNotFound {
			t.Fatalf("BinDir %q: %v", bad, err)
		}
	}
}

func mustEval(t *testing.T, p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
