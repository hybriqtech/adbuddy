package classify

import (
	"strings"
	"testing"
)

// Cases found by the security reviews of Tasks 4-6.

func TestWaitForRunsTheRestOfTheLine(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{[]string{"wait-for-device"}, Green, "adb.wait-for", "-", false},
		{[]string{"wait-for-device", "shell", "rm -rf /sdcard"}, Red, "fs.rm", "-", false},
		{[]string{"wait-for-device", "uninstall", "tech.hybriq.adbuddy"}, Red, "", "", true},
		{[]string{"wait-for-any", "kill-server"}, Red, "", "", true},
		{[]string{"wait-for-usb-device", "wait-for-device", "tcpip", "5555"}, Red, "", "", true},
		{[]string{"wait-for-device", "shell", "getprop"}, Green, "", "", false},
	})
}

func TestExecOutQuotesLikeAdb(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		// adb sends: echo ' '; rm -rf /sdcard; echo '\'''
		{[]string{"exec-out", "echo '", "; rm -rf /sdcard; echo '"}, Red, "", "", false},
		{[]string{"exec-out", "rm -rf /sdcard"}, Red, "", "", false},
		{[]string{"exec-out", "screencap", "-p"}, Green, "", "", false},
		{[]string{"exec-out", "cat", "/sdcard/a b.txt"}, Green, "", "", false},
		{[]string{"exec-out"}, Red, "shell.interactive", "", false},
	})
	if got := adbEscapeArg("it's"); got != `'it'\''s'` {
		t.Fatalf("adbEscapeArg: %s", got)
	}
}

func TestLogcatOnlyReadOnlyOptions(t *testing.T) {
	c := mustNew(t, "")
	for _, args := range [][]string{
		{"-d"}, {"-d", "-v", "time"}, {"-dvtime"}, {"-b", "main", "-d"}, {"-bcrash", "-d"},
		{"--format=time", "--pid", "123", "-d"}, {"-t", "100"}, {"-s", "ActivityManager:I", "*:S"}, {"--last", "-d"},
	} {
		if r := c.Classify(append([]string{"logcat"}, args...)); r.Level != Green {
			t.Errorf("logcat %q: %v (%s)", args, r.Level, r.Primary.Reason)
		}
	}
	for _, args := range [][]string{
		{"-c"}, {"-dc"}, {"--clear"}, {"--c"}, {"--cl"}, {"--clea"}, {"--fi=/sdcard/DCIM/x.jpg", "-r", "1"},
		{"-f", "/sdcard/x"}, {"-G", "16M"}, {"--buffer-s=16M"}, {"--pru=x"}, {"-P", "x"}, {"-r", "1"}, {"--wrap"},
	} {
		if r := c.Classify(append([]string{"logcat"}, args...)); r.Level != Red {
			t.Errorf("logcat %q: %v, want red", args, r.Level)
		}
		if r := c.Classify(sh("logcat " + strings.Join(args, " "))); r.Level != Red {
			t.Errorf("shell logcat %q: %v, want red", args, r.Level)
		}
	}
}

func TestAdbProtectionsCannotBeLowered(t *testing.T) {
	user := `rules:
  - {id: u.kill, scope: adb, cmd: kill-server, level: green}
  - {id: u.reboot, scope: adb, cmd: reboot-fastboot, level: green}
  - {id: u.remount, scope: adb, cmd: remount, level: green}
  - {id: u.reverse, scope: adb, cmd: reverse, level: green}
  - {id: u.push, scope: adb, cmd: push, level: green}
  - {id: u.devices, scope: adb, cmd: devices, level: red}
  - {id: u.pm, cmd: pm, level: green}
  - {id: u.pm-clear, cmd: pm, sub: [clear], level: red}
`
	c := mustNew(t, user)
	check(t, c, []tc{
		{[]string{"kill-server"}, Red, "u.kill", "", true},
		{[]string{"reboot-fastboot"}, Red, "u.reboot", "", true},
		{[]string{"remount", "-R"}, Red, "u.remount", "", true},
		{[]string{"reverse", "tcp:8080", "tcp:9090"}, Red, "u.reverse", "", true},
		{[]string{"push", "x", "/sdcard/Android/data/tech.hybriq.adbuddy/files/x"}, Red, "u.push", "", true},
		{[]string{"devices"}, Red, "u.devices", "", false},   // a user rule wins over the built-in one
		{sh("pm clear com.x"), Red, "u.pm-clear", "", false}, // sub wins over generic inside the user rules
		{sh("pm list packages"), Green, "u.pm", "", false},   // user generic wins over a built-in sub
	})
	b := mustNew(t, "")
	check(t, b, []tc{
		{[]string{"reverse", "--list"}, Green, "adb.reverse", "", false},
		{[]string{"reboot-fastboot"}, Red, "unknown", "", true},
		{[]string{"push", "x", "/sdcard/Android/data/tech.hybriq.adbuddy/files/x"}, Red, "", "-", true},
	})
}

