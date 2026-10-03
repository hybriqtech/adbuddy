package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

const (
	tsHome  = "100.64.0.5:5555" // the phone's Tailscale IP at the port of the remote mode
	otherTS = "100.64.0.9:40000"
	lanAddr = "198.51.100.7:40000"
)

func TestCandidateAddrs(t *testing.T) {
	cases := []struct {
		name     string
		cfg      config.Config
		remoteOn bool
		want     []string
	}{
		{"nothing", config.Config{}, false, nil},
		{"only address", config.Config{Address: lanAddr}, false, []string{lanAddr}},
		{"address and tailscale", config.Config{Address: lanAddr, TailscaleIP: "100.64.0.5", TailscalePort: 41234}, false,
			[]string{lanAddr, tsAddr}},
		{"remote on adds only ts:5555", config.Config{Address: lanAddr, TailscaleIP: "100.64.0.5", TailscalePort: 41234}, true,
			[]string{lanAddr, tsAddr, tsHome}},
		{"remote on without a port still adds ts:5555", config.Config{TailscaleIP: "100.64.0.5"}, true, []string{tsHome}},
		{"remote on without tailscale adds nothing", config.Config{Address: lanAddr}, true, []string{lanAddr}},
		{"duplicates are dropped", config.Config{Address: tsAddr, TailscaleIP: "100.64.0.5", TailscalePort: 41234}, false, []string{tsAddr}},
		{"duplicate of ts:5555", config.Config{Address: tsHome, TailscaleIP: "100.64.0.5", TailscalePort: 41234}, true, []string{tsHome, tsAddr}},
		{"remote off does not add 5555", config.Config{TailscaleIP: "100.64.0.5", TailscalePort: 41234}, false, []string{tsAddr}},
		{"unreadable address does not break", config.Config{Address: "not an address", TailscaleIP: "100.64.0.5"}, true, []string{"not an address", tsHome}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := candidateAddrs(c.cfg, c.remoteOn); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// rig is a fake adb whose rules the test changes between steps.
type rig struct {
	dir, log string
	runner   adb.Runner
}

func newRig(t *testing.T, rules ...testutil.FakeRule) *rig {
	t.Helper()
	bin := fakeADB(t)
	r := &rig{dir: t.TempDir()}
	r.log = filepath.Join(r.dir, "adb.log")
	r.set(t, rules...)
	t.Setenv("FAKEADB_SCRIPT", filepath.Join(r.dir, "fakeadb.json"))
	t.Setenv("FAKEADB_LOG", r.log)
	r.runner = adb.Runner{Path: bin}
	return r
}

func (r *rig) set(t *testing.T, rules ...testutil.FakeRule) {
	t.Helper()
	if _, err := testutil.WriteScript(r.dir, rules...); err != nil {
		t.Fatal(err)
	}
}

func (r *rig) calls(t *testing.T) []string { return loggedCalls(t, r.log) }

func (r *rig) count(t *testing.T, prefix string) int {
	t.Helper()
	n := 0
	for _, c := range r.calls(t) {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func devicesRule(addrs ...string) testutil.FakeRule {
	out := "List of devices attached\n"
	for _, a := range addrs {
		out += a + "\tdevice\n"
	}
	return testutil.FakeRule{Match: "^devices$", Stdout: out}
}

func connectMatch(addr string) string {
	return "^connect " + strings.ReplaceAll(addr, ".", `\.`) + "$"
}

func connectRule(addr string) testutil.FakeRule {
	return testutil.FakeRule{Match: connectMatch(addr), Stdout: "connected to " + addr + "\n"}
}

func failRule(addr string) testutil.FakeRule {
	return testutil.FakeRule{Match: connectMatch(addr), Stdout: "failed to connect to '" + addr + "'\n"}
}

func serialnoIs(s string) testutil.FakeRule {
	return testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: s + "\n"}
}

var noMDNS = testutil.FakeRule{Match: "^mdns services$", Stdout: "List of discovered mdns services\n"}

// --- findKnownPhone with several addresses ---

func TestFindKnownPhoneSecondAddressWorks(t *testing.T) {
	setVar(t, &knownAddrWait, time.Second)
	r := newRig(t, failRule(lanAddr), connectRule(tsAddr), devicesRule(tsAddr), serialnoIs("AAAA1111"), noMDNS)
	var log bytes.Buffer
	k, err := findKnownPhone(context.Background(), r.runner, phoneSerial, []string{lanAddr, tsAddr}, 50*time.Millisecond, &log)
	if err != nil || k.Used != tsAddr || k.Active != tsAddr || !k.Verified {
		t.Fatalf("k %+v err %v log %q", k, err, log.String())
	}
}

func TestFindKnownPhoneSkipsAnotherPhoneAndTriesTheNext(t *testing.T) {
	setVar(t, &knownAddrWait, time.Second)
	wrong := testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "ZZZZ9999\n", Times: 1}
	r := newRig(t, devicesRule(otherTS, tsAddr), wrong, serialnoIs("AAAA1111"), noMDNS)
	var log bytes.Buffer
	k, err := findKnownPhone(context.Background(), r.runner, phoneSerial, []string{otherTS, tsAddr}, 50*time.Millisecond, &log)
	if err != nil || k.Used != tsAddr || !k.Verified {
		t.Fatalf("k %+v err %v log %q", k, err, log.String())
	}
	if !strings.Contains(log.String(), "Warning: the phone at "+otherTS) {
		t.Fatalf("no warning: %q", log.String())
	}
	if r.count(t, "disconnect "+otherTS) != 1 {
		t.Fatalf("the other phone was not disconnected: %q", r.calls(t))
	}
}

func TestFindKnownPhoneNoAddressServesFallsToMDNS(t *testing.T) {
	setVar(t, &knownAddrWait, time.Second)
	r := newRig(t, failRule(lanAddr), failRule(otherTS), devicesRule(), noMDNS)
	var log bytes.Buffer
	k, err := findKnownPhone(context.Background(), r.runner, phoneSerial, []string{lanAddr, otherTS}, 50*time.Millisecond, &log)
	if !errors.Is(err, adb.ErrNoDevice) {
		t.Fatalf("err %v", err)
	}
	if k.AddrErr == nil || !strings.Contains(AsError(k.AddrErr).Message, otherTS) {
		t.Fatalf("AddrErr is not the last error: %v", k.AddrErr)
	}
}

func TestFindKnownPhoneWithoutAddressesIsAsBefore(t *testing.T) {
	r := newRig(t, devicesRule(), noMDNS)
	_, err := findKnownPhone(context.Background(), r.runner, phoneSerial, nil, 50*time.Millisecond, &bytes.Buffer{})
	if !errors.Is(err, adb.ErrNoDevice) {
		t.Fatalf("err %v", err)
	}
	if r.count(t, "connect") != 0 {
		t.Fatalf("connected without an address: %q", r.calls(t))
	}
}

// --- the watcher ---

// watcherFor builds a watcher over r for a session that started on serial.
func watcherFor(t *testing.T, r *rig, cfg config.Config, remoteOn bool, serial string) (*watcher, *activePhone, *bytes.Buffer) {
	t.Helper()
	setVar(t, &knownAddrWait, time.Second)
	active := newActivePhone(adb.Runner{Path: r.runner.Path, Serial: serial})
	log := &bytes.Buffer{}
	w := newWatcher(phoneWatch{
		Active: active, Runner: r.runner, Serial: serial,
		Cfg: func() config.Config { return cfg }, RemoteOn: func() bool { return remoteOn }, Log: log,
		Every: time.Millisecond, Misses: 2, TryWait: 2 * time.Second,
	})
	return w, active, log
}

func tsConfig() config.Config {
	cfg := config.Default()
	cfg.Serial, cfg.TailscaleIP, cfg.TailscalePort = phoneSerial, "100.64.0.5", 41234
	return cfg
}

func steps(w *watcher, n int) {
	for i := 0; i < n; i++ {
		w.step(context.Background())
	}
}

func TestWatchPhoneStillListedNeverTries(t *testing.T) {
	r := newRig(t, devicesRule(phoneSerial), serialnoIs("AAAA1111"))
	w, active, _ := watcherFor(t, r, tsConfig(), true, phoneSerial)
	steps(w, 5)
	if n := r.count(t, "connect"); n != 0 || active.Serial() != phoneSerial {
		t.Fatalf("connects %d serial %q", n, active.Serial())
	}
}

func TestWatchPhoneAnIsolatedMissDoesNothing(t *testing.T) {
	r := newRig(t, devicesRule())
	w, active, log := watcherFor(t, r, tsConfig(), true, phoneSerial)
	steps(w, 1)
	r.set(t, devicesRule(phoneSerial))
	steps(w, 1)
	r.set(t, devicesRule())
	steps(w, 1) // the count started again: still one miss
	if r.count(t, "connect") != 0 || active.Serial() != phoneSerial || log.Len() != 0 {
		t.Fatalf("calls %q serial %q log %q", r.calls(t), active.Serial(), log.String())
	}
}

func TestWatchPhoneSwitchesToTheVerifiedCandidate(t *testing.T) {
	r := newRig(t, devicesRule(tsAddr), connectRule(tsAddr), serialnoIs("AAAA1111"))
	w, active, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
	steps(w, 1)
	if active.Serial() != phoneSerial {
		t.Fatal("switched after one miss")
	}
	steps(w, 1)
	if active.Serial() != tsAddr || !reflect.DeepEqual(active.Serials(), []string{tsAddr, phoneSerial}) {
		t.Fatalf("serial %q serials %q", active.Serial(), active.Serials())
	}
	if !strings.Contains(log.String(), "The phone is now at "+tsAddr) {
		t.Fatalf("log %q", log.String())
	}
	steps(w, 3) // the phone is listed at tsAddr: it is the active one now
	if r.count(t, "connect") != 1 {
		t.Fatalf("connects %q", r.calls(t))
	}
}

// The remote mode's candidate is the phone's Tailscale IP at port 5555.
func TestWatchPhoneTriesTailscale5555WithRemoteOn(t *testing.T) {
	r := newRig(t, failRule(tsAddr), connectRule(tsHome), devicesRule(tsHome), serialnoIs("AAAA1111"))
	w, active, _ := watcherFor(t, r, tsConfig(), true, phoneSerial)
	steps(w, 2)
	if active.Serial() != tsHome {
		t.Fatalf("serial %q calls %q", active.Serial(), r.calls(t))
	}
}

func TestWatchPhoneDoesNotTry5555WithRemoteOff(t *testing.T) {
	r := newRig(t, failRule(tsAddr), connectRule(tsHome), devicesRule(), serialnoIs("AAAA1111"))
	w, active, _ := watcherFor(t, r, tsConfig(), false, phoneSerial)
	steps(w, 4)
	if r.count(t, "connect "+tsHome) != 0 || active.Serial() != phoneSerial {
		t.Fatalf("calls %q serial %q", r.calls(t), active.Serial())
	}
}

// Another phone at the candidate: never used, disconnected, said once.
func TestWatchPhoneWrongPhoneIsNotUsed(t *testing.T) {
	r := newRig(t, devicesRule(tsAddr), connectRule(tsAddr), serialnoIs("ZZZZ9999"))
	w, active, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
	steps(w, 5)
	if active.Serial() != phoneSerial || !reflect.DeepEqual(active.Serials(), []string{phoneSerial}) {
		t.Fatalf("switched to another phone: %q", active.Serials())
	}
	if n := strings.Count(log.String(), "is not the one you set up"); n != 1 {
		t.Fatalf("%d warnings: %q", n, log.String())
	}
	if r.count(t, "disconnect "+tsAddr) == 0 {
		t.Fatalf("the other phone stayed connected: %q", r.calls(t))
	}
	if r.count(t, "connect "+tsAddr) < 2 {
		t.Fatalf("it did not keep trying: %q", r.calls(t))
	}
}

func TestWatchPhoneNothingServesRetriesAndWarnsOnce(t *testing.T) {
	r := newRig(t, devicesRule(), failRule(tsAddr))
	w, active, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
	steps(w, 2)
	if n := r.count(t, "connect"); n != 1 {
		t.Fatalf("connects %d", n)
	}
	steps(w, 3)
	if r.count(t, "connect") != 4 {
		t.Fatalf("it did not retry on every tick: %q", r.calls(t))
	}
	if n := strings.Count(log.String(), "not reachable"); n != 1 {
		t.Fatalf("%d warnings: %q", n, log.String())
	}
	// It comes back by the same serial (mDNS): nothing changes, it is said.
	r.set(t, devicesRule(phoneSerial))
	steps(w, 1)
	if active.Serial() != phoneSerial || !strings.Contains(log.String(), "reachable again") {
		t.Fatalf("serial %q log %q", active.Serial(), log.String())
	}
	// And the next loss warns again.
	r.set(t, devicesRule(), failRule(tsAddr))
	steps(w, 2)
	if n := strings.Count(log.String(), "not reachable"); n != 2 {
		t.Fatalf("%d warnings: %q", n, log.String())
	}
}

// mDNS brings the phone back under another name: verified, then switched.
func TestWatchPhoneMDNSOtherNameIsVerifiedBeforeSwitch(t *testing.T) {
	other := "adb-AAAA1111-bbb (2)._adb-tls-connect._tcp"
	r := newRig(t, devicesRule(other), serialnoIs("AAAA1111"))
	w, active, _ := watcherFor(t, r, tsConfig(), true, phoneSerial)
	steps(w, 2)
	if active.Serial() != other {
		t.Fatalf("serial %q", active.Serial())
	}
	if r.count(t, "shell getprop ro.serialno") == 0 {
		t.Fatal("switched without checking the identity")
	}
	if r.count(t, "connect") != 0 {
		t.Fatalf("connected: %q", r.calls(t))
	}
}

// The guid is announced by another phone: not switched, said once.
func TestWatchPhoneMDNSOtherNameWrongPhone(t *testing.T) {
	other := "adb-AAAA1111-bbb (2)._adb-tls-connect._tcp"
	r := newRig(t, devicesRule(other), serialnoIs("ZZZZ9999"), failRule(tsAddr))
	w, active, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
	steps(w, 5)
	if active.Serial() != phoneSerial {
		t.Fatalf("switched to %q", active.Serial())
	}
	if n := strings.Count(log.String(), "is not the one you set up"); n != 1 {
		t.Fatalf("%d warnings: %q", n, log.String())
	}
}

// A saved name without a usable serial number cannot be checked: nothing is
// tried and nothing is connected, because a Switch is only justified by a
// verified identity (the daemon keeps the identity it started with).
func TestWatchPhoneWithoutVerifiableIdentityTriesNothing(t *testing.T) {
	for _, serial := range []string{"R5CT1234ABC", "adb-unknown-bbb._adb-tls-connect._tcp", "adb-ab-bbb._adb-tls-connect._tcp"} {
		t.Run(serial, func(t *testing.T) {
			r := newRig(t, devicesRule(tsAddr, "adb-unknown-bbb (2)._adb-tls-connect._tcp"), connectRule(tsAddr), serialnoIs("AAAA1111"))
			cfg := tsConfig()
			cfg.Serial = serial
			w, active, log := watcherFor(t, r, cfg, true, serial)
			steps(w, 6)
			if r.count(t, "connect") != 0 || r.count(t, "shell") != 0 || active.Serial() != serial {
				t.Fatalf("calls %q serial %q", r.calls(t), active.Serial())
			}
			if n := strings.Count(log.String(), "cannot be checked"); n != 1 {
				t.Fatalf("%d warnings: %q", n, log.String())
			}
			// It comes back by the same serial: all is as before.
			r.set(t, devicesRule(serial))
			steps(w, 1)
			if active.Serial() != serial {
				t.Fatal("serial changed")
			}
		})
	}
}

// A session for another phone than the one in config.toml never uses the
// addresses of config.toml.
func TestWatchPhoneOtherSessionPhoneDoesNotUseTheSavedAddresses(t *testing.T) {
	r := newRig(t, devicesRule(tsAddr), connectRule(tsAddr), serialnoIs("BBBB2222"))
	other := "adb-BBBB2222-ccc._adb-tls-connect._tcp"
	w, active, _ := watcherFor(t, r, tsConfig(), true, other)
	steps(w, 4)
	if r.count(t, "connect") != 0 || active.Serial() != other {
		t.Fatalf("calls %q serial %q", r.calls(t), active.Serial())
	}
}

// The watcher never opens or closes a port: it only reads it (getprop) when
// the phone comes back (VB-41).
func TestWatchPhoneNeverTouchesThePort(t *testing.T) {
	r := newRig(t, devicesRule(tsAddr), connectRule(tsAddr), serialnoIs("AAAA1111"))
	w, _, _ := watcherFor(t, r, tsConfig(), true, phoneSerial)
	steps(w, 6)
	read := 0
	for _, c := range r.calls(t) {
		if strings.HasPrefix(c, "tcpip") || c == "usb" || strings.Contains(c, "setprop") {
			t.Fatalf("the watcher touched the port: %q", c)
		}
		if strings.HasPrefix(c, "shell getprop") && strings.Contains(c, "adb.tcp.port") {
			read++
		}
	}
	if read == 0 {
		t.Fatalf("the port was not read after the switch: %q", r.calls(t))
	}
}

// Cancelling ends the goroutine even inside a hung `adb connect`.
func TestWatchPhoneCancelEndsAHungConnect(t *testing.T) {
	hang := testutil.FakeRule{Match: "^connect ", SleepMs: 60000}
	r := newRig(t, devicesRule(), hang)
	active := newActivePhone(adb.Runner{Path: r.runner.Path, Serial: phoneSerial})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	w := phoneWatch{Active: active, Runner: r.runner, Serial: phoneSerial,
		Cfg: func() config.Config { return tsConfig() }, RemoteOn: func() bool { return false }, Log: &bytes.Buffer{},
		Every: time.Millisecond, Misses: 1, TryWait: time.Minute}
	go func() { defer close(done); watchPhone(ctx, w) }()
	deadline := time.Now().Add(30 * time.Second)
	for r.count(t, "connect") == 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("it never tried to connect")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("watchPhone did not end after cancel")
	}
	if active.Serial() != phoneSerial {
		t.Fatalf("serial %q", active.Serial())
	}
}

// V38: a look at `adb devices` that hangs is cut at TryWait, so the watcher
// does not stop for good inside one step (adb failing says nothing about the
// phone: nothing changes).
func TestWatchPhoneAHungDevicesIsCut(t *testing.T) {
	hang := testutil.FakeRule{Match: "^devices$", SleepMs: 20000}
	r := newRig(t, hang)
	active := newActivePhone(adb.Runner{Path: r.runner.Path, Serial: phoneSerial})
	w := newWatcher(phoneWatch{Active: active, Runner: r.runner, Serial: phoneSerial,
		Cfg: func() config.Config { return tsConfig() }, RemoteOn: func() bool { return false }, Log: &bytes.Buffer{},
		Every: time.Millisecond, Misses: 1, TryWait: 500 * time.Millisecond})
	start := time.Now()
	w.step(context.Background())
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("one step took %v", d)
	}
	if active.Serial() != phoneSerial || w.count != 0 {
		t.Fatalf("serial %q count %d", active.Serial(), w.count)
	}
}

// Validation 15: the switch of address with the remote mode on. The watcher
// needs watchMisses looks, watchEvery apart, to notice the phone is gone; then
// each candidate address costs up to watchTryWait in `adb connect`
// (connectAddr) plus identityWait in verifyAt, one after the other. With the
// three addresses of a phone that has its home address, its Tailscale address
// with the wireless debugging port and the port of the remote mode, that is
// 2*10 + 3*(8+8) = 68 s. This is the bound of the candidate loop; the
// `adb devices` look (+ watchTryWait) and the mDNS-named entries (each a
// verifyAt: + identityWait) add on top, so 68 s is not a ceiling. It is MORE
// than the ~30 s the validation promised (misses*every + tryWait = 28 s is only
// the best case of the last address answering at once). The test fixes the
// bound so that a change to any of those times, or to the candidates, is a
// decision and not an accident; it does not say 68 s is acceptable (see the
// deferred item of the plan).
func TestWatchPhoneSwitchBudget(t *testing.T) {
	const worstCase = 68 * time.Second
	cfg := config.Default()
	cfg.Address = lanAddr
	cfg.TailscaleIP, cfg.TailscalePort = "100.64.0.5", 40000
	n := len(candidateAddrs(cfg, true))
	if n != 3 {
		t.Fatalf("%d candidates with the remote mode on: %v", n, candidateAddrs(cfg, true))
	}
	if got := candidateAddrs(cfg, true); got[n-1] != tsHome {
		t.Fatalf("the port of the remote mode is not the last candidate: %v", got)
	}
	got := time.Duration(watchMisses)*watchEvery + time.Duration(n)*(watchTryWait+identityWait)
	if got != worstCase {
		t.Fatalf("the worst case of the switch is %v (misses %d * every %v + %d candidates * (try %v + identity %v)), pinned at %v", got, watchMisses, watchEvery, n, watchTryWait, identityWait, worstCase)
	}
}

// VB-41 (review I1): a phone that comes back may have restarted and taken
// persist.adb.tcp.port. The watcher reads the port again (it never closes it
// mid-session) and says once what changed: a port open with the remote mode
// off, and the persist port.
func TestWatchPhoneRereadsThePortWhenThePhoneComesBack(t *testing.T) {
	back := func(t *testing.T, r *rig) {
		r.set(t, devicesRule(phoneSerial), testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"},
			testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: "5555\n"})
	}
	t.Run("remote mode off", func(t *testing.T) {
		r := newRig(t, devicesRule(), failRule(tsAddr))
		w, _, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
		steps(w, 2)
		back(t, r)
		steps(w, 2)
		if strings.Count(log.String(), "adb listens on network port 5555 on the phone again") != 1 || strings.Count(log.String(), "persist.adb.tcp.port is 5555") != 1 {
			t.Fatalf("log %q", log.String())
		}
		if r.count(t, "usb") != 0 || r.count(t, "tcpip") != 0 {
			t.Fatalf("the watcher closed or opened a port: %q", r.calls(t))
		}
		// Lost and back again with the same port: nothing new to say.
		r.set(t, devicesRule(), failRule(tsAddr))
		steps(w, 2)
		back(t, r)
		steps(w, 1)
		if n := strings.Count(log.String(), "persist.adb.tcp.port is 5555"); n != 1 {
			t.Fatalf("said %d times: %q", n, log.String())
		}
	})
	t.Run("remote mode on", func(t *testing.T) {
		r := newRig(t, devicesRule(), failRule(tsAddr), failRule(tsHome))
		w, _, log := watcherFor(t, r, tsConfig(), true, phoneSerial)
		steps(w, 2)
		back(t, r)
		steps(w, 1)
		if strings.Contains(log.String(), "adb listens on network port") || strings.Count(log.String(), "persist.adb.tcp.port is 5555") != 1 {
			t.Fatalf("log %q", log.String())
		}
	})
}

