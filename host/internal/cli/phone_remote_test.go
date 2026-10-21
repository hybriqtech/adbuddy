package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// sentMsg is what the link sent to the app: kind is "rid" (PublishRID),
// "event" (Publish) or "reply" (Reply).
type sentMsg struct {
	kind, typ, rid string
	payload        any
}

type fakeRemoteSrv struct {
	current atomic.Bool
	mu      sync.Mutex
	sent    []sentMsg
}

func (f *fakeRemoteSrv) Current(bridge.Message) bool { return f.current.Load() }

func (f *fakeRemoteSrv) add(s sentMsg) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, s)
	return nil
}

func (f *fakeRemoteSrv) PublishRID(typ, rid string, p any) error {
	return f.add(sentMsg{"rid", typ, rid, p})
}
func (f *fakeRemoteSrv) Publish(typ string, p any) error { return f.add(sentMsg{"event", typ, "", p}) }
func (f *fakeRemoteSrv) Reply(m bridge.Message, typ string, p any) error {
	return f.add(sentMsg{"reply", typ, m.RID, p})
}

func (f *fakeRemoteSrv) messages() []sentMsg {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentMsg{}, f.sent...)
}

// rigInstance is the mDNS instance of phoneSerial's connect service.
const rigInstance = "adb-AAAA1111-bbb"

// mdnsLine is one line of `adb mdns services` for a connect service.
func mdnsLine(name, addr string) string {
	return name + "\t" + pairing.ConnectType + "\t" + addr + "\n"
}

// connADB is the phone as the remote link sees it: the serial adb lists it
// with, and adb's commands. `devices` lists serial in state (device when
// empty; "absent" leaves it out); `mdns services` lists mdns; `shell getprop
// service.adb.tls.port` answers empty with code 0, as on a real phone (SELinux
// keeps the shell from reading it, validation M5b-11). fail makes every
// command fail; gate, when set, makes each one wait for it to close before
// answering; block makes each one wait for its context to end.
type connADB struct {
	mu     sync.Mutex
	serial string
	state  string
	mdns   string
	fail   bool
	calls  []string
	gate   chan struct{}
	block  bool
}

// newConnADB is the rig's phone, connected by mDNS: port is the one its
// connect service announces ("" announces none, "fail" makes adb fail).
func newConnADB(port string) *connADB {
	a := &connADB{serial: phoneSerial}
	switch port {
	case "":
	case "fail":
		a.fail = true
	default:
		a.mdns = mdnsLine(rigInstance, "192.0.2.10:"+port)
	}
	return a
}

func (a *connADB) Serial() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.serial
}

func (a *connADB) Host() pairing.ADB { return a }

func (a *connADB) set(serial, mdns string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.serial, a.mdns = serial, mdns
}

func (a *connADB) setState(state string, fail bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state, a.fail = state, fail
}

func (a *connADB) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if a.block {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	if a.gate != nil {
		<-a.gate
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cmd := strings.Join(args, " ")
	a.calls = append(a.calls, cmd)
	if a.fail {
		return 1, errors.New("adb down")
	}
	switch cmd {
	case "devices":
		io.WriteString(stdout, "List of devices attached\n")
		if state := a.state; state != "absent" {
			if state == "" {
				state = "device"
			}
			io.WriteString(stdout, a.serial+"\t"+state+"\n")
		}
	case "mdns services":
		io.WriteString(stdout, "List of discovered mdns services\n"+a.mdns)
	case "shell getprop service.adb.tls.port":
		io.WriteString(stdout, "\n")
	}
	return 0, nil
}

func (a *connADB) called() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.calls...)
}

// ctlADB is the phone for the remote.Controller: it records the orders and
// fails the ones in fail.
type ctlADB struct {
	mu      sync.Mutex
	calls   []string
	fail    map[string]bool
	hang    map[string]bool // wait for the context to end
	gate    chan struct{}   // `tcpip 5555` waits for it to close
	port    string          // answer of getprop service.adb.tcp.port
	persist string          // answer of getprop persist.adb.tcp.port
	stderr  string          // what a failing command says (default "error: closed")
}

func (a *ctlADB) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
	key := strings.Join(args, " ")
	a.mu.Lock()
	a.calls = append(a.calls, key)
	hang, gate, fail, port, persist, serr := a.hang[key], a.gate, a.fail[key], a.port, a.persist, a.stderr
	a.mu.Unlock()
	if hang {
		<-ctx.Done()
		return -1, ctx.Err()
	}
	if gate != nil && key == "tcpip 5555" {
		<-gate
	}
	if fail {
		if serr == "" {
			serr = "error: closed"
		}
		io.WriteString(stderr, serr)
		return 1, nil
	}
	if key == "shell getprop service.adb.tcp.port" {
		io.WriteString(stdout, port+"\n")
	}
	if key == "shell getprop persist.adb.tcp.port" {
		io.WriteString(stdout, persist+"\n")
	}
	return 0, nil
}

func (a *ctlADB) called() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.calls...)
}

func (a *ctlADB) count(cmd string) int {
	n := 0
	for _, c := range a.called() {
		if c == cmd {
			n++
		}
	}
	return n
}

func (a *ctlADB) setFail(cmd string, on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fail[cmd] = on
}

// testClock is the controller's clock.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type remoteRig struct {
	r      *remoteLink
	store  *cfgStore
	srv    *fakeRemoteSrv
	adb    *connADB
	phone  *ctlADB
	clock  *testClock
	ctl    *remote.Controller
	path   string
	log    *syncBuffer
	saves  atomic.Int32
	ctx    context.Context
	cancel context.CancelFunc
}

