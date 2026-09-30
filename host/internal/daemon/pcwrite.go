package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// checkPCWrites returns why adb pull or adb bugreport would write outside the
// agent's environment or over its own files, or "" if every target stays
// inside. The working directory is the agent's (the shim's), so it is never
// trusted: it must itself be inside the environment. Paths are resolved
// through links where they exist, and compared without case on Windows.
func (d *Daemon) checkPCWrites(cwd string, ws []classify.PCWrite) string {
	for _, w := range ws {
		if why := d.checkPCWrite(cwd, w); why != "" {
			return why
		}
	}
	return ""
}

func (d *Daemon) checkPCWrite(cwd string, w classify.PCWrite) string {
	if w.Reason != "" {
		return w.Cmd + " " + w.Reason
	}
	if d.o.Workspace == "" {
		return w.Cmd + " writes on the PC and the agent environment is unknown"
	}
	root, err := filepath.EvalSymlinks(d.o.Workspace)
	if err != nil {
		return w.Cmd + " writes on the PC and the agent environment cannot be read: " + err.Error()
	}
	outside := w.Cmd + " writes outside the agent environment: "
	own := w.Cmd + " writes over the agent environment's own files: "
	if cwd == "" || !filepath.IsAbs(cwd) {
		return outside + "(unknown working directory " + cwd + ")"
	}
	cwd = filepath.Clean(cwd)
	for _, t := range append([]string{"."}, w.Targets...) {
		p := filepath.Join(cwd, filepath.FromSlash(strings.ReplaceAll(t, `\`, "/")))
		real, err := resolveLinks(p)
		if err != nil {
			return outside + p + " (" + err.Error() + ")"
		}
		rel, inside := within(root, real)
		switch {
		case !inside:
			return outside + p
		case reparseBelow(root, rel):
			return outside + p + " (through a junction or another link ADBuddy does not follow)"
		case rel != "." && !classify.SafeLocalTarget(filepath.ToSlash(rel)):
			return own + p
		}
	}
	return ""
}

// resolveLinks resolves the links in p. The part that does not exist yet (the
// file adb will create) is appended to its deepest existing folder, resolved.
// A link that exists but points nowhere is an error: adb would create its
// target, wherever that is.
func resolveLinks(p string) (string, error) {
	var rest []string
	for {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		if _, lerr := os.Lstat(p); lerr == nil {
			return "", err // it exists, but its links do not resolve
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		rest = append([]string{filepath.Base(p)}, rest...)
		p = parent
	}
}

// within reports whether p is root or inside it, and p relative to root.
// Windows paths are compared without case.
func within(root, p string) (string, bool) {
	if runtime.GOOS == "windows" {
		root, p = strings.ToLower(root), strings.ToLower(p)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// reparseBelow reports whether a folder or file on the way from root to rel
// is a link that filepath.EvalSymlinks left unresolved: on Windows it does
// not follow junctions (mount points), which anyone can create with
// `mklink /J` and which may point anywhere.
func reparseBelow(root, rel string) bool {
	if rel == "." {
		return false
	}
	p := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		fi, err := os.Lstat(p)
		if err != nil {
			return false // the rest does not exist yet
		}
		if fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return true
		}
	}
	return false
}