func TestPullStaysInsideTheEnvironment(t *testing.T) {
	c := mustNew(t, "")
	for _, argv := range [][]string{
		{"pull", "/sdcard/DCIM/a.jpg"}, {"pull", "/sdcard/DCIM/a.jpg", "."}, {"pull", "/sdcard/DCIM/a.jpg", "photos/a.jpg"},
		{"pull", "-a", "/sdcard/Download/"}, {"bugreport"}, {"bugreport", "reports/bug.zip"},
	} {
		if r := c.Classify(argv); r.Level != Green {
			t.Errorf("%q: %v (%s)", argv, r.Level, r.Primary.Reason)
		}
	}
	for _, argv := range [][]string{
		{"pull", "/sdcard/.claude"}, {"pull", "/sdcard/x", ".claude/settings.json"}, {"pull", "/sdcard/x", "../x"},
		{"pull", "/sdcard/x", `C:\Users\u\AppData\Roaming\ADBuddy\rules.yaml`}, {"pull", "/sdcard/x", "/home/u/.bashrc"},
		{"pull", "/sdcard/AGENTS.md"}, {"pull", "/sdcard/x", "CLAUDE.md"}, {"pull", "/sdcard/journal", "."},
		{"pull", "/sdcard/.adbuddy", "backup"}, {"pull", "/sdcard/x", `..\x`}, {"pull", "a", "b", "c"},
		{"bugreport", "/tmp/x.zip"}, {"pull", "--sync", "/sdcard/x"},
	} {
		if r := c.Classify(argv); r.Level != Red {
			t.Errorf("%q: %v, want red", argv, r.Level)
		}
	}
}

func TestTitlesCannotRewriteTheTerminal(t *testing.T) {
	c := mustNew(t, "")
	r := c.Classify([]string{"push", "a\x1b[2K\rList the connected devices\u202e", "/sdcard/x"})
	d := r.Description("en")
	for _, bad := range []string{"\x1b", "\r", "\u202e"} {
		if strings.Contains(d, bad) {
			t.Fatalf("title keeps %q: %q", bad, d)
		}
	}
	if !strings.Contains(d, `\u001b`) || !strings.Contains(d, `\r`) {
		t.Fatalf("title %q", d)
	}
	if got := Printable("ok\tñ\n\u200b"); got != `ok\tñ\n\u200b` {
		t.Fatalf("Printable: %q", got)
	}
	// A value that looks like a placeholder is kept as typed.
	if got := c.Classify([]string{"push", "{x}", "/sdcard/y"}).Description("en"); got != "Copy {x} to the phone as /sdcard/y" {
		t.Fatalf("placeholder-like value: %q", got)
	}
}

