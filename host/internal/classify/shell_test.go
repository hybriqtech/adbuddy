package classify

import "testing"

func TestShellReads(t *testing.T) {
	c := mustNew(t, "")
	var cases []tc
	for _, line := range []string{
		"getprop ro.product.model",
		"getprop",
		"dumpsys battery",
		"dumpsys package com.android.chrome | grep version",
		"pm list packages",
		"pm list packages -s -3",
		"pm path com.android.chrome",
		"cmd package list packages",
		"settings get global window_animation_scale",
		"settings --user 0 get secure android_id",
		"settings list system",
		"ls -la /sdcard/Download",
		"ls /sdcard/*.jpg",
		"cat /proc/meminfo",
		"df -h",
		"du -sh /sdcard/* | sort -h",
		"stat /sdcard/DCIM",
		"screencap -p",
		"uiautomator dump",
		"uiautomator dump --compressed",
		"logcat -d -t 100",
		"logcat -b crash -d",
		"content query --uri content://settings/global",
		"find /sdcard -name '*.apk' -size +10M",
		"sed -n '1,10p' /proc/cpuinfo",
		"sed 's/a/b/g' /proc/version",
		"sed -e 's#/#_#g;1d' /proc/version",
		"awk '{print $1}' /proc/loadavg",
		"dumpsys meminfo | awk -F: '/Total RAM/ {print $2}'",
		"date",
		"date +%s",
		"wm size",
		"wm density",
		"ip addr show wlan0",
		"ifconfig wlan0",
		"ps -A | grep chrome | head -5",
		"echo hello",
		"echo $PATH",
		"id; whoami; uname -a",
		"for p in $(pm list packages -3); do echo $p; done",
		"if [ -e /sdcard/x ]; then echo yes; else echo no; fi",
		"cat /sdcard/x 2>/dev/null || echo missing",
		"ls >/dev/null 2>&1",
		"getprop ro.build.version.sdk && getprop ro.product.manufacturer",
		"toybox ls /sdcard",
		"/system/bin/ls /sdcard",
		"appops get com.whatsapp RUN_IN_BACKGROUND",
		"exec 2>/dev/null",
		"timeout 5 logcat -d",
		"nohup ls",
		"env LANG=C ls",
		"command -v pm",
		"sh -c 'getprop ro.product.model'",
		"sh -c \"pm list packages | grep google\"",
		"su -c 'ls /data'",
		"su 0 ls /data",
		"cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.HOME",
		"cat <<EOF\nhello\nEOF",
		"echo 'rm -rf /sdcard' | cat",
		"X=1 ls",
		"# only a comment",
	} {
		cases = append(cases, tc{argv: sh(line), level: Green})
	}
	check(t, c, cases)
}

