package classify

import (
	"strings"
	"testing"
)

func TestTheAppChannelIsOffLimits(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"content call --uri content://tech.hybriq.adbuddy.bridge --method handshake --extra token:s:x",
		"content call --uri content://tech.hybriq.adbuddy.bridge --method getSetupConsent",
		"content write --uri content://tech.hybriq.adbuddy.bridge/handshake",
		"content read --uri content://tech.hybriq.adbuddy.bridge/handshake",
		"content query --uri content://tech.hybriq.adbuddy.bridge",
		"content query --uri content://tech.hybriq.adbuddy%2ebridge",
		"content query --uri content://TECH.HYBRIQ.ADBUDDY.bridge",
		"run-as tech.hybriq.adbuddy cat shared_prefs/bridge.xml",
		"run-as tech.hybriq.adbuddy ls",
		"am start -n tech.hybriq.adbuddy/.MainActivity -a tech.hybriq.adbuddy.action.SETUP",
		"am force-stop tech.hybriq.adbuddy",
		"pm revoke tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS",
		"pm disable-user tech.hybriq.adbuddy",
		"appops set tech.hybriq.adbuddy RUN_IN_BACKGROUND ignore",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v, want red special", line, r.Level, r.Special)
		}
	}
	for _, line := range []string{
		"pm list packages tech.hybriq.adbuddy",
		"dumpsys package tech.hybriq.adbuddy",
		"content query --uri content://settings/global",
	} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v, want green (%s)", line, r.Level, r.Primary.Reason)
		}
	}
}

// A word only known on the phone ($(…), $VAR, xargs, loops…) could name
// ADBuddy: the classifier fails closed and treats the command as special.
func TestTheAppChannelFailsClosedOnWordsOnlyKnownOnThePhone(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// I1: content with an authority only known on the phone.
		"content query --uri content://tech.hybriq.adbud$(echo dy).bridge",
		`content query --uri "content://tech.hybriq.$X.bridge"`,
		"U=content://tech.hybriq.adbuddy.bridge; content query --uri $U",
		"echo content://tech.hybriq.adbuddy.bridge | xargs content query --uri",
		"for u in content://tech.hybriq.adbuddy.bridge; do content query --uri $u; done",
		"content write --uri content://tech.hybriq.adbud$(echo dy).bridge/handshake",
		`content call --uri "$(printf 'content://tech.hybriq.adbu%sdy.bridge' d)" --method ping`,
		"echo content://tech.hybriq.adbuddy.bridge/handshake | xargs content write --uri",
		"content $(echo query) --uri content://tech.hybriq.adbuddy.bridge",
		// I3: intent: URIs decode %xx.
		"am start 'intent:#Intent;component=tech%2ehybriq%2eadbuddy/.MainActivity;end'",
		"am start 'intent:#Intent;action=tech%2ehybriq%2eadbuddy%2eaction%2eSETUP;end'",
		"cmd activity start 'intent:#Intent;component=tech%2ehybriq%2eadbuddy/.MainActivity;end'",
		"cmd activity start 'intent:#Intent;action=tech%2ehybriq%2eadbuddy%2eaction%2eSETUP;end'",
		"am start 'intent:#Intent;component=TECH%2EHYBRIQ%2EADBUDDY/.MainActivity;end'",
		// Minor 4: reading or changing an app's memory, whatever the process.
		"am dumpheap com.example.app /data/local/tmp/heap.hprof",
		"am dumpheap -n 1234 /data/local/tmp/heap.hprof",
		"am attach-agent com.example.app /data/local/tmp/agent.so",
		"am profile start com.example.app /data/local/tmp/p.trace",
		"am profile stop com.example.app",
		"cmd activity dumpheap com.example.app /data/local/tmp/heap.hprof",
		"am --user 0 dumpheap com.example.app /data/local/tmp/heap.hprof",
		"am start --attach-agent /data/local/tmp/agent.so -n com.example.app/.Main",
		"am $(echo dumpheap) com.example.app /data/local/tmp/heap.hprof",
		// Minor 4: pm and appops with the package only known on the phone.
		"pm disable-user $(echo tech.hybriq.adbuddy)",
		`pm revoke "$P" android.permission.WRITE_SECURE_SETTINGS`,
		"appops set $P RUN_IN_BACKGROUND ignore",
		"am force-stop $(echo tech.hybriq.adbuddy)",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	// I2: run-as with the package only known on the phone.
	for _, line := range []string{
		"run-as $(echo tech.hybriq.adbuddy) cat shared_prefs/bridge.xml",
		`run-as "$P" ls`,
		"run-as tech.hybriq.adbud$(echo dy) ls",
		"echo tech.hybriq.adbuddy | xargs run-as",
		"run-as tech.hybriq.adbuddy ls",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special || r.Primary.Rule != "run-as.adbuddy" {
			t.Errorf("%q: %v special=%v rule=%s, want red special run-as.adbuddy", line, r.Level, r.Special, r.Primary.Rule)
			continue
		}
		if !strings.Contains(r.Primary.Title.EN, "ADBuddy's private files") || !strings.Contains(r.Primary.Title.ES, "ficheros privados de ADBuddy") {
			t.Errorf("%q: title %+v", line, r.Primary.Title)
		}
	}
	// The same commands on other providers or with harmless % escapes stay as they were.
	check(t, c, []tc{
		{sh("content query --uri content://settings/global"), Green, "android.content-query", "", false},
		{sh("am start -d https://example.com/a%20b"), Yellow, "ui.am-start", "", false},
		{sh("run-as com.example.app ls"), Green, "", "", false},
		{sh("pm list packages $X"), Green, "pm.read", "", false},
	})
}

