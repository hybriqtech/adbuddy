package workspace

import (
	"embed"
	"io/fs"
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
