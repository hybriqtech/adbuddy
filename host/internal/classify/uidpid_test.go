package classify

import "testing"

// v1 V03: the classifier has no state and cannot tell whether a UID or a PID
// is ADBuddy's, so a command that stops, restricts or cuts the network of one
// it cannot rule out is a fixed protection: a user rule cannot lower it.
// kill on a literal PID stays a plain 🔴 (testdata/habitual.jsonl): the
// shell's UID cannot signal another app's process.
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