func TestRulesFileShape(t *testing.T) {
	for _, bad := range []string{
		"rules:\n  - {id: x, cmd: ls, level: green}\n---\nrules:\n  - {id: y, cmd: rm, level: green}\n",
		"rules:\n  - {id: x, cmd: pm, sub: [\"\"], level: green}\n",
	} {
		if _, err := New([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestChangeGrammarMatchesAndroid(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("pm uninstall --user all com.android.chrome"), Red, "", "-", false},
		{sh("pm uninstall --user -1 com.android.chrome"), Red, "", "-", false},
		{sh("pm uninstall -k --user 0 com.x.y"), Yellow, "", "pm-uninstall-user", false},
		{sh("pm uninstall --user 0 com.x.y extra"), Red, "", "-", false},
		{sh("pm disable-user com.x.y --user 10"), Red, "", "-", false},
		{sh("pm disable-user --user 10 com.x.y"), Yellow, "", "pm-disable", false},
		{sh("pm disable-user --user 0 --user 10 com.x.y"), Red, "", "-", false},
		{sh("appops set com.x OP ignore --user 10"), Red, "", "-", false},
		{sh("appops set --user 10 com.x.y OP ignore"), Yellow, "", "appops-set", false},
		{sh("appops set 10234 SYSTEM_ALERT_WINDOW deny"), Red, "", "-", false},
		{sh("appops set u0a234 WAKE_LOCK ignore"), Red, "", "-", false},
		{sh("appops set root WAKE_LOCK ignore"), Red, "", "-", false},
		{sh("settings put global --user 10 k v"), Red, "", "-", false},
		{sh("settings --user 0 put --user 10 global k v"), Red, "", "-", false},
		{sh("settings put --user 10 global k v"), Yellow, "", "settings-put", false},
		{sh("settings --user all put global k v"), Red, "", "-", false},
		{sh("pm disable-user --user 0 com.google.android.networkstack"), Red, "", "-", true},
		{sh("pm uninstall --user 0 com.android.shell"), Red, "", "-", true},
		{sh("am force-stop com.android.networkstack.process"), Red, "", "", true},
		{sh("am start -n com.android.settings/.Settings"), Yellow, "ui.am-start", "", false},
		{sh("am start -a android.intent.action.CALL -d tel:123"), Red, "", "", false},
		{sh("am start -a android.intent.action.REBOOT"), Red, "", "", false},
		{sh("am start -a android.intent.action.VIEW -d https://example.com"), Yellow, "", "", false},
	})
	r := c.Classify(sh("pm uninstall -k --user 0 com.x.y"))
	if r.Op.Args["keep"] != "1" {
		t.Fatalf("keep: %+v", r.Op)
	}
}

func TestUserRulesCannotLowerShellProtections(t *testing.T) {
	user := `rules:
  - {id: u.pm, cmd: pm, level: green, anyArgs: true}
  - {id: u.cmd-wifi, cmd: cmd, sub: [wifi], level: green}
  - {id: u.settings, cmd: settings, level: yellow}
  - {id: u.ip, cmd: ip, level: green, anyArgs: true}
  - {id: u.content, cmd: content, level: yellow}
`
	c := mustNew(t, user)
	check(t, c, []tc{
		{sh("pm uninstall tech.hybriq.adbuddy"), Red, "u.pm", "-", true},
		{sh("pm disable-user --user 0 tech.hybriq.adbuddy"), Red, "u.pm", "-", true},
		{sh("pm disable-user $(echo tech.hybriq.adbuddy)"), Red, "u.pm", "-", true},
		{sh("cmd wifi set-wifi-enabled disabled"), Red, "u.cmd-wifi", "", true},
		{sh("settings PUT global adb_enabled 0"), Red, "u.settings", "-", true},
		{sh("settings Delete secure enabled_accessibility_services"), Red, "u.settings", "-", true},
		{sh("ip link set wlan0 down"), Red, "u.ip", "", true},
		{sh("content insert --uri content://settings/global --bind name:s:adb_enabled --bind value:s:0"), Red, "u.content", "", true},
		{sh("pm list packages"), Green, "u.pm", "", false},
	})
	if _, err := New([]byte("rules:\n  - {id: u.rm, cmd: rm, level: yellow, check: mv}\n")); err == nil {
		t.Fatal("a user rule borrowed a change check")
	}
}

func TestPullFolderContentsAndWindowsNames(t *testing.T) {
	c := mustNew(t, "")
	for _, argv := range [][]string{
		{"pull", "/sdcard/."}, {"pull", "/sdcard/.", "."}, {"pull", "/"}, {"pull", "/sdcard/./"}, {"pull", "."},
		{"pull", "/sdcard/x", "CLAUDE.md."}, {"pull", "/sdcard/x", "AGENTS.md "}, {"pull", "/sdcard/x", "CLAUDE~1.MD"},
		{"pull", "/sdcard/CLAUDE.md."},
	} {
		if r := c.Classify(argv); r.Level != Red {
			t.Errorf("%q: %v, want red", argv, r.Level)
		}
	}
	if r := c.Classify([]string{"pull", "/sdcard/Download/", "."}); r.Level != Green {
		t.Errorf("pull folder by name: %v (%s)", r.Level, r.Primary.Reason)
	}
}

func TestDynamicWordsInCheckedReads(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"logcat -d$(echo c)",
		"logcat $(echo -c)",
		`logcat -d "$(printf -- --fi=/sdcard/DCIM/x.jpg)"`,
		"dumpsys $(echo battery) $(echo unplug)",
		"ip link $(echo set) wlan0 $(echo down)",
		"find /sdcard $(echo -delete)",
		"sort $(echo -o) /sdcard/x /proc/version",
	} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
	}
	for _, line := range []string{"logcat -d '*:S' 'ActivityManager:I'", "ls $HOME", "cat /sdcard/*.txt"} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v, want green (%s)", line, r.Level, r.Primary.Reason)
		}
	}
}