func newRemoteRig(t *testing.T, cfg config.Config, port string) *remoteRig {
	t.Helper()
	rig := &remoteRig{adb: newConnADB(port), srv: &fakeRemoteSrv{}, log: &syncBuffer{}, path: filepath.Join(t.TempDir(), "config.toml"),
		phone: &ctlADB{fail: map[string]bool{}, hang: map[string]bool{}}, clock: &testClock{t: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)}}
	rig.ctx, rig.cancel = context.WithCancel(context.Background())
	t.Cleanup(rig.cancel)
	rig.srv.current.Store(true)
	rig.store = newCfgStore(rig.path, cfg)
	rig.store.save = func(p string, c config.Config) error {
		rig.saves.Add(1)
		return config.Save(p, c)
	}
	rig.ctl = remote.New(rig.phone, rig.store, rig.clock.now)
	rig.r = newRemoteLink(rig.ctx, rig.store, rig.ctl, rig.adb, rig.log)
	rig.r.setServer(rig.srv)
	return rig
}

// withTailscale is a config of a phone whose Tailscale address is known.
func withTailscale() config.Config {
	cfg := config.Default()
	cfg.TailscaleIP, cfg.TailscalePort = "100.101.102.103", 40000
	return cfg
}

func setMsg(rid, payload string) bridge.Message {
	return bridge.Message{Type: bridge.TypeRemoteSet, RID: rid, Payload: json.RawMessage(payload)}
}

// waitMsgs waits for n messages to the app and returns them.
func (g *remoteRig) waitMsgs(t *testing.T, n int) []sentMsg {
	t.Helper()
	waitUntil(t, "messages to the app", func() bool { return len(g.srv.messages()) >= n })
	return g.srv.messages()
}

func errOf(t *testing.T, m sentMsg) bridge.ErrorReply {
	t.Helper()
	e, ok := m.payload.(bridge.ErrorReply)
	if m.kind != "reply" || m.typ != bridge.TypeError || !ok {
		t.Fatalf("not an error reply: %+v", m)
	}
	return e
}

func stateOfMsg(t *testing.T, m sentMsg) bridge.RemoteState {
	t.Helper()
	s, ok := m.payload.(bridge.RemoteState)
	if m.kind != "rid" || m.typ != bridge.TypeRemoteState || !ok {
		t.Fatalf("not a remote.state answer: %+v", m)
	}
	return s
}

// idle waits until every report sent so far has been dealt with.
func (g *remoteRig) idle(t *testing.T) {
	t.Helper()
	waitUntil(t, "the report to be handled", func() bool {
		g.r.mu.Lock()
		defer g.r.mu.Unlock()
		return !g.r.running
	})
}

func (g *remoteRig) saved(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Load(g.path)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDeviceInfoSavesTheAddressAndThePort(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	c := g.saved(t)
	if c.TailscaleIP != "100.101.102.103" || c.TailscalePort != 41234 {
		t.Fatalf("saved %q %d", c.TailscaleIP, c.TailscalePort)
	}
	if calls := g.adb.called(); len(calls) != 1 || calls[0] != "mdns services" {
		t.Fatalf("adb calls %v", calls)
	}
}

func TestDeviceInfoRefusesAnAddressOutsideTailscale(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	for range 2 {
		g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "198.51.100.5"})
	}
	g.idle(t)
	if g.saves.Load() != 0 || len(g.adb.called()) != 0 {
		t.Fatalf("saved %d times, adb %v", g.saves.Load(), g.adb.called())
	}
	if n := strings.Count(g.log.String(), "198.51.100.5"); n != 1 {
		t.Fatalf("warned %d times: %q", n, g.log.String())
	}
}

func TestDeviceInfoWithoutAnAddressChangesNothing(t *testing.T) {
	cfg := config.Default()
	cfg.TailscaleIP, cfg.TailscalePort = "100.100.100.100", 40000
	g := newRemoteRig(t, cfg, "41234")
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{ProtectedApps: []string{"com.example.bank"}})
	g.idle(t)
	if g.saves.Load() != 0 || len(g.adb.called()) != 0 || g.store.Config().TailscaleIP != "100.100.100.100" {
		t.Fatalf("saves %d adb %v cfg %+v", g.saves.Load(), g.adb.called(), g.store.Config())
	}
}

func TestDeviceInfoPortFallbacks(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "")
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103", WifiADBPort: 40000})
	g.idle(t)
	if c := g.saved(t); c.TailscalePort != 40000 {
		t.Fatalf("port %d, want the one the app reported", c.TailscalePort)
	}

	// Neither says: the address is saved and the port stays as it was.
	cfg := config.Default()
	cfg.TailscaleIP, cfg.TailscalePort = "100.100.100.100", 30000
	g = newRemoteRig(t, cfg, "fail")
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	if c := g.saved(t); c.TailscaleIP != "100.101.102.103" || c.TailscalePort != 30000 {
		t.Fatalf("saved %q %d", c.TailscaleIP, c.TailscalePort)
	}
}

