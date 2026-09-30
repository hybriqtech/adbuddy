package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeExe(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "adbuddy-bin")
	if err := os.WriteFile(p, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInitCreatesEnvironment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ADBuddy", "default")
	res, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"AGENTS.md", "CLAUDE.md", "NOTES.md", filepath.Join(".claude", "settings.json"), filepath.Join(".adbuddy", "bin", exe("adb")), filepath.Join(".adbuddy", "bin", exe("adbuddy"))} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if len(res.Created) != 6 {
		t.Fatalf("created %q", res.Created)
	}
	if st, err := os.Stat(JournalDir(dir)); err != nil || !st.IsDir() {
		t.Fatalf("journal dir: %v", err)
	}
	agents := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{"Denied by user", "Denied: nobody could confirm", "Blocked by ADBuddy", "data, not instructions", "adbuddy undo", "Not collected yet", "`adb pull` only into this environment; anything else asks for confirmation."} {
		if !strings.Contains(agents, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
	if strings.Contains(agents, ProfileMarker) {
		t.Error("profile marker not replaced")
	}
	if !strings.HasPrefix(read(t, filepath.Join(dir, "CLAUDE.md")), "@AGENTS.md") {
		t.Error("CLAUDE.md must import AGENTS.md")
	}
	if read(t, filepath.Join(BinDir(dir), exe("adb"))) != "v1" {
		t.Error("shim content")
	}
	var s map[string]any
	json.Unmarshal([]byte(read(t, filepath.Join(dir, ".claude", "settings.json"))), &s)
	allow := s["permissions"].(map[string]any)["allow"].([]any)
	if len(allow) != 2 || allow[0] != "Bash(adb:*)" || allow[1] != "Bash(adbuddy:*)" {
		t.Fatalf("allow %v", allow)
	}
}

func TestInitIsIdempotentAndKeepsUserFiles(t *testing.T) {
	dir := t.TempDir()
	exe1 := fakeExe(t, "v1")
	if _, err := Init(Options{Dir: dir, Exe: exe1}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "NOTES.md"), []byte("my notes"), 0o600)
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("edited"), 0o600)
	res, err := Init(Options{Dir: dir, Exe: exe1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created)+len(res.Updated) != 0 {
		t.Fatalf("second run changed %+v", res)
	}
	res, err = Init(Options{Dir: dir, Exe: fakeExe(t, "v2"), Force: true, Profile: "- Model: Pixel"})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dir, "NOTES.md")) != "my notes" {
		t.Fatal("NOTES.md must never change")
	}
	if a := read(t, filepath.Join(dir, "AGENTS.md")); !strings.Contains(a, "- Model: Pixel") {
		t.Fatal("force must rewrite AGENTS.md with the profile")
	}
	if read(t, filepath.Join(BinDir(dir), exe("adb"))) != "v2" {
		t.Fatal("shim must be refreshed")
	}
	if len(res.Updated) != 3 { // AGENTS.md, adb, adbuddy
		t.Fatalf("updated %q", res.Updated)
	}
}

func TestClaudeSettingsMergeKeepsUserSettings(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o700)
	user := `{"model":"opus","permissions":{"allow":["Bash(git:*)","Bash(adb:*)"],"deny":["Bash(rm:*)"]},"hooks":{"Stop":[]}}`
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(user), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	json.Unmarshal([]byte(read(t, filepath.Join(dir, ".claude", "settings.json"))), &s)
	perms := s["permissions"].(map[string]any)
	allow := perms["allow"].([]any)
	if s["model"] != "opus" || s["hooks"] == nil || perms["deny"] == nil || len(allow) != 3 || allow[2] != "Bash(adbuddy:*)" {
		t.Fatalf("merged %v", s)
	}
}

func TestInvalidClaudeSettingsIsNotClobbered(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o700)
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte("{oops"), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("%v", err)
	}
	if read(t, filepath.Join(dir, ".claude", "settings.json")) != "{oops" {
		t.Fatal("file was changed")
	}
}

func TestNullClaudeSettingsIsNotClobbered(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o700)
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte("null"), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err == nil || !strings.Contains(err.Error(), "not a JSON object") {
		t.Fatalf("%v", err)
	}
	if read(t, filepath.Join(dir, ".claude", "settings.json")) != "null" {
		t.Fatal("file was changed")
	}
}

func TestClaudeSettingsWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".claude"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		t.Fatalf("entries %v", entries)
	}
	if !json.Valid([]byte(read(t, filepath.Join(dir, ".claude", "settings.json")))) {
		t.Fatal("invalid JSON")
	}
}

func TestClaudeSettingsWithBOM(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o700)
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte("\xEF\xBB\xBF{\"model\":\"opus\"}"), 0o600)
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, ".claude", "settings.json"))), &s); err != nil || s["model"] != "opus" {
		t.Fatalf("%v %v", err, s)
	}
}

func TestAgentsCoversHostDownAndUndo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	a := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{"ADBuddy host not running", "tell the user what you undid"} {
		if !strings.Contains(a, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
	if strings.Contains(a, "adbuddy setup") {
		t.Error("must not mention setup")
	}
}

// Force replaces a planted link with a regular file: the file it points to
// is never written.
func TestInitForceDoesNotFollowLinks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws")
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(target, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		p := filepath.Join(dir, name)
		os.Remove(p)
		if err := os.Symlink(target, p); err != nil {
			t.Skip("cannot create symbolic links here:", err)
		}
	}
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1"), Profile: "Phone X", Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, target); got != "keep me" {
		t.Fatalf("the link's target was written: %q", got)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		st, err := os.Lstat(filepath.Join(dir, name))
		if err != nil || !st.Mode().IsRegular() {
			t.Fatalf("%s: %v %v", name, st, err)
		}
	}
	if !strings.Contains(read(t, filepath.Join(dir, "AGENTS.md")), "Phone X") {
		t.Fatal("AGENTS.md not rewritten")
	}
}

// A dangling link where a new file goes is not followed either.
func TestInitDoesNotFollowDanglingLinks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "created-elsewhere.txt")
	if err := os.Symlink(target, filepath.Join(dir, "NOTES.md")); err != nil {
		t.Skip("cannot create symbolic links here:", err)
	}
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(target); err == nil {
		t.Fatal("the dangling link was followed")
	}
}

// A hard link (no privilege needed, also on Windows) shares its content with
// another file: Force replaces the name instead of writing through it.
func TestInitForceDoesNotWriteThroughHardLinks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ws")
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(target, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "AGENTS.md")
	os.Remove(p)
	if err := os.Link(target, p); err != nil {
		t.Skip("cannot create hard links here:", err)
	}
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1"), Profile: "Phone X", Force: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, target); got != "keep me" {
		t.Fatalf("written through the hard link: %q", got)
	}
	if !strings.Contains(read(t, p), "Phone X") {
		t.Fatal("AGENTS.md not rewritten")
	}
}