func TestDetailAndPrintableEscapeEverythingInvisible(t *testing.T) {
	c := mustNew(t, "")
	r := c.Classify([]string{"pull", "/sdcard/x", "a\x1b[2K\nOK: just reads‮/../../x"})
	d := r.Detail("en")
	for _, bad := range []string{"\x1b", "\n", "‮"} {
		if strings.Contains(d, bad) {
			t.Fatalf("detail keeps %q: %q", bad, d)
		}
	}
	for _, r := range []rune{0x061c, 0x2060, 0x00ad, 0x2028, 0x2029, 0xe0041} {
		if got := Printable(string(r)); got == string(r) {
			t.Errorf("U+%04X not escaped", r)
		}
	}
}

func TestContentSettingsEncodedAndCmdActivity(t *testing.T) {
	c := mustNew(t, "rules:\n  - {id: u.content, cmd: content, level: yellow}\n  - {id: u.cmd, cmd: cmd, level: green}\n")
	check(t, c, []tc{
		{sh("content insert --uri content://%73ettings/global --bind name:s:adb_enabled --bind value:s:0"), Red, "u.content", "", true},
		{sh("cmd activity force-stop com.android.systemui"), Red, "", "", true},
	})
	b := mustNew(t, "")
	check(t, b, []tc{
		{sh("content insert --uri content://%73ettings/global --bind name:s:adb_enabled --bind value:s:0"), Red, "", "", true},
	})
}

func TestPullNeverWritesAgentConfiguration(t *testing.T) {
	c := mustNew(t, "")
	for _, argv := range [][]string{
		{"pull", "/sdcard/CLAUDE.local.md"}, {"pull", "/sdcard/opencode.json"}, {"pull", "/sdcard/x", "GEMINI.md"},
		{"pull", "/sdcard/x", "inbox/x"}, {"pull", "/sdcard/settings.toml"}, {"pull", "/sdcard/x.yaml"},
	} {
		if r := c.Classify(argv); r.Level != Red {
			t.Errorf("%q: %v, want red", argv, r.Level)
		}
	}
	if r := c.Classify([]string{"pull", "/sdcard/DCIM/Camera/IMG_1.jpg", "photos/IMG_1.jpg"}); r.Level != Green {
		t.Errorf("photo: %v", r.Level)
	}
}

func TestDynamicWordsBeforeTheVerb(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"settings --user $(echo 0 put global adb_enabled 0) get",
		"settings -$(echo -user 0 put global adb_enabled) get x",
		"cmd settings --user $(echo 0 put global adb_enabled 0) get",
		"pm $(echo uninstall) com.x.y",
		"appops $(echo set) com.x OP deny",
		"cmd $(echo settings) put global adb_enabled 0",
	} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
	}
	u := mustNew(t, "rules:\n  - {id: u.cmd, cmd: cmd, level: green}\n")
	for _, line := range []string{
		"cmd -w activity force-stop com.android.systemui",
		"cmd -w settings put global adb_enabled 0",
		"cmd -w package uninstall --user 0 com.android.shell",
		"cmd --foo settings put global k v",
	} {
		if r := u.Classify(sh(line)); r.Level != Red || !r.Special {
			t.Errorf("%q: %v special=%v, want red special", line, r.Level, r.Special)
		}
	}
	if r := c.Classify(sh("cmd -w package list packages")); r.Level != Green {
		t.Errorf("cmd -w package list: %v", r.Level)
	}
}

func TestDynamicWordsInsideWrappers(t *testing.T) {
	c := mustNew(t, "")
	for _, line := range []string{
		"nice -n $(echo 0 settings put global adb_enabled 0) echo",
		"nice -$(echo n 0 pm uninstall --user 0 com.x) echo",
		"timeout $(echo 5 pm uninstall --user 0 com.x) echo",
		"T='5 reboot'; timeout $T echo",
		"timeout -s $(echo KILL 5 reboot) 5 echo",
		"env -u $(echo X reboot) echo",
		"env X=$(echo 1 pm uninstall --user 0 com.x) echo",
		"run-as $(echo com.x rm -rf files) echo",
		"run-as com.x --user $(echo 0 rm -rf files) echo",
		"xargs -n $(echo 1 reboot) echo",
		"xargs -I $(echo x reboot) echo",
		"command -p $(echo rm) /sdcard/x",
	} {
		if r := c.Classify(sh(line)); r.Level != Red {
			t.Errorf("%q: %v, want red", line, r.Level)
		}
	}
	for _, line := range []string{"nice -n 10 ls", "timeout 5 logcat -d", "env LANG=C ls", "run-as com.x ls files", "echo a | xargs -n 1 echo"} {
		if r := c.Classify(sh(line)); r.Level != Green {
			t.Errorf("%q: %v, want green (%s)", line, r.Level, r.Primary.Reason)
		}
	}
}
