// Package precheck runs the checks that need the phone before a change and
// computes its inverse (spec §4.3). Every command it builds for the phone's
// shell quotes its values with package shq.
package precheck

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// Device runs the host's own read-only commands on the phone.
type Device interface {
	// Shell runs `adb shell <command>` and returns its stdout and exit code.
	Shell(ctx context.Context, command string) (stdout string, exitCode int, err error)
	// Pull copies a file from the phone to the PC.
	Pull(ctx context.Context, remote, local string) error
}

// Env is where the command runs.
type Env struct {
	Cwd       string // the shim's working directory (adb push resolves local paths there)
	BackupDir string // where this entry's backups go (journal/backups/<id>)
}

// Prepared is the outcome of the prechecks.
type Prepared struct {
	Level   classify.Level // may be raised to Red
	Reason  string         // why it was raised
	Inverse []string       // adb argv that undoes the change; nil when none
	Backups []string       // files saved under Env.BackupDir
	// After, when set, computes the inverse once the command has run
	// (adb install: the new package is only known afterwards).
	After func(ctx context.Context) ([]string, error)
}

// Prepare runs the prechecks for op. If the phone cannot be read, the change
// is raised to Red: without knowing the previous state there is no undo.
func Prepare(ctx context.Context, dev Device, op *classify.Op, level classify.Level, env Env) Prepared {
	p := Prepared{Level: level}
	if op == nil {
		return p
	}
	var err error
	switch op.Kind {
	case "settings-put":
		err = settingsPut(ctx, dev, op.Args, &p)
	case "pm-disable":
		err = pmDisable(ctx, dev, op.Args, &p)
	case "pm-uninstall-user":
		err = pmUninstallUser(ctx, dev, op.Args, &p)
	case "appops-set":
		err = appopsSet(ctx, dev, op.Args, &p)
	case "deviceidle":
		err = deviceidle(ctx, dev, op.Args, &p)
	case "mv":
		err = mv(ctx, dev, op.Args, &p)
	case "mkdir":
		err = mkdir(ctx, dev, op.Args, &p)
	case "new-file":
		err = newFile(ctx, dev, op.Args, &p)
	case "adb-install":
		err = install(ctx, dev, &p)
	case "adb-push":
		err = push(ctx, dev, op.Args, env, &p)
	default:
		err = fmt.Errorf("unknown change %q", op.Kind)
	}
	if err != nil {
		return Prepared{Level: classify.Red, Reason: "could not check the phone before the change: " + err.Error()}
	}
	return p
}

func shellArgv(words ...string) []string {
	return append([]string{"shell"}, shq.Args(words...)...)
}

func withUser(prefix []string, user string, rest ...string) []string {
	out := append([]string{}, prefix...)
	if user != "" {
		out = append(out, "--user", user)
	}
	return append(out, rest...)
}

// run executes a host-built command and fails on a non-zero exit code.
func run(ctx context.Context, dev Device, words ...string) (string, error) {
	out, code, err := dev.Shell(ctx, shq.Join(words...))
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("%s exited with %d", words[0], code)
	}
	return out, nil
}

// trimLine removes the single trailing newline adb adds, keeping any other
// spaces that belong to the value.
func trimLine(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}

func settingsPut(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	out, err := run(ctx, dev, withUser([]string{"settings"}, a["user"], "get", a["ns"], a["key"])...)
	if err != nil {
		return err
	}
	old := trimLine(out)
	if old == "null" {
		p.Inverse = shellArgv(withUser([]string{"settings"}, a["user"], "delete", a["ns"], a["key"])...)
		return nil
	}
	p.Inverse = shellArgv(withUser([]string{"settings"}, a["user"], "put", a["ns"], a["key"], old)...)
	return nil
}

func pmUninstallUser(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	out, err := run(ctx, dev, "pm", "list", "packages", "-s", a["pkg"])
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "package:"+a["pkg"] {
			p.Inverse = shellArgv("cmd", "package", "install-existing", "--user", a["user"], a["pkg"])
			return nil
		}
	}
	p.Level, p.Reason = classify.Red, "not a system app: uninstalling it cannot be undone"
	return nil
}

var appopsMode = regexp.MustCompile(`(?m)^\s*[A-Z_0-9]+:\s*(allow|ignore|deny|default|foreground|errored)\b`)

func appopsSet(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	out, err := run(ctx, dev, withUser([]string{"appops", "get"}, a["user"], a["pkg"], a["op"])...)
	if err != nil {
		return err
	}
	prev := ""
	if m := appopsMode.FindStringSubmatch(out); m != nil {
		prev = m[1]
	} else if strings.Contains(out, "No operations") {
		prev = "default"
	} else {
		return errors.New("cannot read the previous mode")
	}
	p.Inverse = shellArgv(withUser([]string{"appops", "set"}, a["user"], a["pkg"], a["op"], prev)...)
	return nil
}

