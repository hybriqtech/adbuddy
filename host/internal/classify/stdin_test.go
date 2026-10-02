package classify

import (
	"strings"
	"testing"
)

// A4-1: what runs through an interactive shell (adb shell or exec-out without
// a command) or adb exec-in comes from the agent's stdin, which the daemon
// forwards to adb and nobody sees. Allowing one such command for the session
// would allow whatever comes through stdin next: it is special, so it warns
// and never offers "allow during the session".
func TestStdinShellsAreSpecial(t *testing.T) {
	c := mustNew(t, "")
	for _, argv := range [][]string{
		{"shell"},
		{"exec-out"},
		{"shell", "-t"},
		{"shell", "-tt"},
		{"shell", "-T"},
		{"shell", "-x"},
		{"shell", "-n"},
		{"shell", "-e", "~"},
		{"shell", "-e"},
		{"shell", " "},
		{"shell", "-t", ""},
		{"exec-out", " "},
		{"wait-for-device", "shell"},
		// Options adb reads that ADBuddy does not (getopt: -nt, --): what
		// runs is not known, it may be an interactive shell.
		{"shell", "-nt"},
		{"shell", "--"},
		{"shell", "--frob", "ls"},
		// exec-in pipes stdin into the command.
		{"exec-in", "sh"},
		{"exec-in", "rm"},
		{"exec-in", "cp", "/dev/stdin", "/sdcard/x"},
		// A shell or su on the phone reading the forwarded stdin.
		{"exec-out", "sh"},
		{"shell", "-t", "sh"},
		sh("sh"),
		sh("sh -s"),
		sh("su"),
		sh("sh /dev/stdin"),
		sh(". /dev/stdin"),
		sh(`eval "$(cat)"`),
		sh("xargs -I{} {}"),
	} {
		r := c.Classify(argv)
		if r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v (%s: %s), want red and special", argv, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason.EN)
		}
	}
	// With the global options in front too (adb -s X shell).
	g, args := SplitGlobal([]string{"-s", "X", "shell"})
	if r := c.Classify(args); g.Serial != "X" || r.Level != Red || !r.Special {
		t.Errorf("-s X shell: %v special=%v", r.Level, r.Special)
	}
	// The reason says why, in both languages.
	r := c.Classify([]string{"shell"})
	if !strings.Contains(r.Primary.Reason.EN, "stdin") || !strings.Contains(r.Primary.Reason.ES, "entrada estándar") {
		t.Errorf("reason %+v", r.Primary.Reason)
	}
}

// A user rule cannot lower exec-in: it is decided before the rules.
func TestExecInCannotBeLowered(t *testing.T) {
	c := mustNew(t, "rules:\n  - {id: u.execin, scope: adb, cmd: exec-in, level: green}\n")
	if r := c.Classify([]string{"exec-in", "sh"}); r.Level != Red || !r.Special {
		t.Errorf("exec-in sh with a user rule: %v special=%v (%s)", r.Level, r.Special, r.Primary.Rule)
	}
}

// StdinDecides is the second barrier, independent of the rules: the daemon
// gives no "allow during the session" to a command whose effect may come from
// its stdin, whatever the classifier said.
func TestStdinDecides(t *testing.T) {
	yes := [][]string{
		{"shell"}, {"exec-out"}, {"shell", "-t"}, {"shell", "-n"}, {"shell", "-e", "~"}, {"shell", " "}, {"exec-out", " "},
		{"wait-for-device", "shell"}, {"shell", "--"}, {"shell", "-nt"}, {"shell", "--frob", "ls"},
		{"exec-in", "rm"}, {"exec-in", "cat > /sdcard/x"},
		{"exec-out", "sh"}, {"shell", "-t", "sh"}, sh("sh -s"), sh("/system/bin/sh"), sh("su"), sh("s'h'"), sh(`eval "$(cat)"`),
		sh(". /dev/stdin"), sh("source /proc/self/fd/0"),
		// Arguments read from stdin.
		sh("xargs rm -rf"), sh("rm -rf $(cat)"), sh("rm -rf `head -1`"), sh(`while read f; do rm "$f"; done`),
		sh("pm uninstall $(cat)"), {"shell", "xargs", "pm", "uninstall"},
		// Contents read from stdin.
		sh("pm install -S 1234"), sh("cmd package install -S 1234"), sh("content write --uri content://x"),
		sh("cat > /sdcard/x"), sh("dd of=/sdcard/x"), sh("tee /sdcard/x"), sh("cp /dev/stdin /sdcard/x"), sh("cp - /sdcard/x"),
		{"push", "-", "/sdcard/x"}, {"install", "-"}, {"push", "/dev/stdin", "/sdcard/x"},
	}
	no := [][]string{
		nil, {"reboot"}, {"devices"}, {"push", "a.txt", "/sdcard/a.txt"}, {"install", "app.apk"}, {"uninstall", "com.x"},
		sh("rm -rf /sdcard/DCIM"), sh("reboot"), {"shell", "pm", "clear", "com.x"}, sh("settings put global x 1"),
		{"exec-out", "screencap", "-p"}, sh("svc wifi disable"), sh("pm uninstall --user 0 com.x"), {"shell", "-t", "reboot"},
		sh("am force-stop com.x"), sh("input tap 1 2"), sh("echo $HOME"),
	}
	for _, argv := range yes {
		if !StdinDecides(argv) {
			t.Errorf("%q: StdinDecides false, want true", argv)
		}
	}
	for _, argv := range no {
		if StdinDecides(argv) {
			t.Errorf("%q: StdinDecides true, want false", argv)
		}
	}
}

