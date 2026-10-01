// Package agent launches the coding agent inside the ADBuddy environment.
// M1 runs it with the terminal's own stdio (no PTY; that arrives in M3).
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
)

// Known lists the agents ADBuddy knows, recommended first (spec §3.1 step 9).
var Known = []string{"claude", "codex", "gemini", "opencode"}

// IsKnown reports whether name is one of Known, by its bare name: what the
// session may start when a request (adbuddy new, the phone's session.new)
// names an agent. A path, or a program ADBuddy does not know, is refused.
func IsKnown(name string) bool {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return false
	}
	for _, k := range Known {
		if name == k {
			return true
		}
	}
	return false
}

// InstallHint tells how to install a known agent.
func InstallHint(name string) string {
	switch name {
	case "claude":
		return "Install Claude Code: npm install -g @anthropic-ai/claude-code (https://docs.claude.com/en/docs/claude-code/setup)"
	case "codex":
		return "Install Codex CLI: npm install -g @openai/codex"
	case "gemini":
		return "Install Gemini CLI: npm install -g @google/gemini-cli"
	case "opencode":
		return "Install OpenCode: npm install -g opencode-ai"
	}
	return "Install " + name + " and make sure it is in your PATH"
}

// dropped are variables of the session that launched ADBuddy, which must not
// leak into the agent (spec §5.1), plus the ones ADBuddy sets itself.
func dropped(key string) bool {
	k := strings.ToUpper(key)
	switch {
	case strings.HasPrefix(k, "CLAUDECODE"), strings.HasPrefix(k, "CLAUDE_CODE_"),
		k == "CLAUDE_EFFORT", k == "CLAUDE_PID", k == "AI_AGENT",
		k == "ANDROID_SERIAL", k == "ANDROID_ADB_SERVER_PORT", k == "ADBUDDY_SOURCE":
		return true
	}
	return false
}

// Env builds the agent's environment: the caller's, without the dropped
// variables, with binDir first in PATH, the active phone in ANDROID_SERIAL
// and ADBUDDY_SOURCE=agent so the journal knows who ran each command.
func Env(environ []string, binDir, serial string) []string {
	sep := string(os.PathListSeparator)
	var out []string
	pathSet := false
	for _, kv := range environ {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		key, val := kv[:i], kv[i+1:]
		if dropped(key) {
			continue
		}
		if key == "PATH" || (runtime.GOOS == "windows" && strings.EqualFold(key, "PATH")) {
			if pathSet {
				continue
			}
			out = append(out, key+"="+binDir+sep+val)
			pathSet = true
			continue
		}
		out = append(out, kv)
	}
	if !pathSet {
		out = append(out, "PATH="+binDir)
	}
	return append(out, "ANDROID_SERIAL="+serial, "ADBUDDY_SOURCE=agent")
}

// ErrNotFound means the agent is not installed.
var ErrNotFound = errors.New("agent not found in PATH")

// Options for Run.
type Options struct {
	Agent   string
	Args    []string
	Dir     string // the environment
	BinDir  string
	Serial  string
	Environ []string
	Stdin   *os.File
	Stdout  *os.File
	Stderr  *os.File
}

// Run starts the agent with inherited stdio and waits for it. Ctrl+C belongs
// to the agent: ADBuddy ignores it while the agent runs.
func Run(ctx context.Context, o Options) (int, error) {
	// A relative or empty bin folder would put the current directory first
	// in the agent's PATH.
	if !filepath.IsAbs(o.BinDir) {
		return 0, fmt.Errorf("the bin folder must be an absolute path (got %q)", o.BinDir)
	}
	path, err := exec.LookPath(o.Agent)
	if err != nil {
		return 0, ErrNotFound
	}
	cmd := exec.CommandContext(ctx, path, o.Args...)
	cmd.Dir = o.Dir
	cmd.Env = Env(o.Environ, o.BinDir, o.Serial)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code, nil
		}
		return 1, nil // killed by a signal: -1 would exit 255 (4294967295 on Windows)
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}
