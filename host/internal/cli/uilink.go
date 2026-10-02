package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
)

// uiWait is how long a helper's ui.request lives: the app drops it after
// expiresAt, and the host stops waiting then (M5 spec §2). A variable so tests
// can shorten it.
var uiWait = 10 * time.Second

// uiServer is what uiLink needs of the bridge server.
type uiServer interface {
	Send(typ string, payload any) error
	Current(m bridge.Message) bool
}

// uiLink sends the helpers' operations to the app and waits for their
// answers (M5 spec §2), and keeps the protected apps the app reports (spec
// §4.3), saved in file so they hold while the app is away.
type uiLink struct {
	file  string // protected.json; "" keeps the list in memory only
	log   io.Writer
	now   func() time.Time
	nonce string // starts every id: the app remembers ids, and a new serve must not repeat them

	mu        sync.Mutex
	srv       uiServer // nil: no app
	next      uint64
	waiting   map[string]*uiWaiter
	protected map[string]bool
	known     bool // a list came from the app, now or in an earlier session
	warned    bool // said once that there is no list

	// deviceInfo, when set before the server starts, also gets each device.info
	// (the remote mode's address and port; phone_remote.go).
	deviceInfo func(bridge.Message, bridge.DeviceInfo)
}

// uiWaiter is a request waiting for its answer.
type uiWaiter struct {
	ch   chan bridge.UIResponse // gets the answer
	lost chan struct{}          // closed when the connection it went to is gone
}

// protectedFile is how protected.json is written.
type protectedFile struct {
	ProtectedApps []string  `json:"protectedApps"`
	Updated       time.Time `json:"updated"`
}

// newUILink loads the protected apps saved in file, if any.
func newUILink(file string, log io.Writer) *uiLink {
	var n [4]byte
	_, _ = rand.Read(n[:])
	u := &uiLink{file: file, log: log, now: time.Now, nonce: hex.EncodeToString(n[:]), waiting: map[string]*uiWaiter{}, protected: map[string]bool{}}
	if file == "" {
		return u
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return u
	}
	var p protectedFile
	if err == nil {
		err = json.Unmarshal(data, &p)
	}
	if err == nil {
		err = checkProtected(p.ProtectedApps)
	}
	if err != nil {
		tell(log, "uilink.the_saved_list_of_protected", file, classify.Printable(err.Error()))
		return u
	}
	u.setProtected(p.ProtectedApps)
	return u
}

// setServer sets the bridge server the requests go through.
func (u *uiLink) setServer(srv uiServer) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.srv = srv
}

// area is uiLink's part of the phone link (protocol spec §10).
func (u *uiLink) area() area {
	return area{
		handlers: map[string]func(bridge.Message){
			bridge.TypeUIResponse: u.onResponse,
			bridge.TypeDeviceInfo: u.onDeviceInfo,
		},
		// A request sent to a connection that is gone gets no answer.
		attach: u.loseAll,
		gone:   u.loseAll,
	}
}

// Call runs op on the app (daemon.Options.UI). It is ErrNoApp when no app
// is connected (nothing was sent), and ErrUITimeout when the app did not
// answer in time or its connection dropped after the request went out:
// then it may or may not have been done.
func (u *uiLink) Call(ctx context.Context, op string, args json.RawMessage) (json.RawMessage, error) {
	u.mu.Lock()
	srv := u.srv
	u.next++
	id := fmt.Sprintf("u_%s_%d", u.nonce, u.next)
	w := &uiWaiter{ch: make(chan bridge.UIResponse, 1), lost: make(chan struct{})}
	u.waiting[id] = w
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		delete(u.waiting, id)
		u.mu.Unlock()
	}()
	if srv == nil {
		return nil, daemon.ErrNoApp
	}
	err := srv.Send(bridge.TypeUIRequest, bridge.UIRequest{ID: id, Op: op, Args: args, ExpiresAt: u.now().Add(uiWait)})
	if errors.Is(err, bridge.ErrNoApp) {
		return nil, daemon.ErrNoApp
	}
	if err != nil {
		return nil, err
	}
	t := time.NewTimer(uiWait)
	defer t.Stop()
	select {
	case r := <-w.ch:
		if r.OK {
			return r.Result, nil
		}
		if r.Error == nil {
			return nil, &daemon.UIError{Code: bridge.CodeFailed, Text: "the app gave no reason"}
		}
		return nil, &daemon.UIError{Code: r.Error.Code, Text: r.Error.Text}
	case <-w.lost:
		return nil, daemon.ErrUITimeout
	case <-t.C:
		return nil, daemon.ErrUITimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// loseAll ends every request waiting: their connection is gone.
func (u *uiLink) loseAll() {
	u.mu.Lock()
	defer u.mu.Unlock()
	for id, w := range u.waiting {
		close(w.lost)
		delete(u.waiting, id)
	}
}

// onResponse hands an answer to the request waiting for it. One from a
// connection already replaced, or for an id nobody waits for, is dropped.
func (u *uiLink) onResponse(m bridge.Message) {
	var r bridge.UIResponse
	if json.Unmarshal(m.Payload, &r) != nil {
		return
	}
	u.mu.Lock()
	srv := u.srv
	u.mu.Unlock()
	if srv == nil || !srv.Current(m) {
		return
	}
	u.mu.Lock()
	w, ok := u.waiting[r.ID]
	if ok {
		delete(u.waiting, r.ID)
	}
	u.mu.Unlock()
	if ok {
		w.ch <- r // buffered, and only one answer per waiter
	}
}

// onDeviceInfo is the one handler of device.info: it hands the decoded report
// to the hook, then takes the protected apps the app reports: absent means no
// change, and a list that does not check is refused whole.
func (u *uiLink) onDeviceInfo(m bridge.Message) {
	var info bridge.DeviceInfo
	if json.Unmarshal(m.Payload, &info) != nil {
		return
	}
	if u.deviceInfo != nil {
		u.deviceInfo(m, info)
	}
	if info.ProtectedApps == nil {
		return
	}
	u.mu.Lock()
	srv := u.srv
	u.mu.Unlock()
	if srv == nil || !srv.Current(m) { // from a connection already replaced
		return
	}
	if err := checkProtected(info.ProtectedApps); err != nil {
		tell(u.log, "uilink.the_phone_app_sent_a", classify.Printable(err.Error()))
		return
	}
	u.setProtected(info.ProtectedApps)
	if u.file == "" {
		return
	}
	if err := writeProtected(u.file, protectedFile{ProtectedApps: info.ProtectedApps, Updated: u.now().UTC()}); err != nil {
		tell(u.log, "uilink.could_not_save_the_list", classify.Printable(err.Error()))
	}
}

func checkProtected(list []string) error {
	if len(list) > bridge.MaxProtectedApps {
		return fmt.Errorf("%d apps, at most %d", len(list), bridge.MaxProtectedApps)
	}
	for _, p := range list {
		if !bridge.ValidPackage(p) {
			return fmt.Errorf("%q is not a package name", p)
		}
	}
	return nil
}

func (u *uiLink) setProtected(list []string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.protected = map[string]bool{}
	for _, p := range list {
		u.protected[p] = true
	}
	u.known = true
}

// Protected reports whether pkg is one of the protected apps
// (daemon.Options.Protected). Without a list it says once that only ADBuddy
// is protected.
func (u *uiLink) Protected(pkg string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.known && !u.warned {
		u.warned = true
		tell(u.log, "uilink.the_phone_app_has_not")
	}
	return u.protected[pkg]
}

// writeProtected writes the file through a temporary one, readable only by
// the user.
func writeProtected(file string, p protectedFile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".protected-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
