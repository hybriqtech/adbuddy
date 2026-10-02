package claudecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const rec = "testdata/cc-2.1.286/"

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(rec + name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// --- hooks: payloads recorded from Claude Code 2.1.286 ---

func TestParseHookRecorded(t *testing.T) {
	cases := []struct {
		file, event string
		check       func(HookInput) bool
	}{
		{"hooks/bash-SessionStart.json", EventSessionStart, func(in HookInput) bool { return in.Source == "startup" && in.TranscriptPath != "" }},
		{"hooks/bash-UserPromptSubmit.json", EventUserPromptSubmit, func(in HookInput) bool { return strings.HasPrefix(in.Prompt, "Ejecuta con Bash") && in.PromptID != "" }},
		{"hooks/bash-Stop.json", EventStop, func(in HookInput) bool { return in.LastAssistantMessage == "hecho" }},
		{"hooks/bash-SessionEnd.json", EventSessionEnd, func(in HookInput) bool { return in.Reason == "other" }},
		{"hooks/denied-PermissionRequest.json", EventPermissionRequest, func(in HookInput) bool {
			return in.ToolName == "Bash" && ToolDetail(in.ToolName, in.ToolInput) == "echo dos > out2.txt" && in.PermissionMode == "default"
		}},
	}
	for _, c := range cases {
		in, err := ParseHook(c.event, readFixture(t, c.file))
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if in.Event != c.event || in.SessionID == "" || in.Cwd == "" || !c.check(in) {
			t.Errorf("%s: parsed wrong: %+v", c.file, in)
		}
	}
}

func TestParseHookRefusals(t *testing.T) {
	ok := `{"hook_event_name":"Stop","session_id":"3ee4bd10-5ae3-4f83-b02f-1c26acf6fa35"}`
	for name, c := range map[string]struct{ event, body string }{
		"unknown event":      {"Evil", ok},
		"other event":        {EventSessionStart, ok},
		"not json":           {EventStop, `{"hook_event_name":`},
		"no session id":      {EventStop, `{"hook_event_name":"Stop"}`},
		"session id garbage": {EventStop, `{"session_id":"../../etc"}`},
		"too large":          {EventStop, `{"session_id":"3ee4bd10-5ae3","prompt":"` + strings.Repeat("a", MaxHookInput) + `"}`},
	} {
		if _, err := ParseHook(c.event, []byte(c.body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := ParseHook(EventStop, []byte(ok)); err != nil {
		t.Errorf("the good one: %v", err)
	}
	// An absent hook_event_name is taken from the command line.
	if in, err := ParseHook(EventStop, []byte(`{"session_id":"3ee4bd10-5ae3"}`)); err != nil || in.Event != EventStop {
		t.Errorf("no event in payload: %v %+v", err, in)
	}
}

func TestParseHookCutsFreeText(t *testing.T) {
	body, _ := json.Marshal(map[string]string{"session_id": "3ee4bd10-5ae3", "last_assistant_message": strings.Repeat("é", maxText)})
	in, err := ParseHook(EventStop, body)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.LastAssistantMessage) > maxText+len("…") || !strings.HasSuffix(in.LastAssistantMessage, "…") || strings.ContainsRune(in.LastAssistantMessage, '\uFFFD') {
		t.Errorf("not cut at a character: %d bytes", len(in.LastAssistantMessage))
	}
}

func TestPermissionOutput(t *testing.T) {
	if PermissionOutput(NoDecision, "") != nil {
		t.Error("no decision must say nothing")
	}
	type out struct {
		H struct {
			Name string `json:"hookEventName"`
			D    struct {
				Behavior string `json:"behavior"`
				Message  string `json:"message"`
				Updated  []struct {
					Type        string `json:"type"`
					Behavior    string `json:"behavior"`
					Destination string `json:"destination"`
					Rules       []struct{ ToolName, RuleContent string }
				} `json:"updatedPermissions"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	dec := func(d Decision, rule string) out {
		var o out
		if err := json.Unmarshal(PermissionOutput(d, rule), &o); err != nil {
			t.Fatal(err)
		}
		return o
	}
	if o := dec(Allow, "ignored"); o.H.Name != "PermissionRequest" || o.H.D.Behavior != "allow" || len(o.H.D.Updated) != 0 {
		t.Errorf("allow: %+v", o)
	}
	if o := dec(Deny, ""); o.H.D.Behavior != "deny" || o.H.D.Message != DenyMessage {
		t.Errorf("deny: %+v", o)
	}
	o := dec(AllowAlways, "touch uno.txt")
	u := o.H.D.Updated
	if o.H.D.Behavior != "allow" || len(u) != 1 || u[0].Type != "addRules" || u[0].Behavior != "allow" || u[0].Destination != "localSettings" ||
		len(u[0].Rules) != 1 || u[0].Rules[0].ToolName != "Bash" || u[0].Rules[0].RuleContent != "touch uno.txt" {
		t.Errorf("allow always (the shape Claude Code accepted in the spike): %+v", o)
	}
	if o := dec(AllowAlways, ""); len(o.H.D.Updated) != 0 {
		t.Errorf("an always with no rule is a plain allow: %+v", o)
	}
}

// --- rules ---

func bash(cmd string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"command": cmd})
	return b
}

func TestAlwaysRule(t *testing.T) {
	ok := []string{"touch uno.txt", "npm test", "git status", "go test ./...", "make build", "ls -la src/main.go", "echo a,b=c:d+e", "pytest tests/ -k suite", "cat results.txt"}
	for _, c := range ok {
		if r, yes := AlwaysRule("Bash", bash(c)); !yes || r != c {
			t.Errorf("%q: %q %v", c, r, yes)
		}
	}
	if r, yes := AlwaysRule("Bash", bash("  git status  ")); !yes || r != "git status" {
		t.Errorf("trimmed: %q %v", r, yes)
	}
	refused := []string{
		"", "adb shell ls", "ADB devices", "/usr/bin/adb.exe push a b", "adbuddy send x", "ANDROID_SERIAL=x ls", "echo .adbuddy",
		"npm *", "npm test:*", "rm -rf (x)", `echo "x"`, "echo 'x'", "a; b", "a && b", "a | b", "echo $HOME", "echo `x`", "cat < f", "echo x > f",
		"line1\nline2", "a\tb", `a\b`, strings.Repeat("a", maxRule+1),
		// review I1: globs and expansions of bash that hide a word
		"a?b shell rm -rf /sdcard/DCIM", "a[d]b shell ls", ".a?buddy/bin/a?b shell ls", "a{d,e}b devices", "~/x", "echo !x", "echo #x", "ls {a,b}",
		// bidi, zero width, no-break and other non-ASCII spaces, controls
		"echo \u202eevil", "echo a\u2066b", "echo a\u200bb", "echo a\u00a0b", "echo a\u3000b", "echo a\x00b", "echo a\x1bb", "echo a\x7fb", "café",
		// what touches the phone without saying adb
		"fastboot erase userdata", "FASTBOOT reboot", "/opt/sdk/fastboot flash", "scrcpy", "./gradlew installDebug", "gradlew.bat installDebug", "gradle installDebug",
		"sudo ls", "su root", "/usr/bin/su -c x", "~/platform-tools/x", "echo adbd",
		// round 1, N2: other tools that reach the phone or root
		"flutter install", "npx react-native run-android", "npx cap run android", "bundletool install-apks --apks=a.apks", "./mvnw android:deploy",
		"npx expo run:ios", "doas ls", "pkexec ls", "run0 ls", "react-native start", "FLUTTER run",
		// round 1, N3: invisible fills and combining marks
		"echo a" + string(rune(0x3164)) + "b", "echo a" + string(rune(0x115f)) + "b", "echo a" + string(rune(0x1160)) + "b", "echo a" + string(rune(0xffa0)) + "b",
		"echo a" + string(rune(0x2800)) + "b", "echo a" + string(rune(0x34f)) + "b", "echo e" + string(rune(0x301)), "echo a" + string(rune(0x20dd)) + "b",
	}
	for _, c := range refused {
		if r, yes := AlwaysRule("Bash", bash(c)); yes {
			t.Errorf("%q offered as %q", c, r)
		}
	}
	for _, tool := range []string{"Edit", "Write", "Read", "WebFetch", "mcp__x__y", ""} {
		if _, yes := AlwaysRule(tool, bash("git status")); yes {
			t.Errorf("%s offered", tool)
		}
	}
	if _, yes := AlwaysRule("Bash", json.RawMessage(`not json`)); yes {
		t.Error("garbage input offered")
	}
}

func TestToolSummary(t *testing.T) {
	j := func(s string) json.RawMessage { return json.RawMessage(s) }
	for _, c := range []struct {
		tool, in, want string
	}{
		{"Bash", `{"command":"echo hola > out.txt","description":"Write \"hola\" to out.txt"}`, `Write "hola" to out.txt`},
		{"Bash", `{"command":"git\n  status"}`, "git status"},
		{"Read", `{"file_path":"C:\\Users\\u\\proj\\main.go"}`, "Read main.go"},
		{"Edit", `{"file_path":"/home/u/proj/a/b.kt"}`, "Edit b.kt"},
		{"Write", `{"file_path":"x.md"}`, "Write x.md"},
		{"Grep", `{"pattern":"TODO"}`, "Search TODO"},
		{"Glob", `{"pattern":"**/*.go"}`, "Find **/*.go"},
		{"WebFetch", `{"url":"https://User:pw@Example.com/a/b?q=1"}`, "Fetch example.com"},
		{"mcp__github__create_issue", `{"title":"secret"}`, "github: create_issue"},
		{"TodoWrite", `{"todos":[1]}`, "TodoWrite"},
		{"Bash", `garbage`, ""},
	} {
		if got := ToolSummary(c.tool, j(c.in)); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.tool, c.in, got, c.want)
		}
	}
	long := ToolSummary("Bash", bash(strings.Repeat("é", 500)))
	if r := []rune(long); len(r) != maxSummary || r[len(r)-1] != '…' {
		t.Errorf("long summary: %d runes", len(r))
	}
}

func TestToolDetail(t *testing.T) {
	if d := ToolDetail("Bash", bash("echo dos > out2.txt")); d != "echo dos > out2.txt" {
		t.Error(d)
	}
	if d := ToolDetail("Edit", json.RawMessage(`{"file_path":"/a/b.go","old_string":"x"}`)); d != "/a/b.go" {
		t.Error(d)
	}
	if d := ToolDetail("mcp__a__b", json.RawMessage(`{"x":1}`)); d != `{"x":1}` {
		t.Error(d)
	}
	if d := ToolDetail("Bash", bash(strings.Repeat("a", 5000))); len(d) > maxDetail+len("…") {
		t.Errorf("detail of %d bytes", len(d))
	}
}

func TestToolDetailShortcutsOnlyForFileAndFetchTools(t *testing.T) {
	// The path or the URL stands for the whole input only where it is the
	// whole point of the tool.
	for tool, in := range map[string]string{
		"Read":         `{"file_path":"/a/b.go"}`,
		"Edit":         `{"file_path":"/a/b.go","old_string":"x"}`,
		"MultiEdit":    `{"file_path":"/a/b.go","edits":[]}`,
		"Write":        `{"file_path":"/a/b.go","content":"x"}`,
		"NotebookEdit": `{"notebook_path":"/a/b.ipynb","new_source":"x"}`,
		"WebFetch":     `{"url":"https://example.com/x","prompt":"p"}`,
	} {
		want := map[string]string{"Read": "/a/b.go", "Edit": "/a/b.go", "MultiEdit": "/a/b.go", "Write": "/a/b.go", "NotebookEdit": "/a/b.ipynb", "WebFetch": "https://example.com/x"}[tool]
		if d := ToolDetail(tool, json.RawMessage(in)); d != want {
			t.Errorf("%s: %q, want %q", tool, d, want)
		}
	}
	// Any other tool shows its whole input, even with a url or a path in it.
	for _, in := range []string{
		`{"url":"https://example.com/x","method":"DELETE","body":"{}"}`,
		`{"file_path":"/a/b","mode":"overwrite"}`,
		`{"notebook_path":"/a/b.ipynb","x":1}`,
	} {
		if d := ToolDetail("mcp__http__request", json.RawMessage(in)); d != in {
			t.Errorf("mcp: %q, want the whole input", d)
		}
	}
}

func TestCardSafeOfAToolWhoseWholeInputDoesNotFit(t *testing.T) {
	long := `{"url":"https://example.com/x","method":"POST","body":"` + strings.Repeat("a", maxDetail) + `"}`
	if CardSafe("mcp__http__request", json.RawMessage(long)) {
		t.Error("a card that hides the body was offered")
	}
	if !CardSafe("mcp__http__request", json.RawMessage(`{"url":"https://example.com/x","method":"GET"}`)) {
		t.Error("a short input is shown whole")
	}
	if CardSafe("mcp__a__b", json.RawMessage(`{"file_path":"/a","body":"‮"}`)) {
		t.Error("bidi in the input")
	}
	// The file tools keep showing the path only.
	if !CardSafe("Write", json.RawMessage(`{"file_path":"/a/b.go","content":"`+strings.Repeat("a", 3*maxDetail)+`"}`)) {
		t.Error("the path of a Write is the detail")
	}
}

// --- transcript: recorded from Claude Code 2.1.286 ---

func feedAll(p *Parser, b []byte) []Event {
	var ev []Event
	for _, l := range bytes.Split(b, []byte("\n")) {
		ev = append(ev, p.Feed(l)...)
	}
	return ev
}

func TestParserRecordedBash(t *testing.T) {
	p := NewParser()
	ev := feedAll(p, readFixture(t, "transcript-bash.jsonl"))
	want := []Event{
		{Kind: KindMessage, Role: "user", Text: "Ejecuta con Bash el comando: echo hola > out.txt . Luego responde solo: hecho"},
		{Kind: KindTool, ToolID: "toolu_01E7HMb4B2wvhWq3sfoCMWVR", Tool: "Bash", Summary: `Write "hola" to out.txt`, Status: "running"},
		{Kind: KindTool, ToolID: "toolu_01E7HMb4B2wvhWq3sfoCMWVR", Tool: "Bash", Summary: `Write "hola" to out.txt`, Status: "done"},
		{Kind: KindMessage, Role: "assistant", Text: "hecho"},
	}
	if len(ev) != len(want) {
		t.Fatalf("%d events, want %d: %+v", len(ev), len(want), ev)
	}
	for i, w := range want {
		g := ev[i]
		g.MessageID, g.MessageOf, g.ToolKey = "", "", "" // checked below
		if g != w {
			t.Errorf("event %d: %+v, want %+v", i, g, w)
		}
	}
	if ev[1].ToolKey == "" || ev[2].ToolKey != "" {
		t.Errorf("only the start of a tool use carries its key: %q %q", ev[1].ToolKey, ev[2].ToolKey)
	}
	if ev[1].MessageOf == "" || ev[1].MessageOf != ev[2].MessageOf || !strings.HasPrefix(ev[1].MessageOf, "msg_") {
		t.Errorf("the tool use belongs to its message: %q %q", ev[1].MessageOf, ev[2].MessageOf)
	}
	if ev[3].MessageID == "" || ev[3].MessageID == ev[1].MessageOf {
		t.Errorf("the answer is another message: %q", ev[3].MessageID)
	}
	if p.Version() == "" || !Supported(p.Version()) {
		t.Errorf("version %q should be one of the tested ones", p.Version())
	}
	if p.BadRun() != 0 {
		t.Error("bad run")
	}
}

func TestParserRecordedDenied(t *testing.T) {
	ev := feedAll(NewParser(), readFixture(t, "transcript-denied.jsonl"))
	var kinds []string
	for _, e := range ev {
		kinds = append(kinds, e.Status+e.Role)
	}
	// prompt, tool running, tool error (denied), then the thinking line gives nothing and the text line the answer
	if got := strings.Join(kinds, ","); got != "user,running,error,assistant" {
		t.Fatalf("events %s", got)
	}
	if !strings.HasPrefix(ev[3].Text, "No he podido hacerlo") {
		t.Errorf("answer: %q", ev[3].Text)
	}
}

func line(typ, uuid string, msg any, extra map[string]any) []byte {
	m := map[string]any{"type": typ, "uuid": uuid, "message": msg}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return b
}

func asst(id string, blocks ...map[string]any) any {
	return map[string]any{"id": id, "role": "assistant", "content": blocks}
}

func TestParserJoinsTheLinesOfOneMessage(t *testing.T) {
	p := NewParser()
	a := p.Feed(line("assistant", "u1", asst("msg_1", map[string]any{"type": "text", "text": "Primero."}), nil))
	b := p.Feed(line("assistant", "u2", asst("msg_1", map[string]any{"type": "text", "text": "Segundo."}), nil))
	if len(a) != 1 || a[0].Text != "Primero." || len(b) != 1 || b[0].MessageID != "msg_1" || b[0].Text != "Primero.\n\nSegundo." {
		t.Errorf("%+v %+v", a, b)
	}
	// The same line read twice says nothing new.
	if ev := p.Feed(line("assistant", "u2", asst("msg_1", map[string]any{"type": "text", "text": "Segundo."}), nil)); len(ev) != 0 {
		t.Errorf("a repeated line: %+v", ev)
	}
}

func TestParserIgnores(t *testing.T) {
	p := NewParser()
	for name, l := range map[string][]byte{
		"thinking":       line("assistant", "a", asst("m", map[string]any{"type": "thinking", "thinking": "secret", "signature": "x"}), nil),
		"empty text":     line("assistant", "b", asst("m2", map[string]any{"type": "text", "text": "  "}), nil),
		"sidechain":      line("assistant", "c", asst("m3", map[string]any{"type": "text", "text": "sub"}), map[string]any{"isSidechain": true}),
		"meta":           line("user", "d", map[string]any{"role": "user", "content": "meta"}, map[string]any{"isMeta": true}),
		"attachment":     []byte(`{"type":"attachment","uuid":"e","attachment":{"type":"date"}}`),
		"unknown type":   []byte(`{"type":"brand-new","uuid":"f"}`),
		"slash command":  line("user", "g", map[string]any{"role": "user", "content": "<command-name>/clear</command-name>"}, nil),
		"local stdout":   line("user", "h", map[string]any{"role": "user", "content": "<local-command-stdout></local-command-stdout>"}, nil),
		"blank":          []byte("   "),
		"interrupt text": line("user", "i", map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "[Request interrupted]"}}}, nil),
		"bad message":    []byte(`{"type":"assistant","uuid":"j","message":"nope"}`),
		"bad content":    []byte(`{"type":"assistant","uuid":"k","message":{"id":"m9","content":"nope"}}`),
	} {
		if ev := p.Feed(l); len(ev) != 0 {
			t.Errorf("%s: %+v", name, ev)
		}
	}
	if p.BadRun() != 0 {
		t.Errorf("JSON of an unknown shape is not a bad line: %d", p.BadRun())
	}
}

func TestParserNeverReadsAToolResult(t *testing.T) {
	p := NewParser()
	p.Feed(line("assistant", "a", asst("m", map[string]any{"type": "tool_use", "id": "t1", "name": "Read", "input": map[string]any{"file_path": "/x/y.txt"}}), nil))
	ev := p.Feed(line("user", "b", map[string]any{"role": "user", "content": []map[string]any{
		{"type": "tool_result", "tool_use_id": "t1", "content": "SECRET CONTENT", "is_error": false},
	}}, map[string]any{"toolUseResult": map[string]any{"stdout": "SECRET"}}))
	if len(ev) != 1 || ev[0].Status != "done" || ev[0].Summary != "Read y.txt" || ev[0].Tool != "Read" {
		t.Fatalf("%+v", ev)
	}
	if b, _ := json.Marshal(ev); bytes.Contains(b, []byte("SECRET")) {
		t.Error("the content of a result reached an event")
	}
}

func TestParserBadRunAndForgetting(t *testing.T) {
	p := NewParser()
	for i := 0; i < 3; i++ {
		p.Feed([]byte("{not json"))
	}
	if p.BadRun() != 3 {
		t.Errorf("bad run %d", p.BadRun())
	}
	p.Feed([]byte(`{"type":"x"}`))
	if p.BadRun() != 0 {
		t.Error("a good line resets the run")
	}
}

func TestParserForgetsOnlyTheOldest(t *testing.T) {
	prompt := func(i int) []byte {
		return line("user", fmt.Sprintf("p%d", i), map[string]any{"role": "user", "content": fmt.Sprintf("prompt %d", i)}, nil)
	}
	// Lines: the oldest uuid is forgotten past keepLines, the newest is kept.
	p := NewParser()
	n := keepLines + 10
	for i := 0; i < n; i++ {
		p.Feed(prompt(i))
	}
	if len(p.lines) != keepLines {
		t.Errorf("%d lines kept, want %d", len(p.lines), keepLines)
	}
	if ev := p.Feed(prompt(n - 1)); len(ev) != 0 {
		t.Errorf("the newest line was forgotten: %+v", ev)
	}
	if ev := p.Feed(prompt(0)); len(ev) != 1 {
		t.Errorf("the oldest line should be forgotten: %+v", ev)
	}

	// Messages: the text of the oldest is forgotten past keepMessages.
	p = NewParser()
	text := func(i int, part string) []byte {
		return line("assistant", fmt.Sprintf("u%d%s", i, part), asst(fmt.Sprintf("m%d", i), map[string]any{"type": "text", "text": part}), nil)
	}
	nm := keepMessages + 5
	for i := 0; i < nm; i++ {
		p.Feed(text(i, "uno"))
	}
	if len(p.texts) != keepMessages {
		t.Errorf("%d messages kept, want %d", len(p.texts), keepMessages)
	}
	if ev := p.Feed(text(nm-1, "dos")); len(ev) != 1 || ev[0].Text != "uno\n\ndos" {
		t.Errorf("the newest message keeps its text: %+v", ev)
	}
	if ev := p.Feed(text(0, "dos")); len(ev) != 1 || ev[0].Text != "dos" {
		t.Errorf("the oldest message is forgotten: %+v", ev)
	}

	// Tools: the name of the oldest tool use is forgotten past keepTools.
	p = NewParser()
	nt := keepTools + 5
	for i := 0; i < nt; i++ {
		p.Feed(line("assistant", fmt.Sprintf("a%d", i), asst(fmt.Sprintf("x%d", i), map[string]any{"type": "tool_use", "id": fmt.Sprintf("t%d", i), "name": "Read", "input": map[string]any{"file_path": "/x/y.txt"}}), nil))
	}
	if len(p.tools) != keepTools {
		t.Errorf("%d tools kept, want %d", len(p.tools), keepTools)
	}
	result := func(i int) []byte {
		return line("user", fmt.Sprintf("r%d", i), map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": fmt.Sprintf("t%d", i)}}}, nil)
	}
	if ev := p.Feed(result(nt - 1)); len(ev) != 1 || ev[0].Tool != "Read" {
		t.Errorf("the newest tool is kept: %+v", ev)
	}
	if ev := p.Feed(result(0)); len(ev) != 1 || ev[0].Tool != "" || ev[0].Status != "done" {
		t.Errorf("the oldest tool is forgotten: %+v", ev)
	}
}

func TestParserPromptIDIsTheLineUUID(t *testing.T) {
	l := line("user", "uuid-1", map[string]any{"role": "user", "content": "escrito en el PC"}, nil)
	a := NewParser().Feed(l)
	b := NewParser().Feed(l) // a reader restarted over the same transcript
	if len(a) != 1 || a[0].MessageID != "uuid-1" || len(b) != 1 || b[0].MessageID != a[0].MessageID {
		t.Errorf("%+v %+v", a, b)
	}
}

func TestParserOnlySyntaxErrorsAreBadLines(t *testing.T) {
	p := NewParser()
	for _, l := range []string{`{"type":"user","version":2}`, `[]`, `"text"`, `{"type":5}`, `{"isMeta":"yes"}`, `null`} {
		if ev := p.Feed([]byte(l)); len(ev) != 0 {
			t.Errorf("%s: %+v", l, ev)
		}
		if p.BadRun() != 0 {
			t.Errorf("%s counts as a bad line: %d", l, p.BadRun())
		}
	}
	p.Feed([]byte(`{"type":`))
	if p.BadRun() != 1 {
		t.Errorf("a truncated JSON is a bad line: %d", p.BadRun())
	}
}

func TestParserCapsAMessageBeforeJoining(t *testing.T) {
	p := NewParser()
	big := strings.Repeat("a", maxMessageText/4)
	var last []Event
	for i := 0; i < 20; i++ {
		last = p.Feed(line("assistant", fmt.Sprintf("u%d", i), asst("m", map[string]any{"type": "text", "text": big}), nil))
	}
	total := 0
	for _, s := range p.texts["m"] {
		total += len(s)
	}
	if total > maxMessageText {
		t.Errorf("%d bytes accumulated, limit %d", total, maxMessageText)
	}
	if len(last) != 0 && len(last[0].Text) > maxMessageText+len("…") {
		t.Errorf("event text %d", len(last[0].Text))
	}
}

func TestParserCRLF(t *testing.T) {
	p := NewParser()
	l := append(line("user", "c1", map[string]any{"role": "user", "content": "hola"}, nil), '\r')
	if ev := p.Feed(l); len(ev) != 1 || ev[0].Text != "hola" || p.BadRun() != 0 {
		t.Errorf("%+v %d", ev, p.BadRun())
	}
}

func TestSupported(t *testing.T) {
	for v, want := range map[string]bool{"": true, "2.1.286": true, "2.1.0": true, "2.2.0": false, "3.0.0": false, "1.9": false} {
		if Supported(v) != want {
			t.Errorf("%q: %v", v, !want)
		}
	}
}

// --- tail ---

func write(t *testing.T, path string, mode int, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|mode, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// tailerAt follows path with its own directory as projects/.
func tailerAt(path string, fromEnd bool) *Tailer {
	return NewTailer(Transcript{filepath.Dir(path), filepath.Base(path)}, fromEnd)
}

func textLine(uuid, id, text string) string {
	return string(line("assistant", uuid, asst(id, map[string]any{"type": "text", "text": text}), nil)) + "\n"
}

func TestTailerWaitsForTheFileAndForCompleteLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	tl := tailerAt(path, false)
	if ev, err := tl.Poll(); err != nil || len(ev) != 0 || tl.Started() {
		t.Fatalf("no file yet: %v %v %v", ev, err, tl.Started())
	}
	l1 := textLine("u1", "m1", "uno")
	l2 := textLine("u2", "m2", "dos")
	write(t, path, os.O_TRUNC, l1+l2[:10])
	ev, err := tl.Poll()
	if err != nil || len(ev) != 1 || ev[0].Text != "uno" || !tl.Started() || !tl.Seen() {
		t.Fatalf("first poll: %v %v", ev, err)
	}
	if ev, _ := tl.Poll(); len(ev) != 0 {
		t.Fatalf("nothing new: %v", ev)
	}
	write(t, path, os.O_APPEND, l2[10:])
	if ev, _ := tl.Poll(); len(ev) != 1 || ev[0].Text != "dos" {
		t.Fatalf("the rest of the line: %v", ev)
	}
}

func TestTailerFromEndSkipsTheHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "viejo"))
	tl := tailerAt(path, true)
	if ev, _ := tl.Poll(); len(ev) != 0 {
		t.Fatalf("history: %v", ev)
	}
	write(t, path, os.O_APPEND, textLine("u2", "m2", "nuevo"))
	if ev, _ := tl.Poll(); len(ev) != 1 || ev[0].Text != "nuevo" {
		t.Fatalf("%v", ev)
	}
}

func TestTailerStartsAgainWhenTheFileShrinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "uno")+textLine("u2", "m2", "dos"))
	tl := tailerAt(path, false)
	if ev, _ := tl.Poll(); len(ev) != 2 {
		t.Fatal(ev)
	}
	write(t, path, os.O_TRUNC, textLine("u3", "m3", "tres"))
	if ev, _ := tl.Poll(); len(ev) != 1 || ev[0].Text != "tres" {
		t.Fatalf("after truncation: %v", ev)
	}
}

func TestTailerSkipsAHugeLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	huge := `{"type":"attachment","x":"` + strings.Repeat("a", MaxLine+10) + "\"}\n"
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "antes")+huge+textLine("u2", "m2", "despues"))
	tl := tailerAt(path, false)
	var texts []string
	for i := 0; i < 8; i++ { // the file is read in chunks
		ev, err := tl.Poll()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ev {
			texts = append(texts, e.Text)
		}
	}
	if strings.Join(texts, ",") != "antes,despues" {
		t.Errorf("%v", texts)
	}
	if tl.Parser().BadRun() != 0 {
		t.Error("a skipped line is not a bad one")
	}
}

func TestTailerCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	l := strings.TrimSuffix(textLine("u1", "m1", "uno"), "\n") + "\r\n"
	write(t, path, os.O_TRUNC, l+strings.TrimSuffix(textLine("u2", "m2", "dos"), "\n")+"\r\n")
	tl := tailerAt(path, false)
	ev, _ := tl.Poll()
	if len(ev) != 2 || ev[1].Text != "dos" || tl.Parser().BadRun() != 0 {
		t.Fatalf("%v", ev)
	}
}

func TestTailerStartsAgainWhenTheFileIsReplacedByALargerOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "uno"))
	tl := tailerAt(path, false)
	if ev, _ := tl.Poll(); len(ev) != 1 {
		t.Fatal(ev)
	}
	// Another file takes its place, larger than the offset.
	other := filepath.Join(dir, "new.jsonl")
	write(t, other, os.O_TRUNC, textLine("u2", "m2", "dos")+textLine("u3", "m3", "tres"))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(other, path); err != nil {
		t.Fatal(err)
	}
	ev, err := tl.Poll()
	if err != nil || len(ev) != 2 || ev[0].Text != "dos" || ev[1].Text != "tres" {
		t.Fatalf("after replacement: %v %v", ev, err)
	}
}

func TestTailerRefusesALinkWhenReopening(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "uno"))
	tl := tailerAt(path, false)
	if ev, _ := tl.Poll(); len(ev) != 1 {
		t.Fatal(ev)
	}
	target := filepath.Join(dir, "secret.txt")
	write(t, target, os.O_TRUNC, textLine("u9", "m9", "secreto")+textLine("u8", "m8", "mas"))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if ev, err := tl.Poll(); err == nil || len(ev) != 0 {
		t.Fatalf("a link was read: %v %v", ev, err)
	}
}

