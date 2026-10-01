package claudecode

import (
	"errors"
	"fmt"
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

// ValidTranscript checks a transcript_path that came in a hook payload,
// which a process that is not to be trusted can write (spec M4 §4.1): it must
// be absolute, end in .jsonl, be a regular file (not a link) and lie, once
// links are resolved, under <configDir>/projects. It returns the path to
// read. The file may not exist yet: then its directory is checked instead.
func ValidTranscript(path, configDir string) (string, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".jsonl") {
		return "", errors.New("the transcript path is not an absolute .jsonl path")
	}
	root, err := resolveLoose(filepath.Join(configDir, "projects")) // Claude Code makes it with its first write
	if err != nil {
		return "", fmt.Errorf("Claude Code's projects directory is not readable: %w", err)
	}
	path = filepath.Clean(path)
	fi, err := os.Lstat(path)
	switch {
	case err == nil:
		if !fi.Mode().IsRegular() {
			return "", errors.New("the transcript is not a regular file")
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		if !Inside(root, real) {
			return "", errors.New("the transcript is outside Claude Code's projects directory")
		}
		return real, nil
	case errors.Is(err, os.ErrNotExist):
		dir, err := resolveLoose(filepath.Dir(path))
		if err != nil {
			return "", err
		}
		if !Inside(root, dir) {
			return "", errors.New("the transcript is outside Claude Code's projects directory")
		}
		return filepath.Join(dir, filepath.Base(path)), nil
	default:
		return "", err
	}
}

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
