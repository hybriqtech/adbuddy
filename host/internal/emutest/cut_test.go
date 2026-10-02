package emutest

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// The app's instrumented half of the cut (android/app/src/androidTest/…/CutAccessOnEmulatorTest.kt).
const (
	cutTest   = phoneapp.Package + ".CutAccessOnEmulatorTest#cutThenReactivate"
	cutRunner = instrumentRunner
	// How long the app waits between Cut access and Reactivar. The emulator's
	// adb comes back by itself about 2 s after Reactivar (M2.5b plan, Task 1).
	reactivateAfter = 15 * time.Second
)

// tcpipOnEmulator says whether `adb tcpip 5555` keeps the emulator under its
// emulator-<port> serial (Task 1 of the M2.5b plan, spike S5).
const tcpipOnEmulator = true

// stuckHint is what to do when the emulator's adb does not come back: the
// AVD keeps adb_enabled=0 and boots without adb (M2.5b plan, Task 1, S7).
const stuckHint = "its adb is still cut, and the AVD keeps it cut across reboots: stop it with scripts/emulator.sh stop (same ADBUDDY_EMU_* variables), " +
	"delete userdata-qemu.img* in ~/.android/avd/<ADBUDDY_EMU_AVD>.avd (that AVD only) and start it again"

// Validation 6 of M2: Cut access closes the emulator's adb and Reactivar
// brings it back, both done by the app's own code (CutAccess), in
// CutAccessOnEmulatorTest. The test starts it without waiting for it (`am
// instrument` without -w): the cut takes away the adb that started it. It
// watches the emulator from outside, then reads what the app logged.
func TestCutAccessAndReactivate(t *testing.T) {
	e := newEmu(t)
	// A run that fails half way leaves adb_enabled=0 in the AVD's data, and the
	// emulator then boots without adb (M2.5b plan, Task 1, S7): the CI, which
	// keeps its AVD between jobs, does not run it.
	if os.Getenv("ADBUDDY_EMULATOR_CUT") != "1" {
		t.Skip("set ADBUDDY_EMULATOR_CUT=1 to run it, on a development emulator only")
	}
	v := e.installApp(t)
	e.installTests(t)
	if err := phoneapp.Grant(e.ctx, e.adb, v); err != nil {
		t.Fatalf("pm grant: %v", err)
	}
	t.Run("usb", func(t *testing.T) { e.cutCycle(t, false) })
	t.Run("tcpip", func(t *testing.T) {
		if !tcpipOnEmulator {
			t.Skip("`adb tcpip` takes the emulator off its emulator-<port> serial (M2.5b plan, Task 1): validation 6 with tcpip mode stays manual")
		}
		e.cutCycle(t, true)
	})
}

// adbSettings reads the two settings Cut access changes. A setting never
// written ("null") counts as what AccessSwitch takes it for: adb on, wireless
// debugging off.
func (e *emu) adbSettings(t *testing.T) string {
	t.Helper()
	get := func(name, unset string) string {
		v := e.shell(t, "settings get global "+name)
		if v == "null" {
			v = unset
		}
		return name + "=" + v
	}
	return get("adb_enabled", "1") + " " + get("adb_wifi_enabled", "0")
}

// restartedWith waits for adbd to restart after `adb tcpip` or `adb usb`
// (the emulator leaves "device" for a moment) and then for its
// service.adb.tcp.port to be one of want. It returns the last port read.
func (e *emu) restartedWith(want ...string) (string, bool) {
	// adbd goes away about 2 s after the command; a missed restart only means
	// the port check below runs against the old adbd, which already set it.
	e.waitState(func(s string) bool { return s != "device" }, 10*time.Second)
	var port string
	_, ok := e.waitState(func(s string) bool {
		if s != "device" {
			return false
		}
		ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
		defer cancel()
		out, _, code, err := e.adb.Output(ctx, "shell", "getprop service.adb.tcp.port")
		if err != nil || code != 0 {
			return false
		}
		port = strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
		for _, w := range want {
			if port == w {
				return true
			}
		}
		return false
	}, 60*time.Second)
	return port, ok
}