// VB-41 (review round 1): what the watcher re-reads and says, and how often.
func TestWatchPhoneRecheckSaysOnlyWhatChanged(t *testing.T) {
	persistIs := func(v string) testutil.FakeRule {
		return testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stdout: v + "\n"}
	}
	persistFails := testutil.FakeRule{Match: `^shell getprop persist\.adb\.tcp\.port$`, Stderr: "error: closed\n", Exit: 1}
	serviceIs := func(v string) testutil.FakeRule { return testutil.FakeRule{Match: tcpPortRule, Stdout: v + "\n"} }
	lost := func(t *testing.T, r *rig, w *watcher) {
		r.set(t, devicesRule(), failRule(tsAddr))
		steps(w, 2)
	}
	back := func(t *testing.T, r *rig, w *watcher, rules ...testutil.FakeRule) {
		r.set(t, append([]testutil.FakeRule{devicesRule(phoneSerial)}, rules...)...)
		steps(w, 1)
	}
	const unknownMsg = "Could not read persist.adb.tcp.port"
	const persistMsg = "persist.adb.tcp.port is 5555"

	t.Run("persist unreadable twice in a row says it once", func(t *testing.T) {
		r := newRig(t, devicesRule(), failRule(tsAddr))
		w, _, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
		steps(w, 2)
		back(t, r, w, serviceIs(""), persistFails)
		lost(t, r, w)
		back(t, r, w, serviceIs(""), persistFails)
		if n := strings.Count(log.String(), unknownMsg); n != 1 {
			t.Fatalf("said %d times: %q", n, log.String())
		}
	})
	t.Run("a stable phone is not read again", func(t *testing.T) {
		r := newRig(t, devicesRule(phoneSerial))
		w, _, _ := watcherFor(t, r, tsConfig(), false, phoneSerial)
		steps(w, 8)
		if n := r.count(t, "shell getprop"); n != 0 {
			t.Fatalf("%d getprop with the phone listed all the time: %q", n, r.calls(t))
		}
	})
	t.Run("a change of the service port alone does not repeat persist", func(t *testing.T) {
		r := newRig(t, devicesRule(), failRule(tsAddr))
		w, _, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
		steps(w, 2)
		back(t, r, w, serviceIs(""), persistIs("5555"))
		lost(t, r, w)
		back(t, r, w, serviceIs("7777"), persistIs("5555"))
		if n := strings.Count(log.String(), persistMsg); n != 1 {
			t.Fatalf("persist said %d times: %q", n, log.String())
		}
		if !strings.Contains(log.String(), "network port 7777 on the phone again") {
			t.Fatalf("the new port was not said: %q", log.String())
		}
	})
	t.Run("a good read after an unreadable one says what it holds", func(t *testing.T) {
		r := newRig(t, devicesRule(), failRule(tsAddr))
		w, _, log := watcherFor(t, r, tsConfig(), false, phoneSerial)
		steps(w, 2)
		back(t, r, w, serviceIs(""), persistIs("5555"))
		lost(t, r, w)
		back(t, r, w, serviceIs(""), persistFails)
		lost(t, r, w)
		back(t, r, w, serviceIs(""), persistIs("5555"))
		if n := strings.Count(log.String(), persistMsg); n != 2 {
			t.Fatalf("persist said %d times, want 2 (before and after the unreadable one): %q", n, log.String())
		}
		if n := strings.Count(log.String(), unknownMsg); n != 1 {
			t.Fatalf("unreadable said %d times: %q", n, log.String())
		}
	})
}
