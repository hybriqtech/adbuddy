package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/paths"
	"github.com/mattn/go-isatty"
)

// env bundles what most commands need: directories and configuration.
type env struct {
	dirs paths.Dirs
	cfg  config.Config
	home string
}

func loadEnv(app *App) (env, error) {
	dirs, err := paths.Default(app.Getenv)
	if err != nil {
		return env{}, ConfigError("DIRS_UNKNOWN", err.Error(), "Set ADBUDDY_CONFIG_DIR and ADBUDDY_DATA_DIR.")
	}
	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return env{}, ConfigError("CONFIG_INVALID", err.Error(), "Fix or delete "+dirs.ConfigFile()+".")
	}
	home, _ := os.UserHomeDir()
	return env{dirs: dirs, cfg: cfg, home: home}, nil
}

func (e env) workspace(flag string) string {
	if flag != "" {
		return flag
	}
	return e.cfg.WorkspaceDir(e.home)
}

// serveWorkspace is the environment of the serve that answered p: the folder
// that holds its journal. "" if it does not say.
func serveWorkspace(p ipc.Ping) string {
	if p.Journal == "" {
		return ""
	}
	return filepath.Dir(p.Journal)
}

// sameDir reports whether a and b name the same folder: absolute, with links
// resolved where they exist, and without case on Windows.
func sameDir(a, b string) bool {
	norm := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if real, err := filepath.EvalSymlinks(p); err == nil {
			p = real
		}
		return filepath.Clean(p)
	}
	a, b = norm(a), norm(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// isTerminal reports whether v is an interactive terminal (including mintty).
// It is a variable so tests can pretend there is one.
var isTerminal = func(v any) bool {
	f, ok := v.(*os.File)
	return ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()))
}

var notRunning = NewError("DAEMON_NOT_RUNNING", "the ADBuddy host is not running", "Start it with `adbuddy serve` in another terminal.")

// formatEntry renders a journal entry on one line (plus its inverse).
func formatEntry(e journal.Entry) string {
	lvl := map[string]string{"green": "🟢", "yellow": "🟡", "red": "🔴"}[e.Level]
	if lvl == "" {
		lvl = "·"
	}
	// Everything here may come from the agent or the phone (a forged line
	// can carry anything, even in its id): escape what could rewrite the
	// terminal.
	status := classify.Printable(e.Status)
	if e.UndoneBy != nil {
		status += ", undone"
	}
	if e.Tampered {
		status += ", ⚠ not written by ADBuddy"
	}
	line := fmt.Sprintf("%s %s %-8s %s  [%s] adb %s", e.TS.Local().Format("15:04:05"), lvl, status,
		classify.Printable(e.Description), classify.Printable(e.ID), classify.Printable(strings.Join(e.Argv, " ")))
	if e.Reason != "" && e.Status != journal.StatusOK {
		line += "\n    reason: " + classify.Printable(e.Reason)
	}
	if e.Inverse != nil && e.UndoneBy == nil && e.Source != journal.SourceUndo {
		line += "\n    undo: adb " + classify.Printable(strings.Join(e.Inverse.Argv, " "))
	}
	return line
}
