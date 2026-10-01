package workspace

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// skillFiles lists every embedded skill file, slash-separated, relative to the
// skills folder.
func skillFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := fs.WalkDir(Skills(), ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var skillRootsForTest = []string{filepath.Join(".claude", "skills"), filepath.Join(".agents", "skills")}

func TestInitInstallsSkillsInBothFolders(t *testing.T) {
	dir := t.TempDir()
	res, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range skillFiles(t) {
		want, _ := fs.ReadFile(Skills(), p)
		for _, root := range skillRootsForTest {
			rel := filepath.Join(root, filepath.FromSlash(p))
			if got := read(t, filepath.Join(dir, rel)); got != string(want) {
				t.Errorf("%s differs from the embedded skill", rel)
			}
			found := false
			for _, c := range res.Created {
				found = found || c == rel
			}
			if !found {
				t.Errorf("%s is not in Created %q", rel, res.Created)
			}
		}
	}
}

func TestSkillsFollowTheAgentsMdPolicy(t *testing.T) {
	dir := t.TempDir()
	exe1 := fakeExe(t, "v1")
	if _, err := Init(Options{Dir: dir, Exe: exe1}); err != nil {
		t.Fatal(err)
	}
	first := filepath.FromSlash(skillFiles(t)[0])
	claude := filepath.Join(dir, ".claude", "skills", first)
	agents := filepath.Join(dir, ".agents", "skills", first)

	// A second run changes nothing.
	if res, err := Init(Options{Dir: dir, Exe: exe1}); err != nil || len(res.Created)+len(res.Updated) != 0 {
		t.Fatalf("second run: %+v, %v", res, err)
	}
	// An edited file is kept without Force and rewritten with it.
	os.WriteFile(claude, []byte("edited"), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: exe1}); err != nil {
		t.Fatal(err)
	}
	if read(t, claude) != "edited" {
		t.Fatal("an edited skill must be kept without Force")
	}
	res, err := Init(Options{Dir: dir, Exe: exe1, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, claude) == "edited" || read(t, claude) != read(t, agents) {
		t.Fatal("Force must rewrite the edited skill")
	}
	if len(res.Updated) != 1 {
		t.Fatalf("Force updated %q, want only the edited file", res.Updated)
	}
	// The user's marker keeps the file even with Force.
	os.WriteFile(agents, []byte("mine\n"+UserMarker+"\n"), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: exe1, Force: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, agents), "mine") {
		t.Fatal("a file with the user marker must not be rewritten, not even with Force")
	}
	// A deleted file comes back without Force.
	os.Remove(claude)
	if _, err := Init(Options{Dir: dir, Exe: exe1}); err != nil {
		t.Fatal(err)
	}
	want, _ := fs.ReadFile(Skills(), filepath.ToSlash(first))
	if read(t, claude) != string(want) {
		t.Fatal("a missing skill file must be created again")
	}
}

func TestInitLeavesTheUsersOwnSkillsAlone(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, ".claude", "skills", "mi-skill")
	os.MkdirAll(mine, 0o700)
	os.WriteFile(filepath.Join(mine, "SKILL.md"), []byte("mine"), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1"), Force: true}); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(mine, "SKILL.md")) != "mine" {
		t.Fatal("a skill that is not ADBuddy's must not change")
	}
}

