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
		// Android splits single-dash options (ShellCommand.getNextOption):
		// -P/file is -P with its value attached.
		"am start -P/data/local/tmp/p -a android.intent.action.VIEW -d https://example.com",
		"cmd activity start -P/data/local/tmp/p -n com.example.app/.Main",
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
		// That -w is a value of -e (it takes two), or comes after the
		// component, where am no longer reads options.
		"am instrument -e x -w com.example/.Runner",
		"am instrument -ex -w com.example/.Runner",
		"am instrument -p -w com.example/.Runner",
		"am instrument --user -w com.example/.Runner",
		"am instrument com.example/.Runner -w",
		"am instrument -- -w com.example/.Runner",
	} {
		if r := c.Classify(sh(line)); r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v, want red special", line, r.Level, r.Special)
		}
	}
	for _, line := range []string{
		"am instrument -w com.example.test/.Runner",
		"am instrument -r -e class com.example.T -w com.example.test/.Runner",
		"am instrument --user 0 -p /data/local/tmp/p -w com.example.test/.Runner",
		"am --user 0 instrument -w com.example.test/.Runner",
	} {
		if r := c.Classify(sh(line)); r.Level != Red || r.Special {
			t.Errorf("%q: %v special=%v, want red, not special", line, r.Level, r.Special)
		}
	}
	// nohup without & still runs in the foreground: judged by the wrapped command.
	check(t, c, []tc{
		{sh("nohup ls"), Green, "", "", false},
		{sh("nohup input tap 1 1"), Yellow, "ui.input", "", false},
		{sh("ls && ls"), Green, "", "", false},
	})
}

// What the classifier cannot analyse (a command name only known on the phone,
// eval, a shell reading a file or stdin, running a file, syntax it cannot
// parse, too many nested shells, a wrapper it cannot read) could hide the app
// channel or a delayed tap: it fails closed and is special, so the phone
// offers no "allow during the session" for it.
func TestWhatCannotBeAnalysedIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	deep := "ls"
	for i := 0; i <= maxDepth+1; i++ {
		deep = "sh -c " + adbEscapeArg(deep)
	}
	for _, k := range []struct{ line, rule string }{
		{"eval 'sleep 5 & input tap 1 1'", "shell.script"},
		{"echo 'sleep 5 & input tap 1 1' | sh", "shell.script"},
		{"$(echo content) query --uri content://tech.hybriq.adbuddy.bridge", "dynamic"},
		{"eval content query --uri content://tech.hybriq.adbuddy.bridge", "shell.script"},
		{"echo tech.hybriq.adbuddy | xargs -I {} run-as {} ls", "wrapper"},
		{"sh /data/local/tmp/x.sh", "shell.script"},
		{`sh -c "$X"`, "shell.script"},
		{". /data/local/tmp/x.sh", "shell.script"},
		{"su", "shell.script"},
		{"$C query --uri content://settings/global", "dynamic"},
		{"/data/local/tmp/x.sh", "exec.file"},
		{"ls ((", "shell.parse"},
		{deep, "shell.depth"},
		{"nohup -x ls", "wrapper"},
		{"timeout -z 5 ls", "wrapper"},
		// T3 re-review 2: find runs the command after -exec, -execdir, -ok
		// and -okdir; {} is a file name only known on the phone.
		{`find /sdcard -name x -exec reboot \;`, "sys.reboot"},
		{`find /sdcard -exec svc wifi disable \;`, "sys.svc"},
		{"find /sdcard -exec reboot ;", "sys.reboot"}, // as adb joins it: no terminator
		{"find /sdcard -execdir reboot {} +", "sys.reboot"},
		{`find /sdcard -ok reboot \;`, "sys.reboot"},
		{`find /sdcard -okdir svc power shutdown \;`, "sys.svc"},
		{`find /sdcard -name x -exec sh -c 'content query --uri content://tech.hybriq.adbudd$1.bridge' _ y \;`, ""},
		{`find /sdcard -exec {} \;`, "dynamic"},
		{`find /sdcard -exec \;`, "wrapper"},
		{"find /sdcard $X", "wrapper"}, // $X may split into -exec …
		// builtin runs a builtin like command does.
		{"builtin eval 'sleep 5 & input tap 1 1'", "shell.script"},
		{"builtin -- eval reboot", "shell.script"},
		// trap and alias run a string as shell code, like eval.
		{"trap 'reboot' EXIT", "shell.script"},
		{"trap -- 'input tap 1 1' EXIT", "shell.script"},
		{"trap \"$X\" EXIT", "shell.script"},
		{"alias ls='content query --uri content://tech.hybriq.adbuddy.bridge'", "shell.script"},
		{"trap 'run-as tech.hybriq.adbudd$Y ls' EXIT", ""},
	} {
		r := c.Classify(sh(k.line))
		if r.Level != Red || !r.Special || (k.rule != "" && !hasRule(r, k.rule)) {
			t.Errorf("%s: %v special=%v (%s: %s), want red special %s", display([]string{k.line}), r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.rule)
		}
	}
	// Everyday commands keep their level and are not special.
	for _, k := range []struct {
		line  string
		level Level
	}{
		{"for p in $(pm list packages -3); do echo $p; done", Green},
		{"for p in $(pm list packages -3 | cut -d: -f2); do pm path $p; done", Green},
		{"for p in $(pm list packages -3); do dumpsys package $p | grep versionName; done", Red},
		{"pm list packages $X", Green},
		{"pm path $P", Green},
		{"appops get $P", Green},
		{"nohup ls", Green},
		{"nohup input tap 1 1", Yellow},
		{"sleep 1 && echo ok", Green},
		{"ls /sdcard/*.jpg", Green},
		{"cat /sdcard/$F", Green},
		{"ls | xargs -n 1 echo", Green},
		{"logcat -d | grep -i crash", Green},
		{"sh -c 'ls /sdcard'", Green},
		{"su -c 'ls /data'", Green},
		{"/system/bin/ls /sdcard", Green},
		{"timeout 5 ls", Green},
		{"find /sdcard -name '*.jpg' -exec ls -l {} +", Red},
		{"find /sdcard -name '*.jpg'", Green},
		{`find /sdcard -name '*.jpg' -exec ls {} \;`, Red},
		{"find /sdcard -type f -name '*.tmp'", Green},
		{"trap - EXIT", Red},
		{"trap", Red},
		{"alias", Red},
		{"builtin", Green},
	} {
		r := c.Classify(sh(k.line))
		if r.Level != k.level || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want %v not special", k.line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.level)
		}
	}
	// find -exec is never below the command it runs with a dynamic argument.
	for _, cmd := range []string{"ls", "cat", "input tap 1", "pm path", "rm"} {
		alone := c.Classify(sh(cmd + " $F"))
		viaFind := c.Classify(sh("find /sdcard -exec " + cmd + ` {} \;`))
		if viaFind.Level < alone.Level || (alone.Special && !viaFind.Special) {
			t.Errorf("find -exec %s: %v special=%v, below %v special=%v", cmd, viaFind.Level, viaFind.Special, alone.Level, alone.Special)
		}
	}
}