// Validation M5b-11: the phone's shell cannot read service.adb.tls.port
// (connADB answers it empty with code 0, like a real phone), so the port comes
// from the connection the host already has: the serial adb lists the phone
// with (ip:port, only while `adb devices` lists it online), or the phone's
// connect service in `adb mdns services`. Only a wireless debugging port is
// saved (1-65535, never the remote mode's 5555, never from a loopback,
// unspecified or multicast address), the connection's IP never becomes
// tailscale_ip, and a port that is not known keeps the saved one (40000).
func TestDeviceInfoTakesThePortFromTheConnection(t *testing.T) {
	const other = "adb-CCCC2222-ddd"
	for _, c := range []struct {
		name, serial, mdns string
		state              string // how `adb devices` lists serial ("" device, "absent" not at all)
		fail               bool   // adb fails
		app                int    // the port the app reports (wifiAdbPort)
		want               int    // 0: the saved port stays
	}{
		{name: "connected by a LAN address", serial: "192.0.2.10:41234", want: 41234},
		{name: "connected over Tailscale", serial: "100.101.102.103:41234", want: 41234},
		{name: "connected by IPv6", serial: "[2001:db8::10]:41234", want: 41234},
		{name: "the address wins over the app", serial: "192.0.2.10:41234", app: 41000, want: 41234},
		{name: "the address is offline", serial: "192.0.2.10:41234", state: "offline"},
		{name: "the address is unauthorized", serial: "192.0.2.10:41234", state: "unauthorized"},
		{name: "the address is no longer listed", serial: "192.0.2.10:41234", state: "absent"},
		{name: "adb devices fails", serial: "192.0.2.10:41234", fail: true},
		{name: "the app's port when the address is offline", serial: "192.0.2.10:41234", state: "offline", app: 41000, want: 41000},
		{name: "a loopback forward", serial: "127.0.0.1:15555"},
		{name: "an IPv6 loopback forward", serial: "[::1]:15555"},
		{name: "an unspecified address", serial: "0.0.0.0:41234"},
		{name: "a multicast address", serial: "224.0.0.1:41234"},
		{name: "an emulator", serial: "emulator-5554"},
		{name: "through the remote mode's 5555", serial: "100.101.102.103:5555"},
		{name: "5555 from the app is not wireless debugging", serial: "100.101.102.103:5555", app: 5555},
		{name: "the app's port when the connection has none", serial: "100.101.102.103:5555", app: 41000, want: 41000},
		{name: "by mDNS", serial: phoneSerial, mdns: mdnsLine(rigInstance, "192.0.2.10:41234"), want: 41234},
		{name: "renamed by mDNS: only its own instance", serial: rigInstance + " (2)." + pairing.ConnectType,
			mdns: mdnsLine(rigInstance, "192.0.2.10:39999") + mdnsLine(rigInstance+" (2)", "192.0.2.10:41234"), want: 41234},
		{name: "one port on two interfaces", serial: phoneSerial,
			mdns: mdnsLine(rigInstance, "192.0.2.10:41234") + mdnsLine(rigInstance, "[2001:db8::10]:41234"), want: 41234},
		{name: "mDNS announces two ports", serial: phoneSerial,
			mdns: mdnsLine(rigInstance, "192.0.2.10:41234") + mdnsLine(rigInstance, "192.0.2.66:41235")},
		{name: "mDNS announces 5555", serial: phoneSerial, mdns: mdnsLine(rigInstance, "192.0.2.10:5555")},
		{name: "mDNS announces a loopback address", serial: phoneSerial, mdns: mdnsLine(rigInstance, "127.0.0.1:41234")},
		{name: "another phone's service", serial: phoneSerial, mdns: mdnsLine(other, "192.0.2.20:41234")},
		{name: "the pairing service", serial: phoneSerial, mdns: rigInstance + "\t" + pairing.PairingType + "\t192.0.2.10:41234\n"},
		{name: "mDNS does not list it", serial: phoneSerial},
		{name: "by USB", serial: "AAAA1111"},
		{name: "port out of range", serial: "192.0.2.10:70000"},
		{name: "port 0", serial: "192.0.2.10:0"},
		{name: "not an IP", serial: "phone.example:41234"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := newRemoteRig(t, withTailscale(), "")
			g.adb.set(c.serial, c.mdns)
			g.adb.setState(c.state, c.fail)
			g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.104", WifiADBPort: c.app})
			g.idle(t)
			want := c.want
			if want == 0 {
				want = 40000
			}
			if s := g.saved(t); s.TailscaleIP != "100.101.102.104" || s.TailscalePort != want {
				t.Errorf("saved %s:%d, want 100.101.102.104:%d", s.TailscaleIP, s.TailscalePort, want)
			}
			for _, call := range g.adb.called() {
				if strings.Contains(call, "getprop") {
					t.Errorf("asked the phone's shell: %q", call)
				}
			}
		})
	}
}

// The defect of M5b-11 seen from the watcher: once the port is saved, the
// phone's Tailscale address at its wireless debugging port is a candidate.
func TestTheSavedPortIsACandidateAddress(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "")
	g.adb.set("192.0.2.10:41234", "")
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	if got := candidateAddrs(g.store.Config(), false); !slices.Contains(got, "100.101.102.103:41234") {
		t.Fatalf("candidates %v", got)
	}
}

// serve hands startPhoneLink its activePhone, and the link finds the port
// through it (the serial it has now, adb without -s for mdns services). The
// fake phones of other tests have no connection: only the app's port counts.
func TestConnOfIsTheActivePhone(t *testing.T) {
	p := newActivePhone(adb.Runner{Path: "adb", ServerPort: 5038, Serial: phoneSerial})
	c := connOf(p)
	if c == nil || c.Serial() != phoneSerial {
		t.Fatalf("conn %v", c)
	}
	p.Switch("100.101.102.103:41234")
	if c.Serial() != "100.101.102.103:41234" {
		t.Fatalf("serial after a switch %q", c.Serial())
	}
	if h, ok := c.Host().(adb.Runner); !ok || h != (adb.Runner{Path: "adb", ServerPort: 5038}) || p.Serial() != "100.101.102.103:41234" {
		t.Fatalf("host %#v, phone %q", c.Host(), p.Serial())
	}
	if c := connOf(&appPhone{}); c != nil {
		t.Fatalf("a fake phone has a connection: %v", c)
	}
	g := newRemoteRig(t, config.Default(), "")
	g.r = newRemoteLink(g.ctx, g.store, g.ctl, nil, g.log)
	g.r.setServer(g.srv)
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103", WifiADBPort: 41000})
	g.idle(t)
	if s := g.saved(t); s.TailscaleIP != "100.101.102.103" || s.TailscalePort != 41000 {
		t.Fatalf("without a connection saved %s:%d", s.TailscaleIP, s.TailscalePort)
	}
}

