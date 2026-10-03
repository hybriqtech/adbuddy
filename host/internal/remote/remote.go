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
	"github.com/hybriqtech/adbuddy/host/internal/device"
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
	// Persist is persist.adb.tcp.port as the controller last read it (or was
	// told with Note), when it names a port (device.TCP.Persist): `adb usb`
	// does not clear it, and adbd listens on it again when the phone restarts
	// (VB-41). ADBuddy never sets it and cannot clear it without root. It is
	// the last read, not a live value: a persist set or emptied since shows
	// at the next read.
	Persist string
	// PersistUnknown says the controller's last read of the port failed:
	// whether a port opens again when the phone restarts is not known (never
	// a false "closed").
	PersistUnknown bool
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
	// readWait bounds each read of the port (device.ReadTCPWait as New read
	// it, before any goroutine).
	readWait time.Duration
	// pmu guards persist and persistUnknown, what the last read of the port
	// said (readTCP, Note). State takes only pmu: the snapshot and `status`
	// call it while a change may hold mu.
	pmu            sync.Mutex
	persist        string
	persistUnknown bool
}

func New(a ADB, s Store, now func() time.Time) *Controller {
	if now == nil {
		now = time.Now
	}
	return &Controller{adb: a, st: s, now: now, readWait: device.ReadTCPWait}
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
	var s State
	if until := c.until(); !until.IsZero() {
		s = State{Enabled: true, Closing: !until.After(c.now()), ExpiresAt: until}
		if ip := c.st.TailscaleIP(); ip != "" {
			s.Address = net.JoinHostPort(ip, Port)
		}
	}
	c.pmu.Lock()
	s.Persist, s.PersistUnknown = c.persist, c.persistUnknown
	c.pmu.Unlock()
	return s
}

// readTCP reads adbd's TCP port (device.ReadTCP: service.adb.tcp.port, or
// persist.adb.tcp.port when it is empty) and keeps what it said of
// persist.adb.tcp.port for State; a read that fails makes it unknown. The
// read has its own deadline (readWait): a hung getprop must not eat the
// budget of the `adb usb` that follows.
func (c *Controller) readTCP(ctx context.Context) (device.TCP, error) {
	rctx, cancel := context.WithTimeout(ctx, c.readWait)
	defer cancel()
	t, err := device.ReadTCP(rctx, c.adb)
	c.pmu.Lock()
	c.persist, c.persistUnknown = t.Persist, err != nil
	c.pmu.Unlock()
	return t, err
}

// Note keeps a read of the port made elsewhere (serve's invariant at its
// start, the watcher when the phone comes back) for State, as if the
// controller had made it: `adbuddy remote status` says what
// persist.adb.tcp.port holds from serve's start. It takes only pmu.
// A read of the watcher may cross one of the controller (an off) and
// overwrite the newer; benign: same property, the next read corrects it.
func (c *Controller) Note(t device.TCP) {
	c.pmu.Lock()
	c.persist, c.persistUnknown = t.Persist, false
	c.pmu.Unlock()
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
		// is seen NOT to be open (persist.adb.tcp.port included). If it cannot
		// be read, the expiry stays (a false "on" is better than a false "off"
		// with the port open); Reconcile clears it at the next start if the
		// port turns out closed.
		if t, perr := c.readTCP(ctx); perr == nil && t.Now != Port {
			_ = c.st.SetUntil(time.Time{})
		}
		return c.State(), err
	}
	return c.State(), nil
}

// Disable closes the port (`adb usb`) and only then clears the expiry: if
// adb fails the mode stays on (and says so) and is tried again. It reads the
// port first (device.TCP), so the state it returns says what
// persist.adb.tcp.port holds. With no expiry saved (off as far as the host
// knows), a port that reads closed is left alone: `adb usb` would restart
// adbd, and drop the phone's connection, for nothing. A port that reads open
// (5555, any other, or persist's after a restart), or cannot be read, is
// closed all the same.
func (c *Controller) Disable(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, err := c.readTCP(ctx)
	if c.st.Until().IsZero() && err == nil && t.Now == "" {
		return c.State(), nil
	}
	return c.disable(ctx, t, err)
}

// disable runs `adb usb` and clears the expiry. t and rerr are the read of
// the port just before (readTCP). When clearing failed after `adb usb` closed
// the port, the next call for the same expiry skips `adb usb` (closedFor: each
// one restarts adbd and the phone's connection) only if that read says
// closed: someone may have reopened the port meanwhile, and an unreadable port
// is not known to be closed. The state it returns keeps what the read said of
// persist.adb.tcp.port: `adb usb` does not change it (measured, VB-41), and
// right after it adbd restarts and cannot be read.
func (c *Controller) disable(ctx context.Context, t device.TCP, rerr error) (State, error) {
	until := c.st.Until()
	if !c.closedFor.IsZero() && until.Equal(c.closedFor) && (rerr != nil || t.Now != "") {
		c.closedFor = time.Time{}
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
	return c.State(), nil
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
	t, rerr := c.readTCP(ctx)
	s, err := c.disable(ctx, t, rerr)
	return s, err == nil, err
}

// Reconcile is run once when serve starts, with the phone connected. No
// expiry: off. Port 5555 not open: the expiry is stale (the phone restarted)
// and is cleared. Port open with an expiry in the future: kept. Port open
// with the expiry passed: closed now. The port is device.TCP.Now: after a
// restart with persist.adb.tcp.port=5555, adbd listens on 5555 again and the
// mode is still on. If the port cannot be read, nothing changes (the next
// start looks again).
func (c *Controller) Reconcile(ctx context.Context) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	until := c.until()
	if until.IsZero() {
		return c.State(), nil
	}
	t, err := c.readTCP(ctx)
	if err != nil {
		return c.State(), err
	}
	switch {
	case t.Now != Port:
		if err := c.st.SetUntil(time.Time{}); err != nil {
			return c.State(), err
		}
		return c.State(), nil
	case until.After(c.now()):
		return c.State(), nil
	}
	return c.disable(ctx, t, nil)
}