func TestInitRefusesALinkOrAFileWhereASkillGoes(t *testing.T) {
	name := SkillNames()[0]
	t.Run("symlink", func(t *testing.T) {
		dir, elsewhere := t.TempDir(), t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".claude", "skills"), 0o700)
		if err := os.Symlink(elsewhere, filepath.Join(dir, ".claude", "skills", name)); err != nil {
			t.Skipf("no symlinks here: %v", err)
		}
		_, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")})
		if err == nil || !strings.Contains(err.Error(), "link") {
			t.Fatalf("error %v, want one that mentions the link", err)
		}
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Fatalf("Init wrote through the link: %v", entries)
		}
	})
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".agents", "skills"), 0o700)
		os.WriteFile(filepath.Join(dir, ".agents", "skills", name), []byte("x"), 0o600)
		if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err == nil {
			t.Fatal("a file where a skill folder goes must be an error")
		}
		if read(t, filepath.Join(dir, ".agents", "skills", name)) != "x" {
			t.Fatal("the file was changed")
		}
	})
	t.Run("junction", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			t.Skip("junctions are a Windows thing")
		}
		dir, elsewhere := t.TempDir(), t.TempDir()
		os.MkdirAll(filepath.Join(dir, ".claude", "skills"), 0o700)
		link := filepath.Join(dir, ".claude", "skills", name)
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, elsewhere).CombinedOutput(); err != nil {
			t.Skipf("no junction: %v: %s", err, out)
		}
		if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err == nil || !strings.Contains(err.Error(), "link") {
			t.Fatalf("a junction where a skill folder goes: error %v, want one that mentions the link", err)
		}
		if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
			t.Fatalf("Init wrote through the junction: %v", entries)
		}
	})
}

func TestInitRefusesLinksAndFilesAtEveryLevel(t *testing.T) {
	name := SkillNames()[0]
	levels := [][]string{{".claude"}, {".agents"}, {".claude", "skills"}, {".agents", "skills"}, {".claude", "skills", name}}
	for _, lv := range levels {
		rel := filepath.Join(lv...)
		t.Run("file/"+rel, func(t *testing.T) {
			dir := t.TempDir()
			if len(lv) > 1 {
				os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o700)
			}
			os.WriteFile(filepath.Join(dir, rel), []byte("x"), 0o600)
			if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err == nil {
				t.Fatal("want an error")
			}
			if read(t, filepath.Join(dir, rel)) != "x" {
				t.Fatal("the file was changed")
			}
		})
		t.Run("symlink/"+rel, func(t *testing.T) {
			dir, elsewhere := t.TempDir(), t.TempDir()
			if len(lv) > 1 {
				os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o700)
			}
			if err := os.Symlink(elsewhere, filepath.Join(dir, rel)); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}
			if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err == nil {
				t.Fatal("want an error")
			}
			if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
				t.Fatalf("Init wrote through the link: %v", entries)
			}
		})
	}
}

func TestForceReplacesASkillFileThatIsALink(t *testing.T) {
	dir, elsewhere := t.TempDir(), t.TempDir()
	exe := fakeExe(t, "v1")
	if _, err := Init(Options{Dir: dir, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	first := filepath.FromSlash(skillFiles(t)[0])
	target := filepath.Join(dir, ".claude", "skills", first)
	outside := filepath.Join(elsewhere, "outside")
	os.WriteFile(outside, []byte("outside"), 0o600)
	os.Remove(target)
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if _, err := Init(Options{Dir: dir, Exe: exe, Force: true}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Lstat(target); err != nil || !st.Mode().IsRegular() {
		t.Fatalf("the link was not replaced by a file: %v %v", st, err)
	}
	if read(t, outside) != "outside" {
		t.Fatal("the link's target changed")
	}
}

func TestForceRewritesAFileOfAnotherSize(t *testing.T) {
	dir := t.TempDir()
	exe := fakeExe(t, "v1")
	if _, err := Init(Options{Dir: dir, Exe: exe}); err != nil {
		t.Fatal(err)
	}
	first := filepath.FromSlash(skillFiles(t)[0])
	target := filepath.Join(dir, ".agents", "skills", first)
	os.WriteFile(target, []byte("short"), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: exe, Force: true}); err != nil {
		t.Fatal(err)
	}
	want, _ := fs.ReadFile(Skills(), filepath.ToSlash(first))
	if read(t, target) != string(want) {
		t.Fatal("a file of another size must be rewritten with Force")
	}
}

func TestInitLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	exe := fakeExe(t, "v1")
	for _, force := range []bool{false, true} {
		if _, err := Init(Options{Dir: dir, Exe: exe, Force: force}); err != nil {
			t.Fatal(err)
		}
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && (strings.HasPrefix(d.Name(), ".write-") || strings.HasPrefix(d.Name(), ".settings-")) {
			t.Errorf("temp file left behind: %s", p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