func TestDeviceInfoFromAReplacedConnectionIsIgnored(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	g.srv.current.Store(false)
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	if g.saves.Load() != 0 || len(g.adb.called()) != 0 {
		t.Fatalf("saves %d adb %v", g.saves.Load(), g.adb.called())
	}
	// Without the app there is no server at all.
	n := newRemoteLink(g.ctx, g.store, g.ctl, g.adb, g.log)
	n.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	if g.saves.Load() != 0 || len(g.adb.called()) != 0 {
		t.Fatal("a link without app saved something")
	}
}

func TestDeviceInfoWithTheSameValueDoesNotRewrite(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	info := bridge.DeviceInfo{TailscaleIP: "100.101.102.103"}
	g.r.onDeviceInfo(bridge.Message{}, info)
	g.idle(t)
	g.r.onDeviceInfo(bridge.Message{}, info)
	g.idle(t)
	if n := g.saves.Load(); n != 1 {
		t.Fatalf("saved %d times, want 1", n)
	}
	// A new port is a change.
	g.adb.set(phoneSerial, mdnsLine(rigInstance, "192.0.2.10:41235"))
	g.r.onDeviceInfo(bridge.Message{}, info)
	g.idle(t)
	if c := g.saved(t); g.saves.Load() != 2 || c.TailscalePort != 41235 {
		t.Fatalf("saves %d port %d", g.saves.Load(), c.TailscalePort)
	}
}

// Two reports in a row do not step on each other: the last one stays.
func TestDeviceInfoReportsAreSerialised(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	g.adb.gate = make(chan struct{})
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.104"})
	close(g.adb.gate)
	g.idle(t)
	if c := g.saved(t); c.TailscaleIP != "100.101.102.104" {
		t.Fatalf("saved %q", c.TailscaleIP)
	}
}

// chat_port and the Tailscale address are saved by the same store: neither
// undoes the other.
func TestChatPortDoesNotOverwriteTheTailscaleAddress(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	port, err := chatPort(g.store)
	if err != nil {
		t.Fatal(err)
	}
	c := g.saved(t)
	if c.ChatPort != port || c.TailscaleIP != "100.101.102.103" || c.TailscalePort != 41234 {
		t.Fatalf("saved %+v", c)
	}
}

func TestCfgStoreUndoesAChangeThatDoesNotSave(t *testing.T) {
	s := newCfgStore("x", config.Default())
	s.save = func(string, config.Config) error { return errors.New("disk full") }
	if err := s.Update(func(c *config.Config) { c.TailscaleIP = "100.1.1.1" }); err == nil || s.TailscaleIP() != "" {
		t.Fatalf("err %v ip %q", err, s.TailscaleIP())
	}
}

func TestCfgStoreKeepsItsOwnCopyAndTheExpiry(t *testing.T) {
	cfg := config.Default()
	s := newCfgStore(filepath.Join(t.TempDir(), "config.toml"), cfg)
	cfg.ChatPort = 1234
	if s.Config().ChatPort == 1234 {
		t.Fatal("the store shares its config")
	}
	until := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	if err := s.SetUntil(until); err != nil || !s.Until().Equal(until) {
		t.Fatalf("%v %v", err, s.Until())
	}
	if err := s.SetUntil(time.Time{}); err != nil || !s.Until().IsZero() {
		t.Fatalf("%v %v", err, s.Until())
	}
}

// Through the real link: device.info with only the address reaches the store
// and leaves the protected apps alone, and the other way round.
func TestDeviceInfoThroughTheLink(t *testing.T) {
	e := linkEnv(t)
	f := &appPhone{}
	l, _, err := startLink(t, e, f, &recording{})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	c := openApp(t, e.cfg.ChatPort, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: f.handedTokens()[0], AppVersion: "0.3.0"})
	if m := readApp(t, c); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	_, protected := l.helpers()

	sendApp(t, c, bridge.TypeDeviceInfo, "", bridge.DeviceInfo{TailscaleIP: "100.101.102.103", WifiADBPort: 40000})
	waitUntil(t, "the address", func() bool { return l.remote.store.TailscaleIP() == "100.101.102.103" })
	if protected("com.example.bank") {
		t.Fatal("an address-only report touched the protected apps")
	}

	sendApp(t, c, bridge.TypeDeviceInfo, "", bridge.DeviceInfo{ProtectedApps: []string{"com.example.bank"}})
	waitUntil(t, "the protected apps", func() bool { return protected("com.example.bank") })
	if l.remote.store.TailscaleIP() != "100.101.102.103" {
		t.Fatal("a protected-apps report changed the address")
	}
}

// ---- remote.set, remote.state and the expiry (Task 5) ----

func TestRemoteSetEnablesAndAnswersWithTheRID(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.r.handleSet(setMsg("r1", `{"enabled":true}`))
	msgs := g.waitMsgs(t, 1)
	st := stateOfMsg(t, msgs[0])
	if msgs[0].rid != "r1" || !st.Enabled || st.Address != "100.101.102.103:5555" || st.ExpiresAt.IsZero() {
		t.Fatalf("answer %+v", msgs[0])
	}
	if g.phone.count("tcpip 5555") != 1 {
		t.Fatalf("adb %v", g.phone.called())
	}
	// Answer and event are one call: nothing else was sent.
	time.Sleep(50 * time.Millisecond)
	if n := len(g.srv.messages()); n != 1 {
		t.Fatalf("%d messages: %+v", n, g.srv.messages())
	}
}

