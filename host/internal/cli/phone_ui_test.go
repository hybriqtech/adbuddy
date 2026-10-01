package cli

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// The helpers reach the app through the link: a ui.request goes out as a
// stream message, and the app's ui.response comes back to the caller.
func TestHelpersThroughTheLink(t *testing.T) {
	l, c := linkedApp(t)
	ui, protected := l.helpers()
	if ui == nil {
		t.Fatal("no way to the app")
	}
	type result struct {
		raw json.RawMessage
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := ui(context.Background(), bridge.UIOpForeground, json.RawMessage("{}"))
		done <- result{raw, err}
	}()
	m := readApp(t, c)
	if m.Type != bridge.TypeUIRequest || m.Seq != 0 || m.RID != "" {
		t.Fatalf("got %s seq %d rid %q", m.Type, m.Seq, m.RID)
	}
	var req bridge.UIRequest
	if err := json.Unmarshal(m.Payload, &req); err != nil || req.Op != bridge.UIOpForeground || time.Until(req.ExpiresAt) <= 0 {
		t.Fatalf("request %s (%v)", m.Payload, err)
	}
	sendApp(t, c, bridge.TypeUIResponse, "", bridge.UIResponse{ID: req.ID, OK: true, Result: json.RawMessage(`{"package":"com.android.settings","activity":"","protected":false}`)})
	if r := <-done; r.err != nil || string(r.raw) != `{"package":"com.android.settings","activity":"","protected":false}` {
		t.Fatalf("%s %v", r.raw, r.err)
	}

	// The protected apps the app reports.
	sendApp(t, c, bridge.TypeDeviceInfo, "", bridge.DeviceInfo{ProtectedApps: []string{"com.example.bank"}})
	waitUntil(t, "the list of protected apps", func() bool { return protected("com.example.bank") })
}
