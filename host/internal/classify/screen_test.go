package classify

import "testing"

// Task 10b: screenrecord and adb bugreport are fixed protections;
// dumpsys notification --noredact too. dumpsys itself stays green as a level
// (testdata/habitual.jsonl): the guard closes the screen-reading cases.

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

func TestDumpsysLevels(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("dumpsys notification"), Green, "", "", false},
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
		{sh("dumpsys"), Green, "", "", false},
		{sh("dumpsys -l"), Green, "", "", false},
		{sh("dumpsys --skip meminfo"), Green, "", "", false},
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
