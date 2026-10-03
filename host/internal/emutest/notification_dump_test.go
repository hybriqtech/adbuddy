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
// manual validations plan, 2026-10-03), with the notification posted. {key}
// stands for the notification's key. A read with several dumps several
// services: what it shows depends on the state of the phone (the CI's
// emulator, after other tests, showed the text's code in dumpsys --priority
// NORMAL and dumpsys --skip notification, which the local one never did), so
// it must show at least what is measured, may show more, and must be 🔴
// whatever it shows.
type notificationRead struct {
	cmd          string
	ticker, text bool // shows tickerCode, textCode
	several      bool
}

// notificationReads is the measured table. A read that shows a code must be 🔴
// in classify; one that does not is a regression guard. The redacted dump of
// the notification service keeps the first 8 characters of a ticker longer
// than 16 ("tickerText=G-482913..."), and dumpsys of every service (or of the
// CRITICAL ones, where the notification service puts its records) includes it.
var notificationReads = []notificationRead{
	{"dumpsys notification", true, false, false},
	{"dumpsys -t 10 notification", true, false, false},
	{"dumpsys notification --pkg " + phoneapp.Package, true, false, false},
	{"dumpsys notification --noredact", true, true, false},
	{"dumpsys notification --reveal", true, true, false},
	{"dumpsys notification --proto", false, false, false},
	{"dumpsys notification --stats", false, false, false},
	{"dumpsys --pid notification", false, false, false},
	{"dumpsys", true, false, true},
	{"dumpsys --skip meminfo", true, false, true},
	{"dumpsys --priority CRITICAL", true, false, true},
	{"dumpsys --priority HIGH", false, false, true},
	{"dumpsys --priority NORMAL", false, false, true},
	{"dumpsys --skip notification", false, false, true},
	{"dumpsys -l", false, false, false},
	{"dumpsys --pid", false, false, false},
	{"dumpsys statusbar", false, false, false},
	{"dumpsys activity service com.android.systemui/.SystemUIService", false, false, false},
	{"cmd notification list", false, false, false},
	{"cmd notification get '{key}'", true, true, false},
}

// servicesWithACode dumps each service of dumpsys -l on its own, in one shell
// on the emulator, and returns those whose dump shows a code. The CI's
// emulator found the text's code in a service other than the notification
// one; this names it, and checkServices asks classify to make it 🔴.
func (e *emu) servicesWithACode(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Minute)
	defer cancel()
	out, errOut, code, err := e.adb.Output(ctx, "shell",
		`n=0; for s in $(dumpsys -l | sed 1d); do n=$((n+1)); if dumpsys -T 20000 "$s" 2>&1 | grep -q -e `+tickerCode+` -e `+textCode+`; then echo "code $s"; fi; done; echo "services $n"`)
	if err != nil || code != 0 {
		t.Fatalf("dumpsys of each service: exit %d, %v: %s", code, err, strings.TrimSpace(errOut))
	}
	var with []string
	n := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if s, ok := strings.CutPrefix(line, "code "); ok {
			with = append(with, s)
		}
		if s, ok := strings.CutPrefix(line, "services "); ok {
			n = s
		}
	}
	if n == "" || n == "0" {
		t.Fatalf("dumpsys of each service went through no service: %q", out)
	}
	t.Logf("%s services dumped one by one; with a code: %v", n, with)
	return with
}

// checkServices asks, for each service whose dump shows a code, that classify
// makes dumpsys of it 🔴, and logs where the code is.
func (e *emu) checkServices(t *testing.T, cls *classify.Classifier, when string, services []string) {
	t.Helper()
	for _, s := range services {
		cmd := "dumpsys " + s
		for _, w := range whereTheCodes(e.shell(t, cmd)) {
			t.Logf("    %s %s", s, w)
		}
		if res := cls.Classify([]string{"shell", cmd}); res.Level != classify.Red {
			t.Errorf("%s shows a notification's code %s and classify says %v (%s): it must be 🔴", cmd, when, res.Level, res.Primary.Reason.EN)
		}
	}
}

// whereTheCodes returns, for each line of a dumpsys output that holds a code,
// the service whose dump it is in ("DUMP OF SERVICE …") and the line, cut: the
// evidence of which service shows it.
func whereTheCodes(out string) []string {
	var where []string
	section := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "DUMP OF SERVICE ") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "DUMP OF SERVICE "), ":")
		}
		if strings.Contains(line, tickerCode) || strings.Contains(line, textCode) {
			line = strings.TrimSpace(line)
			if len(line) > 160 {
				line = line[:160] + "…"
			}
			where = append(where, "["+section+"] "+line)
		}
		if len(where) == 20 {
			break
		}
	}
	return where
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
		c, cancel := context.WithTimeout(ctx, 10*time.Minute) // the hold below, and some more
		defer cancel()
		out, errOut, code, err := runner.Output(c, "shell", "am instrument -w -r -e class "+tickerTest+
			" -e adbuddyTicker 1 -e holdMs 540000 "+instrumentRunner)
		done <- result{strings.ReplaceAll(out, "\r", ""), errOut, code, err}
	}()
	released := false
	release := func() {
		if released {
			return
		}
		released = true
		// The test cancels its notification once it gets this (or holdMs later).
		ctx, cancel := context.WithTimeout(context.WithoutCancel(e.ctx), 30*time.Second)
		defer cancel()
		_, _, _, _ = e.adb.Output(ctx, "shell", "am broadcast -a "+tickerDone+" -p "+phoneapp.Package)
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
		for _, w := range whereTheCodes(out) {
			t.Logf("    %s", w)
		}
		switch {
		case r.several && (r.ticker && !ticker || r.text && !text):
			t.Errorf("%s shows the ticker's code %v and the text's %v, measured at least %v and %v: measure again, update notificationReads and classify",
				r.cmd, ticker, text, r.ticker, r.text)
		case !r.several && (ticker != r.ticker || text != r.text):
			t.Errorf("%s shows the ticker's code %v and the text's %v, measured %v and %v: measure again, update notificationReads and classify",
				r.cmd, ticker, text, r.ticker, r.text)
		}
		if !(ticker || text || r.several) {
			continue
		}
		if res := cls.Classify([]string{"shell", r.cmd}); res.Level != classify.Red {
			t.Errorf("%s shows a notification's code (or dumps several services) and classify says %v (%s): it must be 🔴", r.cmd, res.Level, res.Primary.Reason.EN)
		}
	}
	// Each service on its own, with the notification posted: only the
	// notification service was measured to show a code, but whichever does
	// must be 🔴.
	e.checkServices(t, cls, "while posted", e.servicesWithACode(t))

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
	// Canceled, no service was measured to keep a code; whichever does must
	// be 🔴 too.
	e.checkServices(t, cls, "after it was canceled", e.servicesWithACode(t))
}
