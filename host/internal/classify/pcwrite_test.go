package classify

import (
	"reflect"
	"testing"
)

// Final review, I1: adb pull and adb bugreport write on the PC. The
// classifier reports where, so the daemon can check it against the agent's
// environment; a user rule cannot hide it.

func TestPullAndBugreportReportTheirPCTargets(t *testing.T) {
	c := mustNew(t, "")
	cases := []struct {
		argv []string
		want []PCWrite
	}{
		{[]string{"pull", "/sdcard/DCIM/a.jpg"}, []PCWrite{{Cmd: "pull", Targets: []string{"a.jpg"}}}},
		{[]string{"pull", "/sdcard/DCIM/a.jpg", "photos"}, []PCWrite{{Cmd: "pull", Targets: []string{"photos", "photos/a.jpg"}}}},
		{[]string{"bugreport"}, []PCWrite{{Cmd: "bugreport"}}},
		{[]string{"bugreport", "reports"}, []PCWrite{{Cmd: "bugreport", Targets: []string{"reports", "reports.zip"}}}},
		{[]string{"wait-for-device", "pull", "/sdcard/a.txt"}, []PCWrite{{Cmd: "pull", Targets: []string{"a.txt"}}}},
		{[]string{"shell", "ls"}, nil},
	}
	for _, tc := range cases {
		r := c.Classify(tc.argv)
		if !reflect.DeepEqual(r.PCWrites, tc.want) {
			t.Errorf("%q: PCWrites %+v, want %+v", tc.argv, r.PCWrites, tc.want)
		}
	}
	if r := c.Classify([]string{"pull", "/sdcard/x", "../x"}); len(r.PCWrites) != 1 || r.PCWrites[0].Reason == "" {
		t.Errorf("unsafe target without a reason: %+v", r.PCWrites)
	}
}

func TestUserRuleCannotHidePullTargets(t *testing.T) {
	c := mustNew(t, "rules:\n  - {id: u.pull, scope: adb, cmd: pull, level: green}\n")
	r := c.Classify([]string{"pull", "/sdcard/x", "../../x"})
	if len(r.PCWrites) != 1 || r.PCWrites[0].Reason == "" {
		t.Fatalf("user rule hid the unsafe target: level %v, %+v", r.Level, r.PCWrites)
	}
}

func TestPullNeverCreatesProgramsAtTheTop(t *testing.T) {
	c := mustNew(t, "")
	for _, argv := range [][]string{
		{"pull", "/sdcard/Download/x", "run.bat"}, {"pull", "/sdcard/adb.exe"}, {"pull", "/sdcard/x", "adb"},
		{"pull", "/sdcard/x", "adbuddy"}, {"pull", "/sdcard/x", "ADB.CMD"}, {"pull", "/sdcard/adbuddy.anything"},
		{"pull", "/sdcard/x", "x.ps1"}, {"pull", "/sdcard/x", "x.sh"}, {"pull", "/sdcard/x", "x.com"},
		{"pull", "/sdcard/x", "x.cmd"}, {"pull", "/sdcard/x", "x.exe"}, {"pull", "/sdcard/adb"},
		{"bugreport", "adb"},
	} {
		if r := c.Classify(argv); r.Level != Red {
			t.Errorf("%q: %v, want red", argv, r.Level)
		}
	}
	for _, argv := range [][]string{
		{"pull", "/sdcard/x", "tools/run.bat"}, {"pull", "/sdcard/adbx.txt"}, {"pull", "/sdcard/notes.txt"},
	} {
		if r := c.Classify(argv); r.Level != Green {
			t.Errorf("%q: %v, want green (%s)", argv, r.Level, r.Primary.Reason)
		}
	}
}

func TestDumpsysClearAndForceAreRed(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"dumpsys procstats --clear", "dumpsys batterystats --reset-all", "dumpsys usagestats --clear-all",
		"dumpsys activity force", "dumpsys procstats --reset",
	} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
	}
	if r := c.Classify(sh("dumpsys procstats --hours 3")); r.Level != Green {
		t.Errorf("dumpsys procstats --hours 3: %v (%s)", r.Level, r.Primary.Reason)
	}
}
