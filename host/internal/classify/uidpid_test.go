package classify

import "testing"

// v1 V03: the classifier has no state and cannot tell whether a UID or a PID
// is ADBuddy's, so a command that stops, restricts or cuts the network of one
// it cannot rule out is a fixed protection: a user rule cannot lower it.
// kill on a literal PID stays a plain 🔴 only because testdata/habitual.jsonl
// fixes it so (kill -9 1234). On a production phone adbd runs with the
// shell's UID (2000) and kill -9 <adbd's PID> does cut the connection:
// accepted risk, a user rule can lower it.
func TestUIDAndPIDTargetsCannotBeLoweredByAUserRule(t *testing.T) {
	c := mustNew(t, `rules:
  - {id: u.appops, cmd: appops, sub: [set], level: green, anyArgs: true}
  - {id: u.kill, cmd: kill, level: green, anyArgs: true}
  - {id: u.am, cmd: am, level: green, anyArgs: true}
  - {id: u.cmd, cmd: cmd, level: green, anyArgs: true}
`)
	check(t, c, []tc{
		{sh("appops set 10234 RUN_ANY_IN_BACKGROUND ignore"), Red, "", "", true},
		{sh("cmd appops set --user 0 10234 RUN_ANY_IN_BACKGROUND ignore"), Red, "", "", true},
		{sh("kill -9 -1"), Red, "", "", true},
		{sh("am kill-all"), Red, "", "", true},
		{sh("am crash 1234"), Red, "", "", true},
		{sh("cmd netpolicy add restrict-background-blacklist 10234"), Red, "", "", true},
		{sh("cmd netpolicy set metered-network wifi true"), Red, "", "", true},
	})
}

// The parsers must not take an option's value for the target.
func TestUIDAndPIDTargetsReadOptions(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("appops set --user 10 com.x.y OP ignore"), Yellow, "appops.set", "appops-set", false},
		{sh("kill -s 9 1234"), Red, "sys.kill", "", false},
		{sh("kill -9 1234 5678"), Red, "sys.kill", "", false},
		{sh("kill -- 1234"), Red, "sys.kill", "", false},
		{sh("kill -s 9 -- -1"), Red, "", "", true},
		{sh("am crash --user 0 com.x"), Red, "am.other", "", false},
		{sh("am crash"), Red, "am.other", "", false},
		{sh("kill"), Red, "sys.kill", "", false},
		{sh("cmd netpolicy"), Red, "cmd.other", "", false},
	})
}

// Round 1: restarting the system, stopping or switching the user and killing
// adbd by name cut the connection or stop ADBuddy: a user rule cannot lower
// them.
func TestConnectionKillersCannotBeLoweredByAUserRule(t *testing.T) {
	c := mustNew(t, `rules:
  - {id: u.am, cmd: am, level: green, anyArgs: true}
  - {id: u.pkill, cmd: pkill, level: green, anyArgs: true}
  - {id: u.killall, cmd: killall, level: green, anyArgs: true}
`)
	check(t, c, []tc{
		{sh("am restart"), Red, "", "", true},
		{sh("am stop-user 10"), Red, "", "", true},
		{sh("am switch-user 10"), Red, "", "", true},
		{sh("pkill adbd"), Red, "", "", true},
		{sh("pkill -f /apex/com.android.adbd"), Red, "", "", true},
		{sh("killall adbd"), Red, "", "", true},
		{sh("su 0 killall adbd"), Red, "", "", true},
		{sh("pkill com.x"), Green, "u.pkill", "", false},
	})
}

// --checkin consumes state (procstats, batterystats, activity); -c and the
// --checkin of meminfo and package only print.
func TestDumpsysCheckinIsRed(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("dumpsys procstats --checkin"), Red, "", "", false},
		{sh("dumpsys batterystats --checkin"), Red, "", "", false},
		{sh("dumpsys activity --checkin"), Red, "", "", false},
		{sh("dumpsys activity broadcast-stats --checkin"), Red, "", "", false},
		{sh("dumpsys batterystats -c"), Green, "", "", false},
		{sh("dumpsys meminfo --checkin"), Green, "", "", false},
		{sh("dumpsys package --checkin"), Green, "", "", false},
	})
}