// TestEvasions is the corpus of commands built to slip past the classifier
// (spec §1 success criterion 2, §10). Every one of them must be 🔴.
func TestEvasions(t *testing.T) {
	c := mustNew(t, "")
	lines := []string{
		// separators and compound commands
		"ls; rm -rf /sdcard",
		"ls && rm -rf /sdcard",
		"ls || rm -rf /sdcard",
		"ls | rm -rf /sdcard",
		"ls & rm -rf /sdcard",
		"(rm -rf /sdcard)",
		"{ rm -rf /sdcard; }",
		"! rm -rf /sdcard",
		"ls\nrm -rf /sdcard",
		"if true; then rm -rf /sdcard; fi",
		"while true; do reboot; done",
		"for f in a; do pm clear com.x; done",
		"case x in x) rm /sdcard/a;; esac",
		// substitutions
		"echo $(rm -rf /sdcard)",
		"echo `rm -rf /sdcard`",
		"ls \"$(reboot)\"",
		"X=$(rm -rf /sdcard) ls",
		"cat <<EOF\n$(reboot)\nEOF",
		"echo ${ reboot;}",
		"[[ -e $(reboot) ]]",
		"(( $(reboot) ))",
		// nested shells
		"sh -c 'rm -rf /sdcard'",
		"sh -c \"sh -c 'reboot'\"",
		"sh -ec 'rm -rf /sdcard'",
		"bash -c 'rm /sdcard/x'",
		"/system/bin/sh -c 'rm /sdcard/x'",
		"su -c 'rm -rf /data'",
		"su 0 rm -rf /data",
		"su root sh -c 'reboot'",
		"toybox rm -rf /sdcard",
		"busybox rm /sdcard/x",
		"nohup rm -rf /sdcard",
		"timeout 5 rm -rf /sdcard",
		"env rm -rf /sdcard",
		"exec rm -rf /sdcard",
		"nice -n 5 rm /sdcard/x",
		"command rm /sdcard/x",
		"run-as com.x rm -rf files",
		"xargs rm < /sdcard/list",
		"echo /sdcard/x | xargs rm",
		"cmd package clear com.x",
		"sh -c \"$X\"",
		"sh",
		"sh /sdcard/script.sh",
		"sh < /sdcard/script.sh",
		"echo reboot | sh",
		"sh -s",
		". /sdcard/script.sh",
		"source /sdcard/script.sh",
		"eval 'rm -rf /sdcard'",
		// dynamic command names and arguments
		"$CMD /sdcard",
		"${X:-rm} -rf /sdcard",
		"X=rm; $X -rf /sdcard",
		"r\\m -rf /sdcard", // backslash-quoted name is still rm
		"'rm' -rf /sdcard",
		"\"rm\" -rf /sdcard",
		"r''m -rf /sdcard",
		"pm clear $(echo com.whatsapp)",
		"pm disable-user $PKG",
		"settings put global $K 1",
		"pm disable-user com.*",
		"mv /sdcard/* /sdcard/x/",
		// environment tricks
		"PATH=/data/local/tmp ls",
		"export PATH=/data/local/tmp; ls",
		"LD_PRELOAD=/data/local/tmp/x.so ls",
		"IFS=x; ls",
		"env PATH=/data/local/tmp ls",
		"alias ls='rm -rf /sdcard'; ls",
		"ls() { rm -rf /sdcard; }; ls",
		"function ls { reboot; }",
		"unset PATH",
		"trap 'reboot' EXIT",
		// executing files
		"./evil",
		"/data/local/tmp/evil",
		"/sdcard/x.sh",
		"../bin/rm /sdcard/x",
		// redirections
		"echo x > /sdcard/important.txt",
		"echo x >> /sdcard/important.txt",
		"cat /proc/version >| /sdcard/x",
		"ls &> /sdcard/x",
		"ls 1<> /sdcard/x",
		"ls >& /sdcard/x",
		"getprop > $(echo /sdcard/x)",
		"exec 3> /sdcard/x",
		// syntax the parser rejects
		"ls ((",
		"echo 'unterminated",
		// adb shell options that read stdin
		"",
		"   ",
	}
	for _, line := range lines {
		r := c.Classify(sh(line))
		if r.Level != Red {
			t.Errorf("EVASION %q classified %v (primary %s)", line, r.Level, r.Primary.Rule)
		}
	}
	// interactive shells and exec-out
	for _, argv := range [][]string{
		{"shell"},
		{"shell", "-t"},
		{"shell", "-e", "~"},
		{"shell", "--frob", "ls"},
		{"exec-out", "rm -rf /sdcard"},
		{"exec-out", "sh", "-c", "reboot"},
		{"shell", "rm", "-rf", "/sdcard"},
		{"shell", "ls", ";", "reboot"},
		{"shell", "ls;reboot"},
		{"shell", "echo", "a", "&&", "pm", "clear", "com.x"},
	} {
		if r := c.Classify(argv); r.Level != Red {
			t.Errorf("EVASION %q classified %v", argv, r.Level)
		}
	}
}

func TestExecOutReads(t *testing.T) {
	c := mustNew(t, "")
	if r := c.Classify([]string{"exec-out", "screencap", "-p"}); r.Level != Green {
		t.Fatalf("exec-out screencap: %v %s", r.Level, r.Primary.Reason)
	}
	if r := c.Classify([]string{"shell", "-n", "-T", "getprop"}); r.Level != Green {
		t.Fatalf("shell -n -T getprop: %v", r.Level)
	}
}
