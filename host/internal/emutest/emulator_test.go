package emutest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// The debug APKs of `gradle :app:assembleDebug :app:assembleDebugAndroidTest`
// (run from android/), relative to this package.
var (
	appAPK   = filepath.Join("..", "..", "..", "android", "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	testsAPK = filepath.Join("..", "..", "..", "android", "app", "build", "outputs", "apk", "androidTest", "debug", "app-debug-androidTest.apk")
)

// buildHint says how to get the APKs.
const buildHint = "build them first: cd android && gradle :app:assembleDebug :app:assembleDebugAndroidTest"

// emu is the emulator scripts/emulator.sh started, behind its own adb server.
type emu struct {
	ctx    context.Context
	adb    adb.Runner // with Serial: commands for the emulator
	server adb.Runner // without Serial: `adb devices`
}

// checkEmuEnv validates the serial and the adb server port of the
// environment and returns the port: the serial must be an emulator, and the
// port that of the emulator's own adb server, never adb's default 5037 nor
// 5038 (the one ADBuddy's own server uses), where a phone may be.
func checkEmuEnv(serial, portEnv string) (int, error) {
	if !strings.HasPrefix(serial, "emulator-") {
		return 0, fmt.Errorf("ADBUDDY_EMULATOR_SERIAL=%q is not an emulator: these tests never run on a real phone", serial)
	}
	port, err := strconv.Atoi(portEnv)
	if err != nil || port <= 0 || port == 5037 || port == 5038 {
		return 0, fmt.Errorf("ANDROID_ADB_SERVER_PORT=%q: the emulator has an adb server of its own, never adb's default 5037 nor ADBuddy's 5038 where a phone may be (scripts/emulator.sh start prints its port)", portEnv)
	}
	return port, nil
}

func TestCheckEmuEnv(t *testing.T) {
	for _, c := range []struct {
		serial, port string
		ok           bool
	}{
		{"emulator-5590", "5049", true},
		{"emulator-5590", "5037", false},
		{"emulator-5590", "5038", false},
		{"emulator-5590", "", false},
		{"emulator-5590", "0", false},
		{"emulator-5590", "x", false},
		{"R5CT123", "5049", false},
	} {
		if _, err := checkEmuEnv(c.serial, c.port); (err == nil) != c.ok {
			t.Errorf("checkEmuEnv(%q, %q) = %v, want ok=%v", c.serial, c.port, err, c.ok)
		}
	}
}

// newEmu returns the emulator, or skips the test without one. It refuses a
// serial that is not an emulator and a missing adb server port: nothing here
// runs on a real phone or on the adb server a phone is on.
func newEmu(t *testing.T) *emu {
	t.Helper()
	serial, path := os.Getenv("ADBUDDY_EMULATOR_SERIAL"), os.Getenv("ADBUDDY_EMULATOR_ADB")
	if serial == "" || path == "" {
		t.Skip("set ADBUDDY_EMULATOR_SERIAL and ADBUDDY_EMULATOR_ADB (scripts/emulator.sh start prints them)")
	}
	port, err := checkEmuEnv(serial, os.Getenv("ANDROID_ADB_SERVER_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	// The emulator's adb server never auto-connects a phone over mDNS (adb.Runner
	// passes the environment on).
	t.Setenv("ADB_MDNS", "0")
	t.Setenv("ADB_MDNS_AUTO_CONNECT", "0")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	t.Cleanup(cancel)
	e := &emu{
		ctx:    ctx,
		adb:    adb.Runner{Path: path, ServerPort: port, Serial: serial},
		server: adb.Runner{Path: path, ServerPort: port},
	}
	if s := e.state(); s != "device" {
		t.Fatalf("the emulator %s is %q, not online", serial, s)
	}
	return e
}

// run runs adb against the emulator and returns its output without CRs; it
// fails the test if adb fails or exits with an error.
func (e *emu) run(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, 3*time.Minute)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, args...)
	if err != nil || code != 0 {
		t.Fatalf("adb %q: exit %d, %v: %s %s", args, code, err, strings.TrimSpace(errOut), strings.TrimSpace(out))
	}
	return strings.TrimSpace(strings.ReplaceAll(out, "\r", ""))
}

// shell runs command on the emulator as the shell user.
func (e *emu) shell(t *testing.T, command string) string {
	t.Helper()
	return e.run(t, "shell", command)
}

// state is the emulator's state in `adb devices` ("device", "offline"…), or
// "gone" when its adb server does not list it.
func (e *emu) state() string {
	ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
	defer cancel()
	out, _, code, err := e.server.Output(ctx, "devices")
	if err != nil || code != 0 {
		return "gone"
	}
	for _, d := range adb.ParseDevices(out) {
		if d.Serial == e.adb.Serial {
			return d.State
		}
	}
	return "gone"
}

// waitState polls the emulator's state every second until ok accepts it or
// d passes; it returns the last state seen and whether ok accepted it.
func (e *emu) waitState(ok func(string) bool, d time.Duration) (string, bool) {
	deadline := time.Now().Add(d)
	for {
		s := e.state()
		if ok(s) {
			return s, true
		}
		if time.Now().After(deadline) {
			return s, false
		}
		select {
		case <-e.ctx.Done():
			return s, false
		case <-time.After(time.Second):
		}
	}
}

// readAPK returns the absolute path and the bytes of one of the debug APKs,
// or fails the test.
func readAPK(t *testing.T, path string) (string, []byte) {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("%v; %s", err, buildHint)
	}
	return abs, data
}

