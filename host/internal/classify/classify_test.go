package classify

import (
	"strings"
	"testing"
)

func mustNew(t *testing.T, user string) *Classifier {
	t.Helper()
	c, err := New([]byte(user))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLevelText(t *testing.T) {
	for _, l := range []Level{Green, Yellow, Red} {
		b, _ := l.MarshalText()
		var back Level
		if err := back.UnmarshalText(b); err != nil || back != l {
			t.Fatalf("%v: %s %v", l, b, err)
		}
	}
	if _, err := ParseLevel("orange"); err == nil {
		t.Fatal("orange accepted")
	}
}

func TestSplitGlobal(t *testing.T) {
	cases := []struct {
		argv      []string
		serial    string
		forbidden string
		rest      string
	}{
		{[]string{"devices"}, "", "", "devices"},
		{[]string{"-s", "abc", "shell", "ls"}, "abc", "", "shell ls"},
		{[]string{"-sabc", "shell"}, "abc", "", "shell"},
		{[]string{"-P", "5037", "devices"}, "", "-P", "devices"},
		{[]string{"-H", "192.0.2.1", "devices"}, "", "-H", "devices"},
		{[]string{"-d", "shell"}, "", "-d", "shell"},
		{[]string{"-a", "devices"}, "", "-a", "devices"},
		{[]string{"-t", "3", "shell"}, "", "-t", "shell"},
		{[]string{"--exit-on-write-error", "shell"}, "", "", "shell"},
	}
	for _, c := range cases {
		g, rest := SplitGlobal(c.argv)
		if g.Serial != c.serial || strings.Join(g.Forbidden, " ") != c.forbidden || strings.Join(rest, " ") != c.rest {
			t.Errorf("SplitGlobal(%q) = %+v %q", c.argv, g, rest)
		}
	}
}

type tc struct {
	argv    []string
	level   Level
	rule    string // optional: expected primary rule
	op      string // optional: expected op kind ("-" = none)
	special bool
}

func sh(line string) []string { return []string{"shell", line} }

func check(t *testing.T, c *Classifier, cases []tc) {
	t.Helper()
	for _, k := range cases {
		r := c.Classify(k.argv)
		if r.Level != k.level {
			t.Errorf("%q: level %v, want %v (primary %s: %s)", k.argv, r.Level, k.level, r.Primary.Rule, r.Primary.Reason)
			continue
		}
		if k.rule != "" && r.Primary.Rule != k.rule {
			t.Errorf("%q: rule %s, want %s", k.argv, r.Primary.Rule, k.rule)
		}
		switch {
		case k.op == "-" && r.Op != nil:
			t.Errorf("%q: unexpected op %s", k.argv, r.Op.Kind)
		case k.op != "" && k.op != "-" && (r.Op == nil || r.Op.Kind != k.op):
			t.Errorf("%q: op %v, want %s", k.argv, r.Op, k.op)
		}
		if k.special && !r.Special {
			t.Errorf("%q: want special", k.argv)
		}
	}
}

func TestAdbCommands(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{[]string{"devices", "-l"}, Green, "adb.devices", "-", false},
		{[]string{"version"}, Green, "", "", false},
		{[]string{"wait-for-device"}, Green, "adb.wait-for", "", false},
		{[]string{"mdns", "services"}, Green, "adb.mdns", "", false},
		{[]string{"mdns", "whatever"}, Red, "", "", false},
		{[]string{"pull", "/sdcard/DCIM/a.jpg", "."}, Green, "adb.pull", "", false},
		{[]string{"logcat", "-d"}, Green, "", "", false},
		{[]string{"logcat", "-c"}, Red, "", "", false},
		{[]string{"reverse", "--list"}, Green, "", "", false},
		{[]string{"reverse", "tcp:8080", "tcp:8080"}, Red, "", "", false},
		{[]string{"reverse", "--remove-all"}, Red, "", "", true},
		{[]string{"install", "app.apk"}, Yellow, "adb.install", "adb-install", false},
		{[]string{"install", "-r", "app.apk"}, Yellow, "", "adb-install", false},
		{[]string{"install", "-d", "app.apk"}, Red, "", "-", false},
		{[]string{"install", "-g", "app.apk"}, Red, "", "-", false},
		{[]string{"install-multiple", "a.apk", "b.apk"}, Yellow, "", "adb-install", false},
		{[]string{"push", "notes.txt", "/sdcard/Download/notes.txt"}, Yellow, "adb.push", "adb-push", false},
		{[]string{"push", "notes.txt", "sdcard/x"}, Red, "", "-", false},
		{[]string{"push", "a", "b", "/sdcard/"}, Red, "", "-", false},
		{[]string{"push", "--sync", "a", "/sdcard/a"}, Red, "", "-", false},
		{[]string{"uninstall", "com.whatsapp"}, Red, "adb.uninstall", "", false},
		{[]string{"uninstall", "tech.hybriq.adbuddy"}, Red, "", "", true},
		{[]string{"kill-server"}, Red, "", "", true},
		{[]string{"disconnect"}, Red, "", "", true},
		{[]string{"usb"}, Red, "", "", true},
		{[]string{"tcpip", "5555"}, Red, "", "", true},
		{[]string{"reboot"}, Red, "", "", true},
		{[]string{"reboot", "recovery"}, Red, "", "", true},
		{[]string{"root"}, Red, "", "", true},
		{[]string{"connect", "1.2.3.4:5555"}, Red, "", "", false},
		{[]string{"sideload", "x.zip"}, Red, "", "", false},
		{[]string{"abb", "package", "uninstall", "x"}, Red, "unknown", "", false},
		{[]string{"exec-in", "rm"}, Red, "unknown", "", false},
		{[]string{}, Green, "", "", false},
	})
}

func TestBadRules(t *testing.T) {
	for _, bad := range []string{
		"rules:\n  - {id: x, cmd: ls}\n",                            // missing level
		"rules:\n  - {id: x, cmd: ls, level: orange}\n",             // unknown level
		"rules:\n  - {id: x, cmd: ls, level: green, check: nope}\n", // unknown check
		"rules:\n  - {id: x, cmd: ls, level: green, colour: red}\n", // unknown field
		"rules:\n  - {cmd: ls, level: green}\n",                     // missing id
		"rules:\n  - {id: x, cmd: ls, level: green, scope: host}\n", // unknown scope
		"rules: [", // not YAML
	} {
		if _, err := New([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := New(nil); err != nil {
		t.Fatal(err)
	}
}

// A template whose {args} is empty leaves no trailing space ("Restart the
// phone ", seen in the real test).
func TestTitlesHaveNoTrailingSpace(t *testing.T) {
	c := mustNew(t, "")
	for _, argv := range [][]string{{"reboot"}, {"shell", "reboot"}, {"uninstall"}} {
		r := c.Classify(argv)
		for _, lang := range []string{"en", "es"} {
			if d := r.Description(lang); d != strings.TrimSpace(d) || d == "" {
				t.Errorf("%v (%s): %q", argv, lang, d)
			}
		}
	}
	if d := c.Classify([]string{"reboot"}).Description("en"); d != "Restart the phone" {
		t.Fatalf("%q", d)
	}
}
