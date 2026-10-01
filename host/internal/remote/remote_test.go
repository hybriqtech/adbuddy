package remote

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
)

const getprop = "shell getprop service.adb.tcp.port"

var t0 = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

// env is one fake phone and one fake config.toml sharing an event log, so the
// tests can check the order of adb orders and saves.
type env struct {
	events    []string
	port      string // answer of getprop
	portFails bool
	tcpipFail bool
	tcpipErr  string // stderr of a failing tcpip (default "error: closed")
	tcpipGo   error  // error of the adb run itself for tcpip (no exit code)
	usbFail   bool
	saveFail  bool
	ip        string
	until     time.Time
	now       time.Time
}

func newEnv() *env {
	return &env{ip: "100.64.0.5", now: t0}
}

func (e *env) Run(_ context.Context, _ string, args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
	key := strings.Join(args, " ")
	e.events = append(e.events, "adb "+key)
	switch key {
	case getprop:
		if e.portFails {
			io.WriteString(stderr, "error: device offline")
			return 1, nil
		}
		io.WriteString(stdout, e.port+"\r\n")
	case "tcpip 5555":
		if e.tcpipGo != nil {
			return 0, e.tcpipGo
		}
		if e.tcpipFail {
			msg := e.tcpipErr
			if msg == "" {
				msg = "error: closed"
			}
			io.WriteString(stderr, msg)
			return 1, nil
		}
	case "usb":
		if e.usbFail {
			io.WriteString(stderr, "error: closed")
			return 1, nil
		}
	default:
		return 1, nil
	}
	return 0, nil
}

func (e *env) TailscaleIP() string { return e.ip }
func (e *env) Until() time.Time    { return e.until }
func (e *env) SetUntil(t time.Time) error {
	if e.saveFail {
		return errors.New("disk full")
	}
	if t.IsZero() {
		e.events = append(e.events, "save zero")
	} else {
		e.events = append(e.events, "save "+t.Sub(e.now).String())
	}
	e.until = t
	return nil
}

func (e *env) controller() *Controller {
	return New(e, e, func() time.Time { return e.now })
}

func (e *env) ran(cmd string) bool {
	for _, ev := range e.events {
		if ev == "adb "+cmd {
			return true
		}
	}
	return false
}

func (e *env) noAdb(t *testing.T) {
	t.Helper()
	for _, ev := range e.events {
		if strings.HasPrefix(ev, "adb ") {
			t.Fatalf("adb ran: %q", e.events)
		}
	}
}

func TestEnableSavesUntilThenRunsTcpip(t *testing.T) {
	e := newEnv()
	s, err := e.controller().Enable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.events, "|"); got != "save 12h0m0s|adb tcpip 5555" {
		t.Fatalf("events %q", got)
	}
	want := State{Enabled: true, Address: "100.64.0.5:5555", ExpiresAt: t0.Add(Lifetime)}
	if s != want {
		t.Fatalf("state %+v, want %+v", s, want)
	}
}

