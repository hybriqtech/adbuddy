package main

import (
	"os"
	"os/exec"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/testutil"
)

// The PC half of the latency harness (docs/superpowers/plans/2026-10-03-latencia.md):
// what ADBuddy adds, on this machine, to an adb command that the phone would
// answer instantly. fakeadb answers at once, so the difference between going
// through the shim (process start, IPC, classifier, journal, daemon) and
// starting fakeadb directly is ADBuddy's own cost. Run with
//
//	ADBUDDY_LATENCY=1 ADBUDDY_LATENCY_OUT=<file.md> go test ./cmd/adbuddy -run TestLatencyPC -v
func TestLatencyPC(t *testing.T) {
	if !testutil.LatencyOn() {
		t.Skip("set ADBUDDY_LATENCY=1")
	}
	h := start(t)
	rep := testutil.NewReport("PC side: what ADBuddy adds to an adb command (fake adb that answers at once)")
	defer rep.Flush(t.Logf)

	direct := func(args ...string) func() {
		return func() {
			cmd := exec.Command(fakeADB, args...)
			cmd.Env = append(os.Environ(), h.env...)
			cmd.Env = append(cmd.Env, "ANDROID_ADB_SERVER_PORT=5038")
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	rep.Add("fakeadb started directly (process start floor)", testutil.Measure(3, 30, direct("shell", "getprop", "ro.product.model")))
	rep.Add("adbuddy version (our binary's start floor)", testutil.Measure(3, 30, func() {
		if _, _, code := h.adbuddy("version"); code != 0 {
			t.Fatal("version failed")
		}
	}))
	rep.Add("shim, green: adb shell getprop (classify, journal-free)", testutil.Measure(3, 30, func() {
		if _, _, code := h.adb("", "shell", "getprop", "ro.product.model"); code != 0 {
			t.Fatal("green failed")
		}
	}))
	rep.Add("shim, yellow: adb shell settings put (precheck get + journal x2 + put)", testutil.Measure(3, 30, func() {
		if _, _, code := h.adb("", "shell", "settings", "put", "global", "window_animation_scale", "0.5"); code != 0 {
			t.Fatal("yellow failed")
		}
	}))
}
