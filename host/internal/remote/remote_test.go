package remote

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/device"
)

const (
	getprop    = "shell getprop service.adb.tcp.port"
	getpersist = "shell getprop persist.adb.tcp.port"
	// readPort is the read of the port as the events log it: both properties.
	readPort = "adb " + getprop + "|adb " + getpersist
)

var t0 = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

// env is one fake phone and one fake config.toml sharing an event log, so the
// tests can check the order of adb orders and saves.
type env struct {
	events    []string
	port      string // answer of getprop
	portFails bool
	persist   string // answer of getprop persist.adb.tcp.port
	persFails bool
	persHang  bool // getprop persist.adb.tcp.port waits for its context to end
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

func (e *env) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
	key := strings.Join(args, " ")
	e.events = append(e.events, "adb "+key)
	switch key {
	case getprop:
		if e.portFails {
			io.WriteString(stderr, "error: device offline")
			return 1, nil
		}
		io.WriteString(stdout, e.port+"\r\n")
	case getpersist:
		if e.persHang {
			<-ctx.Done()
			return -1, ctx.Err()
		}
		if e.persFails {
			io.WriteString(stderr, "error: device offline")
			return 1, nil
		}
		io.WriteString(stdout, e.persist+"\r\n")
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
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero" {
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
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero" {
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
		if got := strings.Join(e.events, "|"); got != readPort+"|save zero" {
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
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero" {
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
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero" {
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

// VB-41 (review T1): a Reconcile with no expiry runs no adb either, but its
// state carries what Note was given (persist), not a bare State{}.
func TestReconcileWithoutUntilCarriesTheNotedPersist(t *testing.T) {
	e := newEnv()
	ctl := e.controller()
	ctl.Note(device.TCP{Persist: "5555"})
	s, err := ctl.Reconcile(context.Background())
	if err != nil || s != (State{Persist: "5555"}) {
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

// V38: with no expiry saved the host believes the port closed, and `adb usb`
// would restart adbd (and drop the phone's connection) for nothing. The port
// is still looked at: one that reads 5555, or cannot be read, is closed.
func TestDisableWithoutTheModeLooksAtThePortFirst(t *testing.T) {
	for _, c := range []struct {
		name      string
		port      string
		portFails bool
		want      string
	}{
		{"closed", "", false, readPort},
		{"zero", "0", false, readPort},
		{"another port", "37123", false, readPort + "|adb usb|save zero"},
		{"open", "5555", false, readPort + "|adb usb|save zero"},
		{"unreadable", "", true, "adb " + getprop + "|adb usb|save zero"},
	} {
		e := newEnv()
		e.port, e.portFails = c.port, c.portFails
		s, err := e.controller().Disable(context.Background())
		// Unreadable: off, but whether a port comes back after a restart is
		// not known (VB-41).
		if err != nil || s != (State{PersistUnknown: c.portFails}) {
			t.Fatalf("%s: state %+v err %v", c.name, s, err)
		}
		if got := strings.Join(e.events, "|"); got != c.want {
			t.Fatalf("%s: events %q, want %q", c.name, got, c.want)
		}
	}
}

// V38: `adb usb` closed the port but the expiry could not be cleared: the
// next attempts only try to clear it, without restarting adbd each time.
// A new expiry (Enable) closes with `adb usb` again.
func TestExpireAfterAFailedSaveDoesNotRunUsbAgain(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.now = t0.Add(2 * time.Hour)
	e.saveFail = true
	c := e.controller()
	if s, changed, err := c.Expire(context.Background()); err == nil || changed || !s.Enabled || !s.Closing {
		t.Fatalf("first: %+v %v %v", s, changed, err)
	}
	if s, changed, err := c.Expire(context.Background()); err == nil || changed || !s.Enabled || !s.Closing {
		t.Fatalf("second: %+v %v %v", s, changed, err)
	}
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|"+readPort {
		t.Fatalf("events %q: adb usb ran again", got)
	}
	e.saveFail = false
	if s, changed, err := c.Expire(context.Background()); err != nil || !changed || s != (State{}) {
		t.Fatalf("third: %+v %v %v", s, changed, err)
	}
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|"+readPort+"|"+readPort+"|save zero" {
		t.Fatalf("events %q", got)
	}
	// A failed save again, then a new expiry: its port is closed with adb usb.
	e.until, e.saveFail, e.events = t0.Add(time.Hour), true, nil
	if _, _, err := c.Expire(context.Background()); err == nil {
		t.Fatal("the save did not fail")
	}
	e.saveFail = false
	if _, err := c.Enable(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(13 * time.Hour)
	if _, changed, err := c.Expire(context.Background()); err != nil || !changed {
		t.Fatalf("after Enable: %v %v", changed, err)
	}
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save 12h0m0s|adb tcpip 5555|"+readPort+"|adb usb|save zero" {
		t.Fatalf("events %q", got)
	}
}

// V38 (plan review): Enable forgets the expiry whose port was closed before
// it runs `adb tcpip`. If tcpip then fails as "no phone", the old expiry is
// put back, but the order may have reached adbd: the next Expire must close
// with `adb usb` again, not only clear the expiry (a false "off" with the
// port open).
func TestEnableForgetsTheExpiryItsPortWasClosedFor(t *testing.T) {
	e := newEnv()
	old := t0.Add(time.Hour)
	e.until = old
	e.now = t0.Add(2 * time.Hour)
	e.saveFail = true
	c := e.controller()
	if _, _, err := c.Expire(context.Background()); err == nil {
		t.Fatal("the save did not fail")
	}
	e.saveFail = false
	e.tcpipFail, e.tcpipErr = true, "error: no devices/emulators found"
	if _, err := c.Enable(context.Background()); err == nil || !NoPhone(err) {
		t.Fatalf("Enable: %v", err)
	}
	if !e.until.Equal(old) {
		t.Fatalf("until %v: the old expiry was not put back", e.until)
	}
	e.events = nil
	if _, changed, err := c.Expire(context.Background()); err != nil || !changed {
		t.Fatalf("Expire: changed %v err %v", changed, err)
	}
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero" {
		t.Fatalf("events %q: the port was not closed again", got)
	}
}

// V38 (review): after `adb usb` closed the port and clearing the expiry
// failed, someone may have reopened 5555 before the next attempt. The retry
// reads the port (it does not restart adbd) and skips `adb usb` only when the
// read works and is not 5555; unreadable or 5555 runs `adb usb` again.
func TestRetryAfterAFailedSaveLooksAtThePortBeforeSkippingUsb(t *testing.T) {
	for _, c := range []struct {
		name      string
		port      string
		portFails bool
		want      string
	}{
		{"closed", "", false, readPort + "|save zero"},
		{"zero", "0", false, readPort + "|save zero"},
		{"another port", "37123", false, readPort + "|adb usb|save zero"},
		{"reopened", "5555", false, readPort + "|adb usb|save zero"},
		{"unreadable", "", true, "adb " + getprop + "|adb usb|save zero"},
	} {
		e := newEnv()
		e.until = t0.Add(time.Hour)
		e.now = t0.Add(2 * time.Hour)
		e.saveFail = true
		ctl := e.controller()
		if _, _, err := ctl.Expire(context.Background()); err == nil {
			t.Fatalf("%s: the save did not fail", c.name)
		}
		e.saveFail, e.events = false, nil
		e.port, e.portFails = c.port, c.portFails
		if s, changed, err := ctl.Expire(context.Background()); err != nil || !changed || s != (State{PersistUnknown: c.portFails}) {
			t.Fatalf("%s: %+v %v %v", c.name, s, changed, err)
		}
		if got := strings.Join(e.events, "|"); got != c.want {
			t.Fatalf("%s: events %q, want %q", c.name, got, c.want)
		}
	}
}

// VB-41: AOSP adbd takes persist.adb.tcp.port at its start when
// service.adb.tcp.port is empty (after a reboot). "off" with the mode off
// reads both: a persist port with the service property empty is open and is
// closed; with the service property at 0 `adb usb` already ran since the boot
// and is not run again (no loop). A persist property that cannot be read is
// not known to be closed: it is closed all the same, as an unreadable port.
func TestDisableWithoutTheModeReadsPersistToo(t *testing.T) {
	for _, c := range []struct {
		name, port, persist string
		persFails, wantUSB  bool
	}{
		{"persist after a reboot", "", "5555", false, true},
		{"another persist port", "", "7777", false, true},
		{"not a number", "", "abc", false, true},
		{"huge", "", "99999999999999999999999", false, true},
		{"persist unreadable", "", "", true, true},
		{"persist 0", "", "0", false, false},
		{"nothing", "", "", false, false},
		{"service at 0", "0", "5555", false, false},
		{"both set", "5555", "5555", false, true},
	} {
		e := newEnv()
		e.port, e.persist, e.persFails = c.port, c.persist, c.persFails
		if _, err := e.controller().Disable(context.Background()); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if e.ran("usb") != c.wantUSB {
			t.Errorf("%s: adb usb ran %v, want %v (%q)", c.name, e.ran("usb"), c.wantUSB, e.events)
		}
	}
}

// After a reboot with persist.adb.tcp.port=5555 and a live expiry, adbd
// listens on 5555 again: the mode is still on, not a stale expiry to clear
// (that would leave 5555 open with the host saying "off").
func TestReconcileKeepsTheModeOnAPersistPort(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persist = "5555"
	s, err := e.controller().Reconcile(context.Background())
	if err != nil || !s.Enabled || !e.until.Equal(t0.Add(time.Hour)) || e.ran("usb") {
		t.Fatalf("%+v %v until %v (%q)", s, err, e.until, e.events)
	}
}

// Reconcile with persist.adb.tcp.port unreadable changes nothing, as with the
// service property unreadable.
func TestReconcilePersistUnreadableChangesNothing(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persFails = true
	s, err := e.controller().Reconcile(context.Background())
	if err == nil || !s.Enabled || !e.until.Equal(t0.Add(time.Hour)) || e.ran("usb") {
		t.Fatalf("%+v %v until %v (%q)", s, err, e.until, e.events)
	}
}

// tcpip failing for any reason but "no phone": the expiry is cleared only when
// the port reads closed, persist.adb.tcp.port included.
func TestEnableTcpipFailsWithAPersistPortKeepsUntil(t *testing.T) {
	for name, set := range map[string]func(*env){
		"persist 5555":       func(e *env) { e.persist = "5555" },
		"persist unreadable": func(e *env) { e.persFails = true },
	} {
		e := newEnv()
		e.tcpipFail = true
		set(e)
		s, err := e.controller().Enable(context.Background())
		if err == nil || !e.until.Equal(t0.Add(Lifetime)) || !s.Enabled {
			t.Errorf("%s: until %v state %+v err %v", name, e.until, s, err)
		}
	}
}

// After `adb usb` closed the port and clearing the expiry failed, the retry
// skips `adb usb` only if the port reads closed: a persist port with the
// service property empty is open.
func TestRetryAfterAFailedSaveSeesAPersistPort(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.now = t0.Add(2 * time.Hour)
	e.saveFail = true
	ctl := e.controller()
	if _, _, err := ctl.Expire(context.Background()); err == nil {
		t.Fatal("the save did not fail")
	}
	e.saveFail, e.events = false, nil
	e.persist = "5555"
	if _, changed, err := ctl.Expire(context.Background()); err != nil || !changed || !e.ran("usb") {
		t.Fatalf("changed %v err %v (%q)", changed, err, e.events)
	}
}

// Every "off" returns what it read of persist.adb.tcp.port, and State keeps
// it: off is never a plain State{} while persist names a port, or while it
// could not be read.
func TestOffSaysWhatPersistHolds(t *testing.T) {
	type off func(*env, *Controller) (State, error)
	disable := func(_ *env, c *Controller) (State, error) { return c.Disable(context.Background()) }
	expire := func(e *env, c *Controller) (State, error) {
		e.now = t0.Add(2 * time.Hour)
		s, _, err := c.Expire(context.Background())
		return s, err
	}
	reconcile := func(e *env, c *Controller) (State, error) {
		e.now, e.port = t0.Add(2*time.Hour), "5555"
		return c.Reconcile(context.Background())
	}
	for _, c := range []struct {
		name  string
		until bool // the mode is on (an expiry saved)
		port  string
		do    off
		keeps bool // an unreadable port changes nothing (and is an error)
	}{
		{"Disable, mode off, service at 0", false, "0", disable, false},
		{"Disable, mode off, service empty", false, "", disable, false},
		{"Disable, mode on", true, "5555", disable, false},
		{"Expire", true, "5555", expire, false},
		{"Reconcile past the expiry", true, "5555", reconcile, true},
	} {
		for _, p := range []struct {
			persist   string
			persFails bool
			want      State
		}{
			{"5555", false, State{Persist: "5555"}},
			{"7777", false, State{Persist: "7777"}},
			{"", false, State{}},
			{"0", false, State{}},
			{"", true, State{PersistUnknown: true}},
		} {
			e := newEnv()
			if c.until {
				e.until = t0.Add(time.Hour)
			}
			e.port, e.persist, e.persFails = c.port, p.persist, p.persFails
			ctl := e.controller()
			s, err := c.do(e, ctl)
			if c.keeps && p.persFails {
				if err == nil || !s.Enabled || !s.PersistUnknown || e.ran("usb") {
					t.Errorf("%s, persist unreadable: %+v %v (%q)", c.name, s, err, e.events)
				}
				continue
			}
			if err != nil || s != p.want || ctl.State() != p.want {
				t.Errorf("%s, persist %q (fails %v): returned %+v, State %+v, err %v; want %+v (%q)", c.name, p.persist, p.persFails, s, ctl.State(), err, p.want, e.events)
			}
		}
	}
}

// No restart loop: with persist set, `adb usb` runs once (the service property
// was empty, as after a reboot); it leaves the service property at 0, and the
// next "off" only reads and says so again.
func TestPersistDoesNotRestartAdbdAgain(t *testing.T) {
	e := newEnv()
	e.persist = "5555"
	ctl := e.controller()
	for i := 0; i < 3; i++ {
		s, err := ctl.Disable(context.Background())
		if err != nil || s != (State{Persist: "5555"}) {
			t.Fatalf("off %d: %+v %v", i+1, s, err)
		}
		e.port = "0" // what adb usb leaves
	}
	if got := strings.Join(e.events, "|"); got != readPort+"|adb usb|save zero|"+readPort+"|"+readPort {
		t.Fatalf("events %q: want one adb usb", got)
	}
}

// A later read replaces the earlier one: persist emptied (with root) is no
// longer reported, and a read that fails turns it unknown.
func TestStateKeepsTheLastReadOfPersist(t *testing.T) {
	e := newEnv()
	e.port, e.persist = "0", "5555"
	ctl := e.controller()
	if s, _ := ctl.Disable(context.Background()); s.Persist != "5555" {
		t.Fatalf("%+v", s)
	}
	e.persist = ""
	if s, _ := ctl.Disable(context.Background()); s != (State{}) || ctl.State() != (State{}) {
		t.Fatalf("emptied: %+v %+v", s, ctl.State())
	}
	e.persFails = true
	if s, _ := ctl.Disable(context.Background()); s != (State{PersistUnknown: true}) {
		t.Fatalf("unreadable: %+v", s)
	}
}

// `adb usb` failing with a persist port: an error, the mode as it was (on
// stays on, with its expiry), and the persist port still in the state.
func TestUsbFailsWithAPersistPort(t *testing.T) {
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persist, e.usbFail = "5555", true
	s, err := e.controller().Disable(context.Background())
	if err == nil || !s.Enabled || s.Persist != "5555" || !e.until.Equal(t0.Add(time.Hour)) {
		t.Fatalf("mode on: %+v %v until %v", s, err, e.until)
	}
	e = newEnv()
	e.persist, e.usbFail = "5555", true
	if s, err := e.controller().Disable(context.Background()); err == nil || s != (State{Persist: "5555"}) || !e.ran("usb") {
		t.Fatalf("mode off: %+v %v (%q)", s, err, e.events)
	}
}

// The read of the port has its own deadline (device.ReadTCPWait): a hung
// getprop of persist ends as "unknown" and `adb usb` still runs within the
// caller's budget.
func TestReadTCPHasItsOwnWait(t *testing.T) {
	old := device.ReadTCPWait
	device.ReadTCPWait = 20 * time.Millisecond
	defer func() { device.ReadTCPWait = old }()
	e := newEnv()
	e.until = t0.Add(time.Hour)
	e.persHang = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := time.Now()
	s, err := e.controller().Disable(ctx)
	if err != nil || s != (State{PersistUnknown: true}) || !e.ran("usb") || time.Since(start) > 5*time.Second {
		t.Fatalf("%+v %v after %s (%q)", s, err, time.Since(start), e.events)
	}
}

// Note: a read made elsewhere (serve's invariant) feeds State from the start;
// a later read of the controller replaces it.
func TestNoteFeedsState(t *testing.T) {
	e := newEnv()
	ctl := e.controller()
	ctl.Note(device.TCP{Persist: "5555"})
	if s := ctl.State(); s != (State{Persist: "5555"}) {
		t.Fatalf("%+v", s)
	}
	e.port = "0"
	if s, err := ctl.Disable(context.Background()); err != nil || s != (State{}) || ctl.State() != (State{}) {
		t.Fatalf("after a read: %+v %v", s, err)
	}
	e.events = nil
	ctl.Note(device.TCP{})
	if len(e.events) != 0 {
		t.Fatalf("Note ran adb: %q", e.events)
	}
}
