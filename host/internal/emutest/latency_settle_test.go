package emutest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// How long the real screen takes to settle after what the agent does (a tap, Back),
// as the app sees it: it polls the app's foreground and dump after each action and
// notes when the dump last changed and how often the app said "protected" for a
// moment (a window with no content during a transition). It tells settleFirst,
// settleMin and settleMax what real transitions need (docs/superpowers/plans/2026-10-03-latencia.md).
// Off unless ADBUDDY_LATENCY=1.
func TestLatencySettle(t *testing.T) {
	if !LatencyOn() {
		t.Skip("set ADBUDDY_LATENCY=1")
	}
	l := openLink(t)
	e := l.e
	e.restoreAtCleanup(t, "secure", "enabled_accessibility_services", e.shell(t, "settings get secure enabled_accessibility_services"))
	e.restoreAtCleanup(t, "secure", "accessibility_enabled", e.shell(t, "settings get secure accessibility_enabled"))
	e.atCleanup(t, "shell", "input keyevent KEYCODE_HOME")
	if _, err := phoneapp.EnableAccessibility(e.ctx, e.adb, l.v); err != nil {
		t.Fatalf("accessibility: %v", err)
	}
	if !poll(20*time.Second, func() bool { return a11yBound(e.read(t, "", "accessibility")) }) {
		t.Fatal("ADBuddy's service is not bound 20 s after enabling it")
	}
	e.openSettings(t)
	time.Sleep(2 * time.Second)
	ui := l.appUI(t)
	call := func(op string, args any) json.RawMessage {
		raw, _ := json.Marshal(args)
		r, err := ui(context.Background(), op, raw)
		if err != nil {
			return nil
		}
		return r
	}
	type sample struct {
		at        time.Duration
		dump      []byte
		protected bool
	}
	observe := func(act func()) (first, last time.Duration, flickers int, reads int) {
		t0 := time.Now()
		act()
		var prev []byte
		var samples []sample
		for time.Since(t0) < 3*time.Second {
			var fg bridge.UIForeground
			if r := call(bridge.UIOpForeground, bridge.Empty{}); r != nil {
				_ = json.Unmarshal(r, &fg)
			}
			d := call(bridge.UIOpDump, bridge.UIDumpArgs{})
			samples = append(samples, sample{time.Since(t0), d, fg.Protected})
		}
		for _, s := range samples {
			reads++
			if s.protected {
				flickers++
			}
			if prev != nil && !bytes.Equal(prev, s.dump) {
				if first == 0 {
					first = s.at
				}
				last = s.at
			}
			prev = s.dump
		}
		return
	}
	type row struct{ first, last time.Duration }
	run := func(name string, act func(), reset func()) {
		var firsts, lasts []time.Duration
		blocked, total := 0, 0
		for i := 0; i < 12; i++ {
			f, la, fl, rd := observe(act)
			firsts, lasts = append(firsts, f), append(lasts, la)
			blocked += fl
			total += rd
			reset()
			time.Sleep(1500 * time.Millisecond)
		}
		sort.Slice(firsts, func(i, j int) bool { return firsts[i] < firsts[j] })
		sort.Slice(lasts, func(i, j int) bool { return lasts[i] < lasts[j] })
		msg := fmt.Sprintf("| %s | first change median %d ms, max %d ms | last change median %d ms, p90 %d ms, max %d ms | protected in %d of %d reads |",
			name, firsts[len(firsts)/2].Milliseconds(), firsts[len(firsts)-1].Milliseconds(),
			lasts[len(lasts)/2].Milliseconds(), lasts[len(lasts)*9/10].Milliseconds(), lasts[len(lasts)-1].Milliseconds(), blocked, total)
		t.Log(msg)
		reportLine(msg)
	}
	tapNet := func() { call(bridge.UIOpClick, bridge.UIClickArgs{Text: "Network & internet"}) }
	back := func() { e.shell(t, "input keyevent KEYCODE_BACK") }
	run("tap Network & internet (Settings home -> sub page)", tapNet, back)
	// Back from a sub page: observe the return
	var setup = func() { tapNet(); time.Sleep(2 * time.Second) }
	setup()
	run("Back (sub page -> Settings home)", back, setup)
	e.openSettings(t)
	time.Sleep(time.Second)
	run("monkey launch Settings from the launcher", func() {
		e.shell(t, "monkey -p com.android.settings -c android.intent.category.LAUNCHER 1")
	}, func() {
		e.shell(t, "input keyevent KEYCODE_HOME")
	})
	_ = strings.TrimSpace
}
