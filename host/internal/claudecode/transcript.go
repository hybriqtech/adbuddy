package claudecode

import (
	"encoding/json"
	"errors"
	"strings"
)

// The transcript is Claude Code's JSONL log of a session. Its format is not a
// public API (spec v1 §5.4): everything that reads it is in this file and
// tail.go. Recorded in testdata/cc-2.1.286/transcript-*.jsonl.

// TestedVersions are the prefixes of the Claude Code versions the format was
// recorded from. A version outside them still parses, but the adapter does
// not trust it (spec M4 §4.4).
var TestedVersions = []string{"2.1."}

// Supported reports whether a Claude Code version is one the format was
// recorded from. An unknown version ("") is supported: nothing says it is not.
func Supported(version string) bool {
	if version == "" {
		return true
	}
	for _, p := range TestedVersions {
		if strings.HasPrefix(version, p) {
			return true
		}
	}
	return false
}

// Kind is the sort of an Event.
type Kind int

const (
	// KindMessage is the assistant's text so far, or a prompt typed in the
	// terminal. A message of an ID already seen replaces the one before.
	KindMessage Kind = iota + 1
	// KindTool is a tool use that begins (running) or ends (done, error).
	KindTool
)

// Event is what a line of the transcript means for the chat.
type Event struct {
	Kind Kind

	// KindMessage. For a prompt typed in the terminal MessageID is the uuid
	// of its line (stable across readers), or "" if the line had none.
	MessageID string
	Role      string // assistant, or user for a prompt typed in the terminal
	Text      string

	// KindTool
	ToolID    string
	Tool      string // the tool's name (on the end of one whose start was seen too)
	Summary   string
	Status    string // running, done or error
	MessageOf string // the message the tool use belongs to
}

// Limits of what the parser keeps; the transcript has no bound of its own.
const (
	maxMessageText = 256 << 10
	keepMessages   = 64
	keepTools      = 256
	keepLines      = 512
)

// Parser turns lines of a transcript into Events. It keeps what it needs to
// join the lines of one message and to name a tool's end: bounded, and
// forgotten as it grows. Not safe for concurrent use.
type Parser struct {
	version string
	badRun  int

	texts     map[string][]string // message id → the text blocks seen, in order
	textOrder []string
	tools     map[string]toolInfo // tool_use id → its summary
	toolOrder []string
	lines     map[string]bool // uuids of the lines already read: a line read twice means nothing new
	lineOrder []string
}

type toolInfo struct{ name, summary, messageID string }

// NewParser returns an empty Parser.
func NewParser() *Parser {
	return &Parser{texts: map[string][]string{}, tools: map[string]toolInfo{}, lines: map[string]bool{}}
}

// Version is the Claude Code version of the last line that said one.
func (p *Parser) Version() string { return p.version }

// BadRun is how many lines in a row were not JSON.
func (p *Parser) BadRun() int { return p.badRun }

