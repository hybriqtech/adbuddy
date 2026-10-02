package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

var posted = time.Date(2026, 10, 1, 9, 58, 0, 0, time.UTC)

// bankCode is what the tests must never see leave the host.
const bankCode = "Your code is 482913"

func TestNotificationsByTheApp(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	a.results[bridge.UIOpNotifications] = bridge.UINotifications{Notifications: []bridge.UINotification{
		{Key: "0|com.whatsapp|1|null|10123", Package: "com.whatsapp", Title: "Ana", Text: "¿Comemos?", PostedAt: posted},
		{Key: "hidden-1", Package: "com.example.bank", PostedAt: posted, Ongoing: true},
	}}
	s, exit := e.helper(t, bridge.UIOpNotifications, struct{}{}, "adbuddy", "notifications")
	if exit.Code != 0 || len(s.data) != 1 {
		t.Fatalf("exit %+v data %q", exit, s.data)
	}
	var n bridge.UINotifications
	if err := json.Unmarshal(s.data[0], &n); err != nil || len(n.Notifications) != 2 || n.Notifications[0].Text != "¿Comemos?" || n.Notifications[1].Package != "com.example.bank" {
		t.Fatalf("answer %s (%v)", s.data[0], err)
	}
	j := e.last(t)
	if j.Level != "green" || j.Rule != "helper.notifications" || j.Status != journal.StatusOK || j.Reason != "via the app" ||
		j.Source != journal.SourceAgent || strings.Join(j.Argv, " ") != "adbuddy notifications" || j.Description != "Read the notifications" {
		t.Fatalf("journal %+v", j)
	}
	// It does not read the screen: the guard does not ask what is in front,
	// and adb is never used (no dumpsys notification).
	if got := a.called(); len(got) != 1 || !strings.HasPrefix(got[0], "notifications") || len(e.adb.ran()) != 0 {
		t.Fatalf("app %q adb %q", got, e.adb.ran())
	}
}

// The host empties what a protected app says even when the app did not: a
// list that changed, a bug, or an app that is not the real one.
func TestNotificationsAreHiddenAgainByTheHost(t *testing.T) {
	e := newEnv(t)
	a := e.withApp("com.example.bank")
	long := strings.Repeat("ñ", bridge.MaxNotificationText+10)
	many := []bridge.UINotification{
		{Key: "0|com.example.bank|7|otp-482913|10140", Package: "com.example.bank", Title: "Bank", Text: bankCode, PostedAt: posted},
		{Key: "0|tech.hybriq.adbuddy|3|null|10200", Package: "tech.hybriq.adbuddy", Title: "The agent wants to", Text: "adb shell reboot", PostedAt: posted},
		{Key: "k", Package: "not a package", Title: "x", Text: bankCode, PostedAt: posted},
		{Key: "0|com.whatsapp|1|null|1", Package: "com.whatsapp", Title: long, Text: long, PostedAt: posted},
	}
	for len(many) < bridge.MaxNotifications+5 {
		many = append(many, bridge.UINotification{Key: "n", Package: "com.whatsapp", Title: "t", Text: "x", PostedAt: posted})
	}
	a.results[bridge.UIOpNotifications] = bridge.UINotifications{Notifications: many}
	s, exit := e.helper(t, bridge.UIOpNotifications, struct{}{})
	if exit.Code != 0 {
		t.Fatalf("exit %+v", exit)
	}
	out := string(s.data[0])
	for _, secret := range []string{"482913", "otp-", "reboot", "The agent wants", "not a package"} {
		if strings.Contains(out, secret) {
			t.Fatalf("%q left the host: %s", secret, out)
		}
	}
	var n bridge.UINotifications
	if err := json.Unmarshal(s.data[0], &n); err != nil {
		t.Fatal(err)
	}
	if len(n.Notifications) != bridge.MaxNotifications || !n.Truncated {
		t.Fatalf("%d notifications, truncated %v", len(n.Notifications), n.Truncated)
	}
	if got := n.Notifications[0]; got.Package != "com.example.bank" || got.Title != "" || got.Text != "" || got.Key == "" {
		t.Fatalf("bank %+v", got)
	}
	if got := n.Notifications[1]; got.Package != "tech.hybriq.adbuddy" || got.Text != "" {
		t.Fatalf("adbuddy %+v", got)
	}
	if got := n.Notifications[2]; got.Package != "" || got.Text != "" {
		t.Fatalf("invalid package %+v", got)
	}
	if got := n.Notifications[3]; len([]rune(got.Title)) != bridge.MaxNotificationText || len([]rune(got.Text)) != bridge.MaxNotificationText {
		t.Fatalf("not cut: %d %d", len([]rune(got.Title)), len([]rune(got.Text)))
	}
}

func TestNotificationsAnEmptyListIsAnEmptyList(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpNotifications] = bridge.UINotifications{}
	s, exit := e.helper(t, bridge.UIOpNotifications, struct{}{})
	if exit.Code != 0 || !strings.Contains(string(s.data[0]), `"notifications":[]`) {
		t.Fatalf("exit %+v data %q", exit, s.data)
	}
}

