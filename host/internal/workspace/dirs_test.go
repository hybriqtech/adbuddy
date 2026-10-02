package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// names lists the entries of a folder, sorted.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// ensureRealDirs checks every folder before it creates any: one in the way
// leaves the others uncreated (V26).
func TestEnsureRealDirsIsAllOrNothing(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "c"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureRealDirs(base, filepath.Join("a", "b"), "c"); err == nil {
		t.Fatal("a file where a folder goes must be an error")
	}
	if got := names(t, base); strings.Join(got, ",") != "c" {
		t.Fatalf("something was created: %v", got)
	}
	if err := ensureRealDirs(base, filepath.Join("a", "b"), filepath.Join("a", "d")); err != nil {
		t.Fatal(err)
	}
	if got := names(t, filepath.Join(base, "a")); strings.Join(got, ",") != "b,d" {
		t.Fatalf("got %v", got)
	}
}

// planted are the folders of Init where a link is planted in the tests: with
// it, Init used to create .adbuddy/bin (and the shim), the journal or a skill
// on the other side.
var planted = []string{
	".adbuddy", filepath.Join(".adbuddy", "bin"), "journal", ".claude", ".gemini", ".agents",
	filepath.Join(".claude", "skills", SkillNames()[0]), filepath.Join(".agents", "skills", SkillNames()[0]),
}

// checkRefused runs Init on dir, where rel is a link to outside, and checks
// that it fails, writes nothing through the link and creates nothing else.
func checkRefused(t *testing.T, dir, rel, outside string) {
	t.Helper()
	before := names(t, dir)
	_, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")})
	if err == nil || !strings.Contains(err.Error(), "link") {
		t.Fatalf("error %v, want one that mentions the link", err)
	}
	if got := names(t, outside); len(got) != 0 {
		t.Fatalf("Init wrote through the link: %v", got)
	}
	if got := names(t, dir); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Fatalf("Init created %v, had %v: it must create nothing when a folder is a link", got, before)
	}
}

// A junction where one of Init's folders goes is refused, before anything is
// created (V26). Init runs on every serve and run.
func TestInitRefusesAJunctionWhereItsFoldersGo(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows thing")
	}
	for _, rel := range planted {
		t.Run(rel, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(dir, rel)
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
				t.Skipf("no junction: %v: %s", err, out)
			}
			// Remove the junction itself before the temporary folders go: a
			// RemoveAll must never follow it.
			t.Cleanup(func() { os.Remove(link) })
			checkRefused(t, dir, rel, outside)
		})
	}
}

// The environment itself may be a junction (the user's choice): only links
// inside it are refused.
func TestInitWorksWhenTheEnvironmentIsAJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows thing")
	}
	realDir := t.TempDir()
	env := filepath.Join(t.TempDir(), "env")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", env, realDir).CombinedOutput(); err != nil {
		t.Skipf("no junction: %v: %s", err, out)
	}
	t.Cleanup(func() { os.Remove(env) })
	checkInitThroughLink(t, env, realDir)
}

// checkInitThroughLink runs Init on env, a link to realDir, and checks that it
// works and its files are in realDir.
func checkInitThroughLink(t *testing.T, env, realDir string) {
	t.Helper()
	if _, err := Init(Options{Dir: env, Exe: fakeExe(t, "v1")}); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(BinDir(realDir), exe("adb"))) != "v1" {
		t.Fatal("the shim is not in the environment")
	}
	if st, err := os.Lstat(JournalDir(realDir)); err != nil || !st.IsDir() {
		t.Fatalf("no journal folder: %v %v", st, err)
	}
}
