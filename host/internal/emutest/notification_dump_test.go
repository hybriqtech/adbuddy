package emutest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// The app's instrumented half is tickerTest (instrumented_test.go): a
// notification of the app's own with a ticker that starts with a code and a
// text with another one, posted until the host sends tickerDone.
const (
	tickerDone = "tech.hybriq.adbuddy.test.TICKER_DONE"
	// tickerKey is in the notification's key (user|package|id|tag|uid).
	tickerKey = "|" + phoneapp.Package + "|7301|"
	// tickerCode is in the ticker ("G-482913 is your…", the code within its
	// first 8 characters); textCode is in the text.
	tickerCode = "482913"
	textCode   = "735102"
)

// notificationRead is a way the shell reads the notifications, with what it
// was measured to show on the emulator (API 34; M5b-18 / M5c-35, T2 of the
// manual validations plan, 2026-10-03). {key} stands for the notification's
// key.
type notificationRead struct {
	cmd          string
	ticker, text bool // shows tickerCode, textCode
}

// notificationReads is the measured table. A read that shows a code must be 🔴
// in classify; one that does not is a regression guard. The redacted dump of
// the notification service keeps the first 8 characters of a ticker longer
// than 16 ("tickerText=G-482913..."), and dumpsys of every service (or of the
// CRITICAL ones, where the notification service puts its records) includes it.
var notificationReads = []notificationRead{
	{"dumpsys notification", true, false},
	{"dumpsys -t 10 notification", true, false},
	{"dumpsys notification --pkg " + phoneapp.Package, true, false},
	{"dumpsys notification --noredact", true, true},
	{"dumpsys notification --reveal", true, true},
	{"dumpsys notification --proto", false, false},
	{"dumpsys notification --stats", false, false},
	{"dumpsys --pid notification", false, false},
	{"dumpsys", true, false},
	{"dumpsys --skip meminfo", true, false},
	{"dumpsys --priority CRITICAL", true, false},
	{"dumpsys --priority HIGH", false, false},
	{"dumpsys --priority NORMAL", false, false},
	{"dumpsys --skip notification", false, false},
	{"dumpsys -l", false, false},
	{"dumpsys --pid", false, false},
	{"dumpsys statusbar", false, false},
	{"dumpsys activity service com.android.systemui/.SystemUIService", false, false},
	{"cmd notification list", false, false},
	{"cmd notification get '{key}'", true, true},
}

// TestNotificationReadsAndTheCodes posts a notification with codes in its
// ticker and its text and checks, for each read in notificationReads, which
// code its output shows, and that classify asks before each read that shows
// one.
func TestNotificationReadsAndTheCodes(t *testing.T) {
	e := newEmu(t)
	e.installApp(t)
	e.installTests(t)
	cls, err := classify.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	e.shell(t, "pm grant "+phoneapp.Package+" android.permission.POST_NOTIFICATIONS")

	type result struct {
		out, errOut string
		code        int
		err         error
	}
	done := make(chan result, 1)
	runner, ctx := e.adb, e.ctx
	go func() {
		c, cancel := context.WithTimeout(ctx, instrumentTimeout)
		defer cancel()
		out, errOut, code, err := runner.Output(c, "shell", "am instrument -w -r -e class "+tickerTest+
			" -e adbuddyTicker 1 -e holdMs 240000 "+instrumentRunner)
		done <- result{strings.ReplaceAll(out, "\r", ""), errOut, code, err}
	}()
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		// The test cancels its notification once it gets this (or holdMs later).
		_, _, _, _ = e.adb.Output(context.WithoutCancel(e.ctx), "shell", "am broadcast -a "+tickerDone+" -p "+phoneapp.Package)
	}
	defer release()

	// Wait for the notification, and take its key.
	key := ""
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline) && key == ""; time.Sleep(time.Second) {
		for _, line := range strings.Split(e.shell(t, "cmd notification list"), "\n") {
			if strings.Contains(line, tickerKey) {
				key = strings.TrimSpace(line)
			}
		}
		select {
		case r := <-done:
			t.Fatalf("am instrument ended before the notification was posted: exit %d, %v: %s\n%s", r.code, r.err, r.errOut, r.out)
		default:
		}
	}
	if key == "" {
		t.Fatalf("the notification %s is not posted 90 s after am instrument", tickerKey)
	}

	for _, r := range notificationReads {
		r.cmd = strings.ReplaceAll(r.cmd, "{key}", key)
		out := e.shell(t, r.cmd)
		ticker, text := strings.Contains(out, tickerCode), strings.Contains(out, textCode)
		t.Logf("%s: ticker code %v, text code %v (%d bytes)", r.cmd, ticker, text, len(out))
		if ticker != r.ticker || text != r.text {
			t.Errorf("%s shows the ticker's code %v and the text's %v, measured %v and %v: measure again, update notificationReads and classify",
				r.cmd, ticker, text, r.ticker, r.text)
		}
		if !(ticker || text) {
			continue
		}
		if res := cls.Classify([]string{"shell", r.cmd}); res.Level != classify.Red {
			t.Errorf("%s shows a notification's code and classify says %v (%s): it must be 🔴", r.cmd, res.Level, res.Primary.Reason.EN)
		}
	}

	release()
	select {
	case r := <-done:
		if r.err != nil || r.code != 0 {
			t.Fatalf("am instrument: exit %d, %v: %s\n%s", r.code, r.err, strings.TrimSpace(r.errOut), r.out)
		}
		if tests, _, _, failed := parseInstrument(r.out); failed || tests != 1 {
			t.Fatalf("%s did not pass (%d tests ran):\n%s", tickerTest, tests, r.out)
		}
	case <-time.After(2 * time.Minute):
		t.Fatal("am instrument did not end 2 min after " + tickerDone)
	}
	if strings.Contains(e.shell(t, "cmd notification list"), tickerKey) {
		t.Errorf("the notification %s is still posted after the test", tickerKey)
	}
}
