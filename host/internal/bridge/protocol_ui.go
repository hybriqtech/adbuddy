package bridge

import (
	"encoding/json"
	"time"
)

// Helper and remote mode payloads (protocol spec §8): M5. M5's own spec
// fixes each op's args and result.

// UIRequest asks the app to act on the screen. It is a stream message: never
// resent after a reconnection, and dropped by the app once expired.
type UIRequest struct {
	ID        string          `json:"id"`
	Op        string          `json:"op"` // dump, click, setText, foreground or notifications
	Args      json.RawMessage `json:"args"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

// UIResponse answers a UIRequest: Result when OK, Error when not.
type UIResponse struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *ErrorReply     `json:"error,omitempty"`
}

// DeviceInfo is what the app knows of how to reach the phone away from home,
// and the protected apps the host's guard refuses to act on (M5 spec §4.3).
type DeviceInfo struct {
	TailscaleIP   string   `json:"tailscaleIp,omitempty"`
	WifiADBPort   int      `json:"wifiAdbPort,omitempty"`
	ProtectedApps []string `json:"protectedApps,omitempty"`
}

// RemoteSet turns remote mode on (adb tcpip 5555) or off (adb usb).
type RemoteSet struct {
	Enabled bool `json:"enabled"`
}

// RemoteState is the remote mode as the host has it.
type RemoteState struct {
	Enabled   bool      `json:"enabled"`
	Address   string    `json:"address,omitempty"` // ip:port while enabled
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
}
