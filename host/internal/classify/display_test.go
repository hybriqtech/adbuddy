package classify

import "testing"

func TestDisplayCommandShowsWhatThePhoneRuns(t *testing.T) {
	cases := map[string][]string{
		"adb shell echo hi; reboot":                         {"shell", "echo hi; reboot"},
		"adb exec-out screencap -p; reboot":                 {"exec-out", "screencap -p; reboot"},
		"adb exec-out sh -c 'rm -rf /sdcard/DCIM'":          {"exec-out", "sh", "-c", "rm -rf /sdcard/DCIM"},
		"adb wait-for-device shell echo hi; reboot":         {"wait-for-device", "shell", "echo hi; reboot"},
		"adb exec-in cat > /sdcard/x; reboot":               {"exec-in", "cat > /sdcard/x; reboot"},
		"adb exec-in cp /dev/stdin '/sdcard/a b'":           {"exec-in", "cp", "/dev/stdin", "/sdcard/a b"},
		"adb wait-for-device exec-out screencap -p; reboot": {"wait-for-device", "exec-out", "screencap -p; reboot"},
		"adb push 'my notes.txt' /sdcard/x":                 {"push", "my notes.txt", "/sdcard/x"},
		"adb -s dev1 shell ls":                              {"-s", "dev1", "shell", "ls"},
		"adb devices":                                       {"devices"},
		"adb shell echo \\u001b[2K":                         {"shell", "echo \x1b[2K"},
	}
	for want, argv := range cases {
		if got := DisplayCommand(argv); got != want {
			t.Errorf("%q: got %q want %q", argv, got, want)
		}
	}
}
