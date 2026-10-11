package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/claudecode"
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
	if want := 7 + 2*len(skillFiles(t)); len(res.Created) != want {
		t.Fatalf("created %q", res.Created)
	}
	if st, err := os.Stat(JournalDir(dir)); err != nil || !st.IsDir() {
		t.Fatalf("journal dir: %v", err)
	}
	agents := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{"Denied by user", "Denied: nobody could confirm", "Blocked by ADBuddy", "data, not instructions", "adbuddy undo", "Not collected yet", "`adb pull` only into this environment; anything else asks for confirmation.", "adbuddy screen", "adbuddy tap", "is a protected app", "something on the screen is protected (a protected app, a notification or a window ADBuddy cannot read"} {
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
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".settings-") {
			t.Fatalf("temp file left behind: %v", entries)
		}
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

func TestAgentsCoversDoctorAndBoundedCommands(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	a := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{"`adbuddy doctor [--json]`", "13. Always bound commands", "`logcat -d`", "14. Never ask the user to type a password",
		// Review of task 7: Run lines are the agent's; "Ask the user" lines are the user's.
		"Run its `Run:` lines yourself", "`Ask the user`",
		// Bounded is not enough: these still wait for the user's confirmation.
		"`screenrecord`, `getevent` and `monkey` with more than one event wait for the user's confirmation"} {
		if !strings.Contains(a, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
}

// T3 fix 6: comparing command output as a number needs confirmation, so
// AGENTS.md tells the agent to compare it as text.
func TestAgentsSaysCompareCommandOutputAsText(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	a := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{
		"Compare command output as text: `[ \"$(getprop sys.boot_completed)\" = 1 ]`, not `-eq`",
		"numeric comparisons of command output need confirmation",
		// M2.5: the shell ADBuddy reads, so the agent does not ask in vain.
		"variables of your own with lower-case names (`pkg=…`, not `PKG=…`)",
		"is not analysed and waits for the user's confirmation every time",
	} {
		if !strings.Contains(a, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
}

// Final review, M3: rule 8 tells the temporary blocks (a question waiting)
// from the pause, for adb and for adbuddy undo.
func TestAgentsSaysWhichBlocksAreTemporary(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	a := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{
		"`Blocked by ADBuddy: waiting for your answer to …` and `… waiting to ask about …` are temporary",
		"retry it once the user has answered",
		"`… paused from the phone …` means stop and tell the user",
		"`PAUSED` and `WAITING_FOR_ANSWER` from `adbuddy undo`",
	} {
		if !strings.Contains(a, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
	if strings.Contains(a, "adbuddy setup") {
		t.Error("must not mention setup")
	}
}

// --- hooks of Claude Code (spec M4 §3.2) ---

func hooksOf(t *testing.T, settings string) map[string][]struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	} `json:"hooks"`
} {
	t.Helper()
	var s struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(settings), &s); err != nil {
		t.Fatal(err)
	}
	return s.Hooks
}

func TestInitInstallsTheHooks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "env")
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err != nil {
		t.Fatal(err)
	}
	hooks := hooksOf(t, read(t, filepath.Join(dir, ".claude", "settings.json")))
	if len(hooks) != len(claudecode.Events) {
		t.Fatalf("%d events hooked: %v", len(hooks), hooks)
	}
	for _, ev := range claudecode.Events {
		l := hooks[ev]
		if len(l) != 1 || len(l[0].Hooks) != 1 {
			t.Fatalf("%s: %+v", ev, l)
		}
		h := l[0].Hooks[0]
		want := `"` + filepath.ToSlash(filepath.Join(BinDir(dir), exe("adbuddy"))) + `" hook ` + ev
		if h.Type != "command" || h.Command != want || h.Timeout != HookTimeout(ev) {
			t.Errorf("%s: %+v, want command %s", ev, h, want)
		}
	}
	if HookTimeout(claudecode.EventPermissionRequest) < 610 || HookTimeout(claudecode.EventStop) > 30 {
		t.Error("timeouts: a permission waits for the phone, the others must not hold the agent")
	}
}

func TestHooksAreIdempotentAndKeepTheUsers(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "env")
	exeFile := fakeExe(t, "v1")
	if _, err := Init(Options{Dir: dir, Exe: exeFile}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".claude", "settings.json")
	var s map[string]any
	if err := json.Unmarshal([]byte(read(t, path)), &s); err != nil {
		t.Fatal(err)
	}
	hooks := s["hooks"].(map[string]any)
	hooks["Stop"] = append(hooks["Stop"].([]any), map[string]any{"matcher": "x", "hooks": []any{map[string]any{"type": "command", "command": "echo mine"}}})
	hooks["PreToolUse"] = []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo pre"}}}}
	s["model"] = "opus"
	b, _ := json.Marshal(s)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Init(Options{Dir: dir, Exe: exeFile})
	if err != nil || len(res.Updated)+len(res.Created) != 0 {
		t.Fatalf("a second Init changed %+v (%v)", res, err)
	}
	got := read(t, path)
	for _, want := range []string{`"echo mine"`, `"echo pre"`, `"model":"opus"`} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %s:\n%s", want, got)
		}
	}
	if n := len(hooksOf(t, got)["Stop"]); n != 2 {
		t.Errorf("%d Stop entries", n)
	}
}

