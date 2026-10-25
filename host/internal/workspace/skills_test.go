package workspace

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// requiredSkills lists the skills the spec asks for (M6b §1). Each task of the
// plan adds its own until the nine are here.
var requiredSkills = []string{"backup-to-pc", "battery", "build-utility-apk", "debloat", "device-profile", "home-screen", "organize-files", "permissions-audit", "storage-cleanup"}

var (
	skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	ipv4Re      = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	userPathRe  = regexp.MustCompile(`C:\\Users|/home/|/Users/`)
	otherAdbRe  = regexp.MustCompile(`\badb\s+-[A-Za-z]|platform-tools[/\\]adb`)
	serialRe    = regexp.MustCompile(`emulator-\d+|ANDROID_SERIAL=`)
	quotedDesc  = regexp.MustCompile(`(?m)^description: "`)
)

// frontmatter splits a SKILL.md into its YAML header and its body.
func frontmatter(t *testing.T, name, src string) (map[string]any, string) {
	t.Helper()
	src = strings.ReplaceAll(src, "\r\n", "\n")
	rest, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		t.Fatalf("%s: SKILL.md must start with ---", name)
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		t.Fatalf("%s: the frontmatter is not closed with ---", name)
	}
	var m map[string]any
	if err := yaml.Unmarshal([]byte(head), &m); err != nil {
		t.Fatalf("%s: frontmatter is not valid YAML: %v", name, err)
	}
	return m, body
}

func TestSkillsAreExactlyTheRequiredOnes(t *testing.T) {
	want := slices.Clone(requiredSkills)
	slices.Sort(want)
	if got := SkillNames(); !slices.Equal(got, want) {
		t.Fatalf("embedded skills %v, want %v", got, want)
	}
}

func TestSkillsFormat(t *testing.T) {
	skills := Skills()
	// A loose file in the root would be installed as .claude/skills/<file>.
	entries, err := fs.ReadDir(skills, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("skills/%s: only skill folders go in the root", e.Name())
		}
	}
	for _, name := range SkillNames() {
		data, err := fs.ReadFile(skills, name+"/SKILL.md")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		src := strings.ReplaceAll(string(data), "\r\n", "\n")
		meta, _ := frontmatter(t, name, src)
		if len(meta) != 2 || meta["name"] == nil || meta["description"] == nil {
			t.Errorf("%s: frontmatter must have exactly name and description, has %v", name, meta)
		}
		if meta["name"] != name || !skillNameRe.MatchString(name) || len(name) > 64 {
			t.Errorf("%s: name %v must equal the folder and be lower-case words with hyphens (at most 64 characters)", name, meta["name"])
		}
		desc, _ := meta["description"].(string)
		if !strings.HasPrefix(desc, "Use when") || len(desc) > 1024 {
			t.Errorf("%s: description must start with %q and be at most 1024 characters (it has %d)", name, "Use when", len(desc))
		}
		if n := strings.Count(src, "\n"); n > 250 {
			t.Errorf("%s: SKILL.md has %d lines, at most 250", name, n)
		}
		// An unquoted scalar breaks as YAML at the first ": " inside it, and
		// then the agent may not load the skill at all.
		if !quotedDesc.MatchString(src) {
			t.Errorf("%s: write the description between double quotes: description: \"Use when …\"", name)
		}
		if !strings.Contains(src, "The rules in `AGENTS.md` apply") {
			t.Errorf("%s: it must say that \"The rules in `AGENTS.md` apply\" (spec §2.1)", name)
		}
	}
}

func TestSkillsContainNothingPrivateOrUnsafe(t *testing.T) {
	skills := Skills()
	err := fs.WalkDir(skills, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(skills, p)
		if err != nil {
			return err
		}
		s := string(data)
		for what, re := range map[string]*regexp.Regexp{"an IPv4 address": ipv4Re, "a user path": userPathRe, "another adb or a global adb option": otherAdbRe, "a device serial": serialRe} {
			if m := re.FindString(s); m != "" {
				t.Errorf("%s contains %s: %q", p, what, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// V31: the guard also blocks for a notification or a window it cannot read, not
// only for a protected app; the home-screen skill must say so, with the words
// the guard itself uses (daemon.screenProtectedReason).
func TestHomeScreenSkillSaysTheScreenCanBeProtectedForOtherReasons(t *testing.T) {
	b, err := fs.ReadFile(Skills(), "home-screen/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	const want = "something on the screen is protected (a protected app, a notification or a window ADBuddy cannot read"
	if !strings.Contains(string(b), want) {
		t.Errorf("home-screen/SKILL.md lacks %q", want)
	}
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
