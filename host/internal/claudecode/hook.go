// Package claudecode is what ADBuddy knows of Claude Code's own formats: the
// payloads of its hooks, what a hook answers, and its transcript (JSONL). It
// depends on nothing else of ADBuddy, so that the day Claude Code changes a
// format the damage is in this package (spec M4 §2). Every statement about a
// format has a case recorded from a real Claude Code in testdata/ (spike M4).
package claudecode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// The hook events ADBuddy installs (spec M4 §3.1).
const (
	EventSessionStart      = "SessionStart"
	EventUserPromptSubmit  = "UserPromptSubmit"
	EventStop              = "Stop"
	EventNotification      = "Notification"
	EventPermissionRequest = "PermissionRequest"
	EventSessionEnd        = "SessionEnd"
)

// Events is every event ADBuddy installs a hook for.
var Events = []string{
	EventSessionStart, EventUserPromptSubmit, EventStop,
	EventNotification, EventPermissionRequest, EventSessionEnd,
}

// IsEvent reports whether name is one of Events.
func IsEvent(name string) bool {
	return slices.Contains(Events, name)
}

// MaxHookInput bounds what a hook reads from stdin and the daemon accepts.
const MaxHookInput = 1 << 20

// maxText bounds the free text of a payload (a prompt, an answer).
const maxText = 256 << 10

// HookInput is what Claude Code writes on a hook's stdin. Only the fields
// ADBuddy uses; the rest is ignored (the format grows with each version).
// Recorded in testdata/cc-2.1.286/hooks. Everything here comes from a local
// process that is not to be trusted: see Valid.
type HookInput struct {
	Event          string `json:"hook_event_name"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PromptID       string `json:"prompt_id"`
	PermissionMode string `json:"permission_mode"`

	Source               string          `json:"source"`                 // SessionStart: startup, resume, clear, compact
	Prompt               string          `json:"prompt"`                 // UserPromptSubmit
	LastAssistantMessage string          `json:"last_assistant_message"` // Stop
	NotificationType     string          `json:"notification_type"`      // Notification [validar: not recorded]
	ToolName             string          `json:"tool_name"`              // PermissionRequest
	ToolInput            json.RawMessage `json:"tool_input"`             // PermissionRequest
	Reason               string          `json:"reason"`                 // SessionEnd
}

var sessionIDRe = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

// ParseHook reads the payload of a hook of the given event. The event comes
// from the command line (`adbuddy hook <event>`), and a payload that says
// another one is refused. Free text is cut at 256 KiB.
func ParseHook(event string, b []byte) (HookInput, error) {
	var in HookInput
	if !IsEvent(event) {
		return in, fmt.Errorf("unknown hook event %q", event)
	}
	if len(b) > MaxHookInput {
		return in, fmt.Errorf("hook payload of %d bytes exceeds %d", len(b), MaxHookInput)
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return in, fmt.Errorf("hook payload is not JSON: %w", err)
	}
	if in.Event != "" && in.Event != event {
		return in, fmt.Errorf("hook payload is for %q, not %q", in.Event, event)
	}
	in.Event = event
	if !sessionIDRe.MatchString(in.SessionID) {
		return in, fmt.Errorf("hook payload has no valid session_id")
	}
	in.Prompt = cut(in.Prompt, maxText)
	in.LastAssistantMessage = cut(in.LastAssistantMessage, maxText)
	return in, nil
}

// cut shortens s to at most n bytes at a character boundary, with an
// ellipsis when it cuts.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) { // inside a multi-byte character
		s = s[:len(s)-1]
	}
	return s + "…"
}

// Decision is what the person decides about a permission.
type Decision int

const (
	// NoDecision leaves the question to Claude Code's own dialog.
	NoDecision Decision = iota
	Allow
	Deny
	AllowAlways
)

// DenyMessage is what the model is told when the person denies from the
// phone [validar: how it reads to the model].
const DenyMessage = "Denied from the phone."

// PermissionOutput is the stdout of a PermissionRequest hook for a decision
// (recorded: the shape that Claude Code accepted in the spike). NoDecision
// is nil: a hook with no output leaves the dialog to Claude Code. rule is the
// exact command of an AllowAlways (AlwaysRule); an empty rule makes it a
// plain Allow.
func PermissionOutput(d Decision, rule string) []byte {
	type perm struct {
		ToolName    string `json:"toolName"`
		RuleContent string `json:"ruleContent"`
	}
	type update struct {
		Type        string `json:"type"`
		Rules       []perm `json:"rules"`
		Behavior    string `json:"behavior"`
		Destination string `json:"destination"`
	}
	type decision struct {
		Behavior           string   `json:"behavior"`
		Message            string   `json:"message,omitempty"`
		UpdatedPermissions []update `json:"updatedPermissions,omitempty"`
	}
	var dec decision
	switch d {
	case Allow:
		dec.Behavior = "allow"
	case AllowAlways:
		dec.Behavior = "allow"
		if rule != "" {
			dec.UpdatedPermissions = []update{{
				Type: "addRules", Rules: []perm{{ToolName: "Bash", RuleContent: rule}},
				Behavior: "allow", Destination: "localSettings",
			}}
		}
	case Deny:
		dec.Behavior = "deny"
		dec.Message = DenyMessage
	default:
		return nil
	}
	out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": EventPermissionRequest, "decision": dec,
	}})
	return out
}

// Normalize collapses every run of white space to one space and trims: how
// two texts are compared when one went through a terminal and the other did
// not.
func Normalize(s string) string { return strings.Join(strings.Fields(s), " ") }