func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func pmDisable(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	out, err := run(ctx, dev, withUser([]string{"pm", "list", "packages", "-d"}, a["user"], a["pkg"])...)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "package:"+a["pkg"] {
			return nil // already disabled before the change: nothing to undo
		}
	}
	p.Inverse = shellArgv(withUser([]string{"pm", "enable"}, a["user"], a["pkg"])...)
	return nil
}

func deviceidle(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	out, err := run(ctx, dev, "dumpsys", "deviceidle", "whitelist")
	if err != nil {
		return err
	}
	// Lines are category,package,uid. `whitelist +pkg/-pkg` only touches the
	// user list, so only a "user" line counts as present.
	present := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) != 3 || (f[0] != "system-excidle" && f[0] != "system" && f[0] != "user") || !numeric(f[2]) {
			return fmt.Errorf("unexpected deviceidle output %q", line)
		}
		if f[0] == "user" && f[1] == a["pkg"] {
			present = true
		}
	}
	switch {
	case a["sign"] == "+" && !present:
		p.Inverse = shellArgv("dumpsys", "deviceidle", "whitelist", "-"+a["pkg"])
	case a["sign"] == "-" && present:
		p.Inverse = shellArgv("dumpsys", "deviceidle", "whitelist", "+"+a["pkg"])
	}
	return nil
}

// kind reports whether path is a directory, a file (anything else that exists,
// including dangling links) or missing.
func kind(ctx context.Context, dev Device, target string) (string, error) {
	q := shq.Quote(target)
	out, code, err := dev.Shell(ctx, "if [ -d "+q+" ]; then echo dir; elif [ -e "+q+" ] || [ -L "+q+" ]; then echo file; else echo none; fi")
	if err != nil {
		return "", err
	}
	k := strings.TrimSpace(out)
	if code != 0 || (k != "dir" && k != "file" && k != "none") {
		return "", fmt.Errorf("cannot check %s", target)
	}
	return k, nil
}

func mv(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	k, err := kind(ctx, dev, a["dst"])
	if err != nil {
		return err
	}
	if k != "none" {
		p.Level, p.Reason = classify.Red, "the destination already exists and would be overwritten or merged"
		return nil
	}
	p.Inverse = shellArgv("mv", a["dst"], a["src"])
	return nil
}

func mkdir(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	k, err := kind(ctx, dev, a["path"])
	if err != nil {
		return err
	}
	if k == "none" {
		p.Inverse = shellArgv("rmdir", a["path"])
	}
	return nil
}

func newFile(ctx context.Context, dev Device, a map[string]string, p *Prepared) error {
	k, err := kind(ctx, dev, a["path"])
	if err != nil {
		return err
	}
	if k != "none" {
		p.Level, p.Reason = classify.Red, "would overwrite an existing file"
		return nil
	}
	p.Inverse = shellArgv("rm", "-f", a["path"])
	return nil
}

func packages(ctx context.Context, dev Device) (map[string]bool, error) {
	out, err := run(ctx, dev, "pm", "list", "packages")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "package:"); ok {
			set[name] = true
		}
	}
	return set, nil
}

func install(ctx context.Context, dev Device, p *Prepared) error {
	before, err := packages(ctx, dev)
	if err != nil {
		return err
	}
	p.After = func(ctx context.Context) ([]string, error) {
		after, err := packages(ctx, dev)
		if err != nil {
			return nil, err
		}
		var added []string
		for name := range after {
			if !before[name] {
				added = append(added, name)
			}
		}
		sort.Strings(added)
		if len(added) != 1 {
			return nil, nil // an update or nothing new: no inverse
		}
		return []string{"uninstall", added[0]}, nil
	}
	return nil
}

func push(ctx context.Context, dev Device, a map[string]string, env Env, p *Prepared) error {
	local := a["local"]
	if !filepath.IsAbs(local) {
		local = filepath.Join(env.Cwd, local)
	}
	st, err := os.Stat(local)
	if err != nil {
		return err
	}
	if st.IsDir() {
		p.Level, p.Reason = classify.Red, "pushing a folder cannot be undone file by file"
		return nil
	}
	target := a["remote"]
	k, err := kind(ctx, dev, target)
	if err != nil {
		return err
	}
	if k == "dir" {
		target = path.Join(target, filepath.Base(local))
		if k, err = kind(ctx, dev, target); err != nil {
			return err
		}
	} else if strings.HasSuffix(target, "/") {
		p.Level, p.Reason = classify.Red, "the destination folder does not exist"
		return nil
	}
	switch k {
	case "dir":
		p.Level, p.Reason = classify.Red, "the destination is a folder"
	case "none":
		p.Inverse = shellArgv("rm", "-f", target)
	case "file":
		if err := os.MkdirAll(env.BackupDir, 0o700); err != nil {
			return err
		}
		// Fixed name: the remote name is chosen on the phone and must never
		// pick where the backup lands (one backup per journal entry).
		backup := filepath.Join(env.BackupDir, "original")
		if err := dev.Pull(ctx, target, backup); err != nil {
			return fmt.Errorf("backup of %s failed: %w", target, err)
		}
		p.Backups = []string{backup}
		p.Inverse = []string{"push", backup, target}
	}
	return nil
}
