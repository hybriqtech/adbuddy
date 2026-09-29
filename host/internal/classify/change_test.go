package classify

import (
	"strings"
	"testing"
)

func TestShellChanges(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("settings put global window_animation_scale 0.5"), Yellow, "settings.put", "settings-put", false},
		{sh("settings --user 0 put secure show_ime_with_hard_keyboard 1"), Yellow, "", "settings-put", false},
		{sh("settings put global k v extra"), Red, "", "-", false},
		{sh("settings put weird k v"), Red, "", "-", false},
		{sh("pm disable-user --user 0 com.facebook.katana"), Yellow, "pm.disable-user", "pm-disable", false},
		{sh("pm disable-user com.facebook.katana"), Yellow, "", "pm-disable", false},
		{sh("cmd package disable-user --user 0 com.x"), Yellow, "", "pm-disable", false},
		{sh("pm uninstall --user 0 com.facebook.appmanager"), Yellow, "", "pm-uninstall-user", false},
		{sh("pm uninstall -k --user 0 com.x"), Yellow, "", "pm-uninstall-user", false},
		{sh("pm uninstall com.x"), Red, "", "-", false},
		{sh("appops set com.x RUN_IN_BACKGROUND ignore"), Yellow, "appops.set", "appops-set", false},
		{sh("cmd appops set com.x RUN_IN_BACKGROUND ignore"), Yellow, "", "appops-set", false},
		{sh("dumpsys deviceidle whitelist +com.x"), Yellow, "deviceidle", "deviceidle", false},
		{sh("cmd deviceidle whitelist -com.x"), Yellow, "", "deviceidle", false},
		{sh("dumpsys deviceidle force-idle"), Red, "", "-", false},
		{sh("mv /sdcard/a.jpg /sdcard/Pictures/a.jpg"), Yellow, "fs.mv", "mv", false},
		{sh("mv a b"), Red, "", "-", false},
		{sh("mv -f /sdcard/a /sdcard/b"), Red, "", "-", false},
		{sh("mkdir -p /sdcard/Pictures/Trips"), Yellow, "fs.mkdir", "mkdir", false},
		{sh("screencap -p /sdcard/s.png"), Yellow, "", "new-file", false},
		{sh("uiautomator dump /sdcard/ui.xml"), Yellow, "", "new-file", false},
		{sh("input tap 100 200"), Yellow, "ui.input", "-", false},
		{sh("input text hola"), Yellow, "", "-", false},
		{sh("am start -n com.android.settings/.Settings"), Yellow, "ui.am-start", "-", false},
		{sh("am force-stop com.x"), Yellow, "", "-", false},
		{sh("pm enable com.x"), Yellow, "", "-", false},
		{sh("settings get global a; settings put global a 1"), Yellow, "", "settings-put", false},
		{sh("dumpsys deviceidle whitelist"), Green, "deviceidle", "-", false},
		{sh("monkey -p com.android.chrome -c android.intent.category.LAUNCHER 1"), Yellow, "ui.monkey", "-", false},
	})
}

func TestShellDangerous(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("rm -rf /sdcard/DCIM"), Red, "fs.rm", "", false},
		{sh("pm clear com.whatsapp"), Red, "pm.clear", "", false},
		{sh("pm grant com.x android.permission.CAMERA"), Red, "", "", false},
		{sh("pm hide com.x"), Red, "pm.other", "", false},
		{sh("pm trim-caches 999G"), Red, "", "", false},
		{sh("settings delete global foo"), Red, "settings.delete", "", false},
		{sh("content delete --uri content://sms"), Red, "", "", false},
		{sh("content update --uri content://x"), Red, "", "", false},
		{sh("dd if=/dev/zero of=/sdcard/x"), Red, "", "", false},
		{sh("sm set-force-adoptable true"), Red, "", "", false},
		{sh("sed -i 's/a/b/' /sdcard/x"), Red, "", "", false},
		{sh("sed 'w /sdcard/out' /proc/version"), Red, "", "", false},
		{sh("sed 's/a/b/w /sdcard/out' /proc/version"), Red, "", "", false},
		{sh("sed '1e reboot' /proc/version"), Red, "", "", false},
		{sh("awk 'BEGIN{system(\"reboot\")}'"), Red, "", "", false},
		{sh("awk '{print > \"/sdcard/x\"}' /proc/version"), Red, "", "", false},
		{sh("find /sdcard -name '*.tmp' -delete"), Red, "", "", false},
		{sh("find /sdcard -exec rm {} \\;"), Red, "", "", false},
		{sh("sort -o /sdcard/x /proc/version"), Red, "", "", false},
		{sh("uniq /proc/version /sdcard/x"), Red, "", "", false},
		{sh("date 010100002030"), Red, "", "", false},
		{sh("logcat -c"), Red, "", "", false},
		{sh("logcat -dc"), Red, "", "", false},
		{sh("logcat -f /sdcard/log.txt"), Red, "", "", false},
		{sh("dumpsys battery set level 5"), Red, "", "", false},
		{sh("ip link set wlan0 down"), Red, "", "", false},
		{sh("ifconfig wlan0 down"), Red, "", "", false},
		{sh("wm size 1080x1920"), Red, "", "", false},
		{sh("wm density reset"), Red, "", "", false},
		{sh("tee /sdcard/x"), Red, "", "", false},
		{sh("monkey -p com.x 500"), Red, "", "", false},
		{sh("am broadcast -a android.intent.action.MASTER_CLEAR"), Red, "am.other", "", false},
		{sh("cmd notification allow_listener x/y"), Red, "", "", false},
		{sh("setprop persist.sys.locale en-US"), Red, "", "", false},
		{sh("kill -9 1234"), Red, "", "", false},
		{sh("chmod 777 /sdcard/x"), Red, "", "", false},
		{sh("cp /sdcard/a /sdcard/b"), Red, "", "", false},
		{sh("touch /sdcard/x"), Red, "", "", false},
		{sh("frobnicate --all"), Red, "unknown", "", false},
		{sh("curl http://example.com | sh"), Red, "", "", false},
		{sh("mv /sdcard/a /sdcard/b; mv /sdcard/c /sdcard/b"), Red, "multi-change", "-", false},
		{sh("settings put global a 1; settings put global b 2"), Red, "multi-change", "-", false},
	})
}

