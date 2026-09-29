// Package shim is what runs when the adbuddy binary is invoked as `adb`
// (spec §4.1): a thin client that forwards argv, cwd, stdin and the relevant
// environment to the daemon and streams back stdout, stderr and the exit code.
package shim

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/paths"
	"github.com/mattn/go-isatty"
)

// NotRunningMessage is printed when no daemon answers.
const NotRunningMessage = "ADBuddy host not running (run: adbuddy serve)"

// IsShim reports whether the binary was invoked as adb.
func IsShim(argv0 string) bool {
	name := strings.ToLower(filepath.Base(argv0))
	return name == "adb" || name == "adb.exe"
}

// forwardedEnv are the variables the daemon needs from the caller.
var forwardedEnv = []string{"ANDROID_SERIAL", "ADBUDDY_SOURCE"}

// Main runs the shim and returns its exit code.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	dirs, err := paths.Default(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "adbuddy:", err)
		return 1
	}
	cwd, _ := os.Getwd()
	env := map[string]string{}
	for _, k := range forwardedEnv {
		if v := getenv(k); v != "" {
			env[k] = v
		}
	}
	// A terminal on stdin means nobody is piping data: do not steal keystrokes.
	if f, ok := stdin.(*os.File); ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())) {
		stdin = nil
	}
	exit, err := ipc.Call(context.Background(), dirs.Data, ipc.Request{Kind: ipc.KindExec, Argv: args, Cwd: cwd, Env: env}, stdin, stdout, stderr, nil)
	switch {
	case errors.Is(err, ipc.ErrNotRunning):
		fmt.Fprintln(stderr, NotRunningMessage)
		return 1
	case err != nil:
		fmt.Fprintln(stderr, "adbuddy:", err)
		return 1
	}
	if exit.Error != "" {
		fmt.Fprintln(stderr, "adbuddy:", exit.Error)
	}
	return exit.Code
}
