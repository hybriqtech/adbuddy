package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
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
		return env{}, ConfigErrorT("DIRS_UNKNOWN", "error.cause", "err.common.dirs_unknown.fix", err.Error())
	}
	cfg, err := config.Load(dirs.ConfigFile())
	if err != nil {
		return env{}, ConfigErrorT("CONFIG_INVALID", "error.cause", "err.common.config_invalid.fix", err.Error(), dirs.ConfigFile())
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

var notRunning = NewErrorT("DAEMON_NOT_RUNNING", "err.common.daemon_not_running", "err.common.daemon_not_running.fix")

// formatEntryIn renders a journal entry on one line (plus its inverse) in
// lang: the words around the entry (undone, the labels of the reason and of
// the undo) are the catalog's; the status word of the entry, its description
// and its command are data and are not translated.
func formatEntryIn(lang i18n.Lang, e journal.Entry) string {
	return formatEntryAt(lang, e, time.Now())
}

// formatEntryAt is formatEntryIn with the clock given: an entry of the day of
// now shows only its time, any other also its date.
func formatEntryAt(lang i18n.Lang, e journal.Entry, now time.Time) string {
	lvl := map[string]string{"green": "🟢", "yellow": "🟡", "red": "🔴"}[e.Level]
	if lvl == "" {
		lvl = "·"
	}
	// Everything here may come from the agent or the phone (a forged line
	// can carry anything, even in its id): escape what could rewrite the
	// terminal.
	status := classify.Printable(e.Status)
	if e.UndoneBy != nil {
		status += lang.T("journal.status.undone")
	}
	if e.Tampered {
		status += lang.T("journal.status.tampered")
	}
	ts := e.TS.In(now.Location())
	stamp := ts.Format("2006-01-02 15:04:05")
	ny, nm, nd := now.Date()
	if y, m, d := ts.Date(); y == ny && m == nm && d == nd {
		stamp = ts.Format("15:04:05")
	}
	line := fmt.Sprintf("%s %s %-8s %s  [%s] adb %s", stamp, lvl, status,
		classify.Printable(e.Description), classify.Printable(e.ID), classify.Printable(strings.Join(e.Argv, " ")))
	if e.Reason != "" && e.Status != journal.StatusOK {
		line += "\n    " + lang.T("journal.label.reason") + " " + classify.Printable(e.Reason)
	}
	if e.Inverse != nil && e.UndoneBy == nil && e.Source != journal.SourceUndo {
		line += "\n    " + lang.T("journal.label.undo") + " adb " + classify.Printable(strings.Join(e.Inverse.Argv, " "))
	}
	return line
}
