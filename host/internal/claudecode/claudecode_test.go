package claudecode

import (
	"bytes"
	"encoding/json"
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
		g.MessageID, g.MessageOf = "", "" // checked below
		if g != w {
			t.Errorf("event %d: %+v, want %+v", i, g, w)
		}
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
		"tool_result ok": line("user", "i", map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "[Request interrupted]"}}}, nil),
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
	// What it remembers is bounded.
	for i := 0; i < keepMessages*3; i++ {
		id := "m" + string(rune('A'+i%26)) + strings.Repeat("x", i)
		p.Feed(line("assistant", "u"+id, asst(id, map[string]any{"type": "text", "text": "t"}), nil))
	}
	if len(p.texts) > keepMessages {
		t.Errorf("%d messages kept", len(p.texts))
	}
	if len(p.lines) > keepLines {
		t.Errorf("%d lines kept", len(p.lines))
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

func textLine(uuid, id, text string) string {
	return string(line("assistant", uuid, asst(id, map[string]any{"type": "text", "text": text}), nil)) + "\n"
}

func TestTailerWaitsForTheFileAndForCompleteLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	tl := NewTailer(path, false)
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
	tl := NewTailer(path, true)
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
	tl := NewTailer(path, false)
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
	tl := NewTailer(path, false)
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

func TestTailerRefusesADirectory(t *testing.T) {
	if _, err := NewTailer(t.TempDir(), false).Poll(); err == nil {
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
	if p, err := ValidTranscript(good, cfg); err != nil || !Inside(filepath.Join(cfg, "projects"), p) {
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
