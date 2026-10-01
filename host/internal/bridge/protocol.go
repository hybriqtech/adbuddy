// Package bridge is the channel between the host and the phone app (spec
// §5.2-5.3): a WebSocket server on 127.0.0.1 that the app reaches through
// `adb reverse`, authenticated with a token the host hands over by adb.
package bridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ProtocolVersion is the "v" of every message and the /v1 path.
const ProtocolVersion = 1

// BufferSize is how many events the host keeps to resend after a reconnection.
const BufferSize = 2000

// MinAppProtocol is the oldest app protocol this host talks to. An older app
// is closed with CloseIncompatible, so it can tell the person to update it
// instead of misreading messages.
const MinAppProtocol = 1

// CloseIncompatible is the WebSocket close code for an app that is too old.
const CloseIncompatible = 4001

// MaxRID is the longest request id (protocol spec §3).
const MaxRID = 64

// ValidRID reports whether rid is a request id: 1 to MaxRID ASCII letters,
// digits, "_" or "-". A message from the app with any other rid is dropped.
func ValidRID(rid string) bool {
	if rid == "" || len(rid) > MaxRID {
		return false
	}
	for _, c := range rid {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// Message is the envelope of every message in both directions.
type Message struct {
	V    int    `json:"v"`
	Seq  uint64 `json:"seq,omitempty"` // events only (host → app); 0 for replies and streams
	Type string `json:"type"`
	// RID is the request id an app request carries, copied into its reply,
	// its error or the event that confirms it (protocol spec §3).
	RID     string          `json:"rid,omitempty"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload"`

	// conn is the connection an app message came on: Reply answers on that
	// one only. Not part of the JSON.
	conn uint64
}

// Class is how the host sends a message type (protocol spec §3). HostClasses
// gives each type's.
type Class int

const (
	// ClassEvent is numbered, kept and resent after a reconnection: Publish.
	ClassEvent Class = iota + 1
	// ClassReply answers a request, by its rid, on the connection that sent
	// it: Reply.
	ClassReply
	// ClassStream goes to the current connection only and is never resent: Send.
	ClassStream
	// ClassHandshake is the welcome, written by the server itself.
	ClassHandshake
)

// Message types (protocol spec docs/superpowers/specs/2026-10-01-adbuddy-protocolo-m3-m5-design.md).
// HostPayloads and AppPayloads list each one with its payload.
const (
	// M2
	TypeHello        = "hello"
	TypeWelcome      = "welcome"
	TypeSnapshot     = "snapshot"
	TypeSessionState = "session.state"
	TypeConfirmReq   = "confirm.request"
	TypeConfirmReply = "confirm.reply"
	TypeJournalEntry = "journal.entry"
	TypeNotice       = "notice"
	TypePause        = "pause"
	TypeResume       = "resume"

	// Common (§4)
	TypeError = "error"

	// Session and chat (§5): M3, M4
	TypeSessionNew   = "session.new"
	TypeSessionStop  = "session.stop"
	TypeInterrupt    = "agent.interrupt"
	TypeChatSend     = "chat.send"
	TypeChatMessage  = "chat.message"
	TypeToolActivity = "tool.activity"
	TypePermRequest  = "permission.request"
	TypePermReply    = "permission.reply"
	TypePermResolved = "permission.resolved"

	// Attachments and files (§6): M3
	TypeAttachBegin  = "attach.begin"
	TypeAttachReady  = "attach.ready"
	TypeAttachEnd    = "attach.end"
	TypeAttachDone   = "attach.done"
	TypeFileGet      = "file.get"
	TypeFileEnd      = "file.end"
	TypeFileCancel   = "file.cancel"
	TypeCaptureReq   = "capture.request"
	TypeCaptureReady = "capture.ready"

	// Terminal (§7): M3
	TypeTerminalWatch    = "terminal.watch"
	TypeTerminalUnwatch  = "terminal.unwatch"
	TypeTerminalSnapshot = "terminal.snapshot"
	TypeTerminalOutput   = "terminal.output"
	TypeTerminalSize     = "terminal.size"
	TypeTerminalInput    = "terminal.input"
	TypeTerminalResize   = "terminal.resize"

	// Helpers and remote mode (§8): M5
	TypeUIRequest   = "ui.request"
	TypeUIResponse  = "ui.response"
	TypeDeviceInfo  = "device.info"
	TypeRemoteSet   = "remote.set"
	TypeRemoteState = "remote.state"
)

// Hello is the first message of the app.
type Hello struct {
	Protocol     int    `json:"protocol"` // the app's protocol version (ProtocolVersion when both match)
	Token        string `json:"token"`
	LastSeq      uint64 `json:"lastSeq"`
	Session      string `json:"session,omitempty"` // the session of the last welcome: another one means the host restarted
	AppVersion   string `json:"appVersion"`
	DeviceSerial string `json:"deviceSerial"`
}

// Welcome answers a valid hello. FromSeq is the first event that follows.
type Welcome struct {
	Protocol    int    `json:"protocol"` // the host's protocol version; the app checks its own minimum
	Session     string `json:"session"`
	FromSeq     uint64 `json:"fromSeq"`
	HostVersion string `json:"hostVersion"`
}

// SessionState is the state the app shows in its header.
type SessionState struct {
	Agent    string `json:"agent"`
	State    string `json:"state"` // idle, working, waiting or stopped
	Activity string `json:"activity,omitempty"`
	Paused   bool   `json:"paused"`
	// Conversation changes with each session.new and each relaunch (M3).
	Conversation string `json:"conversation,omitempty"`
	View         string `json:"view,omitempty"` // chat (an adapter) or terminal (M3)
	PCAttached   bool   `json:"pcAttached"`     // a PC terminal is attached (M3)
}

// ConfirmRequest asks the app to allow or deny a 🔴 command (spec §4.5).
// Title and Detail come from the rules, never from the agent.
type ConfirmRequest struct {
	ID           string    `json:"id"`
	Level        string    `json:"level"`
	Title        string    `json:"title"`
	Detail       string    `json:"detail"`
	RawCommand   string    `json:"rawCommand"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Special      bool      `json:"special"`
	AllowSession bool      `json:"allowSession"` // whether "allow during the session" may be offered
}

// ConfirmReply is the app's answer.
type ConfirmReply struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // allow, deny or allow_session
}

// Snapshot replaces the events the app missed when there are too many
// (protocol spec §4.3). The terminal is not in it: it has its own stream.
type Snapshot struct {
	State       SessionState        `json:"state"`
	Pending     []ConfirmRequest    `json:"pending"`
	Journal     []json.RawMessage   `json:"journal"`     // the last journal entries, oldest first
	Chat        []ChatMessage       `json:"chat"`        // the last 100 chat messages, oldest first (M3)
	Permissions []PermissionRequest `json:"permissions"` // the agent's questions waiting for an answer (M4)
	Remote      RemoteState         `json:"remote"`      // M5
}

// filled returns s with empty lists instead of nil ones: the app reads a
// list, never null.
func (s Snapshot) filled() Snapshot {
	if s.Pending == nil {
		s.Pending = []ConfirmRequest{}
	}
	if s.Journal == nil {
		s.Journal = []json.RawMessage{}
	}
	if s.Chat == nil {
		s.Chat = []ChatMessage{}
	}
	if s.Permissions == nil {
		s.Permissions = []PermissionRequest{}
	}
	return s
}

// Notice is a message for the person ("disconnected from the agent"…).
type Notice struct {
	Level string `json:"level"` // info, warning or error
	Text  string `json:"text"`
}

// ErrorReply answers a request (by its rid) that failed (protocol spec §4.1).
type ErrorReply struct {
	Code string `json:"code"` // one of the Code* constants
	Text string `json:"text"` // for the person, in English; written by the host, never by the agent
}

// Error codes (protocol spec §4.1 and §8.1).
const (
	CodeUnsupported = "unsupported"
	CodeInvalid     = "invalid"
	CodeNotFound    = "not_found"
	CodeNoAgent     = "no_agent"
	CodeBusy        = "busy"
	CodeTooLarge    = "too_large"
	CodeChecksum    = "checksum"
	CodeFailed      = "failed"
	CodeUnavailable = "unavailable" // ui.response: the accessibility service is off
	CodeProtected   = "protected"   // ui.response: a protected app, or ADBuddy itself
	CodeExpired     = "expired"     // ui.response
	CodeAmbiguous   = "ambiguous"   // ui.response: click matched several elements
)

// Empty is the payload of the messages that carry nothing: {}.
type Empty struct{}

// NewToken returns a random 256-bit token in hex (spec §5.2).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