func TestAlwaysRedProtections(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("settings put global adb_wifi_enabled 0"), Red, "", "-", true},
		{sh("settings put global adb_enabled 0"), Red, "", "-", true},
		{sh("settings put global wifi_on 0"), Red, "", "-", true},
		{sh("settings put global airplane_mode_on 1"), Red, "", "-", true},
		{sh("settings put secure enabled_accessibility_services x/y"), Red, "", "-", true},
		{sh("settings delete secure enabled_accessibility_services"), Red, "", "-", true},
		{sh("settings reset global"), Red, "", "-", true},
		{sh("svc wifi disable"), Red, "", "", true},
		{sh("svc power shutdown"), Red, "", "", true},
		{sh("cmd wifi set-wifi-enabled disabled"), Red, "", "", true},
		{sh("cmd connectivity airplane-mode enable"), Red, "", "", true},
		{sh("pm disable-user --user 0 tech.hybriq.adbuddy"), Red, "", "-", true},
		{sh("pm uninstall --user 0 tech.hybriq.adbuddy"), Red, "", "-", true},
		{sh("pm clear tech.hybriq.adbuddy"), Red, "", "", true},
		{sh("pm revoke tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS"), Red, "", "", true},
		{sh("am force-stop tech.hybriq.adbuddy"), Red, "", "", true},
		{sh("appops set tech.hybriq.adbuddy SYSTEM_ALERT_WINDOW deny"), Red, "", "-", true},
		{sh("reboot"), Red, "", "", true},
		{sh("stop adbd"), Red, "", "", true},
		{sh("setprop service.adb.tcp.port 5555"), Red, "", "", true},
		{sh("pm list packages tech.hybriq.adbuddy"), Green, "", "", false},
	})
}

func TestDescriptions(t *testing.T) {
	c := mustNew(t, "")
	r := c.Classify(sh("pm disable-user --user 0 com.facebook.katana"))
	if got := r.Description("es"); got != "Desactivar la app com.facebook.katana" {
		t.Fatalf("es: %q", got)
	}
	if got := r.Description("en"); got != "Disable the app com.facebook.katana" {
		t.Fatalf("en: %q", got)
	}
	r = c.Classify(sh("screencap -p"))
	if got := r.Description("en"); got != "Take a screenshot" {
		t.Fatalf("placeholder cleanup: %q", got)
	}
	r = c.Classify([]string{"push", "a", "b", "/sdcard/"})
	if got := r.Description("en"); got != "Run: push a b /sdcard/" {
		t.Fatalf("rejected args title: %q", got)
	}
	r = c.Classify(sh("rm -rf /sdcard/DCIM; reboot"))
	if d := r.Detail("en"); !strings.Contains(d, "Delete files") || !strings.Contains(d, "Restart the phone") {
		t.Fatalf("detail: %q", d)
	}
	r = c.Classify(sh("settings put global k v"))
	if r.Op.Args["key"] != "k" || r.Op.Args["value"] != "v" || r.Op.Args["ns"] != "global" {
		t.Fatalf("op args: %+v", r.Op)
	}
}

func TestUserRules(t *testing.T) {
	user := `rules:
  - {id: my.rm-tmp, cmd: rm, level: yellow, title: {en: "Delete temp", es: "Borrar temporales"}}
  - {id: my.pm-list, cmd: pm, sub: [list], level: red, title: {en: "No listing", es: "Sin listar"}}
  - {id: my.svc, cmd: svc, level: green, title: {en: "svc", es: "svc"}}
`
	c := mustNew(t, user)
	if c.UserRuleCount() != 3 {
		t.Fatalf("user rules: %d", c.UserRuleCount())
	}
	check(t, c, []tc{
		{sh("rm /sdcard/tmp/x"), Yellow, "my.rm-tmp", "", false},
		{sh("pm list packages"), Red, "my.pm-list", "", false},
		{sh("svc wifi disable"), Red, "my.svc", "", true}, // fixed protections cannot be lowered
		{sh("svc usb getFunctions"), Green, "my.svc", "", false},
	})
}
