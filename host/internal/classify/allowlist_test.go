package classify

import "testing"

// Spec M2.5 §2.1, criterion 1: the evaluator denies by default. Every shell
// construct, expansion, builtin or variable name it does not list is 🔴
// special, whatever it holds: here each one only wraps harmless reads.
func TestWhatIsNotAdmittedIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		// Statements.
		"case x in x) ls;; esac",
		"select w in a b; do ls; done",
		"f() { ls; }",
		"function f { ls; }",
		"[[ -e /sdcard ]]",
		"(( 1 + 1 ))",
		"let 1+1",
		"typeset w",
		"export w",
		"local w",
		"readonly w",
		"nameref w=v",
		"time -p ls",
		"for w; do ls; done",
		// Assignments.
		"W=1",
		"_w=1",
		"w+=1",
		"w[0]=1",
		"w=(1 2)",
		"w=1 ls",
		"PATH=/system/bin ls",
		"for W in a; do ls; done",
		"read W",
		"read",
		"read -r w?prompt",
		"read -t 1 w",
		// Expansions.
		"echo ${w:=1}",
		"echo ${w=1}",
		"echo ${w:?1}",
		"echo ${w:+1}",
		"echo ${w:1:2}",
		"echo ${w/a/b}",
		"echo ${!w}",
		"echo ${w[0]}",
		"echo ${w[@]}",
		"echo ${#w[@]}",
		"echo ${w@Q}",
		"echo $1",
		"echo $@",
		`echo "$@"`,
		"echo $*",
		"echo $$",
		"echo $!",
		"echo $-",
		"echo $0",
		"echo ${ ls;}",
		"echo ${|REPLY=1;}",
		"cat <(ls)",
		"echo @(a|b)",
		// Quotes in the word of ${x:-…} inside double quotes or a
		// here-document: mksh takes them as text and expands what they hold.
		`echo "${w:-'x'}"`,
		`echo "${w-'x'}"`,
		`echo "${w:-"a b"}"`,
		`echo "${w#'x'}"`,
		`echo "${w:-${v:-'x'}}"`,
		`cat <<<"${w:-'x'}"`,
		"cat <<EOF\n${w:-'x'}\nEOF",
		// Arithmetic.
		"echo $((w))",
		"echo $((w=1))",
		"w=1; echo $((w++))",
		"w=1; echo $((w+=1))",
		"echo $((RANDOM))",
		"echo $(($(ls)))",
		"w=$(ls); echo $((w))",
		"w=1; w=x; echo $((w))",
		"read w; echo $((w))",
		"for w in 1 x; do echo $((w)); done",
		"echo $((1 + w[0]))",
		// test and [.
		"[ $w = x ]",
		"[ $(ls) ]",
		`[ "$w" -eq 1 ]`,
		`[ 1 -lt "$(ls)" ]`,
		`[ "$w" "$v" x ]`,
		`[ "$w" "$v" ]`,
		`[ -v "$w" ]`,
		`[ -o "$w" ]`,
		"[ -e /sdcard",
		`n=x; [ -n "$v" -a -f /sdcard/x ]`, // -n reads n if $v is an integer operator
		// Builtins.
		"trap",
		"alias",
		"unset w",
		"shift",
		"wait",
		"print x",
		"umask",
		"ulimit -a",
		"getopts a w",
		"exit $w",
		"exit x",
		"set -- a b",
		"set -A w 1",
		"set -o vi",
		"set -B",
		"kill $w",
		"type $w",
		// Redirections.
		"ls {w}>/dev/null",
		// Wrappers: the shell's options, and an option where the command
		// should be.
		"sh -co nohup ls",
		"sh -o xtrace -c ls",
		"sh -c -- ls",
		"sh -Tc ls",
		"sh +c ls",
		"sh -c +c ls",
		"command -- ls",
		"builtin command -- ls",
		"nice -n 1 -- ls",
		"nice -n 1 -n 2 ls",
		"timeout -- 5 ls",
		"su 0 -- ls",
	} {
		r := c.Classify(sh(line))
		if r.Level != Red || !r.Special || !notAnalysed(r) {
			t.Errorf("%q: %v special=%v (%s: %s), want red special, not analysed", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
		}
	}
}