// Without access there is no fallback: adb would give the text of every app.
func TestNotificationsHaveNoAdbFallback(t *testing.T) {
	grant := "Notification access is off: ask the user to allow ADBuddy in Settings > Notifications > Device & app notifications"
	for name, err := range map[string]error{
		"no access":     &UIError{Code: bridge.CodeUnavailable, Text: grant},
		"no app":        ErrNoApp,
		"service error": &UIError{Code: bridge.CodeFailed, Text: "boom"},
	} {
		e := newEnv(t)
		a := e.withApp()
		a.errs[bridge.UIOpNotifications] = err
		_, exit := e.helper(t, bridge.UIOpNotifications, struct{}{})
		if name == "no access" && (exit.Code != exitTransient || exit.ErrorCode != "UI_UNAVAILABLE" || !strings.Contains(exit.Error, "allow ADBuddy")) {
			t.Errorf("%s: %+v", name, exit)
		}
		if name == "no app" && (exit.Code != exitTransient || exit.ErrorCode != "UI_UNAVAILABLE" || !strings.Contains(exit.Error, "needs the ADBuddy app")) {
			t.Errorf("%s: %+v", name, exit)
		}
		if exit.Code == 0 || len(e.adb.ran()) != 0 {
			t.Errorf("%s: exit %+v, adb ran %q", name, exit, e.adb.ran())
		}
		if j := e.last(t); j.Status != journal.StatusError {
			t.Errorf("%s: journal %+v", name, j)
		}
	}
	e := newEnv(t)
	e.d.o.UI = nil // serve without the app link at all
	if _, exit := e.helper(t, bridge.UIOpNotifications, struct{}{}); exit.ErrorCode != "UI_UNAVAILABLE" || len(e.adb.ran()) != 0 {
		t.Fatalf("no link: %+v adb %q", exit, e.adb.ran())
	}
}

func TestNotificationsTimeout(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.errs[bridge.UIOpNotifications] = ErrUITimeout
	_, exit := e.helper(t, bridge.UIOpNotifications, struct{}{})
	if exit.ErrorCode != "UI_TIMEOUT" || strings.Contains(exit.Error, "may or may not") || len(e.adb.ran()) != 0 {
		t.Fatalf("%+v adb %q", exit, e.adb.ran())
	}
}

func TestNotificationsCheckTheirArgsAndTheAnswer(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	if _, exit := e.helper(t, bridge.UIOpNotifications, map[string]any{"all": true}); exit.Code != exitUsage || exit.ErrorCode != "UI_INVALID" {
		t.Fatalf("args: %+v", exit)
	}
	if len(a.called()) != 0 {
		t.Fatalf("the app was asked %q", a.called())
	}
	a.results[bridge.UIOpNotifications] = "not a result"
	if _, exit := e.helper(t, bridge.UIOpNotifications, struct{}{}); exit.ErrorCode != "UI_FAILED" {
		t.Fatalf("answer: %+v", exit)
	}
}

func TestNotificationsRespectTheDoors(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpNotifications] = bridge.UINotifications{}
	paused := true
	e.d.o.Paused = func() bool { return paused }
	if _, exit := e.helper(t, bridge.UIOpNotifications, struct{}{}); exit.Code != 126 || exit.ErrorCode != "UI_BLOCKED" || !strings.Contains(exit.Error, "Blocked by ADBuddy: paused") {
		t.Fatalf("paused: %+v", exit)
	}
	paused = false
	q := &question{title: "Reboot the phone", withdraw: func(error) {}}
	if busy := e.d.ask(q); busy != "" {
		t.Fatal(busy)
	}
	if _, exit := e.helper(t, bridge.UIOpNotifications, struct{}{}); exit.Code != 126 || !strings.Contains(exit.Error, `waiting to ask about "Reboot the phone"`) {
		t.Fatalf("question: %+v", exit)
	}
	if got := a.called(); len(got) != 0 {
		t.Fatalf("the app was asked %q", got)
	}
}

// What the journal keeps of a read is the command, never what it returned.
func TestNotificationsLeaveNothingInTheJournal(t *testing.T) {
	e := newEnv(t)
	a := e.withApp()
	a.results[bridge.UIOpNotifications] = bridge.UINotifications{Notifications: []bridge.UINotification{
		{Key: "k", Package: "com.whatsapp", Title: "Ana", Text: "una frase privada", PostedAt: posted},
	}}
	if _, exit := e.helper(t, bridge.UIOpNotifications, struct{}{}); exit.Code != 0 {
		t.Fatal(exit)
	}
	_ = filepath.Walk(e.ws, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), "una frase privada") || strings.Contains(string(b), "Ana") {
				t.Errorf("%s keeps the text of a notification", p)
			}
		}
		return nil
	})
}
