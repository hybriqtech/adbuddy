package classify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The differential tests compare the classifier with a real mksh (spec M2.5
// §5). Every case carries the sentinel @S@, a program that leaves a mark
// when it runs. TestRecordOracle runs the cases on an Android emulator and
// writes which ones ran the sentinel to testdata/oracle.jsonl;
// TestWhatMkshRunsIsSpecial, which needs no emulator, then demands a 🔴
// special verdict for each of those. No case contains a destructive command.

const (
	sentinelMark = "@S@"
	sentinelPath = "/data/local/tmp/m25-sentinel"
	sentinelRan  = "/data/local/tmp/m25-ran"
	diffDir      = "/data/local/tmp/m25-diff"
	diffCwd      = "/data/local/tmp/m25-cwd"
	diffWords    = "/data/local/tmp/m25-words"
)

// withSentinel puts the sentinel's path in a case. It has no spaces and no
// glob characters, so an unquoted expansion does not split it, and the
// classifier reads it as running a file: 🔴 special wherever it sees it as a
// command. It does not carry the app's name: a 🔴 that names the app is
// special whatever it does (namesTheApp), and that would hide a verdict that
// is 🔴 for another reason and not special.
func withSentinel(s string) string { return strings.ReplaceAll(s, sentinelMark, sentinelPath) }

// payloads are the values a case plants in a variable, by the way they
// could run: as the command itself, as a substitution in text that the
// shell expands again, as an array index in text evaluated as arithmetic,
// and as an operator of test.
var payloads = map[string]string{
	"command": "@S@",
	"subst":   "$(@S@)",
	"ticks":   "`@S@`",
	"index":   "a[$(@S@)]",
}

// carriers are the ways a script gives the variable v a value it did not
// write as a word: %s is the payload, %s the body that uses v.
var carriers = []string{
	"v='%s'; %s",
	"for v in '%s'; do %s\ndone",
	"echo '%s' | while read -r v; do %s\ndone",
	"v=$(echo '%s'); %s",
	"w='%s'; v=$w; %s",
}

// admittedSinks use v in every way the evaluator admits.
var admittedSinks = []string{
	`echo $v`, `echo "$v"`, `echo ${v}`, `echo "${v}"`, `echo x$v`, `echo "x${v}y"`,
	`echo ${#v}`, `echo ${v#a}`, `echo ${v##a}`, `echo ${v%a}`, `echo ${v%%a}`, `echo ${v-x}`, `echo ${v:-x}`,
	`echo ${u-$v}`, `echo ${u:-$v}`, `echo ${u#$v}`, `echo "${u:-$v}"`,
	`: $v`, `true $v`, `false "$v"`, `cd $v`, `cd "$v"`,
	`[ "$v" ]`, `[ -n "$v" ]`, `[ -z "$v" ]`, `[ -e "$v" ]`, `[ -f "$v" ]`, `[ -d "$v" ]`, `test -n "$v"`,
	`[ "$v" = x ]`, `[ x = "$v" ]`, `[ "$v" != x ]`, `[ "$v" = "$v" ]`, `test "$v" = x`,
	`[ ! "$v" = x ]`, `[ \( "$v" = x \) ]`, `[ -n "$v" -a -n "$v" ]`, `[ -z "$v" -o "$v" = x ]`,
	`if [ "$v" = x ]; then echo y; else echo n; fi`,
	`if [ -n "$v" ]; then echo y; elif [ -z "$v" ]; then echo n; fi`,
	`while [ "$v" = x ]; do break; done`, `until [ "$v" != x ]; do break; done`,
	`for w in $v; do echo $w; done`, `for w in "$v" x; do echo "$w"; done`,
	`w=$v; echo $w`, `w="$v"; echo "$w"`, `w=x$v; echo $w`,
	`echo $(echo $v)`, `echo "$(echo "$v")"`, "echo `echo $v`",
	`( echo $v )`, `{ echo $v; }`, `echo $v | cat`, `echo $v && echo $v || echo $v`, `! echo $v`,
	`cat <<EOF` + "\n$v\nEOF", `cat <<-EOF` + "\n$v\nEOF", `cat <<<"$v"`, `cat <<<$v`,
	`echo $v >/dev/null`, `echo $v 2>&1`, `echo $v >/dev/null 2>&1`,
	`set -x; echo $v`, `set -e; echo $v`, `set -u; echo $v`, `set -o xtrace; echo $v`,
	`n=1; echo $((n+1)) $v`, `n=1; [ $n -lt 2 ] && echo $v`, `[ $# -eq 0 ] && echo $v`, `[ "$?" -ne 0 ]; echo $v`,
	`echo $v; exit`, `echo $v; exit 0`, `for w in 1 2; do echo $v; break; done`, `for w in 1 2; do echo $v; continue; done`,
	`read -r w </dev/null; echo $v $w`, `command -v ls; echo $v`, `type ls; echo $v`, `time echo $v`,
	`LANG=C echo $v`, `env TZ=UTC echo $v`,
	`echo $v &>/dev/null`, `echo $v >>/dev/null`, `echo $v >|/dev/null`, `echo $v <>/dev/null`,
	`echo $v 2>&-`, `echo $v <&0`, `cat <"$v"`, `cat <$v`,
	`echo $_ $v`, `echo $"$v"`, `echo $'x' $v`, `echo ${v#@(a|b)}`, `echo ${v%"a"}`, `echo ${v:-"x y"}`,
	`n=1; echo $v; exit $n`, `n=1; echo $v; exit $((n+1))`, `echo $v; exit $?`,
	`n=1; for w in 1 2; do echo $v; break $n; done`, `n=1; for w in 1 2; do echo $v; continue $n; done`,
	`read -r a b </dev/null; echo $v $a $b`, `read a </dev/null; echo $v $a`,
	`whence ls; echo $v`, `command -V ls; echo $v`, `command -p ls /nonexistent; echo $v`, `builtin echo $v`,
	`cd /; pwd; echo $v`, `kill -l; echo $v`,
}

// otherSinks use v in ways the evaluator does not admit: recorded to know
// what mksh does with each, and because every one that runs the sentinel
// must be 🔴 special.
var otherSinks = []string{
	`$v`, `"$v"`, `eval "$v"`, `eval echo "$v"`, `sh -c "$v"`, `sh -c "echo $v"`, `trap "$v" EXIT`, `alias q="$v"; q`,
	`echo $((v))`, `echo $(($v))`, `echo $((v+1))`, `echo $(( "$v" ))`, `: $((v))`, `w=$((v))`,
	`((v))`, `((v++))`, `let v`, `let "v"`, `let w=v`, `let "w=$v"`,
	`[ "$v" -eq 1 ]`, `[ 1 -eq "$v" ]`, `[ "$v" -lt 2 ]`, `test "$v" -gt 0`, `[ $v -eq 1 ]`,
	`[[ $v -eq 1 ]]`, `[[ "$v" -eq 1 ]]`, `[[ $v = x ]]`, `[[ -n $v ]]`, `[[ $v ]]`,
	`[ -v "$v" ]`, `[[ -v $v ]]`, `[ -o "$v" ]`,
	`s=abc; echo ${s:v:1}`, `s=abc; echo ${s:0:v}`, `s=abc; echo ${s:$v:1}`,
	`echo ${a[v]}`, `echo ${a[$v]}`, `a[v]=1`, `a[$v]=1`, `a=([v]=1)`, `a=([$v]=1)`, `set -A a -- "$v"`,
	`typeset -i w=$v`, `typeset -i w; w=$v`, `integer w=$v`, `typeset -i v`, `typeset w=$v; echo $w`,
	`typeset -R2 w=$v; echo $w`, `typeset -L2 w=$v; echo $((w))`, `typeset -u w=$v; echo $w`,
	`export w=$v; sh -c 'echo $w'`, `export v; sh -c 'echo $((v))'`, `export v; sh -c 'echo $v'`,
	`local w=$v`, `readonly w=$v`, `nameref w=v; echo $w`, `typeset -n w=v; echo $w`,
	`shift $v`, `shift "$v"`, `exit $v`, `exit "$v"`, `for w in 1; do break "$v"; done`, `for w in 1; do continue "$v"; done`,
	`return "$v"`, `ulimit -n "$v"`, `umask "$v"`, `wait "$v"`, `kill -0 "$v"`, `sleep "$v"`,
	`unset "$v"`, `unset -v "$v"`, `read "$v" </dev/null`, `read -r "$v" </dev/null`, `getopts "$v" w`, `getopts a "$v"`,
	`print "$v"`, `print -r -- "$v"`, `printf "$v"`, `printf %d "$v"`, `printf '%s' "$v"`,
	`echo ${v:=x}`, `echo ${u:=$v}`, `echo ${v:?x}`, `echo ${v:+x}`, `echo ${v/a/b}`, `echo ${v//a/b}`, `echo ${!v}`, `echo ${v@Q}`,
	`echo ${v:0:1}`, `echo ${#v[@]}`, `echo ${v[0]}`,
	`case $v in x) echo y;; esac`, `case x in $v) echo y;; esac`,
	`select w in $v; do break; done </dev/null`,
	`f() { echo $v; }; f`, `function f { echo $v; }; f`,
	`echo <(echo $v)`, `echo ${ echo $v;}`, `echo ${|REPLY=$v;}`,
	`PS4=$v; set -x; true`, `PS1=$v; true`, `ENV=$v sh -c true`, `IFS=$v; echo x`, `PATH=$v; true`,
	`X=$v; echo $X`, `X=$v; echo $((X))`, `SECONDS=$v`, `RANDOM=$v`, `OPTIND=$v`, `COLUMNS=$v`, `TMOUT=$v`, `HISTSIZE=$v`,
	`w=$v true`, `w=$v sh -c 'echo $((w))'`, `env w="$v" sh -c 'echo $((w))'`,
	`echo $v &`, `echo $v |& cat`,
	`set -- "$v"; echo $1`, `set -- "$v"; echo $(($1))`, `set -A w "$v"`, `set -o "$v"`,
	`cd "$v" x`, `whence "$v"`, `type "$v"`, `command -v "$v"`, `command "$v"`, `builtin "$v"`, `exec "$v"`,
	`fc -e "$v"`, `bind "$v"`, `rename "$v" x`, `times "$v"`, `jobs "$v"`, `hash "$v"`,
	`echo $'$v'`, `echo {$v,x}`, `echo ~$v`,
	`echo x >"$v"`, `echo x >&"$v"`,
	`time -p echo $v`, `! [[ $v ]]`,
}

// testOperands are the values a word of data may take inside test or [:
// operators as well as the payload.
var testOperands = []string{"-eq", "-n", "-a", "-o", "=", "!", "(", "-v", "1", "a[$(@S@)]"}

// testShapes are the expressions of test the evaluator admits, with %[1]s
// and %[2]s for the two words of data.
var testShapes = []string{
	`[ "%[1]s" ]`,
	`[ -n "%[1]s" ]`, `[ -z "%[1]s" ]`, `[ -f "%[1]s" ]`, `[ -e "%[1]s" ]`,
	`[ "%[1]s" = x ]`, `[ x = "%[1]s" ]`, `[ "%[1]s" != "%[2]s" ]`, `[ "%[1]s" = "%[2]s" ]`,
	`[ ! "%[1]s" = x ]`, `[ ! -n "%[1]s" ]`, `[ \( "%[1]s" = x \) ]`,
	`[ -n "%[1]s" -a -n "%[2]s" ]`, `[ -z "%[1]s" -o "%[2]s" = x ]`, `[ "%[1]s" = x -a "%[2]s" = y ]`,
	`[ -f "%[1]s" -a -r "%[2]s" ]`, `[ 1 -eq 1 -a -n "%[1]s" ]`, `[ -n "%[1]s" -a 1 -lt 2 ]`,
	`test -n "%[1]s"`, `test "%[1]s" = "%[2]s"`, `test -n "%[1]s" -a -z "%[2]s"`,
}

// testUnary and testBinary are the operators of test the evaluator admits
// with a word of data: each gets a case with the payload as its operand,
// alone and joined to another expression.
var (
	testUnary  = []string{"-b", "-c", "-d", "-e", "-f", "-g", "-G", "-h", "-k", "-L", "-n", "-O", "-p", "-r", "-s", "-S", "-t", "-u", "-w", "-x", "-z"}
	testBinary = []string{"=", "==", "!=", "-nt", "-ot", "-ef"}
)

// quotedTexts are ways to write the payload as text, not as the value of
// a variable: where the parser and mksh could read the quotes differently.
var quotedTexts = []string{`'$(@S@)'`, `"\$(@S@)"`, `\$\(@S@\)`, "'`@S@`'", `$'$(@S@)'`, `'a[$(@S@)]'`}

// textContexts are the places a quoted text can stand in, %s: every
// admitted word form, bare, in double quotes, in a here-string and in a
// here-document.
var textContexts = []string{
	`echo %s`, `echo x%sy`, `u=%s; echo $u`, `for w in %s; do echo $w; done`,
	`echo ${u:-%s}`, `echo ${u-%s}`, `echo ${u#%s}`, `echo ${u##%s}`, `echo ${u%%%s}`, `echo ${u%%%%%s}`,
	`echo "${u:-%s}"`, `echo "${u-%s}"`, `echo "${u#%s}"`, `echo "${u##%s}"`, `echo "${u%%%s}"`, `echo "${u%%%%%s}"`,
	`echo "x${u:-%s}y"`, `echo "${u:-${t:-%s}}"`, `echo ${u:-${t:-%s}}`, `echo "${u:-x%sy}"`,
	`w="${u:-%s}"; echo "$w"`, `[ "${u:-%s}" = x ]`, `for w in "${u:-%s}"; do echo "$w"; done`,
	`echo $(echo "${u:-%s}")`, "echo `echo \"${u:-%s}\"`",
	`cat <<<%s`, `cat <<<"${u:-%s}"`, `cat <<<${u:-%s}`,
	"cat <<EOF\n%s\nEOF", "cat <<EOF\n${u:-%s}\nEOF", "cat <<EOF\n${u#%s}\nEOF", "cat <<-EOF\n\t${u:-%s}\nEOF",
	"cat <<'EOF'\n${u:-%s}\nEOF", "cat <<\"EOF\"\n%s\nEOF", "cat <<\\EOF\n${u:-%s}\nEOF",
	`cat <%s`, `echo x >/dev/null 2>%s`,
}

// wrapperLines put the sentinel behind every wrapper and every way to
// write its options: where a wrapper could read fewer options than the
// phone does and take another word for the command.
var wrapperLines = []string{
	`sh -c @S@`, `sh -c -- @S@`, `sh -c -e @S@`, `sh -e -c @S@`, `sh -ec @S@`, `sh +e -c @S@`, `sh -c @S@ x`,
	`sh -co nohup @S@`, `sh -co xtrace @S@`, `sh -o xtrace -c @S@`, `sh -c -o xtrace @S@`, `sh -T - -c @S@`,
	`sh -xc @S@`, `sh -uc @S@`, `sh -fc @S@`, `sh -vc @S@`, `sh -ac @S@`, `sh -Cc @S@`, `sh -nc @S@`,
	`mksh -c @S@`, `/system/bin/sh -c @S@`, `toybox sh -c @S@`,
	`command @S@`, `command -- @S@`, `command -p @S@`, `command -p -- @S@`, `builtin command -- @S@`, `builtin -- command @S@`,
	`exec @S@`, `exec -- @S@`, `exec -a x @S@`, `exec -c @S@`,
	`nohup @S@`, `nohup -- @S@`, `time @S@`, `time -- @S@`,
	`nice @S@`, `nice -n 1 @S@`, `nice -n 1 -- @S@`, `nice -n 1 -n 2 @S@`, `nice -5 @S@`, `nice -- @S@`,
	`timeout 5 @S@`, `timeout -- 5 @S@`, `timeout -s 9 5 @S@`, `timeout -k 1 5 @S@`, `timeout -s9 5 @S@`, `timeout --foreground 5 @S@`,
	`env @S@`, `env -- @S@`, `env - @S@`, `env -i @S@`, `env -u X @S@`, `env -0 @S@`, `env X=1 @S@`, `env TZ=UTC @S@`, `env -uX @S@`,
	`echo x | xargs @S@`, `echo x | xargs -- @S@`, `echo x | xargs -n 1 @S@`, `echo x | xargs -n1 @S@`, `echo x | xargs -I {} @S@`,
	`echo x | xargs -I{} @S@`, `echo x | xargs -0 @S@`, `echo x | xargs -r @S@`, `echo @S@ | xargs`, `echo @S@ | xargs -n 1`,
	`find /system/bin/sh -exec @S@ \;`, `find /system/bin/sh -exec @S@ {} +`, `find /system/bin/sh -execdir @S@ \;`,
	`find /system/bin/sh -ok @S@ \;`, `find /system/bin/sh -name sh -exec @S@ \; -print`, `find /system/bin/sh -exec sh -c @S@ \;`,
	`su 0 @S@`, `su root @S@`, `su 0 -c @S@`, `su -c @S@`, `su 0 sh -c @S@`, `su 0 sh -co nohup @S@`, `su shell @S@`, `su u0_a1 @S@`,
	`toybox nice @S@`, `toybox timeout 5 @S@`, `toybox env @S@`, `busybox @S@`,
	`ionice @S@`, `ionice -c 3 @S@`, `ionice -c3 @S@`, `ionice -c 3 -- @S@`, `ionice -t @S@`,
	`chrt 0 @S@`, `chrt -o 0 @S@`, `chrt -- 0 @S@`, `taskset 1 @S@`, `taskset -a 1 @S@`, `taskset -- 1 @S@`,
	`logwrapper @S@`, `logwrapper -k @S@`, `logwrapper -- @S@`, `runcon u:r:shell:s0 @S@`,
	`unshare @S@`, `unshare -n @S@`, `unshare -- @S@`, `nsenter @S@`, `nsenter -- @S@`,
	`flock /dev/null @S@`, `flock -n /dev/null @S@`, `flock /dev/null -c @S@`, `flock -- /dev/null @S@`, `flock 1 @S@`,
	`watch @S@`, `watch -n 1 @S@`, `watch -x @S@`, `watch -- @S@`, `watch -n1 @S@`,
	`script -q -c @S@ /dev/null`, `script -c @S@`, `setsid @S@`, `setsid -w @S@`,
	`run-as com.android.shell @S@`, `cmd -l; @S@`,
	`[ -e /x ] || @S@`, `test -e /x || @S@`, `true && @S@`, `false || @S@`, `: ; @S@`,
}

// testNeighbours are the variables a static word next to the data names
// when mksh evaluates it as arithmetic (-n reads n): each holds the payload.
var testNeighbours = "n='a[$(@S@)]'; z=$n; f=$n; e=$n; r=$n; a=$n; o=$n; x=$n; y=$n; "

// generatedCases builds the cases the allowlist makes finite: every way to
// give a variable a payload, used in every admitted way and in the ways
// that are not admitted; and every admitted expression of test with each
// word of data being an operator or the payload.
func generatedCases() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	names := make([]string, 0, len(payloads))
	for name := range payloads {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, carrier := range carriers {
			for _, sinks := range [][]string{admittedSinks, otherSinks} {
				for _, sink := range sinks {
					add(fmt.Sprintf(carrier, payloads[name], sink))
				}
			}
		}
	}
	for _, text := range quotedTexts {
		for _, context := range textContexts {
			add(fmt.Sprintf(context, text))
		}
	}
	for _, line := range wrapperLines {
		add(line)
	}
	testCases := func(shape string, operands []string) {
		two := strings.Contains(shape, "%[2]s")
		for _, p := range operands {
			qs := []string{""}
			if two {
				qs = operands
			}
			for _, q := range qs {
				expr := strings.NewReplacer("%[1]s", "$p", "%[2]s", "$q").Replace(shape)
				set := fmt.Sprintf("p='%s'; q='%s'; ", p, q)
				add(set + expr)
				add(testNeighbours + set + expr)
			}
		}
	}
	for _, shape := range testShapes {
		testCases(shape, testOperands)
	}
	// Every operator, with the operands that matter: an integer operator,
	// the payload and a number.
	short := []string{"-eq", "a[$(@S@)]", "1"}
	for _, op := range testUnary {
		testCases(`[ `+op+` "%[1]s" ]`, short)
		testCases(`[ `+op+` "%[1]s" -a `+op+` "%[2]s" ]`, short)
	}
	for _, op := range testBinary {
		testCases(`[ "%[1]s" `+op+` "%[2]s" ]`, short)
		testCases(`[ "%[1]s" `+op+` x -o x `+op+` "%[2]s" ]`, short)
	}
	return out
}

