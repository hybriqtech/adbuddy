// Package remote is the remote mode (spec §3.3 and M5 §7.2): adb listens on
// port 5555 on every interface until it expires, so the host can reach the
// phone over Tailscale with only mobile data.
package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
)

const (
	Port     = "5555"
	Lifetime = 12 * time.Hour
)

// ErrNoTailscale: the app has not reported a Tailscale address yet.
var ErrNoTailscale = errors.New("Turn on Tailscale on the phone first")

// NoPhone reports whether adb failed because no phone is connected or it is
// offline: the order never reached adbd. Matched on adb's own messages; a
// timeout or a cut connection is not this (the order may have arrived).
func NoPhone(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, adb.ErrNoDevice) {
		return true
	}
	m := strings.ToLower(err.Error())
	for _, s := range []string{"no devices/emulators found", "device offline", "no devices", "no phone connected"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// ADB runs adb against the active phone.
type ADB interface {
	Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// Store is where the mode's persistent facts live (config.toml). It must be
// safe for concurrent use: the controller and the snapshot read it at once.
type Store interface {
	TailscaleIP() string
	Until() time.Time
	SetUntil(time.Time) error // the zero time clears it
}

// State is the mode as the host has it. Enabled stays true from the moment
// the expiry is saved until `adb usb` has really closed the port: an expiry
// that has passed but could not be closed is Enabled and Closing, never "off"
// (a false off would hide an open port 5555).
type State struct {
	Enabled   bool
	Closing   bool   // the expiry passed and the port is not known to be closed yet
	Address   string // ip:port while enabled
	ExpiresAt time.Time
}

type Controller struct {
	mu  sync.Mutex // one change at a time: adbd restarts under each
	adb ADB
	st  Store
	now func() time.Time
	// closedFor is the expiry whose port `adb usb` closed while clearing it
	// failed (under mu): the next attempt only clears it, without restarting
	// adbd again. Any other expiry (Enable saved a new one) runs `adb usb`.
	closedFor time.Time
}

func New(a ADB, s Store, now func() time.Time) *Controller {
	if now == nil {
		now = time.Now
	}
	return &Controller{adb: a, st: s, now: now}
}

// until is the saved expiry. One more than Lifetime (plus a minute of slack
// for clock jitter) ahead cannot come from Enable: it is a config.toml edited
// by hand, and it counts as already expired (now), so Expire and Reconcile
// close the port. It is not clamped to now+Lifetime: that would slide with
// the clock and never expire.
func (c *Controller) until() time.Time {
	u := c.st.Until()
	if u.After(c.now().Add(Lifetime + time.Minute)) {
		return c.now()
	}
	return u
}

func (c *Controller) State() State {
	until := c.until()
	if until.IsZero() {
		return State{}
	}
	s := State{Enabled: true, Closing: !until.After(c.now()), ExpiresAt: until}
	if ip := c.st.TailscaleIP(); ip != "" {
		s.Address = net.JoinHostPort(ip, Port)
	}
	return s
}

func (c *Controller) adbCmd(ctx context.Context, args ...string) (string, error) {
	var out, errb bytes.Buffer
	code, err := c.adb.Run(ctx, "", args, nil, &out, &errb)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("adb %s: %s", args[0], classify.Printable(strings.TrimSpace(errb.String()+" "+out.String())))
	}
	return strings.TrimSpace(strings.ReplaceAll(out.String(), "\r", "")), nil
}

// Enable opens port 5555 (`adb tcpip 5555`). The expiry is saved BEFORE the
// port opens: right after `adb tcpip` adbd restarts and the connection is
// gone, so an `adb usb` to undo a failed save would fail too. If `tcpip`
// fails because no phone is connected, the previous expiry is put back (off
// or Closing, as it was); if the port is then seen closed, the expiry is
// cleared; otherwise (or if that fails) the new expiry stays, which is the
// safe side (Reconcile clears it if the port is closed).
// Already enabled: nothing is run. adbd restarts, so the caller does not
// wait for the connection.
func (c *Controller) Enable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.TailscaleIP() == "" {
		return c.State(), ErrNoTailscale
	}
	if s := c.State(); s.Enabled && !s.Closing {
		return s, nil
	}
	prev := c.st.Until() // zero when off; passed when Closing
	if err := c.st.SetUntil(c.now().Add(Lifetime)); err != nil {
		return c.State(), err
	}
	// The port may open now: whatever expiry ends up saved (prev comes back
	// on "no phone", and the order may still have reached adbd), the next
	// disable closes it with `adb usb`.
	c.closedFor = time.Time{}
	if _, err := c.adbCmd(ctx, "tcpip", Port); err != nil {
		// No phone connected (or offline): the order never reached adbd, so
		// nothing changed on the phone and the previous expiry is put back:
		// off stays off, and Closing stays Closing (the port may still be
		// open from before: never a false "off").
		if NoPhone(err) {
			_ = c.st.SetUntil(prev)
			return c.State(), err
		}
		// Otherwise adbd may have got the order and the failure be the cut
		// connection or the timeout: the expiry is cleared only when the port
		// is seen NOT to be open. If it cannot be read, the expiry stays (a
		// false "on" is better than a false "off" with the port open);
		// Reconcile clears it at the next start if the port turns out closed.
		if port, perr := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); perr == nil && port != Port {
			_ = c.st.SetUntil(time.Time{})
		}
		return c.State(), err
	}
	return c.State(), nil
}

