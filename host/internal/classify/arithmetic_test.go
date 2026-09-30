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

// T3 fix 6 (I-1): a word only known on the phone can be an integer
// operator of test or [: its neighbours are then evaluated as arithmetic.
func TestDynamicTestOperatorIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// The cases of the brief.
		`x='a[$(reboot)]'; op=-eq; [ "$x" "$op" 1 ]`,
		`x='a[$(reboot)]'; op=-eq; test "$x" "$op" 1`,
		`sh -c '[ "$1" "$2" "$3" ]' _ 'a[$(reboot)]' -eq 1`,
		// Other forms.
		`x='a[$(reboot)]'; [ "$x" "$(echo -eq)" 1 ]`,
		`[ "$X" "$OP" 1 ]`,
		`x='a[$(reboot)]'; op=-eq; [ 1 "$op" "$x" ]`,
		`x='a[$(reboot)]'; op=-eq; [ "$x" = 1 -o "$x" "$op" 1 ]`,
		// A static word next to it is evaluated too: -n reads the variable n.
		`n='a[$(reboot)]'; op=-eq; [ -n "$op" 1 ]`,
		// -a and -o read the variables a and o when they are operands.
		`a='x[$(reboot)]'; op=-eq; [ -a "$op" 1 ]`,
		`o='x[$(reboot)]'; op=-eq; [ 1 "$op" -o ]`,
		// [[ ]] only takes literal operators: a dynamic one does not parse.
		`x='a[$(reboot)]'; op=-eq; [[ $x $op 1 ]]`,
		// mksh prefers a binary operator to a unary one: after -n the next
		// word can still be -eq, with -n as its operand.
		`x='a[$(reboot)]'; op=-eq; [ -n "$op" "$x" -eq 1 ]`,
		`x='a[$(reboot)]'; op=-eq; [ -n "$op" "$x" ]`,
		`x='a[$(reboot)]'; op=-eq; [ -f "$op" "$x" -a -r /x ]`,
		// A unary operator whose variable the script sets is an operand.
		`f='x[$(reboot)]'; op=-eq; [ -f "$op" 1 ]`,
		`F=-eq; f='x[$(reboot)]'; [ -f "$F" -a -r "$F" ]`,
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special || !hasRule(r, "shell.script") && !hasRule(r, "shell.parse") {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	for _, line := range []string{
		// The cases of the brief keep their level.
		`[ "$a" = "$b" ]`,
		`[ -f "$F" ]`,
		`[ "$x" != "" ]`,
		`test -n "$x"`,
		// Static operators next to a dynamic word.
		`[ "$a" = x -a "$b" = y ]`,
		`[ "$a" != x -o "$b" = y ]`,
		`[ ! "$a" = x ]`,
		`[ \( "$a" = x \) ]`,
		// Unary file and string operators next to a dynamic word.
		`[ -f "$F" -a -r "$F" ]`,
		`[ -n "$x" -a -n "$y" ]`,
		`[ -z "$x" -o "$x" = 0 ]`,
		`[ -d "$D" -o -L "$D" ]`,
		`test -e "$F" -a -s "$F"`,
		// A dynamic word proven to be a number is no operator.
		`i=1; [ 1 -eq "$i" ]`,
		`i=1; n=2; [ "$n" -gt "$i" ]`,
	} {
		r := c.Classify(sh(line))
		if r.Level != Green || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want green", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
}

// T3 fix 6 (I-2): typeset -L, -R and -Z cut or pad the value, so a number
// such as 16#ab can become the name ab.
func TestJustifiedVariableIsNotNumeric(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// The cases of the brief.
		"typeset -R2 x=16#ab; ab='a[$(reboot)]'; echo $((x))",
		"x=0xab; typeset -R2 x; ab='a[$(reboot)]'; echo $((x))",
		`typeset -R2 x=16#ab; ab='a[$(reboot)]'; [ "$x" -eq 1 ]`,
		// Every option and declaration.
		"typeset -L2 x=16#ab; echo $((x))",
		"typeset -Z x=16#ab; echo $((x))",
		"typeset -R x=16#ab; echo $((x))",
		"typeset -iR2 x=16#ab; echo $((x))",
		"typeset -i -R2 x=16#ab; echo $((x))",
		"typeset -xZ3 x=16#ab; echo $((x))",
		"export -R2 x=16#ab; echo $((x))",
		"readonly -R2 x=16#ab; echo $((x))",
		"f() { local -R2 x=16#ab; echo $((x)); }",
		"integer -R2 x=16#ab; echo $((x))",
		"builtin typeset -R2 x=16#ab; echo $((x))",
		"x=16#ab; export -L2 x; echo $((x))",
		"x=16#ab; echo $((x)); typeset -R2 x",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special || !hasRule(r, "shell.script") {
			t.Errorf("%q: %v special=%v (%s: %s), want red special shell.script", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	for _, line := range []string{
		"typeset -i x=16#ab; echo $((x))",
		"typeset -x x=16#ab; echo $((x))",
		"typeset -R2 s=ab; echo $s",
	} {
		r := c.Classify(sh(line))
		if r.Level != Green || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want green", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
}

// T3 fix 6 (M-1, M-2): unset, read, getopts, integer and set -A evaluate the
// index of an array element they name, and read and getopts set REPLY and
// OPTARG from input.
func TestArrayNamesAndImplicitVariablesAreSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// The cases of the review.
		"unset 'a[$(reboot)]'",
		"read 'a[$(reboot)]'",
		"getopts a 'a[$(reboot)]'",
		"integer 'a[$(reboot)]'",
		"set -A 'a[$(reboot)]' 1",
		`x='a[$(reboot)]'; unset "$x"`,
		"echo 'a[$(reboot)]' | { read; echo $((REPLY)); REPLY=0; }",
		// Other forms.
		"unset -v 'a[$(reboot)]'",
		"unset $X",
		"read -r 'a[$(reboot)]'",
		"read 'a[$(reboot)]?prompt'",
		"integer 'a[$(reboot)]=1'",
		"set +A 'a[$(reboot)]' 1",
		"getopts a o; echo $((OPTARG)); OPTARG=0",
		"read -r; echo $((REPLY))",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	for _, k := range []struct {
		line  string
		level Level
	}{
		{"unset x", Red},
		{"read -r line", Red},
		{"getopts ab o", Red},
		{"set -A a 1 2", Red},
		{"integer i=1", Red},
	} {
		r := c.Classify(sh(k.line))
		if r.Level != k.level || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want %v not special", k.line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.level)
		}
	}
}

// T3 fix 6: AGENTS.md tells the agent to compare command output as text,
// which stays green.
func TestComparingCommandOutputAsTextIsGreen(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		`[ "$(getprop sys.boot_completed)" = 1 ]`,
		`until [ "$(getprop sys.boot_completed)" = 1 ]; do sleep 1; done`,
	} {
		r := c.Classify(sh(line))
		if r.Level != Green || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want green", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	for _, line := range []string{
		`[ "$(getprop sys.boot_completed)" -eq 1 ]`,
		`c=$(getprop x); echo $((c+1))`,
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
}

// T3 fix 6, re-review: a justified variable the enclosing shell exports
// reaches the nested one, where -f, -n or -a read it as an operand.
func TestExportedJustifiedVariableTaintsTheNestedShell(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		`typeset -xR2 f=16#ab; export ab='x[$(reboot)]'; sh -c '[ -f "$1" 1 ]' _ -eq`,
		`typeset -xR2 f=16#ab; export ab='x[$(reboot)]'; sh -c 'op=-eq; [ -f "$op" 1 ]'`,
		`typeset -xR2 a=16#ab; export ab='x[$(reboot)]'; sh -c '[ -a "$1" 1 ]' _ -eq`,
		`export -R2 f=16#ab; export ab='x[$(reboot)]'; sh -c '[ -f "$1" 1 ]' _ -eq`,
		`typeset -xi16 -R2 f=171; export ab='x[$(reboot)]'; sh -c '[ -f "$1" 1 ]' _ -eq`,
		// Declared justified without a value in this command.
		`typeset -xR2 f; export ab='x[$(reboot)]'; sh -c '[ -f "$1" 1 ]' _ -eq`,
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special || !hasRule(r, "shell.script") {
			t.Errorf("%q: %v special=%v (%s: %s), want red special shell.script", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
}

// T3 fix 6, re-review: a declaration run through builtin or command checks
// its names like a parsed one.
func TestWrappedDeclarationNamesAreChecked(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"builtin typeset 'a[$(reboot)]'",
		"command export 'a[$(reboot)]=1'",
		"command readonly $X",
		"builtin local 'a[$(reboot)]'",
		"builtin export PATH=/data/local/tmp; ls",
		"builtin typeset -n r=PATH",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
	for _, line := range []string{
		"builtin typeset x=1",
		`command export x="$(getprop y)"`,
		"builtin typeset -x FOO",
	} {
		r := c.Classify(sh(line))
		if r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want not special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
}
