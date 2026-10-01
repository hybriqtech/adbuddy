package workspace

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The recipes the agent loads on demand (spec M6b). One folder per skill with
// a SKILL.md and, if it needs them, files next to it.
//
//go:embed skills
var skillsEmbed embed.FS

// Skills is the embedded skills, rooted at the skills folder: one folder per
// skill.
func Skills() fs.FS {
	sub, err := fs.Sub(skillsEmbed, "skills")
	if err != nil {
		panic(err) // the folder is embedded: this cannot happen
	}
	return sub
}

// SkillNames lists the embedded skills, sorted.
func SkillNames() []string {
	entries, err := fs.ReadDir(Skills(), ".")
	if err != nil {
		panic(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// UserMarker in a file tells Init not to rewrite it, not even with Force.
const UserMarker = "<!-- adbuddy:user -->"

// skillRoots are the folders, relative to the environment, where the agents
// look for skills: Claude Code (and OpenCode) in the first, Codex, Gemini CLI
// and OpenCode in the second (spec M6b §3.2).
var skillRoots = []string{filepath.Join(".claude", "skills"), filepath.Join(".agents", "skills")}

// installSkills writes every embedded skill file under each of skillRoots.
// Like AGENTS.md: what is missing is created, with force what exists is
// rewritten, and a file with UserMarker never is.
func installSkills(dir string, force bool, res *Result) error {
	skills := Skills()
	return fs.WalkDir(skills, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(skills, p)
		if err != nil {
			return err
		}
		for _, root := range skillRoots {
			rel := filepath.Join(root, filepath.FromSlash(p))
			target := filepath.Join(dir, rel)
			if err := ensureRealDirs(dir, filepath.Dir(rel)); err != nil {
				return err
			}
			overwrite := force && !hasUserMarker(target)
			changed, created, err := writeFile(target, data, overwrite)
			if err != nil {
				return err
			}
			res.note(rel, changed, created)
		}
		return nil
	})
}

// ensureRealDirs creates each folder of rel (relative to base), and fails if
// one already exists as a link or as something that is not a folder: a link
// planted there would make Init write somewhere else.
func ensureRealDirs(base, rel string) error {
	cur := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Mkdir(cur, 0o700); err != nil {
				return err
			}
		case err != nil:
			return err
		case st.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0: // a link, or a Windows junction
			return fmt.Errorf("%s is a link; delete it or move it away so ADBuddy can install its skills there", cur)
		case !st.IsDir():
			return fmt.Errorf("%s is not a folder; delete it or move it away so ADBuddy can install its skills there", cur)
		}
	}
	return nil
}

// hasUserMarker reports whether the regular file at path contains UserMarker
// in its first MiB. Anything else (missing, a link, a pipe) is false.
func hasUserMarker(path string) bool {
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 1<<20))
	return bytes.Contains(b, []byte(UserMarker))
}