// A user rule cannot lower these protections either.
func TestTheAppChannelProtectionsSurviveUserRules(t *testing.T) {
	c := mustNew(t, `rules:
  - {id: u.am, cmd: am, level: green, anyArgs: true}
  - {id: u.content, cmd: content, level: green, anyArgs: true}
  - {id: u.pm, cmd: pm, level: green, anyArgs: true}
`)
	for _, line := range []string{
		"am dumpheap com.example.app /data/local/tmp/heap.hprof",
		"am attach-agent com.example.app /data/local/tmp/agent.so",
		"content query --uri $U",
		"pm disable-user $P",
	} {
		if r := c.Classify(sh(line)); r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v, want red special", line, r.Level, r.Special)
		}
	}
}

// Anything that keeps running on the phone after the command returns could
// press "Allow" on ADBuddy's confirmation later: always special.
func TestBackgroundJobsAreSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"nohup sh -c 'sleep 10; input tap 500 1500' >/dev/null 2>&1 &",
		"sh -c 'sleep 10; input tap 1 1' &",
		"(sleep 5; input tap 1 1) &",
		"{ sleep 5; input tap 1 1; } &",
		"sleep 5 & input tap 1 1",
		"ls & ls",
		"sh -c 'sleep 10 & ls'",
		"echo $(sleep 5 &)",
		"setsid sh -c 'input tap 1 1'",
		"setsid input tap 1 1",
		"setsid -w ls",
		"toybox setsid ls",
		"/system/bin/setsid ls",
		"daemonize ls",
		"start-stop-daemon -S -b -x /system/bin/input -- tap 1 1",
		"ls; disown",
		"nice setsid ls",
		"echo ls | xargs setsid",
		"sleep 5 |&",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
			continue
		}
		if !hasRule(r, "shell.background") {
			t.Errorf("%q: no shell.background finding (%s)", line, r.Primary.Rule)
		}
	}
	r := c.Classify(sh("sleep 5 &"))
	if r.Primary.Rule != "shell.background" || r.Primary.Title.EN != "Leave a command running on the phone after this one returns: sleep 5 &" ||
		!strings.Contains(r.Primary.Title.ES, "después de devolver el control") {
		t.Errorf("title %+v", r.Primary.Title)
	}
	// am instrument without -w returns while the instrumentation (which can
	// drive the screen) keeps running.
	for _, line := range []string{
		"am instrument com.example.test/androidx.test.runner.AndroidJUnitRunner",
		"cmd activity instrument -e class com.example.T com.example.test/.Runner",
	} {
		if r := c.Classify(sh(line)); r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v, want red special", line, r.Level, r.Special)
		}
	}
	if r := c.Classify(sh("am instrument -w com.example.test/.Runner")); r.Level != Red || r.Special {
		t.Errorf("am instrument -w: %v special=%v, want red, not special", r.Level, r.Special)
	}
	// nohup without & still runs in the foreground: judged by the wrapped command.
	check(t, c, []tc{
		{sh("nohup ls"), Green, "", "", false},
		{sh("nohup input tap 1 1"), Yellow, "ui.input", "", false},
		{sh("ls && ls"), Green, "", "", false},
	})
}

func hasRule(r Result, rule string) bool {
	for _, f := range r.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