// cutCycle runs one Cut access and Reactivar, with or without tcpip mode,
// and checks that the settings end as they began.
func (e *emu) cutCycle(t *testing.T, tcpip bool) {
	before := e.adbSettings(t)
	if strings.Contains(before, "adb_enabled=0") {
		t.Fatalf("%s before Cut access: a cut left over by an earlier run; put it back with `adb shell settings put global adb_enabled 1`", before)
	}
	if tcpip {
		e.run(t, "tcpip", "5555")
		if p, ok := e.restartedWith("5555"); !ok {
			t.Fatalf("service.adb.tcp.port = %q (emulator %q) 60 s after adb tcpip 5555", p, e.state())
		}
		defer func() {
			// No t.Fatal here: it may run while the test is already failing.
			if s := e.state(); s != "device" {
				t.Errorf("adb usb not run: the emulator is %q", s)
				return
			}
			if _, errOut, code, err := e.adb.Output(e.ctx, "usb"); err != nil || code != 0 {
				t.Errorf("adb usb: exit %d, %v: %s", code, err, strings.TrimSpace(errOut))
				return
			}
			if p, ok := e.restartedWith("0", ""); !ok {
				t.Errorf("service.adb.tcp.port = %q (emulator %q) 60 s after adb usb", p, e.state())
			}
		}()
	}
	e.shell(t, "logcat -c")
	// Without -w `am instrument` returns once the test is started; adb may
	// also go away under it, so its answer only matters if nothing happens.
	ctx, cancel := context.WithTimeout(e.ctx, time.Minute)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell", "am instrument -r -e class "+cutTest+" -e adbuddyCut 1 -e reactivateAfterMs "+
		strconv.FormatInt(reactivateAfter.Milliseconds(), 10)+" "+cutRunner)
	// The cut: the emulator's adb goes away.
	if s, ok := e.waitState(func(s string) bool { return s != "device" }, 30*time.Second); !ok {
		t.Fatalf("the emulator is still %q 30 s after Cut access; am instrument: exit %d, %v: %s %s\nlogcat:\n%s",
			s, code, err, strings.TrimSpace(errOut), strings.TrimSpace(out), e.shell(t, "logcat -d -s ADBuddyEmuCut:I TestRunner:*"))
	}
	// Reactivar, reactivateAfter later: it comes back by itself.
	if s, ok := e.waitState(func(s string) bool { return s == "device" }, reactivateAfter+90*time.Second); !ok {
		t.Fatalf("the emulator is %q after Reactivar: %s", s, stuckHint)
	}
	// On the emulator adbd always listens on the guest's 5555 (the emulator's
	// own adb transport, M2.5b plan, Task 1, S2), with or without tcpip mode:
	// the two "listening" lines show that the cut closed adbd, not that it
	// closed tcpip mode in particular. That stays a manual check on a phone.
	want := []string{"listening before the cut: true", "cut=true isCut=true", "listening after the cut: false", "restored=true isCut=false"}
	// The app's last line may reach logcat a moment after adb is back.
	var logs string
	for range 5 {
		logs = e.shell(t, "logcat -d -s ADBuddyEmuCut:I")
		if missing(logs, want) == "" {
			break
		}
		time.Sleep(time.Second)
	}
	if m := missing(logs, want); m != "" {
		t.Errorf("the app did not log %q:\n%s", m, logs)
	}
	if after := e.adbSettings(t); after != before {
		t.Errorf("after Reactivar %s, before Cut access %s", after, before)
	}
	// Reactivar keeps tcpip mode until the host runs `adb usb` (spec §4.6).
	if tcpip {
		if p := e.shell(t, "getprop service.adb.tcp.port"); p != "5555" {
			t.Errorf("service.adb.tcp.port = %q after Reactivar, want 5555 until adb usb", p)
		}
	}
}

// missing returns the first of want that logs lacks, or "".
func missing(logs string, want []string) string {
	for _, w := range want {
		if !strings.Contains(logs, w) {
			return w
		}
	}
	return ""
}
