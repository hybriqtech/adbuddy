package classify

import (
	"strings"
	"testing"
)

// Task 10b: screenrecord and adb bugreport are fixed protections;
// dumpsys notification --noredact too. dumpsys itself stays green as a level
// (testdata/habitual.jsonl): the guard closes the screen-reading cases. Since
// T2 of the manual validations, what dumps the notification service is 🔴
// (TestDumpsysOfTheNotificationsIsRed).

func TestScreenrecordIsSpecialRed(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("screenrecord /sdcard/x.mp4"), Red, "screen.record", "", true},
		{sh("screenrecord --time-limit 3 /sdcard/x.mp4"), Red, "screen.record", "", true},
		{[]string{"exec-out", "screenrecord", "--output-format=h264", "-"}, Red, "screen.record", "", true},
		{sh("sh -c 'screenrecord /sdcard/x.mp4'"), Red, "", "", true},
		{sh("nohup screenrecord /sdcard/x.mp4"), Red, "", "", true},
		{sh("busybox screenrecord /sdcard/x.mp4"), Red, "", "", true},
		{sh("screenrecord /sdcard/x.mp4 &"), Red, "", "", true},
		{sh("screenrecord $X"), Red, "", "", true},
	})
	r := c.Classify(sh("screenrecord /sdcard/x.mp4"))
	if got := r.Primary.Title.EN; got != "Record the screen" {
		t.Errorf("title en %q", got)
	}
	if got := r.Primary.Title.ES; got != "Grabar la pantalla" {
		t.Errorf("title es %q", got)
	}
}

func TestScreenrecordCannotBeLoweredByAUserRule(t *testing.T) {
	c := mustNew(t, "rules:\n  - {id: u.rec, cmd: screenrecord, level: green, anyArgs: true}\n  - {id: u.bug, scope: adb, cmd: bugreport, level: green}\n")
	check(t, c, []tc{
		{sh("screenrecord /sdcard/x.mp4"), Red, "", "", true},
		{[]string{"bugreport"}, Red, "", "", true},
		{[]string{"bugreport", "reports/bug.zip"}, Red, "", "", true},
	})
}

func TestBugreportIsSpecialRedAndKeepsItsPCWrite(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{[]string{"bugreport"}, Red, "", "", true},
		{[]string{"bugreport", "reports/bug.zip"}, Red, "", "", true},
		{[]string{"wait-for-device", "bugreport"}, Red, "", "", true},
	})
	r := c.Classify([]string{"bugreport", "reports"})
	if len(r.PCWrites) != 1 || r.PCWrites[0].Cmd != "bugreport" || len(r.PCWrites[0].Targets) != 2 {
		t.Errorf("pc writes %+v", r.PCWrites)
	}
}

// T2 of the manual validations (M5b-18, M5c-35), measured on the emulator by
// emutest's TestNotificationReadsAndTheCodes: the redacted dump of the
// notification service keeps the first 8 characters of a ticker longer than
// 16 ("tickerText=G-482913..."), so dumpsys notification, with any argument,
// and dumpsys of every service or of the CRITICAL ones (the notification
// service's records are there) show the start of a code. They are 🔴, not
// special: a "yes, always" rule may lower them. What does not dump the
// notification service stays 🟢.
func TestDumpsysOfTheNotificationsIsRed(t *testing.T) {
	c := mustNew(t, "")
	var cases []tc
	for _, line := range []string{
		"dumpsys notification", "dumpsys -t 10 notification", "dumpsys -T 500 notification",
		"dumpsys --proto notification", "dumpsys -- notification", "dumpsys --dump notification",
		"dumpsys --priority CRITICAL notification", "dumpsys notification --stats",
		"dumpsys notification | grep tickerText",
		"dumpsys", "dumpsys -t 10", "dumpsys --proto", "dumpsys --dump --pid", "dumpsys --pid --dump",
		"dumpsys --skip meminfo", "dumpsys --skip meminfo activity", "dumpsys --skip", "dumpsys --skip --",
		"dumpsys --priority CRITICAL", "dumpsys --priority=CRITICAL", "dumpsys -T 500 --priority CRITICAL",
		"dumpsys --priority critical", "dumpsys --priority",
		"dumpsys | grep -A 30 NotificationRecord",
	} {
		cases = append(cases, tc{sh(line), Red, "android.dumpsys", "", false})
	}
	for _, line := range []string{
		"dumpsys -l", "dumpsys -l notification", "dumpsys --help", "dumpsys --help notification",
		"dumpsys --pid", "dumpsys --pid notification", "dumpsys --thread", "dumpsys --clients", "dumpsys --stability", "dumpsys --pid --thread",
		"dumpsys --priority HIGH", "dumpsys --priority NORMAL", "dumpsys --priority=NORMAL",
		"dumpsys --skip notification", "dumpsys --skip meminfo notification", "dumpsys -t 10 --skip notification",
		"dumpsys battery", "dumpsys statusbar", "dumpsys activity service com.android.systemui/.SystemUIService",
	} {
		cases = append(cases, tc{sh(line), Green, "android.dumpsys", "", false})
	}
	check(t, c, cases)
	for _, line := range []string{"dumpsys notification", "dumpsys"} {
		r := c.Classify(sh(line))
		if r.Primary.Reason.EN == "" || r.Primary.Reason.ES == "" || !strings.Contains(r.Primary.Reason.EN, "ticker") {
			t.Errorf("%s: reason %+v, want one that names the ticker", line, r.Primary.Reason)
		}
	}
}