func TestEnableTwiceRunsTcpipOnce(t *testing.T) {
	e := newEnv()
	c := e.controller()
	first, err := c.Enable(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e.events = nil
	e.now = t0.Add(time.Hour)
	second, err := c.Enable(context.Background())
	if err != nil || second != first || len(e.events) != 0 {
		t.Fatalf("second %+v err %v events %q", second, err, e.events)
	}
}

func TestEnableNeedsTailscaleIP(t *testing.T) {
	e := newEnv()
	e.ip = ""
	_, err := e.controller().Enable(context.Background())
	if !errors.Is(err, ErrNoTailscale) {
		t.Fatalf("err %v", err)
	}
	e.noAdb(t)
	if !e.until.IsZero() || len(e.events) != 0 {
		t.Fatalf("something saved: %q", e.events)
	}
}

func TestEnableTcpipFailsPortClosedClearsUntil(t *testing.T) {
	for _, port := range []string{"", "37123"} {
		e := newEnv()
		e.tcpipFail = true
		e.port = port
		s, err := e.controller().Enable(context.Background())
		if err == nil {
			t.Fatalf("port %q: no error", port)
		}
		if !e.until.IsZero() || s.Enabled {
			t.Fatalf("port %q: until %v state %+v", port, e.until, s)
		}
		if !e.ran(getprop) {
			t.Fatalf("port %q: the port was not read: %q", port, e.events)
		}
	}
}

func TestEnableTcpipFailsPortOpenKeepsUntil(t *testing.T) {
	e := newEnv()
	e.tcpipFail = true
	e.port = "5555"
	s, err := e.controller().Enable(context.Background())
	if err == nil {
		t.Fatal("no error")
	}
	if !e.until.Equal(t0.Add(Lifetime)) || !s.Enabled {
		t.Fatalf("until %v state %+v", e.until, s)
	}
}

func TestEnableTcpipFailsPortUnreadableKeepsUntil(t *testing.T) {
	e := newEnv()
	e.tcpipFail = true
	e.portFails = true
	s, err := e.controller().Enable(context.Background())
	if err == nil {
		t.Fatal("no error")
	}
	if !e.until.Equal(t0.Add(Lifetime)) || !s.Enabled {
		t.Fatalf("until %v state %+v", e.until, s)
	}
}

func TestEnableSaveFailsRunsNothing(t *testing.T) {
	e := newEnv()
	e.saveFail = true
	if _, err := e.controller().Enable(context.Background()); err == nil {
		t.Fatal("no error")
	}
	e.noAdb(t)
}

func TestDisableRunsUsbThenClears(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	s, err := e.controller().Disable(context.Background())
	if err != nil || s != (State{}) {
		t.Fatalf("state %+v err %v", s, err)
	}
	if got := strings.Join(e.events, "|"); got != "adb usb|save zero" {
		t.Fatalf("events %q", got)
	}
}

func TestDisableUsbFailsKeepsEnabled(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.usbFail = true
	s, err := e.controller().Disable(context.Background())
	if err == nil || !s.Enabled || !e.until.Equal(t0.Add(time.Hour)) {
		t.Fatalf("state %+v until %v err %v", s, e.until, err)
	}
}

func TestExpire(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	c := e.controller()
	s, changed, err := c.Expire(context.Background())
	if err != nil || changed || !s.Enabled || s.Closing || len(e.events) != 0 {
		t.Fatalf("before: %+v %v %v %q", s, changed, err, e.events)
	}
	e.now = t0.Add(time.Hour)
	s, changed, err = c.Expire(context.Background())
	if err != nil || !changed || s != (State{}) {
		t.Fatalf("after: %+v %v %v", s, changed, err)
	}
	if got := strings.Join(e.events, "|"); got != "adb usb|save zero" {
		t.Fatalf("events %q", got)
	}
	// Nothing saved: nothing to expire.
	e.events = nil
	if _, changed, err := c.Expire(context.Background()); changed || err != nil || len(e.events) != 0 {
		t.Fatalf("off: %v %v %q", changed, err, e.events)
	}
}

func TestExpireUsbFailsStaysClosing(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.now = t0.Add(2 * time.Hour)
	e.usbFail = true
	s, changed, err := e.controller().Expire(context.Background())
	if err == nil || changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	if !s.Enabled || !s.Closing {
		t.Fatalf("state %+v: an open port must never show as off", s)
	}
	if e.until.IsZero() {
		t.Fatal("until cleared though usb failed")
	}
}

func TestReconcileClearsStaleUntil(t *testing.T) {
	for _, port := range []string{"", "37123"} {
		e := newEnv()
		e.until = t0.Add(time.Hour)
		e.port = port
		s, err := e.controller().Reconcile(context.Background())
		if err != nil || s != (State{}) || !e.until.IsZero() {
			t.Fatalf("port %q: %+v %v until %v", port, s, err, e.until)
		}
		if got := strings.Join(e.events, "|"); got != "adb "+getprop+"|save zero" {
			t.Fatalf("port %q: events %q", port, got)
		}
	}
}

func TestReconcileKeepsLiveUntil(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.port = "5555"
	s, err := e.controller().Reconcile(context.Background())
	if err != nil || !s.Enabled || s.Closing || !e.until.Equal(t0.Add(time.Hour)) {
		t.Fatalf("%+v %v until %v", s, err, e.until)
	}
	if e.ran("usb") {
		t.Fatal("usb ran")
	}
}

func TestReconcileClosesExpiredOpenPort(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(-time.Minute)
	e.port = "5555"
	s, err := e.controller().Reconcile(context.Background())
	if err != nil || s != (State{}) || !e.until.IsZero() {
		t.Fatalf("%+v %v until %v", s, err, e.until)
	}
	if got := strings.Join(e.events, "|"); got != "adb "+getprop+"|adb usb|save zero" {
		t.Fatalf("events %q", got)
	}
}

func TestReconcilePortUnreadableChangesNothing(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.portFails = true
	s, err := e.controller().Reconcile(context.Background())
	if err == nil || !s.Enabled || !e.until.Equal(t0.Add(time.Hour)) {
		t.Fatalf("%+v %v until %v", s, err, e.until)
	}
	if e.ran("usb") {
		t.Fatal("usb ran")
	}
}

// A remote_until too far ahead to come from Enable counts as expired: it must
// not slide with the clock.
func TestUntilBeyondLifetimeCountsAsExpired(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(30 * 24 * time.Hour)
	c := e.controller()
	if s := c.State(); !s.Enabled || !s.Closing {
		t.Fatalf("%+v", s)
	}
	e.now = t0.Add(6 * time.Hour) // the clock moving does not push it away
	if s := c.State(); !s.Closing {
		t.Fatalf("slid with the clock: %+v", s)
	}
	st, changed, err := c.Expire(context.Background())
	if err != nil || !changed || st != (State{}) {
		t.Fatalf("%+v %v %v", st, changed, err)
	}
	if got := strings.Join(e.events, "|"); got != "adb usb|save zero" {
		t.Fatalf("events %q", got)
	}
}

func TestUntilBeyondLifetimeReconcileCloses(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(30 * 24 * time.Hour)
	e.port = "5555"
	s, err := e.controller().Reconcile(context.Background())
	if err != nil || s != (State{}) || !e.until.IsZero() || !e.ran("usb") {
		t.Fatalf("%+v %v until %v %q", s, err, e.until, e.events)
	}
}

func TestUntilWithinLifetimeIsKept(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(Lifetime + 30*time.Second)
	s := e.controller().State()
	if !s.Enabled || s.Closing || !s.ExpiresAt.Equal(e.until) {
		t.Fatalf("%+v", s)
	}
}

func TestReconcileWithoutUntilRunsNoAdb(t *testing.T) {
	e := newEnv()
	s, err := e.controller().Reconcile(context.Background())
	if err != nil || s != (State{}) {
		t.Fatalf("%+v %v", s, err)
	}
	e.noAdb(t)
}

func TestEnableWhileClosingSavesAgainAndRunsTcpip(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(-time.Minute)
	s, err := e.controller().Enable(context.Background())
	if err != nil || !s.Enabled || s.Closing || !s.ExpiresAt.Equal(t0.Add(Lifetime)) {
		t.Fatalf("%+v %v", s, err)
	}
	if got := strings.Join(e.events, "|"); got != "save 12h0m0s|adb tcpip 5555" {
		t.Fatalf("events %q", got)
	}
}

func TestDisableSaveFailsStaysEnabled(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.saveFail = true
	s, err := e.controller().Disable(context.Background())
	if err == nil || !s.Enabled || !e.ran("usb") {
		t.Fatalf("%+v %v %q", s, err, e.events)
	}
}

// Final review, minor 2: `tcpip` that fails because no phone is connected
// never reached adbd, so the port did not open: the expiry is cleared and the
// mode stays off, even though the port cannot be read either.
func TestEnableWithoutPhoneClearsUntil(t *testing.T) {
	for name, set := range map[string]func(*env){
		"no devices": func(e *env) { e.tcpipFail, e.tcpipErr = true, "error: no devices/emulators found" },
		"offline":    func(e *env) { e.tcpipFail, e.tcpipErr = true, "error: device offline" },
		"no phone":   func(e *env) { e.tcpipGo = adb.ErrNoDevice },
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv()
			e.portFails = true // no phone: getprop fails too
			set(e)
			s, err := e.controller().Enable(context.Background())
			if err == nil || !NoPhone(err) {
				t.Fatalf("err %v", err)
			}
			if !e.until.IsZero() || s.Enabled {
				t.Fatalf("until %v state %+v (%q)", e.until, s, e.events)
			}
		})
	}
}

