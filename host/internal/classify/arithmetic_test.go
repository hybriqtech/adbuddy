package classify

import "testing"

// T3 round 5: mksh evaluates the value of a variable used in arithmetic as an
// expression, and an array index in it is expanded in full, $(…) included:
// x='a[$(reboot)]'; echo $((x)) runs reboot. Every arithmetic operand not
// proven to be a number cannot be analysed.
func TestArithmeticOnAValueNotProvenNumericIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// The cases of the brief.
		"x='a[$(reboot)]'; echo $((x))",
		"x='a[$(reboot)]'; ((x))",
		"let x='a[$(reboot)]'",
		"typeset -i n='a[$(reboot)]'",
		`x='a[$(reboot)]'; [ "$x" -eq 1 ]`,
		"x='a[$(reboot)]'; echo ${s:x:1}",
		"echo $(( $(getprop x) ))",
		"echo $((X))",
		"read x; echo $((x))",
		// The same contexts, other forms.
		"x='a[$(reboot)]'; echo $(( $x ))",
		`x='a[$(reboot)]'; echo $(( "$x" + 1 ))`,
		"x='a[$(reboot)]'; ((x++))",
		"x='a[$(reboot)]'; ((x+=1))",
		"x='a[$(reboot)]'; echo ${a[x]}",
		"x='a[$(reboot)]'; a[x]=1",
		"x='a[$(reboot)]'; test $x -gt 0",
		"x='a[$(reboot)]'; [[ $x -eq 1 ]]",
		"x='a[$(reboot)]'; echo ${s:0:x}",
		`let "x=a[\$(reboot)]"`,
		"let x=$(getprop x)",
		"local -i n=$(getprop x)",
		"integer n='a[$(reboot)]'",
		"typeset -i n; read n",
		"typeset -i n; n='a[$(reboot)]'",
		"n='a[$(reboot)]'; typeset -i n",
		"SECONDS='a[$(reboot)]'",
		"for i in $(getprop x); do echo $((i)); done",
		"for i in 1 2; do :; done; i=$(getprop x); echo $((i))",
		"echo $(($1))",
		"typeset -i n='PATH=1'",
		// mksh's shift and ulimit evaluate their argument too.
		"x='a[$(reboot)]'; shift x",
		"ulimit -n $(getprop x)",
		// A value that splits into an integer comparison inside [ ].
		"x='a[$(reboot)] -eq 1'; [ $x ]",
		`sh -c '[ "$@" ]' _ 'a[$(reboot)]' -eq 1`,
		// A value set by the enclosing shell reaches a nested one.
		`x='a[$(reboot)]' sh -c 'echo $((x++))'`,
		`export x='a[$(reboot)]'; sh -c '((x++))'`,
		`env x='a[$(reboot)]' sh -c 'echo $((x++))'`,
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special || !hasRule(r, "shell.script") {
			t.Errorf("%q: %v special=%v (%s: %s), want red special shell.script", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	// Numbers the classifier can prove keep their level.
	for _, k := range []struct {
		line  string
		level Level
	}{
		// The cases of the brief.
		{"((i=1+2))", Green},
		{"echo $((1+2))", Green},
		{"typeset -i n=0; n=n+1", Green},
		{"for i in 1 2 3; do echo $((i*2)); done", Green},
		{"i=0; while [ $i -lt 3 ]; do i=$((i+1)); done", Green},
		{"[ $# -eq 0 ]", Green},
		// Other everyday arithmetic.
		{"echo $((i++))", Green},
		{"((i=i+1))", Green},
		{"n=5; echo $((n*2))", Green},
		{"echo $((RANDOM % 10))", Green},
		{"echo $((16#ff + 0x1f))", Green},
		{"echo ${s:1:2}", Green},
		{"n=2; echo ${s:n:1}", Green},
		{`i=0; [[ $i -lt 3 ]] && echo ok`, Green},
		{`[ "$?" -ne 0 ] && echo failed`, Green},
		{`n=$(( $# + 1 )); echo $n`, Green},
		{"[ -e /sdcard/x ]", Green},
		{`[ -n "$X" ]`, Green},
		{`[ "$(getprop sys.boot_completed)" = 1 ]`, Green},
		{"sh -c 'for i in 1 2; do echo $((i)); done'", Green},
	} {
		r := c.Classify(sh(k.line))
		if r.Level != k.level || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want %v not special", k.line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.level)
		}
	}
}