// Disable closes the port (`adb usb`) and only then clears the expiry: if
// adb fails the mode stays on (and says so) and is tried again. With no
// expiry saved (off as far as the host knows) it first reads the port:
// `adb usb` would restart adbd, and drop the phone's connection, for nothing.
// A port that reads open (5555 or any other), or cannot be read, is closed all the same.
func (c *Controller) Disable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Until().IsZero() {
		if port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); err == nil && portClosed(port) {
			return State{}, nil
		}
	}
	return c.disable(ctx)
}

// portClosed says whether service.adb.tcp.port reads as no network port (empty
// or 0, the criterion of device.EnsureNoTCPPort, spec 3.3): any other value,
// 5555 or not, is open and `adb usb` closes it.
func portClosed(port string) bool { return port == "" || port == "0" }

// disable runs `adb usb` and clears the expiry. When clearing failed after
// `adb usb` closed the port, the next call for the same expiry skips `adb usb`
// (closedFor: each one restarts adbd and the phone's connection) only if the
// port reads as closed: someone may have reopened 5555 meanwhile, and an
// unreadable port is not known to be closed.
func (c *Controller) disable(ctx context.Context) (State, error) {
	until := c.st.Until()
	if !c.closedFor.IsZero() && until.Equal(c.closedFor) {
		if port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port"); err != nil || !portClosed(port) {
			c.closedFor = time.Time{}
		}
	}
	if c.closedFor.IsZero() || !until.Equal(c.closedFor) {
		if _, err := c.adbCmd(ctx, "usb"); err != nil {
			return c.State(), err
		}
	}
	if err := c.st.SetUntil(time.Time{}); err != nil {
		c.closedFor = until
		return c.State(), err
	}
	c.closedFor = time.Time{}
	return State{}, nil
}

// Expire closes the mode when its time has passed. The bool says the state
// changed (it is off now).
func (c *Controller) Expire(ctx context.Context) (State, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.until()
	if until.IsZero() || until.After(c.now()) {
		return c.State(), false, nil
	}
	s, err := c.disable(ctx)
	return s, err == nil, err
}

// Reconcile is run once when serve starts, with the phone connected. No
// expiry: off. Port 5555 not open: the expiry is stale (the phone restarted)
// and is cleared. Port open with an expiry in the future: kept. Port open
// with the expiry passed: closed now. If the port cannot be read, nothing
// changes (the next start looks again).
func (c *Controller) Reconcile(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.until()
	if until.IsZero() {
		return State{}, nil
	}
	port, err := c.adbCmd(ctx, "shell", "getprop service.adb.tcp.port")
	if err != nil {
		return c.State(), err
	}
	switch {
	case port != Port:
		if err := c.st.SetUntil(time.Time{}); err != nil {
			return c.State(), err
		}
		return State{}, nil
	case until.After(c.now()):
		return c.State(), nil
	}
	return c.disable(ctx)
}