// Any other failure may come after adbd got the order (a cut connection, a
// timeout): the expiry stays while the port cannot be read.
func TestEnableTimeoutKeepsUntil(t *testing.T) {
	e := newEnv()
	e.portFails = true
	e.tcpipGo = context.DeadlineExceeded
	s, err := e.controller().Enable(context.Background())
	if err == nil || NoPhone(err) {
		t.Fatalf("err %v", err)
	}
	if !e.until.Equal(t0.Add(Lifetime)) || !s.Enabled {
		t.Fatalf("until %v state %+v", e.until, s)
	}
}

// Re-review I1: `on` while Closing (the expiry passed and `adb usb` could not
// close the port, usually because the phone cannot be reached) that fails for
// no phone puts the old expiry back: still Closing, never a false "off" with
// the port maybe open, and Expire keeps trying.
func TestEnableWithoutPhoneWhileClosingKeepsTheOldUntil(t *testing.T) {
	e := newEnv()
	old := t0.Add(-time.Hour)
	e.until = old
	e.portFails = true
	e.tcpipFail, e.tcpipErr = true, "error: no devices/emulators found"
	c := e.controller()
	s, err := c.Enable(context.Background())
	if err == nil || !NoPhone(err) {
		t.Fatalf("err %v", err)
	}
	if !e.until.Equal(old) || !s.Enabled || !s.Closing {
		t.Fatalf("until %v state %+v (%q)", e.until, s, e.events)
	}
	e.usbFail = true
	e.events = nil
	if _, changed, err := c.Expire(context.Background()); changed || err == nil || !e.ran("usb") {
		t.Fatalf("Expire: changed %v err %v (%q)", changed, err, e.events)
	}
}
