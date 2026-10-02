package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func geminiSettings(t *testing.T, content string) (path string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "settings.json")
	if content != "" {
		os.WriteFile(path, []byte(content), 0o600)
	}
	return path
}

func fileNames(t *testing.T, path string) []any {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal([]byte(read(t, path)), &s); err != nil {
		t.Fatal(err)
	}
	return s["context"].(map[string]any)["fileName"].([]any)
}

func TestGeminiSettingsAreCreated(t *testing.T) {
	p := geminiSettings(t, "")
	changed, created, err := mergeGeminiSettings(p)
	if err != nil || !changed || !created {
		t.Fatalf("changed=%v created=%v err=%v", changed, created, err)
	}
	if n := fileNames(t, p); len(n) != 2 || n[0] != "GEMINI.md" || n[1] != "AGENTS.md" {
		t.Fatalf("fileName %v", n)
	}
}

func TestGeminiSettingsKeepWhatTheUserHas(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []any
	}{
		"string":        {`{"model":"x","context":{"fileName":"GEMINI.md","discoveryMaxDirs":5}}`, []any{"GEMINI.md", "AGENTS.md"}},
		"list":          {`{"context":{"fileName":["A.md"]}}`, []any{"A.md", "AGENTS.md"}},
		"already there": {`{"context":{"fileName":["AGENTS.md","B.md"]}}`, []any{"AGENTS.md", "B.md"}},
		"empty context": {`{"context":{}}`, []any{"GEMINI.md", "AGENTS.md"}},
		"empty list":    {`{"context":{"fileName":[]}}`, []any{"GEMINI.md", "AGENTS.md"}},
		"empty string":  {`{"context":{"fileName":""}}`, []any{"GEMINI.md", "AGENTS.md"}},
		"empty entries": {`{"context":{"fileName":["","A.md"]}}`, []any{"A.md", "AGENTS.md"}},
		"no context":    {`{"model":"x"}`, []any{"GEMINI.md", "AGENTS.md"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			p := geminiSettings(t, c.in)
			changed, created, err := mergeGeminiSettings(p)
			if err != nil || created {
				t.Fatalf("created=%v err=%v", created, err)
			}
			wantChanged := name != "already there"
			if changed != wantChanged {
				t.Fatalf("changed=%v, want %v", changed, wantChanged)
			}
			got := fileNames(t, p)
			if len(got) != len(c.want) {
				t.Fatalf("fileName %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("fileName %v, want %v", got, c.want)
				}
			}
			var s map[string]any
			json.Unmarshal([]byte(read(t, p)), &s)
			if name == "string" && (s["model"] != "x" || s["context"].(map[string]any)["discoveryMaxDirs"] != float64(5)) {
				t.Fatalf("the user's other settings were lost: %v", s)
			}
		})
	}
}

func TestGeminiSettingsAreNeverClobbered(t *testing.T) {
	for name, in := range map[string]string{
		"invalid JSON":       "{oops",
		"not an object":      `[1]`,
		"context is a list":  `{"context":[]}`,
		"fileName is number": `{"context":{"fileName":3}}`,
		"fileName has 3":     `{"context":{"fileName":["A.md",3]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			p := geminiSettings(t, in)
			if _, _, err := mergeGeminiSettings(p); err == nil {
				t.Fatal("want an error")
			}
			if read(t, p) != in {
				t.Fatal("the file was changed")
			}
		})
	}
}

func TestInitConfiguresGeminiAndMentionsSkills(t *testing.T) {
	dir := t.TempDir()
	res, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")})
	if err != nil {
		t.Fatal(err)
	}
	if n := fileNames(t, filepath.Join(dir, ".gemini", "settings.json")); len(n) != 2 || n[0] != "GEMINI.md" || n[1] != "AGENTS.md" {
		t.Fatalf("fileName %v", n)
	}
	found := false
	for _, c := range res.Created {
		found = found || c == filepath.Join(".gemini", "settings.json")
	}
	if !found {
		t.Fatalf("Created %q lacks .gemini/settings.json", res.Created)
	}
	agents := read(t, filepath.Join(dir, "AGENTS.md"))
	for _, want := range []string{"## Skills", ".agents/skills/", "not an authorization"} {
		if !strings.Contains(agents, want) {
			t.Errorf("AGENTS.md lacks %q", want)
		}
	}
}

func TestInitLeavesNoTemporaryFilesInGemini(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".gemini"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".settings-") {
			t.Fatalf("temporary file left: %s", e.Name())
		}
	}
}

func TestInitFailsWithoutWritingWhenGeminiIsALink(t *testing.T) {
	dir := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, ".gemini")); err != nil {
		t.Skipf("no symlink privilege: %v", err)
	}
	if _, err := Init(Options{Dir: dir, Exe: fakeExe(t, "v1")}); err == nil {
		t.Fatal("want an error")
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatalf("something was written through the link: %v", entries)
	}
}