func TestDumpsysLevels(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("dumpsys notification"), Red, "", "", false},
		{sh("dumpsys notification --noredact"), Red, "notifications.reveal", "", true},
		{sh("dumpsys notification --noredact --proto"), Red, "notifications.reveal", "", true},
		{sh("dumpsys -t 10 notification --noredact"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --noredact notification"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --skip meminfo notification --noredact"), Red, "notifications.reveal", "", true},
		// AOSP DumpFilter.parseFromArguments treats --reveal like --noredact.
		{sh("dumpsys notification --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys -t 10 notification --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --reveal notification"), Red, "notifications.reveal", "", true},
		{sh("dumpsys notification --reveal --proto"), Red, "notifications.reveal", "", true},
		// Without a service (or with another one) the options reach every
		// service, the notification one included.
		{sh("dumpsys --noredact"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --noredact battery"), Red, "notifications.reveal", "", true},
		{sh("dumpsys -t 10 --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys meminfo --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys window windows"), Green, "", "", false},
		{sh("dumpsys activity top"), Green, "", "", false},
		{sh("dumpsys -t 10 activity top"), Green, "", "", false},
		{sh("dumpsys"), Red, "", "", false},
		{sh("dumpsys -l"), Green, "", "", false},
		{sh("dumpsys --skip meminfo"), Red, "", "", false},
		{sh("dumpsys battery"), Green, "", "", false},
	})
}

func TestNotificationRevealCannotBeLoweredByAUserRule(t *testing.T) {
	c := mustNew(t, `rules:
  - {id: u.dump, cmd: dumpsys, level: green, anyArgs: true}
`)
	check(t, c, []tc{
		{sh("dumpsys notification --noredact"), Red, "notifications.reveal", "", true},
		{sh("dumpsys notification --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys -t 10 notification --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --noredact"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys --noredact battery"), Red, "notifications.reveal", "", true},
		{sh("dumpsys -t 10 --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys meminfo --reveal"), Red, "notifications.reveal", "", true},
		{sh("dumpsys notification"), Green, "", "", false},
	})
	r := c.Classify(sh("dumpsys notification --reveal"))
	if r.Primary.Title.EN == "" || r.Primary.Title.ES == "" || r.Primary.Title.EN == r.Primary.Title.ES {
		t.Errorf("titles %+v", r.Primary.Title)
	}
}

// v1 V02: bugreportz, dumpstate and the shell's bugreport build the same
// report as adb bugreport (dumpsys activity, notification…): a "yes,
// always" rule must not lower them.
func TestBugReportsCannotBeLoweredByAUserRule(t *testing.T) {
	c := mustNew(t, `rules:
  - {id: u.bugz, cmd: bugreportz, level: green, anyArgs: true}
  - {id: u.bug, cmd: bugreport, level: green, anyArgs: true}
  - {id: u.dumpstate, cmd: dumpstate, level: green, anyArgs: true}
  - {id: u.am, cmd: am, sub: [bug-report], level: green, anyArgs: true}
`)
	check(t, c, []tc{
		{sh("bugreportz"), Red, "", "", true},
		{sh("bugreportz -p"), Red, "", "", true},
		{sh("bugreport"), Red, "", "", true},
		{sh("dumpstate -z -o /data/local/tmp/br"), Red, "", "", true},
		{sh("am bug-report --progress"), Red, "", "", true},
		{sh("cmd activity bug-report"), Red, "", "", true},
		{sh("sh -c bugreportz"), Red, "", "", true},
	})
}