type rawLine struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	Version     string          `json:"version"`
	IsSidechain bool            `json:"isSidechain"`
	IsMeta      bool            `json:"isMeta"`
	Message     json.RawMessage `json:"message"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

// ignoredPrompts are the prefixes of "prompts" that Claude Code writes for
// its own commands, not typed text [validar with a real /clear, /compact, !cmd].
var ignoredPrompts = []string{"<command-", "<local-command", "<system-reminder", "<bash-", "<user-prompt"}

// Feed reads one line (without its newline). A line that is not JSON counts
// toward BadRun; one that is JSON but not of a shape this parser knows is
// ignored without counting.
func (p *Parser) Feed(line []byte) []Event {
	line = trimSpace(line)
	if len(line) == 0 {
		return nil
	}
	var l rawLine
	if err := json.Unmarshal(line, &l); err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			p.badRun++
			return nil
		}
		// Valid JSON whose field has another type than this parser knows:
		// not a bad line, just one it does not understand.
		p.badRun = 0
		return nil
	}
	p.badRun = 0
	if l.Version != "" {
		p.version = l.Version
	}
	if l.IsSidechain || l.IsMeta || (l.Type != "assistant" && l.Type != "user") {
		return nil
	}
	if l.UUID != "" {
		if p.lines[l.UUID] {
			return nil
		}
		p.lines[l.UUID] = true
		p.lineOrder = append(p.lineOrder, l.UUID)
		if len(p.lineOrder) > keepLines {
			delete(p.lines, p.lineOrder[0])
			p.lineOrder = p.lineOrder[1:]
		}
	}
	var m rawMessage
	if json.Unmarshal(l.Message, &m) != nil {
		return nil
	}
	if l.Type == "assistant" {
		return p.assistant(l, m)
	}
	return p.user(l, m)
}

func (p *Parser) assistant(l rawLine, m rawMessage) []Event {
	var blocks []rawBlock
	if json.Unmarshal(m.Content, &blocks) != nil {
		return nil
	}
	id := m.ID
	if id == "" {
		id = l.UUID
	}
	if id == "" {
		return nil
	}
	var out []Event
	grew := false
	for _, b := range blocks {
		switch b.Type {
		case "text": // "thinking" and the rest are not shown
			if strings.TrimSpace(b.Text) == "" {
				continue
			}
			if !p.addText(id, b.Text) {
				continue // the message is full: the rest is not kept
			}
			grew = true
		case "tool_use":
			if b.ID == "" {
				continue
			}
			sum := ToolSummary(b.Name, b.Input)
			p.addTool(b.ID, toolInfo{b.Name, sum, id})
			out = append(out, Event{Kind: KindTool, ToolID: b.ID, Tool: b.Name, Summary: sum, Status: "running", MessageOf: id})
		}
	}
	if grew {
		text := cut(strings.Join(p.texts[id], "\n\n"), maxMessageText)
		out = append([]Event{{Kind: KindMessage, MessageID: id, Role: "assistant", Text: text}}, out...)
	}
	return out
}

func (p *Parser) user(l rawLine, m rawMessage) []Event {
	var prompt string
	if json.Unmarshal(m.Content, &prompt) == nil {
		t := strings.TrimSpace(prompt)
		if t == "" {
			return nil
		}
		for _, pre := range ignoredPrompts {
			if strings.HasPrefix(t, pre) {
				return nil
			}
		}
		return []Event{{Kind: KindMessage, MessageID: l.UUID, Role: "user", Text: cut(prompt, maxMessageText)}}
	}
	var blocks []rawBlock
	if json.Unmarshal(m.Content, &blocks) != nil {
		return nil
	}
	var out []Event
	for _, b := range blocks {
		if b.Type != "tool_result" || b.ToolUseID == "" {
			continue // the result's own text is never read (spec M4 §4.2)
		}
		st := "done"
		if b.IsError {
			st = "error"
		}
		ti := p.tools[b.ToolUseID]
		out = append(out, Event{Kind: KindTool, ToolID: b.ToolUseID, Tool: ti.name, Summary: ti.summary, Status: st, MessageOf: ti.messageID})
	}
	return out
}

// addText keeps a block of text of message id, up to maxMessageText bytes
// in all; it reports whether anything was added.
func (p *Parser) addText(id, text string) bool {
	have := 0
	for _, t := range p.texts[id] {
		have += len(t)
	}
	if have >= maxMessageText {
		return false
	}
	if room := maxMessageText - have; len(text) > room {
		text = strings.ToValidUTF8(text[:room], "")
	}
	if _, ok := p.texts[id]; !ok {
		p.textOrder = append(p.textOrder, id)
		if len(p.textOrder) > keepMessages {
			delete(p.texts, p.textOrder[0])
			p.textOrder = p.textOrder[1:]
		}
	}
	p.texts[id] = append(p.texts[id], text)
	return true
}

func (p *Parser) addTool(id string, ti toolInfo) {
	if _, ok := p.tools[id]; !ok {
		p.toolOrder = append(p.toolOrder, id)
		if len(p.toolOrder) > keepTools {
			delete(p.tools, p.toolOrder[0])
			p.toolOrder = p.toolOrder[1:]
		}
	}
	p.tools[id] = ti
}

func trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t' || b[len(b)-1] == '\r' || b[len(b)-1] == '\n') {
		b = b[:len(b)-1]
	}
	return b
}