func TestRemoteSetWithoutTailscaleIsUnavailable(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "")
	g.r.handleSet(setMsg("r1", `{"enabled":true}`))
	e := errOf(t, g.waitMsgs(t, 1)[0])
	if e.Code != bridge.CodeUnavailable || !strings.Contains(e.Text, "Turn on Tailscale on the phone first") {
		t.Fatalf("error %+v", e)
	}
	if len(g.phone.called()) != 0 {
		t.Fatalf("adb %v", g.phone.called())
	}
}

func TestRemoteSetOffRunsUsb(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.handleSet(setMsg("r2", `{"enabled":false}`))
	msgs := g.waitMsgs(t, 1)
	if st := stateOfMsg(t, msgs[0]); st.Enabled || msgs[0].rid != "r2" {
		t.Fatalf("answer %+v", msgs[0])
	}
	if calls := g.phone.called(); strings.Join(calls, "|") != tcpPortCalls+"|usb" {
		t.Fatalf("adb %v", calls)
	}
}

func TestRemoteSetOffThatFailsStaysOn(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.refresh()
	g.phone.setFail("usb", true)
	g.r.handleSet(setMsg("r3", `{"enabled":false}`))
	if e := errOf(t, g.waitMsgs(t, 1)[0]); e.Code != bridge.CodeFailed {
		t.Fatalf("error %+v", e)
	}
	var s bridge.Snapshot
	g.r.area().snapshot(&s)
	if !s.Remote.Enabled {
		t.Fatalf("the snapshot says off with the port open: %+v", s.Remote)
	}
}

func TestRemoteSetFromAReplacedConnectionIsIgnored(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.srv.current.Store(false)
	g.r.handleSet(setMsg("r1", `{"enabled":true}`))
	time.Sleep(50 * time.Millisecond)
	if len(g.srv.messages()) != 0 || len(g.phone.called()) != 0 {
		t.Fatalf("msgs %+v adb %v", g.srv.messages(), g.phone.called())
	}
}

func TestRemoteSetWithoutRIDIsDroppedWithAWarning(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.r.handleSet(setMsg("", `{"enabled":true}`))
	time.Sleep(50 * time.Millisecond)
	if len(g.srv.messages()) != 0 || len(g.phone.called()) != 0 {
		t.Fatalf("msgs %+v adb %v", g.srv.messages(), g.phone.called())
	}
	if !strings.Contains(g.log.String(), "remote.set") {
		t.Fatalf("no warning: %q", g.log.String())
	}
}

func TestRemoteSetEveryPathAnswers(t *testing.T) {
	for name, tc := range map[string]struct {
		payload string
		noCtl   bool
		fail    bool
		code    string // "" for a remote.state
	}{
		"unreadable payload": {payload: `"x"`, code: bridge.CodeInvalid},
		"no object":          {payload: `null`, code: bridge.CodeInvalid},
		"no enabled":         {payload: `{}`, code: bridge.CodeInvalid},
		"empty payload":      {payload: ``, code: bridge.CodeInvalid},
		"enabled not a bool": {payload: `{"enabled":"yes"}`, code: bridge.CodeInvalid},
		"no controller":      {payload: `{"enabled":true}`, noCtl: true, code: bridge.CodeUnavailable},
		"success":            {payload: `{"enabled":true}`},
		"failure":            {payload: `{"enabled":true}`, fail: true, code: bridge.CodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			g := newRemoteRig(t, withTailscale(), "")
			if tc.noCtl {
				g.r = newRemoteLink(g.ctx, g.store, nil, g.adb, g.log)
				g.r.setServer(g.srv)
			}
			if tc.fail {
				g.phone.setFail("tcpip 5555", true)
			}
			g.r.handleSet(setMsg("rid", tc.payload))
			m := g.waitMsgs(t, 1)[0]
			if m.rid != "rid" {
				t.Fatalf("answer without the rid: %+v", m)
			}
			if tc.code == "" {
				stateOfMsg(t, m)
			} else if e := errOf(t, m); e.Code != tc.code {
				t.Fatalf("code %q, want %q", e.Code, tc.code)
			}
		})
	}
}

func TestRemoteSetTwiceRunsTcpipOnce(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.r.handleSet(setMsg("a", `{"enabled":true}`))
	first := stateOfMsg(t, g.waitMsgs(t, 1)[0])
	g.r.handleSet(setMsg("b", `{"enabled":true}`))
	msgs := g.waitMsgs(t, 2)
	if second := stateOfMsg(t, msgs[1]); second != first || msgs[1].rid != "b" {
		t.Fatalf("second %+v first %+v", msgs[1], first)
	}
	if n := g.phone.count("tcpip 5555"); n != 1 {
		t.Fatalf("tcpip ran %d times", n)
	}
}

func TestSnapshotCarriesTheRemoteStateAndAttachPublishesIt(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	var s bridge.Snapshot
	g.r.area().snapshot(&s)
	if s.Remote.Enabled || s.Remote.Address != "" || !s.Remote.ExpiresAt.IsZero() {
		t.Fatalf("off: %+v", s.Remote)
	}
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.refresh()
	g.r.area().snapshot(&s)
	if !s.Remote.Enabled || s.Remote.Address != "100.101.102.103:5555" || s.Remote.ExpiresAt.IsZero() {
		t.Fatalf("on: %+v", s.Remote)
	}
	g.r.area().attach()
	m := g.waitMsgs(t, 1)[0]
	if m.kind != "event" || m.typ != bridge.TypeRemoteState || !m.payload.(bridge.RemoteState).Enabled {
		t.Fatalf("attach published %+v", m)
	}
}

