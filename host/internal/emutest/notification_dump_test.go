package emutest

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

// The app's instrumented half is tickerTest (instrumented_test.go): a
// notification of the app's own with a ticker that starts with a code and a
// text with another one, posted until the host sends tickerDone (or holdMs
// passes).
const (
	tickerDone = "tech.hybriq.adbuddy.test.TICKER_DONE"
	// tickerKey is in the notification's key (user|package|id|tag|uid).
	tickerKey = "|" + phoneapp.Package + "|7301|"
	// tickerMark is the first 8 characters of the ticker ("ADBZQ-482913 is
	// your…"), what the redacted dump of the notification service keeps of it;
	// textMark is the code in the text, whole. Letters and dashes, so that no
	// dump holds them by chance: a 6-digit code alone can turn up among the
	// numbers (times, counters) of a 15 MB dump.
	tickerMark = "ADBZQ-48"
	textMark   = "ZXQ-735102-KWV"
	// tickerHold is how long the instrumented test keeps the notification
	// posted if the host never releases it: the wait for it, the reads and the
	// dump of each service while it is posted fit well within it.
	tickerHold = 5 * time.Minute
	// tickerBudget is the whole test's time with the emulator.
	tickerBudget = 15 * time.Minute
)

// notificationRead is a way the shell reads the notifications, with what it
// was measured to show on the emulator (API 34; M5b-18 / M5c-35, T2 of the
// manual validations plan, 2026-10-03), with the notification posted. {key}
// stands for the notification's key.
//
// A read of several services (dumpArgs is not empty) is 🔴 whatever it shows
// (classify's dumpsysSeveral): what it shows depends on the state of the phone,
// so it must show at least what is measured and may show more. Where it shows
// a mark, the service whose dump holds it (its "DUMP OF SERVICE" section) is
// the culprit, and dumpsys of that service alone, with and without the
// arguments dumpsys passed it (dumpArgs), must be 🔴 too.
type notificationRead struct {
	cmd          string
	ticker, text bool // shows tickerMark, textMark
	// dumpArgs are the arguments dumpsys passes each service it dumps
	// (frameworks/native/cmds/dumpsys, setServiceArgs): -a without --priority
	// or with NORMAL, and --dump-priority with one.
	dumpArgs string
}

// notificationReads is the measured table. A read that shows a mark must be 🔴
// in classify; one that does not is a regression guard. The redacted dump of
// the notification service keeps the first 8 characters of a ticker longer
// than 16 ("tickerText=ADBZQ-48..."), and dumpsys of the CRITICAL services
// (where the notification service puts its records) includes it. Of the
// reads of every service (about 15 MB each) only --skip notification is
// here, as the evidence of another service: the dump of each service on its
// own (servicesWithAMark) covers the rest.
var notificationReads = []notificationRead{
	{"dumpsys notification", true, false, ""},
	{"dumpsys -t 10 notification", true, false, ""},
	{"dumpsys notification --pkg " + phoneapp.Package, true, false, ""},
	{"dumpsys notification --noredact", true, true, ""},
	{"dumpsys notification --reveal", true, true, ""},
	{"dumpsys notification --proto", false, false, ""},
	{"dumpsys notification --stats", false, false, ""},
	{"dumpsys --pid notification", false, false, ""},
	{"dumpsys --priority CRITICAL", true, false, "--dump-priority CRITICAL"},
	{"dumpsys --priority HIGH", false, false, "--dump-priority HIGH"},
	{"dumpsys --skip notification", false, false, "-a"},
	{"dumpsys -l", false, false, ""},
	{"dumpsys --pid", false, false, ""},
	{"dumpsys --thread", false, false, ""},
	{"dumpsys --clients", false, false, ""},
	{"dumpsys --stability", false, false, ""},
	{"dumpsys statusbar", false, false, ""},
	{"dumpsys activity service com.android.systemui/.SystemUIService", false, false, ""},
	{"cmd notification list", false, false, ""},
	{"cmd notification get '{key}'", true, true, ""},
}

// eachServiceArgs are the ways servicesWithAMark dumps each service: alone,
// as classify judges dumpsys <service>, and with what dumpsys of every service
// (or --skip) and dumpsys --priority NORMAL pass it.
var eachServiceArgs = []string{"", "-a", "--dump-priority NORMAL -a"}

