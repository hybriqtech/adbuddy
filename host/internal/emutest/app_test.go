package emutest

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// A port nothing listens on: the app keeps trying to connect to it, which is
// all these tests need.
const handshakePort = 47123

var nonceRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Validation 1 of M2, the shell's side: the provider answers the shell user,
// as adb reaches it. The other side (it refuses everyone else) is
// TokenProviderTest, instrumented.
func TestTheShellUserIsAnswered(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	res, err := phoneapp.Call(e.ctx, e.adb, "ping")
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if res["ok"] != "true" || res["port"] != "0" {
		t.Errorf("ping of a fresh install = %v, want ok=true and port=0", res)
	}
	res, err = phoneapp.Call(e.ctx, e.adb, "getSetupConsent")
	if err != nil {
		t.Fatalf("getSetupConsent: %v", err)
	}
	if res["consent"] != string(phoneapp.ConsentPending) {
		t.Errorf("consent of a fresh install = %q, want pending", res["consent"])
	}
}

// Validation 2 of M2: `content write` with the handshake on stdin reaches the
// provider's openFile. phoneapp.Handshake only returns nil once ping gives
// back that handshake's nonce and port; the token never comes back out.
func TestTheHandshakeOnStdinReachesTheApp(t *testing.T) {
	e := newEmu(t)
	v := e.installApp(t)
	token, err := bridge.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := phoneapp.Handshake(e.ctx, e.adb, v, token, handshakePort, "emutest"); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	res, err := phoneapp.Call(e.ctx, e.adb, "ping")
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if res["port"] != strconv.Itoa(handshakePort) || !nonceRe.MatchString(res["nonce"]) {
		t.Errorf("ping after the handshake = %v, want port %d and a nonce", res, handshakePort)
	}
	for k, val := range res {
		if strings.Contains(val, token) {
			t.Errorf("ping returned the token in %s", k)
		}
	}
	out, _, _, _ := e.adb.Output(e.ctx, "shell", "content read --uri content://"+phoneapp.Authority+"/handshake")
	if strings.Contains(out, token) {
		t.Error("the handshake could be read back")
	}
}

// Validation 3 of M2: after the handshake the provider starts BridgeService
// in the foreground, with the app in the background (never opened) and out
// of battery optimisation, as `adbuddy setup` leaves it (AllowBackground).
func TestTheServiceIsInTheForegroundAfterTheHandshake(t *testing.T) {
	e := newEmu(t)
	v := e.installApp(t)
	if err := phoneapp.AllowBackground(e.ctx, e.adb, v); err != nil {
		t.Fatalf("battery optimisation: %v", err)
	}
	if acts := e.shell(t, "dumpsys activity activities"); strings.Contains(acts, phoneapp.Package+"/") {
		t.Fatal("the app has an activity: it is not in the background")
	}
	token, err := bridge.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := phoneapp.Handshake(e.ctx, e.adb, v, token, handshakePort, "emutest"); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	var last string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		last = e.shell(t, "dumpsys activity services "+phoneapp.Package)
		if inForeground(last, ".bridge.BridgeService") {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("BridgeService is not in the foreground 20 s after the handshake:\n%s", last)
}

// inForeground reads `dumpsys activity services <package>`: the record of
// the service whose short name is svc says isForeground=true. The record's
// first line ends in "/<svc>}" or, from Android 13, "/<svc> c:<package>}".
func inForeground(dump, svc string) bool {
	name := regexp.MustCompile("/" + regexp.QuoteMeta(svc) + "[ }]")
	recs := strings.Split(dump, "* ServiceRecord{")
	for _, rec := range recs[1:] {
		head, _, _ := strings.Cut(rec, "\n")
		if name.MatchString(head) && strings.Contains(rec, "isForeground=true") {
			return true
		}
	}
	return false
}

// TestInForeground runs without an emulator: the reading of dumpsys.
func TestInForeground(t *testing.T) {
	dump := `ACTIVITY MANAGER SERVICES (dumpsys activity services)
  User 0 active services:
  * ServiceRecord{5c1e2a1 u0 tech.hybriq.adbuddy/.bridge.BridgeService c:tech.hybriq.adbuddy}
    intent={cmp=tech.hybriq.adbuddy/.bridge.BridgeService}
    isForeground=true foregroundId=1 types=0x40000000 foregroundNoti=Notification(channel=conversation)
  * ServiceRecord{77aa001 u0 tech.hybriq.adbuddy/.listener.AgentNotificationListener c:tech.hybriq.adbuddy}
    isForeground=false foregroundId=0
`
	if !inForeground(dump, ".bridge.BridgeService") {
		t.Error("BridgeService in the foreground not seen (Android 13+ record, with c:)")
	}
	if old := strings.ReplaceAll(dump, " c:tech.hybriq.adbuddy}", "}"); !inForeground(old, ".bridge.BridgeService") {
		t.Error("BridgeService in the foreground not seen (record without c:)")
	}
	if inForeground(dump, ".bridge.Bridge") {
		t.Error("a prefix of the service's name read as the service")
	}
	if inForeground(dump, ".listener.AgentNotificationListener") {
		t.Error("a background service read as in the foreground")
	}
	if inForeground("  (nothing)\n", ".bridge.BridgeService") {
		t.Error("no services, yet one in the foreground")
	}
	bg := strings.Replace(dump, "isForeground=true", "isForeground=false", 1)
	if inForeground(bg, ".bridge.BridgeService") {
		t.Error("isForeground=false read as the foreground")
	}
}
