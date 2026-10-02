package claudecode

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
)

func rawJSON(s string) json.RawMessage { return json.RawMessage(s) }

// M6c §3: the verb of a tool summary is in the language of the PC; what it
// works on is the agent's and is shown as it is.
func TestToolSummaryVerbInSpanishAndTheObjectUntouched(t *testing.T) {
	for _, c := range []struct {
		tool, in, en, es string
	}{
		{"Read", `{"file_path":"/home/ana/notas.txt"}`, "Read notas.txt", "Leer notas.txt"},
		{"Edit", `{"file_path":"/x/main.go"}`, "Edit main.go", "Editar main.go"},
		{"MultiEdit", `{"file_path":"/x/main.go"}`, "Edit main.go", "Editar main.go"},
		{"Write", `{"file_path":"/x/out.md"}`, "Write out.md", "Escribir out.md"},
		{"NotebookEdit", `{"notebook_path":"/x/n.ipynb"}`, "Edit n.ipynb", "Editar n.ipynb"},
		{"Grep", `{"pattern":"TODO"}`, "Search TODO", "Buscar TODO"},
		{"Glob", `{"pattern":"**/*.go"}`, "Find **/*.go", "Buscar ficheros **/*.go"},
		{"WebFetch", `{"url":"https://Example.com/a"}`, "Fetch example.com", "Descargar example.com"},
		{"WebSearch", `{"query":"adb wifi"}`, "Search the web: adb wifi", "Buscar en la web: adb wifi"},
	} {
		if got := ToolSummaryIn(i18n.EN, c.tool, rawJSON(c.in)); got != c.en {
			t.Errorf("%s en: %q, want %q", c.tool, got, c.en)
		}
		if got := ToolSummary(c.tool, rawJSON(c.in)); got != c.en {
			t.Errorf("%s ToolSummary must stay English: %q", c.tool, got)
		}
		if got := ToolSummaryIn(i18n.ES, c.tool, rawJSON(c.in)); got != c.es {
			t.Errorf("%s es: %q, want %q", c.tool, got, c.es)
		}
	}
	// What is not a verb of ours is the agent's, in any language.
	for _, lang := range []i18n.Lang{i18n.EN, i18n.ES} {
		if got := ToolSummaryIn(lang, "Bash", rawJSON(`{"description":"Run the tests"}`)); got != "Run the tests" {
			t.Errorf("%s: a Bash description is the agent's: %q", lang, got)
		}
		if got := ToolSummaryIn(lang, "mcp__github__create_issue", rawJSON(`{}`)); got != "github: create_issue" {
			t.Errorf("%s: an MCP tool: %q", lang, got)
		}
	}
	long := ToolSummaryIn(i18n.ES, "Grep", rawJSON(`{"pattern":"`+strings.Repeat("é", 500)+`"}`))
	if n := len([]rune(long)); n > maxSummary {
		t.Errorf("a summary of %d characters", n)
	}
}

// The parser reads the language once and the tool lines of the transcript
// carry the verb in it.
func TestParserSummaryFollowsItsLanguage(t *testing.T) {
	line := `{"type":"assistant","uuid":"u1","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{"file_path":"/a/b.txt"}}]}}`
	for lang, want := range map[i18n.Lang]string{i18n.EN: "Read b.txt", i18n.ES: "Leer b.txt"} {
		p := NewParser()
		p.SetLang(lang)
		evs := p.Feed([]byte(line))
		found := false
		for _, e := range evs {
			if e.Kind == KindTool {
				found = true
				if e.Summary != want {
					t.Errorf("%s: %q, want %q", lang, e.Summary, want)
				}
			}
		}
		if !found {
			t.Fatalf("%s: no tool event in %+v", lang, evs)
		}
	}
}
