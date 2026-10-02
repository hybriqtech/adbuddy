package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
)

// V46: a list the app sends while serve runs is what the guard uses on the
// next command, without restarting. liveADB is the phone's adb with an app in
// front that the test changes.
type liveADB struct {
	mu    sync.Mutex
	front string
}

func (a *liveADB) setFront(pkg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.front = pkg
}

func (a *liveADB) Shell(_ context.Context, cmd string) (string, int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch cmd {
	case "dumpsys activity activities":
		return "ACTIVITY MANAGER ACTIVITIES (dumpsys activity activities)\n  topResumedActivity=ActivityRecord{1 u0 " + a.front + "/.Main t1}\n", 0, nil
	case "dumpsys window | grep -E 'mCurrentFocus'":
		return "  mCurrentFocus=Window{1a2b3c u0 " + a.front + "/" + a.front + ".Main}\n", 0, nil
	case "dumpsys window windows":
		return "WINDOW MANAGER WINDOWS\n" +
			"  Window #0 Window{1 u0 StatusBar}:\n    mOwnerUid=10123 showForAllUsers=true package=com.android.systemui appop=NONE\n    mViewVisibility=0x0 mHaveFrame=true\n" +
			"  Window #1 Window{3 u0 " + a.front + "/" + a.front + ".Main}:\n    mOwnerUid=10150 package=" + a.front + " appop=NONE\n    mViewVisibility=0x0 mHaveFrame=true\n", 0, nil
	}
	return "", 1, nil
}

func (a *liveADB) Pull(context.Context, string, string) error { return errors.New("no pull") }

func (a *liveADB) Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) (int, error) {
	return 0, nil
}

type liveStream struct {
	out, errOut bytes.Buffer
	data        [][]byte
}

func (s *liveStream) Stdin() io.Reader  { return strings.NewReader("") }
func (s *liveStream) Stdout() io.Writer { return &s.out }
func (s *liveStream) Stderr() io.Writer { return &s.errOut }
func (s *liveStream) Data(v any) error {
	b, err := json.Marshal(v)
	s.data = append(s.data, b)
	return err
}

// V46 (b): the real wiring uiLink -> daemon. Between two commands a device.info
// that removes the app changes what the guard decides, with no restart: the
// app in front, an `am start` that names it, and the notifications.
func TestProtectedChangeReachesTheGuardLive(t *testing.T) {
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	u := newUILink(filepath.Join(t.TempDir(), "protected.json"), &bytes.Buffer{})
	u.setServer(&fakeUIServer{current: true})
	h := u.area().handlers[bridge.TypeDeviceInfo]
	setList := func(list ...string) {
		h(uiMessage(t, bridge.TypeDeviceInfo, bridge.DeviceInfo{ProtectedApps: list}))
	}
	adb := &liveADB{front: "com.android.settings"}
	ws := t.TempDir()
	d := daemon.New(daemon.Options{
		Classifier: cls, ADB: adb, Journal: journal.OpenSigned(filepath.Join(ws, "journal"), []byte("test key")),
		Confirmer: confirmerFunc(func(context.Context, confirm.Request) confirm.Decision { return confirm.Allow }),
		Serial:    "dev1", Session: "s_1", Lang: "en", Workspace: ws,
		Protected: u.Protected,
		// The app answers the notifications only: the foreground falls back
		// to dumpsys, as without the app.
		UI: func(_ context.Context, op string, _ json.RawMessage) (json.RawMessage, error) {
			if op != bridge.UIOpNotifications {
				return nil, daemon.ErrNoApp
			}
			return json.RawMessage(`{"notifications":[{"key":"k1","package":"com.whatsapp","title":"Ana","text":"your code is 123456"}],"truncated":false}`), nil
		},
	})
	agent := func(req ipc.Request) (*liveStream, ipc.Exit) {
		t.Helper()
		req.Cwd, req.Env = ws, map[string]string{"ADBUDDY_SOURCE": "agent"}
		s := &liveStream{}
		return s, d.Handle(context.Background(), req, s)
	}
	// exec returns the exit and, when blocked, the reason (on stderr).
	exec := func(argv ...string) (exit ipc.Exit) {
		s, exit := agent(ipc.Request{Kind: ipc.KindExec, Argv: argv})
		exit.Error = s.errOut.String()
		return exit
	}
	notified := func() string {
		s, exit := agent(ipc.Request{Kind: ipc.KindUI, UIOp: bridge.UIOpNotifications, UIArgs: json.RawMessage("{}")})
		if exit.Code != 0 || len(s.data) != 1 {
			t.Fatalf("notifications: %+v %d", exit, len(s.data))
		}
		return string(s.data[0])
	}

	// 1. The app in front.
	setList("com.whatsapp", "com.example.bank")
	adb.setFront("com.whatsapp")
	if exit := exec("shell", "input", "tap", "10", "10"); exit.Code != daemon.ExitDenied || !strings.Contains(exit.Error, "protected app") {
		t.Fatalf("in front, protected: %+v", exit)
	}
	setList("com.example.bank") // WhatsApp removed, the list not emptied
	if exit := exec("shell", "input", "tap", "10", "10"); exit.Code != 0 {
		t.Fatalf("in front, after unprotecting: %+v", exit)
	}
	setList("com.whatsapp", "com.example.bank") // and protected again
	if exit := exec("shell", "input", "tap", "10", "10"); exit.Code != daemon.ExitDenied {
		t.Fatalf("in front, protected again: %+v", exit)
	}

	// 2. am start that names it.
	adb.setFront("com.android.settings")
	if exit := exec("shell", "am", "start", "-n", "com.whatsapp/.Main"); exit.Code != daemon.ExitDenied || !strings.Contains(exit.Error, "protected app") {
		t.Fatalf("am start, protected: %+v", exit)
	}
	setList("com.example.bank")
	if exit := exec("shell", "am", "start", "-n", "com.whatsapp/.Main"); exit.Code != 0 {
		t.Fatalf("am start, after unprotecting: %+v", exit)
	}

	// 3. The notifications filter.
	if got := notified(); !strings.Contains(got, "your code is 123456") {
		t.Fatalf("unprotected app hidden: %s", got)
	}
	setList("com.whatsapp", "com.example.bank")
	if got := notified(); strings.Contains(got, "123456") || strings.Contains(got, "Ana") {
		t.Fatalf("protected app shown: %s", got)
	}
	setList("com.example.bank")
	if got := notified(); !strings.Contains(got, "your code is 123456") {
		t.Fatalf("after unprotecting, still hidden: %s", got)
	}
}
