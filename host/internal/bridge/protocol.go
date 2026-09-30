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

// Message is the envelope of every message in both directions.
type Message struct {
	V       int             `json:"v"`
	Seq     uint64          `json:"seq,omitempty"` // host → app only; 0 for replies that are not events
	Type    string          `json:"type"`
	TS      time.Time       `json:"ts"`
	Payload json.RawMessage `json:"payload"`
}

// Message types (spec §5.3). Only the ones M2 uses; the rest arrive with M3-M5.
const (
	TypeHello         = "hello"
	TypeWelcome       = "welcome"
	TypeSnapshot      = "snapshot"
	TypeSessionState  = "session.state"
	TypeConfirmReq    = "confirm.request"
	TypeConfirmReply  = "confirm.reply"
	TypeJournalEntry  = "journal.entry"
	TypeNotice        = "notice"
	TypePause         = "pause"
	TypeResume        = "resume"
	TypeDeviceInfo    = "device.info"
	TypeSessionNew    = "session.new"
	TypeSessionStop   = "session.stop"
	TypeInterrupt     = "agent.interrupt"
	TypeChatSend      = "chat.send"
	TypeCaptureReq    = "capture.request"
	TypePermReply     = "permission.reply"
	TypeTerminalInput = "terminal.input"
	TypeUIResponse    = "ui.response"
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

// Snapshot replaces the events the app missed when there are too many.
type Snapshot struct {
	State   SessionState      `json:"state"`
	Pending []ConfirmRequest  `json:"pending"`
	Journal []json.RawMessage `json:"journal"` // the last journal entries, oldest first
}

// Notice is a message for the person ("disconnected from the agent"…).
type Notice struct {
	Level string `json:"level"` // info, warning or error
	Text  string `json:"text"`
}

// NewToken returns a random 256-bit token in hex (spec §5.2).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