func TestSnapshotTakesNoStoreLock(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.store.mu.Lock() // the store is busy: the snapshot must not wait for it
	defer g.store.mu.Unlock()
	done := make(chan struct{})
	go func() {
		var s bridge.Snapshot
		g.r.area().snapshot(&s)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the snapshot waited for the config store")
	}
}

func TestWatchRemoteExpiresAndPublishes(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.refresh()
	go watchRemote(g.ctx, g.log, g.ctl, g.r, 5*time.Millisecond, time.Second)
	time.Sleep(30 * time.Millisecond)
	if len(g.srv.messages()) != 0 || g.phone.count("usb") != 0 {
		t.Fatal("it expired early")
	}
	g.clock.advance(2 * time.Hour)
	m := g.waitMsgs(t, 1)[0]
	if m.kind != "event" || m.typ != bridge.TypeRemoteState || m.payload.(bridge.RemoteState).Enabled {
		t.Fatalf("published %+v", m)
	}
	if !g.store.Until().IsZero() || g.phone.count("usb") != 1 {
		t.Fatalf("until %v usb %d", g.store.Until(), g.phone.count("usb"))
	}
	if !strings.Contains(g.log.String(), "Remote mode expired") {
		t.Fatalf("log %q", g.log.String())
	}
	var s bridge.Snapshot
	g.r.area().snapshot(&s)
	if s.Remote.Enabled {
		t.Fatalf("snapshot %+v", s.Remote)
	}
}

// An expiry that cannot close the port: one warning, the snapshot says on
// (the app shows "closing"), and `adb usb` is not run on every tick.
func TestWatchRemoteThatCannotCloseWarnsOnceAndBacksOff(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.refresh()
	g.clock.advance(2 * time.Hour)
	g.phone.setFail("usb", true)
	ctx, cancel := context.WithCancel(g.ctx)
	done := make(chan struct{})
	go func() { watchRemote(ctx, g.log, g.ctl, g.r, 5*time.Millisecond, time.Second); close(done) }()
	time.Sleep(150 * time.Millisecond) // about 30 ticks
	cancel()
	<-done
	if n := strings.Count(g.log.String(), "could not be closed"); n != 1 {
		t.Fatalf("warned %d times: %q", n, g.log.String())
	}
	if !strings.Contains(g.log.String(), "port 5555") {
		t.Fatalf("log %q", g.log.String())
	}
	if n := g.phone.count("usb"); n < 1 || n > 7 {
		t.Fatalf("adb usb ran %d times in about 30 ticks", n)
	}
	var s bridge.Snapshot
	g.r.area().snapshot(&s)
	if !s.Remote.Enabled {
		t.Fatalf("snapshot says off with the port open: %+v", s.Remote)
	}
	if len(g.srv.messages()) != 0 {
		t.Fatalf("published %+v", g.srv.messages())
	}
}

func TestStillOnNotice(t *testing.T) {
	at := time.Date(2026, 10, 1, 21, 30, 0, 0, time.Local)
	got := stillOnNoticeIn(i18n.EN, remote.State{Enabled: true, ExpiresAt: at})
	for _, want := range []string{"Remote mode is still on", "21:30", "adbuddy remote off", "start serve again"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q does not say %q", got, want)
		}
	}
}

// Task 4 findings.

func TestTLSPortFollowsTheLinkContext(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "")
	g.adb.block = true
	g.cancel()
	start := time.Now()
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("reading the port ignored the link's context (%v)", d)
	}
}

func TestDeviceInfoChecksTheConnectionAgainBeforeSaving(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	g.adb.gate = make(chan struct{})
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.srv.current.Store(false) // replaced while adb answered
	close(g.adb.gate)
	g.idle(t)
	if g.saves.Load() != 0 {
		t.Fatalf("saved %d times for a replaced connection", g.saves.Load())
	}
}

func TestDeviceInfoRemembersFewWarnedAddresses(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	for i := range 100 {
		g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "10.0.0." + strconv.Itoa(i)})
	}
	g.r.mu.Lock()
	n := len(g.r.warned)
	g.r.mu.Unlock()
	if n > maxWarnedIPs {
		t.Fatalf("remembers %d addresses", n)
	}
}

// Through the real link: remote.set gets its answer with the rid.
func TestRemoteSetThroughTheLink(t *testing.T) {
	e := linkEnv(t)
	e.cfg.TailscaleIP, e.cfg.TailscalePort = "100.101.102.103", 40000
	f := &appPhone{}
	phone := &ctlADB{fail: map[string]bool{}}
	store := linkStore(e)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	app := &App{Stderr: &syncBuffer{}, Stdout: io.Discard, Getenv: func(string) string { return "" }}
	if _, err := startPhoneLink(ctx, app, e, store, remote.New(phone, store, time.Now), f, "s_1", "claude", &recording{}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	c := openApp(t, e.cfg.ChatPort, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: f.handedTokens()[0], AppVersion: "0.3.0"})
	if m := readApp(t, c); m.Type != bridge.TypeWelcome {
		t.Fatalf("first message %q", m.Type)
	}
	sendApp(t, c, bridge.TypeRemoteSet, "rq1", bridge.RemoteSet{Enabled: true})
	for range 20 {
		m := readApp(t, c)
		if m.Type != bridge.TypeRemoteState || m.RID != "rq1" {
			continue
		}
		var st bridge.RemoteState
		if err := json.Unmarshal(m.Payload, &st); err != nil || !st.Enabled || st.Address != "100.101.102.103:5555" {
			t.Fatalf("state %+v %v", st, err)
		}
		if phone.count("tcpip 5555") != 1 {
			t.Fatalf("adb %v", phone.called())
		}
		return
	}
	t.Fatal("no remote.state with the rid")
}

// ---- Round 1 of fixes ----

func (a *ctlADB) setHang(cmd string, on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hang[cmd] = on
}