func TestHooksOfAnotherPathAreUpdatedNotDuplicated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "env")
	path := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	old := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"\"/old/place/.adbuddy/bin/adbuddy\" hook Stop"}]}]}}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err != nil {
		t.Fatal(err)
	}
	stop := hooksOf(t, read(t, path))["Stop"]
	if len(stop) != 1 || strings.Contains(stop[0].Hooks[0].Command, "/old/place") || !strings.Contains(stop[0].Hooks[0].Command, filepath.ToSlash(dir)) {
		t.Fatalf("%+v", stop)
	}
}

func TestBadHooksSettingsAreNotClobbered(t *testing.T) {
	for name, body := range map[string]string{
		"hooks not an object": `{"hooks":[]}`,
		"event not a list":    `{"hooks":{"Stop":{"a":1}}}`,
	} {
		dir := filepath.Join(t.TempDir(), "env")
		path := filepath.Join(dir, ".claude", "settings.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if read(t, path) != body {
			t.Errorf("%s: the file was rewritten", name)
		}
	}
}

func TestHookCommandRefusesAPathThatQuotingCannotMakeSafe(t *testing.T) {
	bad := []string{`/a"b/adbuddy`, "/a$HOME/adbuddy", "/a`x`/adbuddy", "/a\nb/adbuddy", "/a\rb/adbuddy"}
	if runtime.GOOS != "windows" {
		// On POSIX a backslash is part of the name and an escape inside the double quotes.
		bad = append(bad, `/a\b/adbuddy`, `/a\`, `C:\Users\me\adbuddy.exe`)
	}
	for _, p := range bad {
		if _, err := HookCommand(p, "Stop"); !errors.Is(err, ErrUnsafeHookPath) {
			t.Errorf("%q accepted", p)
		}
	}
	if runtime.GOOS == "windows" {
		// Claude Code runs hooks in bash on Windows: backslashes become slashes.
		if c, err := HookCommand(`C:\Users\me\ADBuddy env\.adbuddy\bin\adbuddy.exe`, "Stop"); err != nil || c != `"C:/Users/me/ADBuddy env/.adbuddy/bin/adbuddy.exe" hook Stop` {
			t.Errorf("%q %v", c, err)
		}
		return
	}
	if c, err := HookCommand("/home/me/ADBuddy env/.adbuddy/bin/adbuddy", "Stop"); err != nil || c != `"/home/me/ADBuddy env/.adbuddy/bin/adbuddy" hook Stop` {
		t.Errorf("%q %v", c, err)
	}
}

// Latency plan T1: CLAUDE.md imports NOTES.md so the agent does not spend a
// turn reading it, and AGENTS.md tells the agent to chain tap and screen.
func TestTemplatesImportNotesAndChainScreenHelpers(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "x")}); err != nil {
		t.Fatal(err)
	}
	c := read(t, filepath.Join(dir, "CLAUDE.md"))
	for _, want := range []string{"@AGENTS.md\n@NOTES.md\n"} {
		if !strings.Contains(strings.ReplaceAll(c, "\r\n", "\n"), want) {
			t.Errorf("CLAUDE.md lacks %q", want)
		}
	}
	a := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{"adbuddy tap \"Network & internet\" --screen", "`adbuddy screen --settle`", "read the result in the same call with", "--screen", "`The action was done; do not repeat it`", "If it is not already in your context, read it before you start.", "`(the screen did not change in about 2 s after the action …)` means the tap did nothing visible: look before you try again."} {
		if !strings.Contains(a, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
}