// servicesWithAMark dumps each service of dumpsys -l on its own, in one shell
// on the emulator, in each way of eachServiceArgs, and returns the commands
// whose dump shows a mark ("dumpsys <service> <args>"). The CI's emulator
// found the text's code in a read of several services; this names the
// service, and checkForms asks classify to make it 🔴.
func (e *emu) servicesWithAMark(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Minute)
	defer cancel()
	ways := ""
	for _, a := range eachServiceArgs {
		ways += " '" + a + "'"
	}
	out, errOut, code, err := e.adb.Output(ctx, "shell",
		`n=0; for s in $(dumpsys -l | sed 1d); do n=$((n+1)); for a in`+ways+`; do `+
			`if dumpsys -T 20000 "$s" $a 2>&1 | grep -q -F -e `+tickerMark+` -e `+textMark+`; then echo "mark dumpsys $s $a"; fi; `+
			`done; done; echo "services $n"`)
	if err != nil || code != 0 {
		t.Fatalf("dumpsys of each service: exit %d, %v: %s", code, err, strings.TrimSpace(errOut))
	}
	var with []string
	n := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if s, ok := strings.CutPrefix(line, "mark "); ok {
			with = append(with, strings.TrimSpace(s))
		}
		if s, ok := strings.CutPrefix(line, "services "); ok {
			n = s
		}
	}
	if n == "" || n == "0" {
		t.Fatalf("dumpsys of each service went through no service: %q", out)
	}
	t.Logf("%s services dumped one by one (%q); with a mark: %v", n, eachServiceArgs, with)
	return with
}

// checkForms fails the test, naming the command, for each command that shows
// a notification's mark and that classify does not make 🔴. why says where
// the mark was seen.
func checkForms(t *testing.T, cls *classify.Classifier, why string, cmds []string) {
	t.Helper()
	var green []string
	for _, cmd := range cmds {
		if res := cls.Classify([]string{"shell", cmd}); res.Level != classify.Red {
			green = append(green, cmd+" ("+res.Level.String()+")")
		}
	}
	if len(green) > 0 {
		t.Fatalf("%s, and classify does not make it 🔴: %s. Make dumpsys of that service 🔴 in checkDumpsys and measure it with the notification posted",
			why, strings.Join(green, ", "))
	}
}

// sectionsWithAMark returns the services ("DUMP OF SERVICE …" sections) of a
// dumpsys output whose dump holds a mark; "" stands for a mark before any
// section.
func sectionsWithAMark(out string) []string {
	seen := map[string]bool{}
	section := ""
	for _, line := range strings.Split(out, "\n") {
		if s, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "DUMP OF SERVICE "); ok {
			section = strings.TrimSuffix(strings.TrimSpace(s), ":")
			if p := strings.Fields(section); len(p) == 2 && (p[0] == "CRITICAL" || p[0] == "HIGH" || p[0] == "NORMAL") {
				section = p[1] // "DUMP OF SERVICE CRITICAL notification:"
			}
		}
		if strings.Contains(line, tickerMark) || strings.Contains(line, textMark) {
			seen[section] = true
		}
	}
	var with []string
	for s := range seen {
		with = append(with, s)
	}
	sort.Strings(with)
	return with
}

// TestSectionsWithAMark checks, without an emulator, that the culprit of a
// read of several services is the section whose dump holds a mark, with the
// priority dumpsys --priority prints before its name, and that a mark before
// any section is reported as "".
func TestSectionsWithAMark(t *testing.T) {
	out := "x " + textMark + "\r\n" +
		"DUMP OF SERVICE CRITICAL notification:\r\n  tickerText=" + tickerMark + "...\r\n" +
		"DUMP OF SERVICE activity:\r\n  nothing\r\n" +
		"DUMP OF SERVICE android.hardware.x.IFoo/default:\r\n  text " + textMark + "\r\n  again " + tickerMark + "\r\n" +
		"DUMP OF SERVICE window:\r\n  735102 482913\r\n"
	got := strings.Join(sectionsWithAMark(out), ",")
	if want := ",android.hardware.x.IFoo/default,notification"; got != want {
		t.Errorf("sectionsWithAMark = %q, want %q", got, want)
	}
}