// installApp installs the debug app on a clean slate (the uninstall takes
// its data, the saved link included) and returns the proof the host's
// functions ask for, with that exact APK trusted as the development build,
// as `adbuddy setup --apk` does. The app is never opened: it stays in the
// background.
func (e *emu) installApp(t *testing.T) phoneapp.Verified {
	t.Helper()
	abs, data := readAPK(t, appAPK)
	// Not installed is fine: the answer does not matter.
	_, _, _, _ = e.adb.Output(e.ctx, "uninstall", phoneapp.Package)
	if err := phoneapp.Install(e.ctx, e.adb, abs); err != nil {
		t.Fatalf("install %s: %v", abs, err)
	}
	e.uninstallAtCleanup(t, phoneapp.Package)
	sum := sha256.Sum256(data)
	trust := phoneapp.Trust{DevAPK: hex.EncodeToString(sum[:])}
	v, err := phoneapp.NewVerifier(func() phoneapp.Trust { return trust }, phoneapp.MinAppVersionCode).Verify(e.ctx, e.adb)
	if err != nil {
		t.Fatalf("verify the installed app: %v", err)
	}
	return v
}

// installTests installs the app's instrumented tests (tech.hybriq.adbuddy.test).
func (e *emu) installTests(t *testing.T) {
	t.Helper()
	abs, _ := readAPK(t, testsAPK)
	if out := e.run(t, "install", "-r", "--no-incremental", abs); !strings.Contains(out, "Success") {
		t.Fatalf("install %s: %s", abs, out)
	}
	e.uninstallAtCleanup(t, phoneapp.Package+".test")
}

// TestWhatIsInstalledIsUninstalled checks uninstallAtCleanup: once a test
// that installed the app ends, the app is gone.
func TestWhatIsInstalledIsUninstalled(t *testing.T) {
	e := newEmu(t)
	t.Run("install", func(t *testing.T) { newEmu(t).installApp(t) })
	for _, line := range strings.Split(e.shell(t, "pm list packages "+phoneapp.Package), "\n") {
		if strings.TrimSpace(line) == "package:"+phoneapp.Package {
			t.Errorf("%s is still installed after the test that installed it", phoneapp.Package)
		}
	}
}

// uninstallAtCleanup leaves the emulator as the test found it: the CI keeps
// its AVD between jobs. Only while the emulator is online (a test that left
// its adb cut has nothing to clean through it).
func (e *emu) uninstallAtCleanup(t *testing.T, pkg string) {
	t.Cleanup(func() {
		// e.ctx comes from t.Context(), which is canceled before cleanups run.
		c := *e
		c.ctx = context.WithoutCancel(e.ctx)
		if c.state() != "device" {
			return
		}
		ctx, cancel := context.WithTimeout(c.ctx, time.Minute)
		defer cancel()
		_, _, _, _ = c.adb.Output(ctx, "uninstall", pkg)
	})
}