// I1: an `adb usb` that hangs does not keep the controller for ever.
func TestWatchRemoteExpireHasATimeout(t *testing.T) {
	setVar(t, &remoteOpTimeout, 150*time.Millisecond)
	g := newRemoteRig(t, withTailscale(), "")
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.refresh()
	g.clock.advance(2 * time.Hour)
	g.phone.setHang("usb", true)
	go watchRemote(g.ctx, g.log, g.ctl, g.r, 5*time.Millisecond, remoteOpTimeout)
	waitUntil(t, "usb to start", func() bool { return g.phone.count("usb") >= 1 })
	g.r.handleSet(setMsg("r1", `{"enabled":true}`))
	waitUntil(t, "an answer to remote.set", func() bool { return len(g.srv.messages()) >= 1 })
	if !strings.Contains(g.log.String(), "could not be closed") {
		t.Fatalf("log %q", g.log.String())
	}
}

// I2: a failed remote.set tells the app the real state too.
func TestRemoteSetErrorPublishesTheRealState(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.phone.setFail("tcpip 5555", true)
	g.phone.setFail("shell getprop service.adb.tcp.port", true) // the port cannot be read: the expiry stays
	g.r.handleSet(setMsg("r1", `{"enabled":true}`))
	msgs := g.waitMsgs(t, 2)
	if e := errOf(t, msgs[0]); e.Code != bridge.CodeFailed {
		t.Fatalf("error %+v", e)
	}
	if msgs[1].kind != "event" || msgs[1].typ != bridge.TypeRemoteState || !msgs[1].payload.(bridge.RemoteState).Enabled {
		t.Fatalf("second message %+v", msgs[1])
	}
}