// whereTheMarks returns, for each line of a dumpsys output that holds a mark,
// the service whose dump it is in ("DUMP OF SERVICE …") and the line, cut: the
// evidence of which service shows it.
func whereTheMarks(out string) []string {
	var where []string
	section := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "DUMP OF SERVICE ") {
			section = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(line, "DUMP OF SERVICE ")), ":")
		}
		if strings.Contains(line, tickerMark) || strings.Contains(line, textMark) {
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
// mark its output shows, and that classify asks before each read that shows
// one; then it dumps each service on its own, with the notification posted and
// after it is canceled, and asks the same of each service that shows one.
func TestNotificationReadsAndTheCodes(t *testing.T) {
	start := time.Now()
	e := newEmuTimeout(t, tickerBudget)
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
		c, cancel := context.WithTimeout(ctx, tickerHold+2*time.Minute)
		defer cancel()
		out, errOut, code, err := runner.Output(c, "shell", "am instrument -w -r -e class "+tickerTest+
			" -e adbuddyTicker 1 -e holdMs "+strconv.FormatInt(tickerHold.Milliseconds(), 10)+" "+instrumentRunner)
		done <- result{strings.ReplaceAll(out, "\r", ""), errOut, code, err}
	}()
	// ended is am instrument's result once it is in.
	var ended *result
	// release sends tickerDone, upon which the instrumented test cancels its
	// notification, and waits for am instrument to end; it sends it again if
	// it does not. Each try has its own short timeout, apart from the test's:
	// a hung adb must not leave the notification posted for the rest of the
	// hold (and the instrumented test cancels it in its finally anyway).
	release := func() {
		for try := 0; try < 3 && ended == nil; try++ {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(e.ctx), 15*time.Second)
			_, _, _, _ = e.adb.Output(ctx, "shell", "am broadcast -a "+tickerDone+" -p "+phoneapp.Package)
			cancel()
			select {
			case r := <-done:
				ended = &r
			case <-time.After(30 * time.Second):
			}
		}
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
			ended = &r
			t.Fatalf("am instrument ended before the notification was posted: exit %d, %v: %s\n%s", r.code, r.err, r.errOut, r.out)
		default:
		}
	}
	if key == "" {
		t.Fatalf("the notification %s is not posted 90 s after am instrument", tickerKey)
	}
	t.Logf("posted after %s", time.Since(start).Round(time.Second))

	for _, r := range notificationReads {
		r.cmd = strings.ReplaceAll(r.cmd, "{key}", key)
		out := e.shell(t, r.cmd)
		ticker, text := strings.Contains(out, tickerMark), strings.Contains(out, textMark)
		t.Logf("%s: ticker mark %v, text mark %v (%d bytes)", r.cmd, ticker, text, len(out))
		for _, w := range whereTheMarks(out) {
			t.Logf("    %s", w)
		}
		several := r.dumpArgs != ""
		switch {
		case several && (r.ticker && !ticker || r.text && !text):
			t.Errorf("%s shows the ticker's mark %v and the text's %v, measured at least %v and %v: measure again, update notificationReads and classify",
				r.cmd, ticker, text, r.ticker, r.text)
		case !several && (ticker != r.ticker || text != r.text):
			t.Errorf("%s shows the ticker's mark %v and the text's %v, measured %v and %v: measure again, update notificationReads and classify",
				r.cmd, ticker, text, r.ticker, r.text)
		}
		if several {
			// The service that holds a mark, alone, with and without what
			// dumpsys passed it, must be 🔴: the read of several is not the
			// only way to it.
			for _, s := range sectionsWithAMark(out) {
				if s == "" {
					t.Fatalf("%s shows a notification's mark before any DUMP OF SERVICE section: the service that holds it is unknown", r.cmd)
				}
				checkForms(t, cls, r.cmd+" shows a notification's mark in the dump of the service "+s,
					[]string{"dumpsys " + s, "dumpsys " + s + " " + r.dumpArgs})
			}
		}
		if !(ticker || text || several) {
			continue
		}
		if res := cls.Classify([]string{"shell", r.cmd}); res.Level != classify.Red {
			t.Errorf("%s shows a notification's mark (or dumps several services) and classify says %v (%s): it must be 🔴", r.cmd, res.Level, res.Primary.Reason.EN)
		}
	}
	t.Logf("reads done after %s", time.Since(start).Round(time.Second))
	// Each service on its own, with the notification posted: only the
	// notification service was measured to show a mark, but whichever does
	// must be 🔴.
	checkForms(t, cls, "with the notification posted, a notification's mark is in", e.servicesWithAMark(t))
	t.Logf("each service, posted, done after %s", time.Since(start).Round(time.Second))

	release()
	switch {
	case ended == nil:
		t.Fatalf("am instrument did not end after %s, sent 3 times", tickerDone)
	case ended.err != nil || ended.code != 0:
		t.Fatalf("am instrument: exit %d, %v: %s\n%s", ended.code, ended.err, strings.TrimSpace(ended.errOut), ended.out)
	}
	if tests, _, _, failed := parseInstrument(ended.out); failed || tests != 1 {
		t.Fatalf("%s did not pass (%d tests ran):\n%s", tickerTest, tests, ended.out)
	}
	if strings.Contains(e.shell(t, "cmd notification list"), tickerKey) {
		t.Errorf("the notification %s is still posted after the test", tickerKey)
	}
	// Canceled, no service was measured to keep a mark; whichever does must
	// be 🔴 too.
	checkForms(t, cls, "after the notification was canceled, a notification's mark is in", e.servicesWithAMark(t))
	t.Logf("each service, canceled, done after %s", time.Since(start).Round(time.Second))
}
