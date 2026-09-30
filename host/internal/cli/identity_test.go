package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// otherPhoneRules: something on the network answers as the saved phone
// (by its guid, or at the address mDNS announces for that guid), but its
// ro.serialno is another phone's.
var otherPhoneRules = map[string][]testutil.FakeRule{
	"by name": {
		{Match: "^devices$", Stdout: phoneOnline},
	},
	"announced address": {
		{Match: "^devices$", Stdout: "List of devices attached\n192.0.2.20:41000\tdevice\n"},
		{Match: "^mdns services$", Stdout: "List of discovered mdns services\nadb-AAAA1111-bbb\t_adb-tls-connect._tcp\t192.0.2.20:41000\n"},
		{Match: `^connect 192\.0\.2\.20:41000$`, Stdout: "connected to 192.0.2.20:41000\n"},
	},
}

// Final review of M1c, I1: the phone found by mDNS is checked too.
func TestServeRefusesAnotherPhoneFoundByMDNS(t *testing.T) {
	setVar(t, &deviceWait, 3*time.Second)
	setVar(t, &autoConnectWait, 0)
	for name, rules := range otherPhoneRules {
		t.Run(name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Serial = phoneSerial
			rules := append(append([]testutil.FakeRule{}, rules...),
				testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "ZZZZ9999\n"},
				testutil.FakeRule{Match: tcpPortRule, Stdout: "\n"})
			code, errOut, _, _ := serveOnce(t, rules, cfg)
			if code != ExitFailure || !strings.Contains(errOut, "WRONG_PHONE") || strings.Contains(errOut, "ADBuddy is serving") {
				t.Fatalf("exit %d stderr %q", code, errOut)
			}
		})
	}
}

// Final review of M1c, I1: setup does not save another phone found by mDNS,
// neither right after pairing nor as the known phone.
func TestSetupRefusesAnotherPhoneFoundByMDNS(t *testing.T) {
	setVar(t, &autoConnectWait, 0)
	setVar(t, &knownPhoneWait, 3*time.Second)
	for name, rules := range otherPhoneRules {
		t.Run(name, func(t *testing.T) {
			rules := append(append(append([]testutil.FakeRule{}, rules...),
				testutil.FakeRule{Match: `^shell getprop ro\.serialno$`, Stdout: "ZZZZ9999\n"}), phoneRules()...)

			e := newSetupEnv(t, rules)
			_, errOut, code := runWithInput(t, "192.0.2.10:37000\n123456\n", "setup", "--code", "--adb", fakeADB(t), "--workspace", e.ws)
			if code != ExitFailure || !strings.Contains(errOut, "WRONG_PHONE") {
				t.Fatalf("after pairing: exit %d stderr %q", code, errOut)
			}
			if exists(filepath.Join(e.cfgDir, "config.toml")) || exists(e.ws) {
				t.Fatal("after pairing: saved")
			}

			e = newSetupEnv(t, rules)
			saveConfig(t, e, phoneSerial)
			_, errOut, code = runWithInput(t, "", "setup", "--adb", fakeADB(t), "--workspace", e.ws)
			if code != ExitFailure || !strings.Contains(errOut, "WRONG_PHONE") || strings.Contains(errOut, "Prepare your phone") {
				t.Fatalf("known phone: exit %d stderr %q", code, errOut)
			}
			if exists(e.ws) {
				t.Fatal("known phone: environment written")
			}
		})
	}
}

// Final review of M1c, I2: a phone that does not answer the identity check
// costs at most identityWait; the saved address is then skipped for mDNS.
func TestIdentityCheckHasATimeLimit(t *testing.T) {
	shortWaits(t)
	setVar(t, &deviceWait, 10*time.Second)
	setVar(t, &identityWait, 300*time.Millisecond)
	cfg := config.Default()
	cfg.Serial, cfg.Address = phoneSerial, tsAddr
	rules := append([]testutil.FakeRule{
		{Match: `^shell getprop ro\.serialno$`, SleepMs: 20000, Times: 1}, // the address hangs
		{Match: "^devices$", Stdout: "List of devices attached\n" + tsAddr + "\tdevice\n", Times: 1},
		{Match: "^devices$", Stdout: phoneOnline},
	}, remoteRules("AAAA1111")...)
	start := time.Now()
	code, errOut, ping, _ := serveOnce(t, rules, cfg)
	if code != 0 || ping.Serial != phoneSerial || !strings.Contains(errOut, "did not answer") {
		t.Fatalf("exit %d ping %+v stderr %q", code, ping, errOut)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %s", d)
	}
}

// Final review of M1c, M1: a phone whose identity cannot be checked (its
// guid does not carry a usable serial number) is not given the stable name:
// the journal and undo know it by its active serial, and it is said.
func TestServeIdentityOfAPhoneThatCannotBeChecked(t *testing.T) {
	shortWaits(t)
	const weak = "adb-unknown-abc123._adb-tls-connect._tcp"
	cfg := config.Default()
	cfg.Serial, cfg.Address = weak, tsAddr
	code, errOut, ping, _ := serveOnce(t, remoteRules("unknown"), cfg)
	if code != 0 || ping.Serial != tsAddr || ping.Identity != tsAddr || !strings.Contains(errOut, "could not check") {
		t.Fatalf("exit %d ping %+v stderr %q", code, ping, errOut)
	}
	// Checked: the stable name.
	cfg.Serial = phoneSerial
	code, errOut, ping, _ = serveOnce(t, remoteRules("AAAA1111"), cfg)
	if code != 0 || ping.Identity != phoneSerial || strings.Contains(errOut, "could not check") {
		t.Fatalf("checked: exit %d ping %+v stderr %q", code, ping, errOut)
	}
}