// I3: after a success the next failure warns again and starts with the
// smallest pause.
func TestWatchRemoteResetsAfterASuccess(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	expire := func() {
		if err := g.store.SetUntil(g.clock.now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	expire()
	g.phone.setFail("usb", true)
	go watchRemote(g.ctx, g.log, g.ctl, g.r, 5*time.Millisecond, time.Second)
	warnings := func() int { return strings.Count(g.log.String(), "could not be closed") }
	waitUntil(t, "the first warning", func() bool { return warnings() == 1 })
	// Someone turns it off by other means: Expire finds nothing to do (no
	// error, no change), which also ends the failure.
	if err := g.store.SetUntil(time.Time{}); err != nil {
		t.Fatal(err)
	}
	// No fixed pause: the watcher cannot say when it saw the clean state, so
	// the expiry is switched off and on until a second warning shows up. That
	// warning only exists if a success reset the "already warned" flag in
	// between; without the reset the later failures stay silent.
	clean := false
	waitUntil(t, "a second warning", func() bool {
		if warnings() == 2 {
			return true
		}
		if clean {
			expire()
		} else if err := g.store.SetUntil(time.Time{}); err != nil {
			t.Fatal(err)
		}
		clean = !clean
		return false
	})
}

func TestRemoteBackoffCapIsAboutTwoMinutes(t *testing.T) {
	if got := int(maxRemoteBackoff / (30 * time.Second)); got != 4 {
		t.Fatalf("a 30 s tick skips at most %d ticks", got)
	}
}

// M1: requests run one at a time, in the order they came.
func TestRemoteSetRunsInOrder(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.phone.gate = make(chan struct{})
	g.r.handleSet(setMsg("a", `{"enabled":true}`))
	waitUntil(t, "tcpip to start", func() bool { return g.phone.count("tcpip 5555") == 1 })
	g.r.handleSet(setMsg("b", `{"enabled":false}`))
	close(g.phone.gate)
	msgs := g.waitMsgs(t, 2)
	if msgs[0].rid != "a" || msgs[1].rid != "b" || stateOfMsg(t, msgs[1]).Enabled {
		t.Fatalf("answers %+v", msgs)
	}
	if calls := g.phone.called(); strings.Join(calls, "|") != "tcpip 5555|"+tcpPortCalls+"|usb" {
		t.Fatalf("adb %v", calls)
	}
	var s bridge.Snapshot
	g.r.area().snapshot(&s)
	if s.Remote.Enabled || g.store.Until() != (time.Time{}) {
		t.Fatalf("the last request was off: %+v", s.Remote)
	}
}

// M1: a full queue still answers.
func TestRemoteSetFullQueueAnswersUnavailable(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.phone.gate = make(chan struct{})
	g.r.handleSet(setMsg("run", `{"enabled":true}`))
	waitUntil(t, "tcpip to start", func() bool { return g.phone.count("tcpip 5555") == 1 })
	for i := range maxRemoteQueue {
		g.r.handleSet(setMsg("q"+strconv.Itoa(i), `{"enabled":true}`))
	}
	g.r.handleSet(setMsg("extra", `{"enabled":true}`))
	m := g.waitMsgs(t, 1)[0]
	if e := errOf(t, m); m.rid != "extra" || e.Code != bridge.CodeUnavailable {
		t.Fatalf("answer %+v", m)
	}
	close(g.phone.gate)
	msgs := g.waitMsgs(t, maxRemoteQueue+2)
	if len(msgs) != maxRemoteQueue+2 {
		t.Fatalf("%d answers", len(msgs))
	}
}

// M3: an expiry that has passed is not "on until" a past hour.
func TestRemoteNoticesForAClosingMode(t *testing.T) {
	at := time.Date(2026, 10, 1, 21, 30, 0, 0, time.Local)
	for name, got := range map[string]string{
		"serve start": remoteOnNoticeIn(i18n.EN, remote.State{Enabled: true, Closing: true, ExpiresAt: at}),
		"serve stop":  stillOnNoticeIn(i18n.EN, remote.State{Enabled: true, Closing: true, ExpiresAt: at}),
	} {
		if !strings.Contains(got, "Remote mode expired but port 5555 could not be closed yet") || strings.Contains(got, "21:30") {
			t.Errorf("%s: %q", name, got)
		}
	}
	if got := remoteOnNoticeIn(i18n.EN, remote.State{Enabled: true, ExpiresAt: at}); !strings.Contains(got, "Remote mode is on until 21:30") {
		t.Errorf("%q", got)
	}
}

// M4: serve's wiring of Reconcile and the invariant.
func TestServeKeepsPort5555OnlyWhileRemoteIsOn(t *testing.T) {
	shortWaits(t)
	setVar(t, &usbDropWait, 100*time.Millisecond)
	base := config.Default()
	base.Serial, base.Address, base.TailscaleIP = phoneSerial, tsAddr, "100.101.102.103"
	open := testutil.FakeRule{Match: tcpPortRule, Stdout: "5555\n"}

	t.Run("remote_until in the future", func(t *testing.T) {
		cfg := base
		cfg.RemoteUntil = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		_, errOut, _, log := serveOnce(t, append([]testutil.FakeRule{open}, remoteRules("AAAA1111")...), cfg)
		for _, c := range loggedCalls(t, log) {
			if c == "usb" {
				t.Fatalf("adb usb ran with the remote mode on: %q", loggedCalls(t, log))
			}
		}
		if !strings.Contains(errOut, "Remote mode is on until") {
			t.Fatalf("stderr %q", errOut)
		}
	})
	t.Run("no remote_until", func(t *testing.T) {
		_, _, _, log := serveOnce(t, append([]testutil.FakeRule{open, {Match: "^usb$"}}, remoteRules("AAAA1111")...), base)
		found := false
		for _, c := range loggedCalls(t, log) {
			found = found || c == "usb"
		}
		if !found {
			t.Fatalf("the open port was not closed: %q", loggedCalls(t, log))
		}
	})
}

// Validation 20: when the app connects through the real link, the host tells
// it the remote mode after the welcome (onAttach), off or on with its expiry.
func TestAttachPublishesRemoteStateThroughTheLink(t *testing.T) {
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	for _, c := range []struct {
		name  string
		until time.Time
	}{{"off", time.Time{}}, {"on with an expiry", until}} {
		t.Run(c.name, func(t *testing.T) {
			e := linkEnv(t)
			e.cfg.TailscaleIP, e.cfg.TailscalePort = "100.101.102.103", 40000
			store := linkStore(e)
			if !c.until.IsZero() {
				if err := store.SetUntil(c.until); err != nil {
					t.Fatal(err)
				}
			}
			f := &appPhone{}
			phone := &ctlADB{fail: map[string]bool{}}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			app := &App{Stderr: &syncBuffer{}, Stdout: io.Discard, Getenv: func(string) string { return "" }}
			if _, err := startPhoneLink(ctx, app, e, store, remote.New(phone, store, time.Now), f, "s_1", "claude", &recording{}); err != nil {
				t.Fatal(err)
			}
			waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
			conn := openApp(t, e.cfg.ChatPort, bridge.Hello{Protocol: bridge.ProtocolVersion, Token: f.handedTokens()[0], AppVersion: "0.3.0"})
			if m := readApp(t, conn); m.Type != bridge.TypeWelcome {
				t.Fatalf("first message %q", m.Type)
			}
			// The app asked for nothing: the state comes by itself.
			for range 20 {
				m := readApp(t, conn)
				if m.Type != bridge.TypeRemoteState {
					continue
				}
				if m.RID != "" {
					t.Fatalf("an unsolicited remote.state carries rid %q", m.RID)
				}
				var st bridge.RemoteState
				if err := json.Unmarshal(m.Payload, &st); err != nil {
					t.Fatal(err)
				}
				if c.until.IsZero() {
					if st.Enabled || st.Address != "" || !st.ExpiresAt.IsZero() {
						t.Fatalf("state %+v, want off", st)
					}
					return
				}
				if !st.Enabled || st.Address != "100.101.102.103:5555" || !st.ExpiresAt.Equal(c.until) {
					t.Fatalf("state %+v, want on until %v at 100.101.102.103:5555", st, c.until)
				}
				return
			}
			t.Fatal("no remote.state after the welcome")
		})
	}
}

// VB-41: an "off" from the app that finds persist.adb.tcp.port set says so
// in serve's log, once.
func TestRemoteSetOffSaysThePersistPort(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.phone.persist = "5555"
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.handleSet(setMsg("r2", `{"enabled":false}`))
	g.waitMsgs(t, 1)
	if strings.Count(g.log.String(), "persist.adb.tcp.port is 5555") != 1 {
		t.Fatalf("log %q", g.log.String())
	}
}

// VB-41: the expiry that closes the port says the persist port too, once.
func TestWatchRemoteSaysThePersistPortWhenItExpires(t *testing.T) {
	g := newRemoteRig(t, withTailscale(), "")
	g.phone.persist = "7777"
	if err := g.store.SetUntil(g.clock.now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	g.r.refresh()
	go watchRemote(g.ctx, g.log, g.ctl, g.r, 5*time.Millisecond, time.Second)
	g.clock.advance(2 * time.Hour)
	g.waitMsgs(t, 1)
	waitUntil(t, "the warning", func() bool { return strings.Contains(g.log.String(), "persist.adb.tcp.port is 7777") })
	time.Sleep(30 * time.Millisecond) // more ticks: nothing more to say
	if n := strings.Count(g.log.String(), "persist.adb.tcp.port is 7777"); n != 1 || g.phone.count("usb") != 1 {
		t.Fatalf("said %d times, usb %d: %q", n, g.phone.count("usb"), g.log.String())
	}
}
