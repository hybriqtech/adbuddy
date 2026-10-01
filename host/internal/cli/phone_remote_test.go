package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/config"
)

type fakeRemoteSrv struct{ current atomic.Bool }

func (f *fakeRemoteSrv) Current(bridge.Message) bool { return f.current.Load() }

// tlsADB answers `getprop service.adb.tls.port`. gate, when set, makes each
// call wait for it to close before answering.
type tlsADB struct {
	mu    sync.Mutex
	port  string
	calls []string
	gate  chan struct{}
}

func (a *tlsADB) Run(ctx context.Context, _ string, args []string, _ io.Reader, stdout, _ io.Writer) (int, error) {
	if a.gate != nil {
		<-a.gate
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, strings.Join(args, " "))
	if a.port == "fail" {
		return 1, errors.New("adb down")
	}
	io.WriteString(stdout, a.port+"\n")
	return 0, nil
}

func (a *tlsADB) called() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string{}, a.calls...)
}

type remoteRig struct {
	r     *remoteLink
	store *cfgStore
	srv   *fakeRemoteSrv
	adb   *tlsADB
	path  string
	log   *bytes.Buffer
	saves atomic.Int32
}

func newRemoteRig(t *testing.T, cfg config.Config, port string) *remoteRig {
	t.Helper()
	rig := &remoteRig{adb: &tlsADB{port: port}, srv: &fakeRemoteSrv{}, log: &bytes.Buffer{}, path: filepath.Join(t.TempDir(), "config.toml")}
	rig.srv.current.Store(true)
	rig.store = newCfgStore(rig.path, cfg)
	rig.store.save = func(p string, c config.Config) error {
		rig.saves.Add(1)
		return config.Save(p, c)
	}
	rig.r = newRemoteLink(rig.store, rig.adb, rig.log)
	rig.r.setServer(rig.srv)
	return rig
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
	if calls := g.adb.called(); len(calls) != 1 || calls[0] != "shell getprop service.adb.tls.port" {
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

func TestDeviceInfoFromAReplacedConnectionIsIgnored(t *testing.T) {
	g := newRemoteRig(t, config.Default(), "41234")
	g.srv.current.Store(false)
	g.r.onDeviceInfo(bridge.Message{}, bridge.DeviceInfo{TailscaleIP: "100.101.102.103"})
	g.idle(t)
	if g.saves.Load() != 0 || len(g.adb.called()) != 0 {
		t.Fatalf("saves %d adb %v", g.saves.Load(), g.adb.called())
	}
	// Without the app there is no server at all.
	n := newRemoteLink(g.store, g.adb, g.log)
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
	g.adb.mu.Lock()
	g.adb.port = "41235"
	g.adb.mu.Unlock()
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
