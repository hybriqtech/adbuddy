package cli

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestGroupSkillPaths(t *testing.T) {
	in := []string{
		"AGENTS.md",
		filepath.Join(".claude", "skills", "debloat", "SKILL.md"),
		filepath.Join(".claude", "skills", "debloat", "notes.md"),
		filepath.Join(".claude", "skills", "battery", "SKILL.md"),
		filepath.Join(".agents", "skills", "debloat", "SKILL.md"),
		filepath.Join(".gemini", "settings.json"),
	}
	want := []string{
		"  created AGENTS.md",
		"  created .gemini/settings.json",
		"  created 2 skills in .claude/skills",
		"  created 1 skill in .agents/skills",
	}
	if got := groupSkillPaths("created", in); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