func TestTailerFromEndDropsTheRestOfALineInProgress(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	l2 := textLine("u2", "m2", "a medias")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "viejo")+l2[:15])
	tl := tailerAt(path, true)
	if ev, _ := tl.Poll(); len(ev) != 0 {
		t.Fatalf("history: %v", ev)
	}
	write(t, path, os.O_APPEND, l2[15:])
	if ev, _ := tl.Poll(); len(ev) != 0 || tl.Parser().BadRun() != 0 || tl.Seen() {
		t.Fatalf("the rest of the line was read: %v bad=%d", ev, tl.Parser().BadRun())
	}
	write(t, path, os.O_APPEND, textLine("u3", "m3", "nuevo"))
	if ev, _ := tl.Poll(); len(ev) != 1 || ev[0].Text != "nuevo" {
		t.Fatalf("%v", ev)
	}
}

func TestTailerFromEndOnACompleteLineKeepsTheNext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "viejo"))
	tl := tailerAt(path, true)
	tl.Poll()
	write(t, path, os.O_APPEND, textLine("u2", "m2", "nuevo"))
	if ev, _ := tl.Poll(); len(ev) != 1 || ev[0].Text != "nuevo" {
		t.Fatalf("%v", ev)
	}
}

func TestTailerRefusesADirectory(t *testing.T) {
	if _, err := tailerAt(t.TempDir(), false).Poll(); err == nil {
		t.Error("a directory was read as a transcript")
	}
}

