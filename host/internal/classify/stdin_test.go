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
