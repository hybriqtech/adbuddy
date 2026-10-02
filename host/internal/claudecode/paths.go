package claudecode

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ConfigDir is Claude Code's configuration directory: $CLAUDE_CONFIG_DIR, or
// ~/.claude.
func ConfigDir(getenv func(string) string, home func() (string, error)) (string, error) {
	if d := getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	h, err := home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".claude"), nil
}

// Inside reports whether path is dir or lies under it, comparing as the
// platform does (case does not count on Windows).
func Inside(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

// Transcript is a transcript path checked by ValidTranscript, kept as
// Claude Code's projects directory and the path under it: every open goes
// through an os.Root on that directory, so no component under it can take the
// reading out of it, even one that becomes a link after the check (V24).
type Transcript struct {
	dir string // <configDir>/projects; a link here is the user's (a moved .claude or projects/)
	rel string // the path under dir: none of its components may be a link out of dir
}

// String is the path, for messages.
func (t Transcript) String() string { return filepath.Join(t.dir, t.rel) }

// ValidTranscript checks a transcript_path that came in a hook payload,
// which a process that is not to be trusted can write (spec M4 §4.1): it must
// be absolute, end in .jsonl, lie under <configDir>/projects and be a regular
// file (not a link): under projects/, a junction, or a link that leads out
// of projects/, is refused; one in the path of projects/ itself is followed. The file may not exist yet, nor
// projects/ (Claude Code makes it with its first write).
func ValidTranscript(path, configDir string) (Transcript, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return Transcript{}, errors.New("the transcript path is not an absolute .jsonl path")
	}
	path = filepath.Clean(path)
	dir := filepath.Join(configDir, "projects")
	if !Inside(dir, path) {
		// The path may name projects/ by another of its names (/private/var
		// on macOS, an 8.3 short name on Windows).
		real, err := resolveLoose(dir)
		if err != nil {
			return Transcript{}, fmt.Errorf("Claude Code's projects directory is not readable: %w", err)
		}
		dir = real
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == "." || !Inside(dir, path) {
		return Transcript{}, errOutside
	}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Transcript{dir, rel}, nil // nothing under it yet: the tailer checks at every open
	}
	if err != nil {
		return Transcript{}, fmt.Errorf("Claude Code's projects directory is not readable: %w", err)
	}
	defer root.Close()
	fi, err := root.Lstat(rel)
	switch {
	case err == nil:
		if !fi.Mode().IsRegular() {
			return Transcript{}, errNotRegular
		}
	case errors.Is(err, fs.ErrNotExist):
		// Not written yet. The components are checked in order: one that
		// exists and leads out of projects/ is an error, not ErrNotExist.
	default:
		return Transcript{}, err // a link out of projects/: "path escapes from parent"
	}
	return Transcript{dir, rel}, nil
}

var (
	errOutside    = errors.New("the transcript is outside Claude Code's projects directory")
	errNotRegular = errors.New("the transcript is not a regular file")
)

// resolveLoose resolves the links of the part of path that exists and keeps
// the rest as it is: a directory that is not there yet cannot hold a link.
func resolveLoose(path string) (string, error) {
	path = filepath.Clean(path)
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return real, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	up, err := resolveLoose(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(up, filepath.Base(path)), nil
}