// differentialCases are the cases written by hand in differential.jsonl plus
// the generated ones.
func differentialCases(t *testing.T) []corpusCase {
	t.Helper()
	cases := loadCorpus(t, "differential.jsonl")
	seen := map[string]bool{}
	for _, k := range cases {
		// A case names the sentinel with the mark or, when PATH is what
		// finds it, by its file name.
		if !strings.Contains(k.Sh, sentinelMark) && !strings.Contains(k.Sh, filepath.Base(sentinelPath)) {
			t.Fatalf("differential.jsonl: %q has no %s", k.Sh, sentinelMark)
		}
		if seen[k.Sh] {
			t.Fatalf("differential.jsonl: %q is repeated", k.Sh)
		}
		seen[k.Sh] = true
	}
	for _, sh := range generatedCases() {
		if !seen[sh] {
			seen[sh] = true
			cases = append(cases, corpusCase{Sh: sh})
		}
	}
	return cases
}

// oracle is the header of oracle.jsonl: the mksh that answered.
type oracleHeader struct {
	KSH string `json:"ksh"`
	SDK string `json:"sdk"`
}

// loadOracle reads testdata/oracle.jsonl: the header and, per case, whether
// the real mksh ran the sentinel.
func loadOracle(t *testing.T) (oracleHeader, map[string]bool) {
	t.Helper()
	type line struct {
		oracleHeader
		Sh  *string `json:"sh"`
		Ran bool    `json:"ran"`
	}
	var head oracleHeader
	ran := map[string]bool{}
	for i, l := range readJSONL[line](t, "oracle.jsonl") {
		if i == 0 {
			if l.Sh != nil || l.KSH == "" {
				t.Fatal("oracle.jsonl: the first line must be the header with the mksh version")
			}
			head = l.oracleHeader
			continue
		}
		if l.Sh == nil {
			t.Fatalf("oracle.jsonl: line %d has no case", i+1)
		}
		ran[*l.Sh] = l.Ran
	}
	return head, ran
}

