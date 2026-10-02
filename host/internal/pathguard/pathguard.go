// Package pathguard decides whether a path stays inside a folder once links
// are followed. It is the one guard of the daemon's checks of adb pull and
// adb bugreport (daemon/pcwrite.go) and of adbuddy send (cli/send.go): a
// change here changes both.
package pathguard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveLinks resolves the links in p. The part that does not exist yet is
// appended to its deepest existing folder, resolved. A link that exists but
// points nowhere is an error.
func ResolveLinks(p string) (string, error) {
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

// Within reports whether p is root or inside it, and p relative to root.
// Windows paths are compared without case.
func Within(root, p string) (string, bool) {
	if runtime.GOOS == "windows" {
		root, p = strings.ToLower(root), strings.ToLower(p)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// ReparseBelow reports whether a folder or file on the way from root to rel
// is a link that filepath.EvalSymlinks left unresolved: on Windows it does
// not follow junctions (mount points), which anyone can create with
// `mklink /J` and which may point anywhere.
func ReparseBelow(root, rel string) bool {
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
