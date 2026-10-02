package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

// checkPCWrites returns why adb pull or adb bugreport would write outside the
// agent's environment or over its own files, or an empty Text if every target
// stays inside. The working directory is the agent's (the shim's), so it is
// never trusted: it must itself be inside the environment. Paths are resolved
// through links where they exist, and compared without case on Windows. The
// answer is the reason of the 🔴 question, in both languages.
func (d *Daemon) checkPCWrites(cwd string, ws []classify.PCWrite) classify.Text {
	for _, w := range ws {
		if why := d.checkPCWrite(cwd, w); !why.Empty() {
			return why
		}
	}
	return classify.Text{}
}

func (d *Daemon) checkPCWrite(cwd string, w classify.PCWrite) classify.Text {
	same, cat := classify.Same, classify.Cat
	if !w.Reason.Empty() {
		return cat(same(w.Cmd+" "), w.Reason)
	}
	if d.o.Workspace == "" {
		return cat(same(w.Cmd), classify.Text{EN: " writes on the PC and the agent environment is unknown", ES: " escribe en el PC y no se conoce el entorno del agente"})
	}
	root, err := filepath.EvalSymlinks(d.o.Workspace)
	if err != nil {
		return cat(same(w.Cmd), classify.Text{EN: " writes on the PC and the agent environment cannot be read: ", ES: " escribe en el PC y no se puede leer el entorno del agente: "}, same(err.Error()))
	}
	outside := cat(same(w.Cmd), classify.Text{EN: " writes outside the agent environment: ", ES: " escribe fuera del entorno del agente: "})
	own := cat(same(w.Cmd), classify.Text{EN: " writes over the agent environment's own files: ", ES: " escribe sobre los ficheros propios del entorno del agente: "})
	if cwd == "" || !filepath.IsAbs(cwd) {
		return cat(outside, classify.Text{EN: "(unknown working directory " + cwd + ")", ES: "(directorio de trabajo desconocido " + cwd + ")"})
	}
	// adb runs in cwd as sent, and Linux and macOS resolve "out/.." on disk
	// (in the parent of out's target) while filepath.Clean just drops it.
	if cwd != filepath.Clean(cwd) || hasDotDot(cwd) {
		return cat(outside, classify.Text{EN: "(working directory not in clean form " + cwd + ")", ES: "(el directorio de trabajo no está en forma limpia: " + cwd + ")"})
	}
	for _, t := range append([]string{"."}, w.Targets...) {
		p := filepath.Join(cwd, filepath.FromSlash(strings.ReplaceAll(t, `\`, "/")))
		real, err := resolveLinks(p)
		if err != nil {
			return cat(outside, same(p+" ("+err.Error()+")"))
		}
		rel, inside := within(root, real)
		switch {
		case !inside:
			return cat(outside, same(p))
		case reparseBelow(root, rel):
			return cat(outside, same(p), classify.Text{EN: " (through a junction or another link ADBuddy does not follow)", ES: " (a través de una unión (junction) o de otro enlace que ADBuddy no sigue)"})
		case rel != "." && !classify.SafeLocalTarget(filepath.ToSlash(rel)):
			return cat(own, same(p))
		}
	}
	return classify.Text{}
}

func hasDotDot(p string) bool {
	for _, part := range strings.Split(filepath.ToSlash(p), "/") {
		if part == ".." {
			return true
		}
	}
	return false
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