// --- paths ---

func TestValidTranscript(t *testing.T) {
	cfg := t.TempDir()
	proj := filepath.Join(cfg, "projects", "p1")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(proj, "s.jsonl")
	write(t, good, os.O_TRUNC, "")
	if p, err := ValidTranscript(good, cfg); err != nil || p.String() != good {
		t.Errorf("existing: %q %v", p, err)
	}
	if _, err := ValidTranscript(filepath.Join(proj, "later.jsonl"), cfg); err != nil {
		t.Errorf("not yet written: %v", err)
	}
	if _, err := ValidTranscript(filepath.Join(cfg, "projects", "new-dir", "later.jsonl"), cfg); err != nil {
		t.Errorf("its directory does not exist yet: %v", err)
	}
	// Claude Code makes projects/ with its first write: at SessionStart it may not exist.
	fresh := t.TempDir()
	if _, err := ValidTranscript(filepath.Join(fresh, "projects", "p", "s.jsonl"), fresh); err != nil {
		t.Errorf("no projects directory yet: %v", err)
	}
	if _, err := ValidTranscript(filepath.Join(fresh, "elsewhere", "s.jsonl"), fresh); err == nil {
		t.Error("outside, with no projects directory")
	}
	outside := filepath.Join(t.TempDir(), "x.jsonl")
	write(t, outside, os.O_TRUNC, "")
	for name, p := range map[string]string{
		"outside":     outside,
		"not jsonl":   filepath.Join(proj, "s.txt"),
		"relative":    "s.jsonl",
		"dotdot":      filepath.Join(proj, "..", "..", "..", "x.jsonl"),
		"dir":         proj,
		"empty":       "",
		"config only": filepath.Join(cfg, "s.jsonl"),
	} {
		if _, err := ValidTranscript(p, cfg); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	link := filepath.Join(proj, "link.jsonl")
	if err := os.Symlink(outside, link); err == nil {
		if _, err := ValidTranscript(link, cfg); err == nil {
			t.Error("a link out of the projects directory was accepted")
		}
	} // on Windows without the privilege a link cannot be made: the other cases hold
}

// V24 with symbolic links (the junctions are in paths_windows_test.go): a
// directory under projects/ linked out of it is refused, also when it appears
// after the check; a linked projects/ is followed.
func TestTranscriptLinksUnderAndAboveProjects(t *testing.T) {
	out := t.TempDir()
	write(t, filepath.Join(out, "s.jsonl"), os.O_TRUNC, textLine("u9", "m9", "secreto"))
	cfg := t.TempDir()
	p1 := filepath.Join(cfg, "projects", "p1")
	if err := os.MkdirAll(p1, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(cfg, "projects", "L")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	for _, name := range []string{"s.jsonl", "later.jsonl"} {
		if _, err := ValidTranscript(filepath.Join(cfg, "projects", "L", name), cfg); err == nil {
			t.Errorf("%s accepted through a linked directory", name)
		}
	}
	path := filepath.Join(p1, "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u1", "m1", "uno"))
	tr, err := ValidTranscript(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	tl := NewTailer(tr, false)
	if ev, err := tl.Poll(); err != nil || len(ev) != 1 {
		t.Fatalf("first poll: %v %v", ev, err)
	}
	if err := os.Rename(p1, p1+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, p1); err != nil {
		t.Fatal(err)
	}
	if ev, err := tl.Poll(); err == nil || len(ev) != 0 {
		t.Errorf("read through a directory linked after the check: %v %v", ev, err)
	}

	moved := t.TempDir()
	if err := os.MkdirAll(filepath.Join(moved, "p1"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg2 := t.TempDir()
	if err := os.Symlink(moved, filepath.Join(cfg2, "projects")); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(cfg2, "projects", "p1", "s.jsonl")
	write(t, path, os.O_TRUNC, textLine("u2", "m2", "dos"))
	tr, err = ValidTranscript(path, cfg2)
	if err != nil {
		t.Fatalf("linked projects/: %v", err)
	}
	if ev, err := NewTailer(tr, false).Poll(); err != nil || len(ev) != 1 || ev[0].Text != "dos" {
		t.Errorf("linked projects/: %v %v", ev, err)
	}
}

func TestInsideAndConfigDir(t *testing.T) {
	d := t.TempDir()
	if !Inside(d, d) || !Inside(d, filepath.Join(d, "a", "b")) || Inside(d, filepath.Dir(d)) || Inside(filepath.Join(d, "a"), filepath.Join(d, "ab")) {
		t.Error("Inside")
	}
	env := func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return "/custom"
		}
		return ""
	}
	if c, _ := ConfigDir(env, func() (string, error) { return "/h", nil }); c != "/custom" {
		t.Error(c)
	}
	if c, _ := ConfigDir(func(string) string { return "" }, func() (string, error) { return "/h", nil }); c != filepath.Join("/h", ".claude") {
		t.Error(c)
	}
}

func TestCardSafe(t *testing.T) {
	js := func(cmd, desc string) json.RawMessage {
		b, _ := json.Marshal(map[string]string{"command": cmd, "description": desc})
		return b
	}
	ok := []json.RawMessage{
		js("touch uno.txt", "do it"),
		js("echo a\n\tb", "multi\nline"),
		js("echo café", "añade café"),
		js(strings.Repeat("a", maxDetail), ""),
	}
	for i, in := range ok {
		if !CardSafe("Bash", in) {
			t.Errorf("ok[%d] refused", i)
		}
	}
	bad := map[string]json.RawMessage{
		"too long":          js(strings.Repeat("a", maxDetail+1), ""),
		"long, tail hidden": js(strings.Repeat("a", maxDetail)+"; rm -rf ~", ""),
		"escape":            js("echo a\x1b[2Jb", ""),
		"cr":                js("echo a\rb", ""),
		"nul":               js("echo a\x00b", ""),
		"bidi":              js("echo \u202eb", ""),
		"zero width":        js("echo a\u200bb", ""),
		"nbsp":              js("echo a\u00a0b", ""),
		"line sep":          js("echo a\u2028b", ""),
		"bidi in desc":      js("ls", "safe \u202e"),
		"control in desc":   js("ls", "a\x07b"),
		"invalid utf8":      json.RawMessage("{\"command\":\"a\xffb\"}"),
	}
	for name, in := range bad {
		if CardSafe("Bash", in) {
			t.Errorf("%s: accepted", name)
		}
	}
	hide := map[string]string{
		"long run of spaces": "npm test" + strings.Repeat(" ", 300) + "&& curl x|sh",
		"nine blanks":        "a" + strings.Repeat(" ", 9) + "b",
		"mixed blanks":       "a" + strings.Repeat(" \t", 5) + "b",
		"trailing blank":     "ls \nrm x",
		"trailing at end":    "ls ",
		"trailing tab":       "ls\t\nx",
	}
	for name, c := range hide {
		if CardSafe("Bash", js(c, "")) {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, c := range []string{"a" + strings.Repeat(" ", 8) + "b", "if x; then\n\tls\nfi\n", "echo a\n\n  b"} {
		if !CardSafe("Bash", js(c, "")) {
			t.Errorf("%q refused", c)
		}
	}
	for _, r := range []rune{0x3164, 0x115f, 0x1160, 0xffa0, 0x2800, 0x34f, 0x301, 0x20dd} {
		if CardSafe("Bash", js("echo a"+string(r)+"b", "")) {
			t.Errorf("U+%04X accepted", r)
		}
		if CardSafe("Bash", js("ls", "a"+string(r)+"b")) {
			t.Errorf("U+%04X accepted in the title", r)
		}
	}
	long, _ := json.Marshal(map[string]string{"file_path": "/" + strings.Repeat("a", maxDetail)})
	if CardSafe("Write", long) {
		t.Error("a path that does not fit was accepted")
	}
	if !CardSafe("Write", json.RawMessage(`{"file_path":"/tmp/a.txt","content":"x\u202ey"}`)) {
		t.Error("what the card does not show must not matter")
	}
	if CardSafe("Grep", json.RawMessage(`{"pattern":"a\u202eb"}`)) {
		t.Error("bidi in the pattern of the title was accepted")
	}
}

// V18: what the card cannot show as it is (private use, unassigned, object
// replacement, noncharacters, surrogates, invalid bytes) goes to the terminal;
// text with a glyph does not.
func TestCardSafeUnicodeTheCardCannotShow(t *testing.T) {
	js := func(cmd, desc string) json.RawMessage {
		b, _ := json.Marshal(map[string]string{"command": cmd, "description": desc})
		return b
	}
	for _, r := range []rune{0xe000, 0xf8ff, 0x378, 0xfffc, 0xfffe, 0xfdd0, 0x10ffff, 0xf0000, 0xd800} {
		if shownRune(r) || shown("a"+string(r)+"b") {
			t.Errorf("U+%04X shown", r)
		}
		if CardSafe("Bash", js("echo a"+string(r)+"b", "")) {
			t.Errorf("U+%04X accepted in the command", r)
		}
		if CardSafe("Bash", js("ls", "a"+string(r)+"b")) {
			t.Errorf("U+%04X accepted in the description", r)
		}
	}
	if shown("a\xffb") || CardSafe("Bash", json.RawMessage("{\"command\":\"a\xffb\"}")) {
		t.Error("invalid bytes accepted")
	}
	for _, s := range []string{"é", "日本語", "Ж", "😀", "ñ", "\t", "\n", " ", "echo é日Ж😀ñ\n\tx"} {
		if !shown(s) {
			t.Errorf("%q refused", s)
		}
		if !CardSafe("Bash", js("x"+s+"y", "x"+s+"y")) {
			t.Errorf("%q refused by CardSafe", s)
		}
	}
}

// V04: the card of a Bash shows the command and the description only. A field
// that changes how it runs where the card does not say (out of the sandbox, in
// the background, or one this host does not know) leaves the question on the
// terminal: a "yes" would approve the whole input.
func TestCardSafeBashFieldsTheCardDoesNotShow(t *testing.T) {
	ok := []string{
		`{"command":"ls","description":"list"}`,
		`{"command":"ls","description":"list","timeout":600000}`,
		`{"command":"ls","run_in_background":false,"dangerouslyDisableSandbox":false}`,
		`{"command":"ls","run_in_background":null}`,
	}
	for _, in := range ok {
		if !CardSafe("Bash", json.RawMessage(in)) {
			t.Errorf("%s refused", in)
		}
	}
	bad := []string{
		`{"command":"ls","description":"list","dangerouslyDisableSandbox":true}`,
		`{"command":"ls","description":"list","run_in_background":true}`,
		`{"command":"ls","run_in_background":true,"dangerouslyDisableSandbox":true}`,
		`{"command":"ls","dangerouslyDisableSandbox":"true"}`,
		`{"command":"ls","run_in_background":1}`,
		`{"command":"ls","DangerouslyDisableSandbox":true}`,
		`{"command":"ls","sandbox":{"network":"any"}}`,
		`{"command":"ls","shell":""}`,
	}
	for _, in := range bad {
		if CardSafe("Bash", json.RawMessage(in)) {
			t.Errorf("%s accepted", in)
		}
	}
	// Tools whose card shows the whole input as JSON show these fields too.
	if !CardSafe("PowerShell", json.RawMessage(`{"command":"ls","run_in_background":true}`)) {
		t.Error("a tool whose whole input is the detail was refused")
	}
}

func TestParserVersionIsKeptOnlyIfItLooksLikeOne(t *testing.T) {
	long := "2." + strings.Repeat("1", 40)
	for in, want := range map[string]string{
		"2.1.286":      "2.1.286",
		"3.0.0-rc.1+b": "3.0.0-rc.1+b",
		"":             "",
		"2.1 286":      "",
		"2.1\n.286":    "",
		"2.1.‮1":       "",
		"<b>2</b>":     "",
		long:           "",
	} {
		p := NewParser()
		p.Feed(line("assistant", "u1", asst("msg_1", map[string]any{"type": "text", "text": "x"}), map[string]any{"version": in}))
		if p.Version() != want {
			t.Errorf("version %q -> %q, want %q", in, p.Version(), want)
		}
	}
	// A later bad version does not erase the earlier valid one.
	p := NewParser()
	p.Feed(line("assistant", "u1", asst("msg_1", map[string]any{"type": "text", "text": "x"}), map[string]any{"version": "2.1.286"}))
	p.Feed(line("assistant", "u2", asst("msg_2", map[string]any{"type": "text", "text": "y"}), map[string]any{"version": "bad version"}))
	if p.Version() != "2.1.286" || p.BadVersion() {
		t.Errorf("version %q bad %v", p.Version(), p.BadVersion())
	}
	// With no valid version yet, a bad one is remembered as bad; a valid one clears it.
	p = NewParser()
	p.Feed(line("assistant", "u1", asst("msg_1", map[string]any{"type": "text", "text": "x"}), map[string]any{"version": "bad version"}))
	if p.Version() != "" || !p.BadVersion() {
		t.Errorf("version %q bad %v", p.Version(), p.BadVersion())
	}
	p.Feed(line("assistant", "u2", asst("msg_2", map[string]any{"type": "text", "text": "y"}), map[string]any{"version": "2.1.9"}))
	if p.Version() != "2.1.9" || p.BadVersion() {
		t.Errorf("version %q bad %v", p.Version(), p.BadVersion())
	}
}

func TestParserVersionLengthLimit(t *testing.T) {
	for n, ok := range map[int]bool{32: true, 33: false} {
		v := strings.Repeat("1", n)
		p := NewParser()
		p.Feed(line("assistant", "u1", asst("msg_1", map[string]any{"type": "text", "text": "x"}), map[string]any{"version": v}))
		if (p.Version() == v) != ok || p.BadVersion() == ok {
			t.Errorf("%d chars: version %q bad %v", n, p.Version(), p.BadVersion())
		}
	}
}

func TestParserToolIdsMustBePlain(t *testing.T) {
	bad := "tool use\n\u202e"
	p := NewParser()
	ev := p.Feed(line("assistant", "u1", asst("msg_1",
		map[string]any{"type": "text", "text": "hola"},
		map[string]any{"type": "tool_use", "id": bad, "name": "Bash", "input": map[string]any{"command": "ls"}},
		map[string]any{"type": "tool_use", "id": "toolu_ok", "name": "Read", "input": map[string]any{"file_path": "/a/b.go"}}), nil))
	if len(ev) != 2 || ev[0].Kind != KindMessage || ev[1].ToolID != "toolu_ok" {
		t.Fatalf("a tool with a bad id is ignored, the rest of the line is not: %+v", ev)
	}
	res := func(uuid, id string) []byte {
		return line("user", uuid, map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": id}}}, nil)
	}
	if ev := p.Feed(res("r1", bad)); len(ev) != 0 {
		t.Errorf("a result with a bad tool_use_id is ignored: %+v", ev)
	}
	if ev := p.Feed(res("r2", "toolu_ok")); len(ev) != 1 || ev[0].Status != "done" {
		t.Errorf("%+v", ev)
	}
}

func TestParserCountsAssistantMessagesWithoutAnId(t *testing.T) {
	p := NewParser()
	if ev := p.Feed(line("assistant", "bad id", asst("bad id", map[string]any{"type": "text", "text": "x"}), nil)); len(ev) != 0 || p.Dropped() != 1 {
		t.Errorf("%+v dropped %d", ev, p.Dropped())
	}
	// message.id bad, uuid good: one id for all the blocks of the line.
	ev := p.Feed(line("assistant", "u-ok", asst("bad id", map[string]any{"type": "text", "text": "a"}, map[string]any{"type": "text", "text": "b"}), nil))
	if len(ev) != 1 || ev[0].MessageID != "u-ok" || ev[0].Text != "a\n\nb" || p.Dropped() != 1 {
		t.Errorf("%+v dropped %d", ev, p.Dropped())
	}
}

func TestParserIgnoresIdsThatAreNotPlainTokens(t *testing.T) {
	bad := []string{"a b", "a\nb", "a/b", "a‮b", strings.Repeat("a", 65), "ñ"}
	for _, id := range bad {
		// a user prompt: no id, the adapter makes a random one
		ev := NewParser().Feed(line("user", id, map[string]any{"role": "user", "content": "hola"}, nil))
		if len(ev) != 1 || ev[0].MessageID != "" {
			t.Errorf("user uuid %q: %+v", id, ev)
		}
		// an assistant message: its own id is ignored, the line's uuid stands in
		ev = NewParser().Feed(line("assistant", "u-ok", asst(id, map[string]any{"type": "text", "text": "x"}), nil))
		if len(ev) != 1 || ev[0].MessageID != "u-ok" {
			t.Errorf("message.id %q: %+v", id, ev)
		}
		// neither is usable: the message is not shown
		ev = NewParser().Feed(line("assistant", id, asst(id, map[string]any{"type": "text", "text": "x"}), nil))
		if len(ev) != 0 {
			t.Errorf("both %q: %+v", id, ev)
		}
	}
	// A bad uuid does not make two lines count as the same one.
	p := NewParser()
	a := p.Feed(line("user", "a b", map[string]any{"role": "user", "content": "uno"}, nil))
	b := p.Feed(line("user", "a b", map[string]any{"role": "user", "content": "dos"}, nil))
	if len(a) != 1 || len(b) != 1 {
		t.Errorf("%+v %+v", a, b)
	}
	if ev := NewParser().Feed(line("user", strings.Repeat("a", 64), map[string]any{"role": "user", "content": "hola"}, nil)); len(ev) != 1 || len(ev[0].MessageID) != 64 {
		t.Errorf("a 64-char id is fine: %+v", ev)
	}
}

// V04, round 1: a file tool's card shows the path, not the content. A write to
// what configures the agent or git (rules, hooks, MCP servers) could approve,
// unseen, what skips every later question: it is asked on the terminal.
func TestCardSafeFileToolsOnTheAgentsConfig(t *testing.T) {
	in := func(key, p string) json.RawMessage {
		b, _ := json.Marshal(map[string]string{key: p, "content": "x"})
		return b
	}
	bad := []string{
		"/home/u/proj/.claude/settings.local.json",
		".claude/hooks/x.sh",
		"/home/u/proj/.git/hooks/pre-commit",
		"/home/u/proj/.mcp.json",
		".mcp.json",
		"sub/.Claude/x",
		"/home/u/.CLAUDE/settings.json",
		"src/../.claude/settings.json",
		"/home/u/proj/.claude/../x.go", // .claude may be a link: refused before cleaning too
		`C:\Users\u\proj\.claude\settings.json`,
		`C:\Users\u\proj\.GIT\config`,
		`C:\Users\u\proj\sub\..\.mcp.JSON`,
		`C:\Users\u\proj\.claude.\settings.json`, // Windows drops trailing dots and spaces
		`C:\Users\u\proj\.git \config`,
		`C:\Users\u\proj\.git::$INDEX_ALLOCATION\config`, // a stream of the folder
		`C:\Users\u\proj\CLAUDE~1\settings.json`,         // the 8.3 name of .claude
		`C:\Users\u\proj\GIT~1\config`,
		`C:\Users\u\proj\MCP~1.JSO`,
	}
	for _, tool := range []string{"Edit", "MultiEdit", "Write", "NotebookEdit"} {
		key := "file_path"
		if tool == "NotebookEdit" {
			key = "notebook_path"
		}
		for _, p := range bad {
			if CardSafe(tool, in(key, p)) {
				t.Errorf("%s %s accepted", tool, p)
			}
		}
		for _, p := range []string{"/home/u/proj/src/main.go", `C:\Users\RUNNER~1\proj\a.ipynb`, "/home/u/proj/claude/x.md", "/home/u/proj/.github/workflows/ci.yml", "/home/u/proj/docs/mcp.json", "/home/u/proj/.gitignore"} {
			if !CardSafe(tool, in(key, p)) {
				t.Errorf("%s %s refused", tool, p)
			}
		}
	}
	// Reading them changes nothing: Read keeps its card.
	if !CardSafe("Read", in("file_path", "/home/u/proj/.claude/settings.json")) {
		t.Error("Read of the config refused")
	}
}

// The summary Claude Code writes after /compact is a "user" line with
// isCompactSummary: it is not something the person typed. This is the minimum
// record; the exact fields of a real one are still to be recorded (/compact in
// the plan's manual validations).
func TestParserIgnoresTheCompactSummary(t *testing.T) {
	p := NewParser()
	rec := []byte(`{"type":"user","uuid":"c1","isCompactSummary":true,"message":{"role":"user","content":"This session is being continued from a previous conversation."}}`)
	if ev := p.Feed(rec); len(ev) != 0 {
		t.Fatalf("the summary of /compact is shown as typed in the terminal: %+v", ev)
	}
	typed := []byte(`{"type":"user","uuid":"c2","isCompactSummary":false,"message":{"role":"user","content":"hola"}}`)
	if ev := p.Feed(typed); len(ev) != 1 || ev[0].Text != "hola" {
		t.Fatalf("a normal prompt is lost: %+v", ev)
	}
}

// Two readers of one transcript (a second SessionStart, the host restarted)
// must give the same ids in the same order: the chat's ids are the same on every
// read (spec M4 §4.2), so that what the phone already has is replaced, not
// repeated.
func TestTwoTailersOfOneTranscriptGiveTheSameIDsInTheSameOrder(t *testing.T) {
	for _, name := range []string{"transcript-bash.jsonl", "transcript-denied.jsonl"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.jsonl")
			body := string(readFixture(t, name)) +
				string(line("user", "typed1", map[string]any{"role": "user", "content": "otra cosa en la terminal"}, nil)) + "\n" +
				textLine("late1", "msg_late", "Fin.")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			read := func() []string {
				tl := tailerAt(path, false)
				ev, err := tl.Poll()
				if err != nil {
					t.Fatal(err)
				}
				var ids []string
				for _, e := range ev {
					ids = append(ids, fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s", e.Kind, e.MessageID, e.ToolID, e.Status, e.MessageOf, e.Role, e.ToolKey))
				}
				return ids
			}
			a, b := read(), read()
			if len(a) < 4 {
				t.Fatalf("only %d events: %v", len(a), a)
			}
			if strings.Join(a, "\n") != strings.Join(b, "\n") {
				t.Fatalf("two readers differ:\n%v\n%v", a, b)
			}
			for _, id := range a {
				if strings.Contains(id, "|||||") {
					t.Errorf("an event with no id at all: %q", id)
				}
			}
		})
	}
}