// H1: StdinDecides is an allowlist. A program on the phone keeps the key only
// if it is known not to read stdin, or to read it only as data it filters to
// its output; anything else may be an interpreter of what comes through stdin
// (awk's system(), getline and print | cmd; sed's e command…), whatever its
// name.
func TestStdinDecidesIsAnAllowlist(t *testing.T) {
	yes := [][]string{
		// awk and sed run what comes through stdin.
		sh(`awk '{system($0)}'`), sh(`awk '{while ((getline c) > 0) system(c)}'`), sh(`awk '{print | $0}'`),
		sh("awk 1"), sh("sed e"), sh("sed -n p"), sh(`ls | awk '{system($0)}'`), sh("rm -rf /sdcard/x; awk 1"),
		sh("rm -rf /sdcard/x && sed e"), sh("FOO=1 awk 1"), sh("if true; then awk 1; fi"), sh("! awk 1"), sh("(awk 1)"),
		sh("{ awk 1; }"), sh("while true; do awk 1; done"), sh("rm x\nawk 1"), sh("rm x & awk 1"),
		// Programs ADBuddy does not know, wrappers that run another program,
		// a program that is not the system's, a program known only at run time.
		sh("frob"), sh("env rm -rf /sdcard/x"), sh("nice rm -rf /sdcard/x"), sh("find /sdcard -delete"),
		sh("/data/local/tmp/rm -rf /sdcard/x"), sh("./rm x"), sh("$X"), sh("${X} rm"), sh("cmd package list packages"),
		sh("for f in a b; do rm $f; done"), sh("f() { rm x; }; f"),
		// A filter that reads stdin writes it to a file: other contents next time.
		sh("head -c 10 > /sdcard/x"), sh("grep x > /sdcard/y"), sh("sort -o /sdcard/x"), sh("tr a b >> /sdcard/x"),
		sh("rm -rf /sdcard/x; cat > /sdcard/y"),
		// adb commands ADBuddy does not know.
		{"frob"}, {"emu", "kill"}, {"pair", "1.2.3.4:5"},
	}
	no := [][]string{
		// Filters that only read stdin as data and write to their output.
		sh("rm -rf /sdcard/DCIM | wc -l"), sh("ls /sdcard | head -5"), sh("logcat -d | grep x | tail -3"),
		sh("pm list packages | sort | cut -d: -f2"), sh("dumpsys battery | tr a-z A-Z"), sh("cat /proc/cpuinfo | grep x"),
		sh("cat"), sh("rm -rf /sdcard/x | cat"),
		// Redirections that are not a file written from stdin.
		sh("rm x 2>/dev/null"), sh("ls >&2"), sh("rm x 2>&1 | head"), sh("ls > /sdcard/list.txt"),
		// Assignments, keywords and the system's paths.
		sh("foo=1 rm x"), sh("if true; then rm x; fi"), sh("/system/bin/rm -rf /sdcard/x"), sh("rm x && reboot || svc wifi disable"),
		sh("settings put global x 1; am force-stop com.x; input keyevent 3"),
		{"push", "a.txt", "/sdcard/a.txt"}, {"sync"}, {"root"}, {"tcpip", "5555"},
	}
	for _, argv := range yes {
		if !StdinDecides(argv) {
			t.Errorf("%q: StdinDecides false, want true", argv)
		}
	}
	for _, argv := range no {
		if StdinDecides(argv) {
			t.Errorf("%q: StdinDecides true, want false", argv)
		}
	}
}
