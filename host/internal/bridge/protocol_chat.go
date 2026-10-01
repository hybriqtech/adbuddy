package bridge

import "time"

// Session and chat payloads (protocol spec §5): M3 and M4.

// Attachment is a file the host keeps: one uploaded from the phone, a
// capture or one the agent sends (protocol spec §4.2). The app fetches it
// with file.get and never sees a path on the PC.
type Attachment struct {
	ID     string `json:"id"` // 32 lower-case hex digits: the FrameID of its binary frames
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Thumb  string `json:"thumb,omitempty"` // base64 JPEG of at most 16 KiB, images only
}

// ChatMessage is one message of the chat. One with an id already seen
// replaces it (the transcript writes a message over several lines).
type ChatMessage struct {
	ID           string       `json:"id"`
	Role         string       `json:"role"` // user or assistant
	Text         string       `json:"text"` // markdown
	Attachments  []Attachment `json:"attachments"`
	Conversation string       `json:"conversation"`
}

// ChatSend is a message the person writes on the phone. ID, chosen by the
// app like a rid, becomes the ChatMessage's: the host drops a chat.send whose
// id it already has, so retrying never duplicates a message.
type ChatSend struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Attachments []string `json:"attachments"` // ids of attachments uploaded or captured
}

// SessionNew starts a new conversation, with another agent if given.
type SessionNew struct {
	Agent string `json:"agent,omitempty"`
}

// ToolActivity is what the agent is doing with a tool. One with an id
// already seen replaces it.
type ToolActivity struct {
	ID        string `json:"id"`
	MessageID string `json:"messageId,omitempty"`
	Summary   string `json:"summary"`
	Status    string `json:"status"` // running, done or error
}

// PermissionRequest is a question of the agent itself (Claude Code's
// PermissionRequest hook), not one of ADBuddy's confirmations.
type PermissionRequest struct {
	ID          string    `json:"id"`
	Tool        string    `json:"tool"`
	Summary     string    `json:"summary"`
	Detail      string    `json:"detail"`
	ExpiresAt   time.Time `json:"expiresAt"`
	AllowAlways bool      `json:"allowAlways"` // whether "yes, always" may be offered
}

// PermissionReply is the app's answer to a PermissionRequest.
type PermissionReply struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // allow, allow_always or deny
}

// PermissionResolved takes a PermissionRequest off the phone, wherever it
// was answered.
type PermissionResolved struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // allow, allow_always or deny
	By       string `json:"by"`       // phone, terminal, timeout or agent
}