// Spec M2.5 §2.1, criteria 3 and 4: every case is recorded against a real
// mksh, and whatever ran the sentinel there is 🔴 special here.
func TestWhatMkshRunsIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	_, ran := loadOracle(t)
	cases := differentialCases(t)
	missing := 0
	for _, k := range cases {
		did, recorded := ran[k.Sh]
		if !recorded {
			if missing++; missing <= 10 {
				t.Errorf("%q is not in oracle.jsonl: record it (see host/README.md)", k.Sh)
			}
			continue
		}
		if !did {
			continue
		}
		if r := c.Classify(sh(withSentinel(k.Sh))); r.Level != Red || !r.Special {
			t.Errorf("mksh runs the sentinel in %q and the classifier says %v special=%v (%s)", k.Sh, r.Level, r.Special, r.Primary.Rule)
		}
	}
	if missing > 10 {
		t.Errorf("…and %d more cases are not in oracle.jsonl", missing-10)
	}
	if len(ran) != len(cases)-missing {
		t.Errorf("oracle.jsonl has %d cases and the corpus %d: record it again", len(ran), len(cases))
	}
}

// Spec M2.5 §5.3: a payload that never fires proves nothing. Each form of
// payload, and each carrier, runs the sentinel in at least one recorded case.
func TestEveryPayloadFiresSomewhere(t *testing.T) {
	_, ran := loadOracle(t)
	fires := func(match func(sh string) bool) bool {
		for sh, did := range ran {
			if did && match(sh) {
				return true
			}
		}
		return false
	}
	for name, p := range payloads {
		for _, carrier := range carriers {
			prefix := fmt.Sprintf(carrier[:strings.LastIndex(carrier, "%s")], p)
			if !fires(func(sh string) bool { return strings.HasPrefix(sh, prefix) }) {
				t.Errorf("payload %s never runs the sentinel through %q", name, carrier)
			}
		}
	}
}