// T3 re-review 2, defence in depth: a 🔴 that names the app in its raw text
// (any case, after decoding %xx) is special even if no rule saw the app
// channel. It never changes a 🟢 or a 🟡.
func TestARedThatNamesTheAppIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, argv := range [][]string{
		sh("rm /sdcard/Download/Hybriq.txt"),
		sh("rm /sdcard/ADBUDDY-notes"),
		sh("rm /sdcard/%41dbudd"),   // %41 is A
		sh("rm /sdcard/%2541dbudd"), // %25 is %: decoded twice
		sh("rm /sdcard/hyb'r'iq"),   // quotes do not hide it
		sh("unknowncmd tech.hybriq.adbudd$Y"),
		{"shell", "trap", "'run-as tech.hybriq.adbudd$Y ls'", "EXIT"},
	} {
		r := c.Classify(argv)
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s), want red special", argv, r.Level, r.Special, r.Primary.Rule)
		}
	}
	for _, k := range []struct {
		line  string
		level Level
	}{
		{"ls /sdcard/hybriq", Green},
		{"cat /sdcard/Download/adbuddy.log", Green},
		{"input text hybriq", Yellow},
	} {
		r := c.Classify(sh(k.line))
		if r.Level != k.level || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want %v not special", k.line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.level)
		}
	}
	// A plain 🔴 without the name is still not special.
	if r := c.Classify(sh("rm /sdcard/x")); r.Level != Red || r.Special {
		t.Errorf("rm /sdcard/x: %v special=%v", r.Level, r.Special)
	}
}

func hasRule(r Result, rule string) bool {
	for _, f := range r.Findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

// T3 round 4: variables the shell itself runs or expands as code (PS4 before
// each `set -x` line, prompts, ENV, BASH_ENV, FPATH, EXECSHELL) hide code
// from the classifier: assigning one, in any form, is special.
func TestAssigningACodeVariableIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"PS4='$(reboot)'; set -x; ls",
		"export PS4='$(reboot)'; set -x; ls",
		"PS4='$(reboot)' sh -xc ls",
		"env ENV=/data/local/tmp/x sh -i -c ls",
		"FPATH=/data/local/tmp; foo",
		"PS4=x",
		"typeset PS4=x",
		"readonly PS4=x",
		"export ENV=/data/local/tmp/x",
		"typeset -x FPATH=/data/local/tmp",
		`sh -c "PS4='\$(reboot)'; set -x; ls"`,
		"echo $(PS4=x; set -x; ls)",
		"for PS4 in '$(reboot)'; do set -x; ls; done",
		": ${PS4='$(reboot)'}",
		"echo '$(reboot)' | read PS4",
		"read -r PS4?prompt",
		"read $X",
		"set -A PS4 x",
		"env -i PS4=x ls",
		"env -u A PS4=x ls",
		"BASH_ENV=/data/local/tmp/x ls",
		"EXECSHELL=/data/local/tmp/x ls",
		"PS1='$(reboot)'",
		"typeset -n r=PS4; r=x",
		`typeset "$X"=x`,
		"(( PS4 = 1 ))",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special || !hasRule(r, "shell.script") {
			t.Errorf("%q: %v special=%v (%s: %s), want red special shell.script", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	// Without a code variable, nothing changes.
	for _, k := range []struct {
		line    string
		level   Level
		special bool
	}{
		{"set -x; ls", Green, false},
		{"X=1; echo $X", Green, false},
		{"export FOO=1", Green, false},
		{"for p in a b; do echo $p; done", Green, false},
		{"IFS=x; ls", Red, false}, // PATH is special since round 5
		{"read line", Red, false},
		{"set -- PS4", Green, false},
	} {
		r := c.Classify(sh(k.line))
		if r.Level != k.level || r.Special != k.special {
			t.Errorf("%q: %v special=%v (%s: %s), want %v special=%v", k.line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.level, k.special)
		}
	}
}