// T3 round 5 ruling: PATH, LD_PRELOAD and the like make an ordinary command
// run a file the classifier never sees, like running the file itself.
func TestAssigningAVariableThatPicksTheFileIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// The cases of the brief.
		"PATH=/data/local/tmp ls",
		"LD_PRELOAD=/data/local/tmp/x.so getprop",
		"CLASSPATH=/data/local/tmp/x.jar app_process / X",
		// Every form.
		"export PATH=/data/local/tmp; ls",
		"typeset PATH=/data/local/tmp; ls",
		"for PATH in /data/local/tmp; do ls; done",
		": ${PATH:=/data/local/tmp}; ls",
		"(( PATH = 1 )); ls",
		"let PATH=1; ls",
		`let "PATH=1"; ls`,
		"integer PATH=1; ls",
		"read PATH; ls",
		"set -A PATH /data/local/tmp; ls",
		"printf -v PATH /data/local/tmp; ls",
		"env PATH=/data/local/tmp ls",
		"PATH[0]=/data/local/tmp; ls",
		// The others.
		"LD_LIBRARY_PATH=/data/local/tmp ls",
		"LD_AUDIT=/data/local/tmp/x.so ls",
		"LD_CONFIG_FILE=/data/local/tmp/ld.config.txt ls",
		"BOOTCLASSPATH=/data/local/tmp/x.jar app_process / X",
		"SYSTEMSERVERCLASSPATH=/data/local/tmp/x.jar app_process / X",
		"DEX2OATBOOTCLASSPATH=/data/local/tmp/x.jar app_process / X",
		"ANDROID_ROOT=/data/local/tmp am start -a android.intent.action.VIEW",
		"ANDROID_DATA=/data/local/tmp am start -a android.intent.action.VIEW",
		"ANDROID_ART_ROOT=/data/local/tmp app_process / X",
		"ANDROID_I18N_ROOT=/data/local/tmp app_process / X",
		"ANDROID_TZDATA_ROOT=/data/local/tmp app_process / X",
		"ANDROID_RUNTIME_ROOT=/data/local/tmp app_process / X",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	for _, k := range []struct {
		line    string
		level   Level
		special bool
	}{
		{"export FOO=1; getprop", Green, false},
		{"typeset -x PATH; ls", Green, false},
		{"echo $PATH", Green, false},
		{"IFS=x; ls", Red, false}, // splits words, runs no file
	} {
		r := c.Classify(sh(k.line))
		if r.Level != k.level || r.Special != k.special {
			t.Errorf("%q: %v special=%v (%s: %s), want %v special=%v", k.line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.level, k.special)
		}
	}
}

// T3 round 5: commands that run their argument as a command are judged by
// that command; the ones that cannot be read are special.
func TestCommandsThatRunTheirArgumentAreUnwrapped(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// The cases of the brief.
		"watch reboot",
		"taskset 1 reboot",
		"chroot / sh -c 'x'",
		// Unwrapped: the inner command decides.
		"watch -n 1 reboot",
		"watch -x reboot",
		"watch 'ls; reboot'",
		"toybox watch reboot",
		"logwrapper reboot",
		"logwrapper -k reboot",
		"taskset -a 3 reboot",
		"chrt -f 1 reboot",
		"chrt 0 reboot",
		"ionice -c 3 reboot",
		"ionice -c3 -n7 reboot",
		"runcon u:r:shell:s0 reboot",
		"nsenter -t 1 -n reboot",
		"unshare -n reboot",
		"flock /data/local/tmp/l reboot",
		"flock -n /data/local/tmp/l -c reboot",
		"script -q -c reboot /dev/null",
		// Not unwrapped safely: special wrappers.
		"chroot /data/local/tmp ls",
		"taskset -p 1 1234",
		"chrt -p 1234",
		"ionice -p 1234",
		"nsenter -t 1 -m ls",
		"nsenter -t 1",
		"unshare",
		"watch -z ls",
		"script",
		"fc -s",
		"fc -e vi",
		"fc -e - ls",
		"fc",
		"r",
		"logwrapper -z ls",
		"flock -z /data/local/tmp/l ls",
		"taskset $M reboot",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	// Never below the command they run.
	for _, cmd := range []string{"getprop", "ls /sdcard", "input tap 1 1", "rm /sdcard/x", "pm path com.x"} {
		alone := c.Classify(sh(cmd))
		for _, w := range []string{"logwrapper", "taskset 1", "chrt 0", "ionice -c 3", "runcon u:r:shell:s0", "nsenter -t 1 -n", "unshare -n", "flock /data/local/tmp/l", "watch -x", "watch"} {
			got := c.Classify(sh(w + " " + cmd))
			if got.Level < alone.Level || (alone.Special && !got.Special) {
				t.Errorf("%s %s: %v special=%v, below %v special=%v", w, cmd, got.Level, got.Special, alone.Level, alone.Special)
			}
		}
	}
	check(t, c, []tc{
		{sh("logwrapper getprop"), Green, "sys.getprop", "", false},
		{sh("taskset 1 ls"), Green, "fs.ls", "", false},
		{sh("fc -l"), Red, "", "", false},
		// unshare -m copies the same mounts: the command is the same file.
		{sh("unshare -m ls"), Green, "fs.ls", "", false},
		// Writing the lock file or the session is a plain write.
		{sh("flock 3 ls"), Green, "fs.ls", "", false},
		{sh("flock /data/local/tmp/l ls"), Red, "shell.redirect", "", false},
		{sh("script -c ls"), Red, "shell.redirect", "", false},
		{sh("script -q -c ls /dev/null"), Green, "fs.ls", "", false},
	})
}
