package classify

import (
	"strings"
	"testing"
)

// Independent security review of 2026-10-03, findings A1-1, A1-2 and A1-3:
// settings that hand an app what you type, the phone's traffic, its location
// or its lock screen, and reading the text messages, are fixed protections
// (always 🔴, no "allow during the session", a user rule cannot lower them).

// checkPlain is check for cases that must not be special.
func checkPlain(t *testing.T, c *Classifier, cases []tc) {
	t.Helper()
	check(t, c, cases)
	for _, k := range cases {
		if r := c.Classify(k.argv); !k.special && r.Special {
			t.Errorf("%q: special (%s: %s), want not special", k.argv, r.Primary.Rule, r.Primary.Reason)
		}
	}
}

// A1-1: the keyboard is the same keylogger as an accessibility service.
func TestKeyboardSettingsAreSpecial(t *testing.T) {
	c := mustNew(t, "")
	check(t, c, []tc{
		{sh("settings put secure default_input_method com.evil/.Ime"), Red, "settings.put", "-", true},
		{sh("settings put secure enabled_input_methods com.google.android.inputmethod.latin/.LatinIME:com.evil/.Ime"), Red, "", "-", true},
		{sh("settings put secure disabled_system_input_methods ''"), Red, "", "-", true},
		{sh("settings --user 0 put secure default_input_method com.evil/.Ime"), Red, "", "-", true},
		{sh("settings put --user 0 secure default_input_method com.evil/.Ime"), Red, "", "-", true},
		{sh("settings PUT secure default_input_method com.evil/.Ime"), Red, "", "-", true},
		{sh("settings put secure DEFAULT_INPUT_METHOD com.evil/.Ime"), Red, "", "-", true},
		{sh("settings put secure 'default_input_method' com.evil/.Ime"), Red, "", "-", true},
		{sh("settings put global default_input_method com.evil/.Ime"), Red, "", "-", true},
		{sh("settings put system enabled_input_methods com.evil/.Ime"), Red, "", "-", true},
		{sh("cmd settings put secure default_input_method com.evil/.Ime"), Red, "", "-", true},
		{sh("settings delete secure enabled_input_methods"), Red, "", "-", true},
		{sh("sh -c 'settings put secure default_input_method com.evil/.Ime'"), Red, "", "-", true},
		{sh("settings get secure default_input_method; settings put secure default_input_method com.evil/.Ime"), Red, "", "-", true},
		{[]string{"exec-out", "settings", "put", "secure", "default_input_method", "com.evil/.Ime"}, Red, "", "-", true},
		{sh("content insert --uri content://settings/secure --bind name:s:default_input_method --bind value:s:com.evil/.Ime"), Red, "", "", true},
	})
	checkPlain(t, c, []tc{
		{sh("settings get secure default_input_method"), Green, "settings.read", "", false},
		{sh("settings get secure enabled_input_methods"), Green, "settings.read", "", false},
		{sh("settings put secure selected_input_method_subtype 0"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put secure show_ime_with_hard_keyboard 1"), Yellow, "settings.put", "settings-put", false},
	})
	r := c.Classify(sh("settings put secure default_input_method com.evil/.Ime"))
	if !strings.Contains(r.Detail("en"), "keyboard") || !strings.Contains(r.Detail("es"), "teclado") {
		t.Errorf("detail en %q, es %q: want the keyboard named", r.Detail("en"), r.Detail("es"))
	}
}

// sensitiveKeys are the keys A1-2 adds, each with a namespace it lives in.
var sensitiveKeys = [][2]string{
	{"secure", "default_input_method"}, {"secure", "enabled_input_methods"}, {"secure", "disabled_system_input_methods"},
	{"secure", "autofill_service"}, {"secure", "credential_service"}, {"secure", "credential_service_primary"},
	{"secure", "assistant"}, {"secure", "voice_interaction_service"}, {"secure", "voice_recognition_service"},
	{"secure", "enabled_notification_listeners"}, {"secure", "enabled_notification_assistant"},
	{"secure", "enabled_notification_policy_access_packages"}, {"secure", "sms_default_application"},
	{"global", "http_proxy"}, {"global", "global_http_proxy_host"}, {"global", "global_http_proxy_port"},
	{"global", "global_http_proxy_exclusion_list"}, {"global", "global_proxy_pac_url"},
	{"global", "private_dns_mode"}, {"global", "private_dns_specifier"},
	{"global", "captive_portal_http_url"}, {"global", "captive_portal_https_url"}, {"global", "captive_portal_fallback_url"},
	{"global", "captive_portal_other_fallback_urls"}, {"global", "captive_portal_server"},
	{"secure", "always_on_vpn_app"}, {"secure", "always_on_vpn_lockdown"},
	{"secure", "location_mode"}, {"secure", "location_providers_allowed"},
	{"global", "device_provisioned"}, {"secure", "user_setup_complete"}, {"global", "secure_frp_mode"},
	{"secure", "lockscreen.disabled"}, {"secure", "lock_screen_lock_after_timeout"},
	{"secure", "lock_screen_allow_private_notifications"},
	// Review round 1 of the task.
	{"secure", "selected_spell_checker"}, {"secure", "accessibility_shortcut_target_service"},
	{"secure", "accessibility_button_targets"},
}

// A1-2: every key is special whatever the way it is written, and a user rule
// cannot lower it.
func TestSensitiveSettingsAreSpecial(t *testing.T) {
	c := mustNew(t, "")
	u := mustNew(t, "rules:\n  - {id: u.settings, cmd: settings, level: green, anyArgs: true}\n  - {id: u.content, cmd: content, level: green, anyArgs: true}\n")
	for _, k := range sensitiveKeys {
		ns, key := k[0], k[1]
		lines := []string{
			"settings put " + ns + " " + key + " 1",
			"settings --user 0 put " + ns + " " + key + " 1",
			"cmd settings put " + ns + " " + key + " 1",
			"settings delete " + ns + " " + key,
			"content insert --uri content://settings/" + ns + " --bind name:s:" + key + " --bind value:s:1",
		}
		for _, line := range lines {
			for _, cl := range []*Classifier{c, u} {
				r := cl.Classify(sh(line))
				if r.Level != Red || !r.Special {
					t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
					continue
				}
				if r.Op != nil {
					t.Errorf("%q: op %s, want none", line, r.Op.Kind)
				}
			}
		}
		r := c.Classify(sh("settings put " + ns + " " + key + " 1"))
		if why := r.Primary.Reason; why.EN == "" || why.ES == "" || why.EN == why.ES {
			t.Errorf("%s: reason %+v, want it in English and Spanish", key, why)
		}
	}
}

// What A1-2 leaves alone: harmless settings stay yellow, and reading any
// setting stays green.
func TestHarmlessSettingsStayYellow(t *testing.T) {
	c := mustNew(t, "")
	checkPlain(t, c, []tc{
		{sh("settings put global stay_on_while_plugged_in 7"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put global window_animation_scale 0.5"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put system screen_off_timeout 600000"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put global adaptive_battery_management_enabled 1"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put global bugreport_in_power_menu 1"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put secure lock_screen_show_notifications 1"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put system show_password 0"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put secure spell_checker_enabled 0"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put global captive_portal_mode 1"), Yellow, "settings.put", "settings-put", false},
		{sh("settings put secure location_changer 1"), Yellow, "settings.put", "settings-put", false},
		{sh("settings get global http_proxy"), Green, "settings.read", "", false},
		{sh("settings get secure location_mode"), Green, "settings.read", "", false},
		{sh("settings list secure"), Green, "settings.read", "", false},
		{sh("content query --uri content://settings/secure"), Green, "android.content-query", "", false},
	})
}

// A1-3: reading the text messages (and the telephony provider) is special;
// every other content provider keeps its level.
func TestReadingMessagesIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	u := mustNew(t, "rules:\n  - {id: u.content, cmd: content, level: green, anyArgs: true}\n")
	lines := []string{
		"content query --uri content://sms",
		"content query --uri content://sms/",
		"content query --uri content://sms/inbox",
		"content query --uri content://sms/inbox/12",
		"content query --uri content://sms/icc",
		"content query --uri content://SMS/inbox",
		"content query --uri CONTENT://Sms/Inbox",
		"content query --uri content://0@sms/inbox",
		"content query --uri content://10@sms",
		"content query --uri content://sms:80/inbox",
		"content query --uri content://%73ms/inbox",
		"content query --uri content://%2573ms/inbox",
		"content query --uri 'content://sms/inbox'",
		`content query --uri "content://sms/inbox"`,
		"content query --user 0 --uri content://sms/inbox",
		"content query --uri content://sms/inbox --user 0",
		`content query --uri content://sms/inbox --projection address:body:date --where "read=0" --sort "date DESC"`,
		`content query --projection body --uri content://sms/inbox`,
		"content query --uri content://sms?limit=5",
		"content query --uri content://sms#x",
		"content query --uri content://mms",
		"content query --uri content://mms/inbox",
		"content query --uri content://mms/part",
		"content query --uri content://mms-sms/conversations",
		"content query --uri content://mms-sms/conversations?simple=true",
		"content query --uri content://mms-sms/complete-conversations",
		"content query --uri content://sms-changes",
		"content query --uri content://telephony/carriers",
		"content query --uri content://telephony/siminfo",
		// android.net.Uri ends the authority at a backslash too.
		`content query --uri 'content://telephony\carriers'`,
		"content query --uri content://com.android.mms.SuggestionsProvider",
		"content query --uri content://com.example.smsprovider/messages",
		"content read --uri content://mms/part/7",
		"content read --uri content://sms/inbox",
		"content query --uri content://sms/inbox | grep -i code",
		"sh -c 'content query --uri content://sms/inbox'",
		"timeout 5 content query --uri content://sms/inbox",
		"if true; then content query --uri content://sms/inbox; fi",
	}
	for _, line := range lines {
		for _, cl := range []*Classifier{c, u} {
			if r := cl.Classify(sh(line)); r.Level != Red || !r.Special {
				t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
			}
		}
	}
	check(t, c, []tc{
		{sh("content query --uri content://sms/inbox"), Red, "sms.read", "", true},
		{sh("content read --uri content://mms/part/7"), Red, "sms.read", "", true},
		{[]string{"exec-out", "content", "query", "--uri", "content://sms/inbox"}, Red, "sms.read", "", true},
	})
	r := c.Classify(sh("content query --uri content://sms/inbox"))
	if r.Primary.Title.EN == "" || r.Primary.Title.ES == "" || r.Primary.Title.EN == r.Primary.Title.ES {
		t.Errorf("titles %+v", r.Primary.Title)
	}
	if why := r.Primary.Reason; why.EN == "" || why.ES == "" || why.EN == why.ES || !strings.Contains(why.EN, "code") {
		t.Errorf("reason %+v: want it in both languages, naming the codes", why)
	}
}

func TestOtherContentProvidersKeepTheirLevel(t *testing.T) {
	c := mustNew(t, "")
	checkPlain(t, c, []tc{
		{sh("content query --uri content://settings/global"), Green, "android.content-query", "", false},
		{sh("content query --uri content://com.android.contacts/contacts"), Green, "android.content-query", "", false},
		{sh("content query --uri content://call_log/calls"), Green, "android.content-query", "", false},
		{sh("content query --uri content://media/external/images/media"), Green, "android.content-query", "", false},
		{sh("content query --uri content://com.android.calendar/events"), Green, "android.content-query", "", false},
		{sh("content query --uri content://user_dictionary/words"), Green, "android.content-query", "", false},
		{sh("content query --uri content://com.example.comms/x"), Green, "android.content-query", "", false},
		{sh("content query --uri content://telephonyx/x"), Green, "android.content-query", "", false},
		{sh("content query --uri content://settings/global --where name=\"'sms_default_application'\""), Green, "android.content-query", "", false},
		{sh("content delete --uri content://sms"), Red, "", "", false},
		{sh("content update --uri content://x"), Red, "", "", false},
	})
}

// Review round 1: PROJECT_MEDIA lets an app capture the screen without
// Android's own consent dialog. Allowing it (allow, foreground or a numeric
// mode other than ignore, deny and default; the op by name, android: name or
// code 46) is special; denying it is an ordinary appops set.
func TestProjectMediaIsSpecial(t *testing.T) {
	c := mustNew(t, "")
	u := mustNew(t, "rules:\n  - {id: u.appops, cmd: appops, level: green, anyArgs: true}\n")
	for _, line := range []string{
		"appops set com.x PROJECT_MEDIA allow",
		"appops set com.x project_media allow",
		"appops set com.x android:project_media allow",
		"appops set com.x Android:Project_Media allow",
		"appops set com.x PROJECT_MEDIA foreground",
		"appops set com.x PROJECT_MEDIA 0",
		"appops set com.x PROJECT_MEDIA 4",
		"appops set com.x PROJECT_MEDIA ALLOW",
		"appops set com.x 46 allow",
		"appops set --user 0 com.x PROJECT_MEDIA allow",
		"appops set --uid com.x PROJECT_MEDIA allow",
		"appops set com.x PROJECT_MEDIA",
		"cmd appops set com.x PROJECT_MEDIA allow",
		"appops set com.x PROJECT_MEDIA $(echo allow)",
	} {
		for _, cl := range []*Classifier{c, u} {
			r := cl.Classify(sh(line))
			if r.Level != Red || !r.Special || r.Op != nil {
				t.Errorf("%q: %v special=%v op=%v (%s: %s), want red special without op", line, r.Level, r.Special, r.Op, r.Primary.Rule, r.Primary.Reason)
			}
		}
	}
	r := c.Classify(sh("appops set com.x PROJECT_MEDIA allow"))
	if why := r.Primary.Reason; !strings.Contains(why.EN, "record the screen") || why.ES == "" || why.EN == why.ES {
		t.Errorf("reason %+v", why)
	}
	checkPlain(t, c, []tc{
		{sh("appops set com.x PROJECT_MEDIA deny"), Yellow, "appops.set", "appops-set", false},
		{sh("appops set com.x PROJECT_MEDIA ignore"), Yellow, "appops.set", "appops-set", false},
		{sh("appops set com.x PROJECT_MEDIA default"), Yellow, "appops.set", "appops-set", false},
		{sh("appops get com.x PROJECT_MEDIA"), Green, "appops.read", "", false},
		{sh("appops set com.x CAMERA allow"), Yellow, "appops.set", "appops-set", false},
		// Product decision (no capar): these stay yellow.
		{sh("appops set com.x SYSTEM_ALERT_WINDOW allow"), Yellow, "appops.set", "appops-set", false},
		{sh("appops set com.x MANAGE_EXTERNAL_STORAGE allow"), Yellow, "appops.set", "appops-set", false},
	})
}

// Review round 1: the same keyboard, notification access, roles, device
// admins and screen lock through their own commands are special.
func TestKeyboardRoleAdminAndLockCommandsAreSpecial(t *testing.T) {
	c := mustNew(t, "")
	u := mustNew(t, `rules:
  - {id: u.ime, cmd: ime, level: green, anyArgs: true}
  - {id: u.cmd, cmd: cmd, level: green, anyArgs: true}
  - {id: u.dpm, cmd: dpm, level: green, anyArgs: true}
  - {id: u.lock, cmd: locksettings, level: green, anyArgs: true}
`)
	for _, line := range []string{
		"ime enable com.evil/.Ime",
		"ime set com.evil/.Ime",
		"ime enable --user 0 com.evil/.Ime",
		"ime SET com.evil/.Ime",
		"ime $(echo set) com.evil/.Ime",
		"cmd input_method enable com.evil/.Ime",
		"cmd input_method set com.evil/.Ime",
		"cmd -w input_method set com.evil/.Ime",
		"cmd notification allow_listener com.evil/.Listener",
		"cmd notification allow_assistant com.evil/.Assistant",
		"cmd notification allow_dnd com.evil",
		"cmd role add-role-holder android.app.role.SMS com.evil",
		"cmd role add-role-holder --user 0 android.app.role.BROWSER com.evil",
		"dpm set-active-admin com.evil/.Admin",
		"dpm set-active-admin --user 0 com.evil/.Admin",
		"dpm set-device-owner com.evil/.Admin",
		"dpm set-profile-owner com.evil/.Admin",
		"cmd device_policy set-active-admin com.evil/.Admin",
		"locksettings set-disabled true",
		"locksettings clear --old 1234",
		"locksettings set-pin 1234",
		"locksettings set-password hunter2",
		"locksettings set-pattern 1234",
		"cmd lock_settings set-disabled true",
		"sh -c 'ime set com.evil/.Ime'",
	} {
		for _, cl := range []*Classifier{c, u} {
			if r := cl.Classify(sh(line)); r.Level != Red || !r.Special {
				t.Errorf("%q: %v special=%v (%s: %s), want red special", line, r.Level, r.Special, r.Primary.Rule, r.Primary.Reason)
			}
		}
		r := c.Classify(sh(line))
		if why := r.Primary.Reason; why.EN == "" || why.ES == "" || why.EN == why.ES {
			t.Errorf("%q: reason %+v, want it in both languages", line, why)
		}
	}
	checkPlain(t, c, []tc{
		{sh("ime list -s"), Green, "android.ime-list", "", false},
		{sh("cmd input_method list -s"), Red, "", "", false},
		{sh("cmd notification disallow_listener com.x/.L"), Red, "", "", false},
		{sh("cmd notification disallow_dnd com.x"), Red, "", "", false},
		{sh("cmd role remove-role-holder android.app.role.SMS com.x"), Red, "", "", false},
		{sh("cmd role get-role-holders android.app.role.SMS"), Red, "", "", false},
		{sh("dpm list-owners"), Red, "", "", false},
		{sh("locksettings get-disabled"), Red, "", "", false},
		{sh("locksettings verify --old 1234"), Red, "", "", false},
	})
}
