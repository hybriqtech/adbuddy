package daemon

import (
	"path/filepath"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/pathguard"
)

// checkPCWrites returns why adb pull or adb bugreport would write outside the
// agent's environment or over its own files, or an empty Text if every target
// stays inside. The working directory is the agent's (the shim's), so it is
// never trusted: it must itself be inside the environment. Paths are resolved
// through links where they exist, and compared without case on Windows, by
// pathguard, the guard adbuddy send uses too. The answer is the reason of the
// 🔴 question, in both languages.
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
		// The part that does not exist yet (the file adb will create) is
		// appended to its deepest existing folder, resolved; a link that
		// exists but points nowhere is an error: adb would create its target,
		// wherever that is.
		real, err := pathguard.ResolveLinks(p)
		if err != nil {
			return cat(outside, same(p+" ("+err.Error()+")"))
		}
		rel, inside := pathguard.Within(root, real)
		switch {
		case !inside:
			return cat(outside, same(p))
		case pathguard.ReparseBelow(root, rel):
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