// What the evaluator admits keeps the level of its commands.
func TestWhatIsAdmittedKeepsTheLevelOfItsCommands(t *testing.T) {
	c := mustNew(t, "")
	for _, k := range []struct {
		line  string
		level Level
	}{
		{"ls; ls && ls || ls | cat", Green},
		{"! ls", Green},
		{"( cd /sdcard && ls )", Green},
		{"{ ls; ls; }", Green},
		{"time ls", Green},
		{"if ls; then ls; elif ls; then ls; else ls; fi", Green},
		{"while ls; do ls; break; done", Green},
		{"until ls; do ls; continue; done", Green},
		{"for w in a b; do echo $w; done", Green},
		{"w=1; echo $w ${w} \"$w\" ${#w} ${w#a} ${w##a} ${w%a} ${w%%a} ${w-x} ${w:-x}", Green},
		{"echo $HOME $? $#", Green},
		{"w=$(ls); echo \"$w\"", Green},
		{"w=`ls`; echo $w", Green},
		{"w=1; w=$((w+1)); echo $((w * 2 + 0x10 + 16#ff))", Green},
		{"echo $(( $# + $? ))", Green},
		{"for w in 1 2 3; do echo $((w)); done", Green},
		{"ls >/dev/null 2>&1 </dev/null", Green},
		{"cat <<EOF\n$HOME\nEOF", Green},
		{"cat <<<\"$HOME\"", Green},
		{"ls | while read -r w; do echo \"$w\"; done", Green},
		{"read -r w v </proc/version; echo $w", Green},
		{"LANG=C ls", Green},
		{"TZ=UTC date", Green},
		{"env LC_ALL=C ls", Green},
		{"set -e; set -u; set -x; set +x; set -o pipefail; ls", Green},
		{"ls; exit", Green},
		{"ls; exit 1", Green},
		{"ls; exit $?", Green},
		{"n=1; for w in 1 2; do break $n; done; for w in 1 2; do continue $((n+1)); done", Green},
		{"cd /sdcard; pwd", Green},
		{"command -v ls; type ls; whence ls", Green},
		{"echo ${w:-'x y'} ${w:-\"x y\"} ${w#'x'}", Green},
		{"cat <<'EOF'\n${w:-'x'}\nEOF", Green},
		{"sh -c ls; sh -ec ls; sh -e -c ls; sh -c -e ls", Green},
		{"su 0 ls /data; su root ls /data; su u0_a1 ls", Green},
		{`[ -n "$w" -a -f /sdcard/x ] && ls`, Green},
		{`w=1; [ "$w" -lt 3 ] && ls`, Green},
		{"ls; rm /sdcard/x", Red},
		{"w=/sdcard/x; rm $w", Red},
		{"for w in a b; do input tap 1 1; done", Yellow},
	} {
		r := c.Classify(sh(k.line))
		if r.Level != k.level || r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want %v not special", k.line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason, k.level)
		}
	}
}

// Every operator of test the evaluator admits has a differential case:
// the generator builds them from testUnary and testBinary.
func TestEveryTestOperatorHasACase(t *testing.T) {
	has := func(list []string, op string) bool {
		for _, v := range list {
			if v == op {
				return true
			}
		}
		return false
	}
	for op := range testUnaryOps {
		if !has(testUnary, op) {
			t.Errorf("the unary operator %s has no differential case: add it to testUnary and record the oracle", op)
		}
	}
	for op := range testBinaryOps {
		if !has(testBinary, op) {
			t.Errorf("the binary operator %s has no differential case: add it to testBinary and record the oracle", op)
		}
	}
	if len(testUnary) != len(testUnaryOps) || len(testBinary) != len(testBinaryOps) {
		t.Errorf("testUnary and testBinary list operators the evaluator does not admit")
	}
}