// The numbers that make the differential corpus worth trusting: most
// admitted uses are 🟢 here, and none of them runs the sentinel there.
func TestAdmittedSinksStayBelowRed(t *testing.T) {
	c := mustNew(t, "")
	_, ran := loadOracle(t)
	for _, sink := range admittedSinks {
		for _, carrier := range carriers {
			sh := fmt.Sprintf(carrier, payloads["index"], sink)
			r := c.Classify([]string{"shell", withSentinel(sh)})
			if r.Special {
				t.Errorf("%q: %v special (%s: %s), want an admitted form", sh, r.Level, r.Primary.Rule, r.Primary.Reason)
			}
			if ran[sh] {
				t.Errorf("mksh runs the sentinel in %q, which the evaluator admits", sh)
			}
		}
	}
}

// TestRecordOracle runs every differential case on the mksh of an Android
// emulator and rewrites testdata/oracle.jsonl and testdata/mksh-builtins.tsv.
// It needs ADBUDDY_MKSH_SERIAL and ADBUDDY_MKSH_ADB (scripts/emulator.sh
// start prints them) and refuses anything that is not an emulator.
func TestRecordOracle(t *testing.T) {
	serial, adbPath := os.Getenv("ADBUDDY_MKSH_SERIAL"), os.Getenv("ADBUDDY_MKSH_ADB")
	if serial == "" || adbPath == "" {
		t.Skip("set ADBUDDY_MKSH_SERIAL and ADBUDDY_MKSH_ADB to record (scripts/emulator.sh start)")
	}
	if !strings.HasPrefix(serial, "emulator-") {
		t.Fatalf("ADBUDDY_MKSH_SERIAL=%q is not an emulator: the cases never run on a real phone", serial)
	}
	adb := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(adbPath, append([]string{"-s", serial}, args...)...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("adb %q: %v\n%s", args, err, stderr.String())
		}
		return strings.ReplaceAll(string(out), "\r\n", "\n")
	}
	cases := differentialCases(t)
	local := filepath.Join(t.TempDir(), "m25-diff")
	if err := os.Mkdir(local, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(local, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i, k := range cases {
		write(strconv.Itoa(i), withSentinel(k.Sh))
	}
	write("sentinel", "#!/system/bin/sh\n: >"+sentinelRan+"\n")
	// Each case runs the way adb runs a command: sh -c with the text, in
	// its own process, with no input and a time limit. The sentinel is put
	// back before each one: a case may overwrite it (echo x >"$v"). A case
	// that leaves something in the background gets a second to fire, and
	// one that hits the time limit counts as having run the sentinel: what
	// it would have done next is not known. Each case runs in a session of
	// its own, and whatever it leaves alive is killed before the next one
	// (watch would go on running the sentinel): the session first, then
	// the processes of the shell user that lost their parent.
	write("run.sh", `n=$1
i=0
orphans() {
	ps -u shell -o PID,PPID | while read -r pid ppid; do
		[ "$ppid" = 1 ] && echo "$pid"
	done
}
base=" $(orphans | tr '\n' ' ') "
while [ $i -lt $n ]; do
	rm -rf `+sentinelRan+` `+diffCwd+` `+sentinelPath+`
	cp `+diffDir+`/sentinel `+sentinelPath+`
	chmod 755 `+sentinelPath+`
	mkdir `+diffCwd+`
	cd `+diffCwd+`
	text=$(cat `+diffDir+`/$i)
	setsid -w /system/bin/sh -c 'echo $$ >`+diffDir+`/sid; exec timeout 5 /system/bin/sh -c "$0"' "$text" </dev/null >/dev/null 2>&1
	code=$?
	case $text in *' & '*|*' &') sleep 1;; esac
	if [ -e `+sentinelRan+` ]; then
		echo "ran $i"
	elif [ $code -eq 124 ] || [ $code -eq 137 ] || [ $code -eq 143 ]; then
		echo "ran $i"
	fi
	pkill -9 -s "$(cat `+diffDir+`/sid)" 2>/dev/null
	for pid in $(orphans); do
		case $base in *" $pid "*) ;; *) kill -9 "$pid" 2>/dev/null ;; esac
	done
	i=$((i+1))
done
echo "done $n"
`)
	clean := "rm -rf " + diffDir + " " + diffCwd + " " + sentinelPath + " " + sentinelRan + " " + diffWords
	adb("shell", clean)
	defer adb("shell", clean)
	// Only the shell matters here. On the Android 17 image surfaceflinger
	// crashes every few seconds (Assertion failed:
	// !rcEnc->featureInfo()->hasReadColorBufferDma) and Android restarts
	// its framework over and over: cases slow down until they hit the time
	// limit, and that would count as running the sentinel. So the framework
	// is stopped while the cases run (root: the image is userdebug) and
	// started again afterwards.
	adb("shell", "su 0 stop")
	defer adb("shell", "su 0 start")
	adb("push", local, diffDir)
	out := adb("shell", "sh "+diffDir+"/run.sh "+strconv.Itoa(len(cases)))
	if !strings.Contains(out, "done "+strconv.Itoa(len(cases))) {
		t.Fatalf("the run did not finish:\n%s", out)
	}
	ran := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "ran "); ok {
			i, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil {
				t.Fatalf("bad line %q", line)
			}
			ran[i] = true
		}
	}
	if len(ran) == 0 {
		t.Fatal("no case ran the sentinel: the sentinel does not work")
	}

	head := oracleHeader{
		KSH: strings.TrimSpace(adb("shell", `echo "$KSH_VERSION"`)),
		SDK: strings.TrimSpace(adb("shell", "getprop ro.build.version.sdk")),
	}
	type row struct {
		Sh  string `json:"sh"`
		Ran bool   `json:"ran"`
	}
	rows := make([]row, len(cases))
	for i, k := range cases {
		rows[i] = row{Sh: k.Sh, Ran: ran[i]}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Sh < rows[j].Sh })
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(head); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join("testdata", "oracle.jsonl"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// The builtins: every lower-case word in the shell's binary, asked to
	// whence -v, plus the predefined aliases.
	bin := filepath.Join(t.TempDir(), "sh")
	adb("pull", "/system/bin/sh", bin)
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	words := map[string]bool{".": true, ":": true, "[": true}
	for _, w := range regexp.MustCompile(`[a-z][a-z_]{0,15}`).FindAll(data, -1) {
		words[string(w)] = true
	}
	list := make([]string, 0, len(words))
	for w := range words {
		list = append(list, w)
	}
	sort.Strings(list)
	wordsFile := filepath.Join(t.TempDir(), "m25-words")
	if err := os.WriteFile(wordsFile, []byte(strings.Join(list, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adb("push", wordsFile, diffWords)
	kinds := map[string]string{}
	for _, line := range strings.Split(adb("shell", `while read -r w; do whence -v "$w"; done <`+diffWords+` 2>&1; true`), "\n") {
		name, rest, ok := strings.Cut(line, " is a")
		if !ok {
			continue
		}
		switch {
		case strings.Contains(rest, "special shell builtin"):
			kinds[name] = "special builtin"
		case strings.Contains(rest, "shell builtin"):
			kinds[name] = "builtin"
		case strings.Contains(rest, "reserved word"):
			kinds[name] = "reserved"
		case strings.HasPrefix(rest, "n alias for"):
			kinds[name] = "alias"
		}
	}
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	var tsv strings.Builder
	tsv.WriteString("# Builtins, reserved words and predefined aliases of " + head.KSH + " (whence -v). Written by TestRecordOracle.\n")
	for _, name := range names {
		tsv.WriteString(name + "\t" + kinds[name] + "\n")
	}
	if err := os.WriteFile(filepath.Join("testdata", "mksh-builtins.tsv"), []byte(tsv.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d cases, %d ran the sentinel, %d builtins; %s", len(cases), len(ran), len(names), head.KSH)
}
