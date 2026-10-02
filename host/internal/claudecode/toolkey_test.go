package claudecode

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestToolKeyIsTheSameForTheHookAndTheTranscriptOfARealSession(t *testing.T) {
	in, err := ParseHook(EventPermissionRequest, readFixture(t, "hooks/denied-PermissionRequest.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := ToolKey(in.ToolName, in.ToolInput)
	if want == "" {
		t.Fatal("no key for the hook of a real session")
	}
	p := NewParser()
	var got []string
	for _, l := range bytes.Split(readFixture(t, "transcript-denied.jsonl"), []byte{'\n'}) {
		for _, e := range p.Feed(l) {
			if e.Kind == KindTool && e.Status == "running" {
				got = append(got, e.ToolKey)
			}
		}
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("the transcript's tool use has key %q, the hook's %q", got, want)
	}
}

func TestToolKeyTellsInputsApartAndIgnoresLayout(t *testing.T) {
	a := ToolKey("Bash", json.RawMessage(`{"command":"ls","description":"d"}`))
	b := ToolKey("Bash", json.RawMessage(`{ "description" : "d",  "command":"ls" }`))
	if a == "" || a != b {
		t.Errorf("same input, other layout: %q %q", a, b)
	}
	for name, k := range map[string]string{
		"other command": ToolKey("Bash", json.RawMessage(`{"command":"rm x","description":"d"}`)),
		"other tool":    ToolKey("PowerShell", json.RawMessage(`{"command":"ls","description":"d"}`)),
		"big numbers":   ToolKey("Bash", json.RawMessage(`{"n":9007199254740993}`)),
	} {
		if k == a {
			t.Errorf("%s has the same key", name)
		}
	}
	if ToolKey("Bash", json.RawMessage(`{"n":9007199254740993}`)) == ToolKey("Bash", json.RawMessage(`{"n":9007199254740992}`)) {
		t.Error("numbers went through float64")
	}
	for _, bad := range []string{``, `{`, `{"a":1} x`} {
		if k := ToolKey("Bash", json.RawMessage(bad)); k != "" {
			t.Errorf("%q has key %q: it could match anything", bad, k)
		}
	}
}
