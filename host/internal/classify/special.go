package classify

import (
	"slices"
	"strconv"
	"strings"
)

// ADBuddyPackage is the Android app's package name.
const ADBuddyPackage = "tech.hybriq.adbuddy"

// Fixed protections (spec §4.2 "Siempre 🔴"). They are applied after the
// rules and user rules cannot lower them.

// adbAlwaysRed are adb commands that cut the connection or change how the
// phone listens; remote mode belongs to the host, not the agent.
var adbAlwaysRed = map[string]Text{
	"kill-server":       reason("stops the adb server"),
	"disconnect":        reason("disconnects the phone"),
	"usb":               reason("restarts adbd"),
	"tcpip":             reason("opens a network port on the phone"),
	"reconnect":         reason("drops the connection"),
	"root":              reason("restarts adbd"),
	"unroot":            reason("restarts adbd"),
	"reboot":            reason("restarts the phone"),
	"reboot-bootloader": reason("restarts the phone"),
}

// sensitiveSettings would cut the connection, disable ADBuddy's service or
// weaken the phone's security. The value says why; an empty one gives the
// generic reason. The keys are the names in AOSP's Settings.java (the value
// of each constant, not its Java name), compared in lower case; all of them
// are there except package_verifier_enable, from older Android.
// lockscreen.disabled is a hidden System setting there, but what keyguard
// reads is the LockSettings key of the same name (locksettings set-disabled,
// special too). Since Android 10 the role manager owns
// sms_default_application and assistant, and writing them has no effect;
// they stay for older Android and OEM builds that still read them.
// Reversible is not harmless: a keyboard or a proxy is undone with one more
// command, after it has seen what you typed or your traffic (security review
// 2026-10-03, A1-1 and A1-2). It is still a denylist: any other setting is
// yellow. Left out on purpose: stay_on_while_plugged_in,
// bugreport_in_power_menu, captive_portal_mode and the other switches that
// only turn a check on or off, lock_screen_show_notifications (what they
// show is lock_screen_allow_private_notifications), show_password,
// mock_location (a no-op since Android 6: mock locations are an app op now),
// and keys that are not in Settings.java (allow_mock_location,
// unknown_sources, device_admin): installing from unknown sources is an app
// op and device admins are dpm (special below).
var sensitiveSettings = map[string]Text{
	"adb_enabled":                    {},
	"adb_wifi_enabled":               {},
	"development_settings_enabled":   {},
	"adb_allowed_connection_time":    {},
	"enabled_accessibility_services": {},
	"accessibility_enabled":          {},
	"wifi_on":                        {},
	"airplane_mode_on":               {},
	"package_verifier_enable":        {},
	"verifier_verify_adb_installs":   {},
	"install_non_market_apps":        {},

	// An app that sees what you type, fill in or say.
	"default_input_method":          whyKeyboard,
	"enabled_input_methods":         whyKeyboard,
	"disabled_system_input_methods": whyKeyboard,
	"autofill_service":              whyPasswords,
	"credential_service":            whyPasswords,
	"credential_service_primary":    whyPasswords,
	"assistant":                     whyAssistant,
	"voice_interaction_service":     whyAssistant,
	"voice_recognition_service":     whyAssistant,

	// An app that reads or silences the notifications, or gets the SMS.
	"enabled_notification_listeners":              whyNotifications,
	"enabled_notification_assistant":              whyNotifications,
	"enabled_notification_policy_access_packages": whyNotifications,
	"sms_default_application":                     whySMSApp,

	// Where the phone's traffic goes.
	"http_proxy":                         whyTraffic,
	"global_http_proxy_host":             whyTraffic,
	"global_http_proxy_port":             whyTraffic,
	"global_http_proxy_exclusion_list":   whyTraffic,
	"global_proxy_pac_url":               whyTraffic,
	"private_dns_mode":                   whyTraffic,
	"private_dns_specifier":              whyTraffic,
	"captive_portal_http_url":            whyTraffic,
	"captive_portal_https_url":           whyTraffic,
	"captive_portal_fallback_url":        whyTraffic,
	"captive_portal_other_fallback_urls": whyTraffic,
	"captive_portal_server":              whyTraffic,
	"always_on_vpn_app":                  whyVPN,
	"always_on_vpn_lockdown":             whyVPN,

	// Where the phone is.
	"location_mode":              whyLocation,
	"location_providers_allowed": whyLocation,

	// The phone's setup state and its lock screen.
	"device_provisioned":                      whySetup,
	"user_setup_complete":                     whySetup,
	"secure_frp_mode":                         whySetup,
	"lockscreen.disabled":                     whyLock,
	"lock_screen_lock_after_timeout":          whyLock,
	"lock_screen_allow_private_notifications": whyLock,

	// Review round 1: what you type, and the accessibility service a
	// shortcut or the accessibility button turns on.
	"selected_spell_checker":                whySpellChecker,
	"accessibility_shortcut_target_service": whyA11yShortcut,
	"accessibility_button_targets":          whyA11yShortcut,
}

var (
	whyKeyboard      = reason("chooses the keyboard, which sees everything you type, passwords included")
	whyPasswords     = reason("chooses the app that fills in your passwords and sees the login forms")
	whyAssistant     = reason("chooses the assistant, which can read the screen or hear what you say")
	whyNotifications = reason("lets an app read or silence every notification, codes included")
	whySMSApp        = reason("chooses the app that gets your text messages, codes included")
	whyTraffic       = reason("sends the phone's traffic or its network checks to a server the command chooses")
	whyVPN           = reason("sets an always-on VPN, which carries all the traffic and can cut the connection")
	whyLocation      = reason("turns location on or changes how the phone finds where it is")
	whySetup         = reason("changes the phone's setup state, which can lock features or start the setup again")
	whyLock          = reason("weakens the lock screen")
	whySpellChecker  = reason("chooses the spell checker, which sees what you type")
	whyA11yShortcut  = reason("chooses the accessibility service a shortcut turns on, which can read and control the screen")
)

// grant is a command whose subcommands hand an app what you type, the
// notifications or a role, make it a device admin, or change the screen
// lock: a user rule or "allow during the session" must not lower them.
type grant struct {
	subs []string
	why  Text
}

var grants = map[string]grant{
	"ime":          {[]string{"enable", "set"}, reason("turns on or chooses a keyboard, which sees everything you type, passwords included")},
	"notification": {[]string{"allow_listener", "allow_assistant", "allow_dnd"}, whyNotifications},
	"role":         {[]string{"add-role-holder", "clear-role-holders", "set-bypassing-role-qualification"}, reason("changes who holds a role, such as the default SMS app, browser or assistant")},
	"dpm":          {[]string{"set-active-admin", "set-device-owner", "set-profile-owner"}, reason("makes an app a device administrator or owner, which can lock or wipe the phone")},
	"locksettings": {[]string{"set-disabled", "clear", "set-pin", "set-password", "set-pattern"}, reason("changes or removes the screen lock")},
}

// grantServices maps cmd's service names to those commands: ime, dpm and
// locksettings are scripts that run cmd input_method, cmd device_policy and
// cmd lock_settings.
var grantServices = map[string]string{
	"input_method": "ime", "notification": "notification", "role": "role",
	"device_policy": "dpm", "lock_settings": "locksettings",
}

// grantReason says why a command's subcommand (the word right after the
// command or the service: these tools read it first) grants one of them. A
// subcommand only known on the phone counts.
func grantReason(cmd string, sub Word) Text {
	g, ok := grants[cmd]
	if ok && (sub.Dynamic || slices.Contains(g.subs, strings.ToLower(sub.Value))) {
		return g.why
	}
	return Text{}
}

// grantsProjectMedia reports whether appops set allows PROJECT_MEDIA, with
// which an app captures the screen without Android's consent dialog.
// AppOpsService's shell takes the op as android:project_media, PROJECT_MEDIA
// or its code (46, read with Integer.parseInt: 046 and +46 too), and the
// mode as a name or a number: anything but ignore, deny or default (1, 2, 3),
// or no mode at all, counts.
func grantsProjectMedia(ws []Word) bool {
	sub, at := subAt(ws)
	if strings.ToLower(sub) != "set" {
		return false
	}
	for i := at + 1; i < len(ws); i++ {
		op := strings.TrimPrefix(strings.ToLower(ws[i].Value), "android:")
		if n, err := strconv.Atoi(op); err == nil {
			// Integer.parseInt, like Atoi, takes a sign and leading zeros.
			op = strconv.Itoa(n)
		}
		if ws[i].Dynamic || (op != "project_media" && op != "46") {
			continue
		}
		if i+1 >= len(ws) || ws[i+1].Dynamic {
			return true
		}
		if !slices.Contains([]string{"ignore", "deny", "default", "1", "2", "3"}, ws[i+1].Value) {
			return true
		}
	}
	return false
}

// sensitiveSetting reports whether w names a sensitive setting, and why.
func sensitiveSetting(w string) (Text, bool) {
	why, ok := sensitiveSettings[strings.ToLower(w)]
	if ok && why.Empty() {
		why = reason("setting {key} affects the connection, ADBuddy or the phone's security", "key", w)
	}
	return why, ok
}

// readsMessages reports whether a content command reads the text messages
// or the telephony provider: query or read (the verbs that return data) with
// a URI whose authority is one of them. The content tool takes the URI after
// --uri, in any position, and Uri.getAuthority() decodes %xx; ADBuddy looks
// at every word that has "://" in it, decoded until nothing changes, so a
// --where or a --projection that names such a URI counts too (fail closed).
func readsMessages(v []string) bool {
	if verb := strings.ToLower(subOf(lits(v))); verb != "query" && verb != "read" {
		return false
	}
	for _, w := range v[1:] {
		s := strings.ToLower(w)
		for range 4 {
			d := strings.ToLower(percentDecode(s))
			if d == s {
				break
			}
			s = d
		}
		for {
			i := strings.Index(s, "://")
			if i < 0 {
				break
			}
			s = s[i+3:]
			end := strings.IndexAny(s, `/\?#`) // android.net.Uri's parseAuthority ends at a backslash too
			if end < 0 {
				end = len(s)
			}
			if messagesAuthority(s[:end]) {
				return true
			}
		}
	}
	return false
}

// messagesAuthority: AOSP's TelephonyProvider serves sms, mms, mms-sms,
// sms-changes and telephony (carriers and SIM information, the phone number
// included). An authority may carry a user (0@sms) or a port. Any other
// authority with a part that contains "sms" or starts with "mms" (an OEM's
// message store) counts too; "comms" does not.
func messagesAuthority(a string) bool {
	if at := strings.LastIndexByte(a, '@'); at >= 0 {
		a = a[at+1:]
	}
	if colon := strings.IndexByte(a, ':'); colon >= 0 {
		a = a[:colon]
	}
	a = strings.TrimSpace(a)
	if a == "telephony" {
		return true
	}
	for _, part := range strings.FieldsFunc(a, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		if strings.Contains(part, "sms") || strings.HasPrefix(part, "mms") {
			return true
		}
	}
	return false
}

func applySpecial(scope string, ws []Word, f *Finding) {
	if why := specialReason(scope, ws, f.Level); !why.Empty() {
		f.Level = Red
		f.Special = true
		f.Op = nil
		f.AppChannel = f.AppChannel || touchesAppChannel(scope, ws)
		setIdentity(scope, ws, f)
		f.Reason = Join(why, f.Reason)
	}
}

// touchesAppChannel reports whether a special command reaches the app's own
// channel or private files: content on its authority (or one only known on the
// phone), and am (start, broadcast…) naming the app. run-as is marked in
// wrappers.go. Not the channel: killing, disabling or uninstalling the app,
// which the host may lift under YOLO (they do not change where the app links to).
func touchesAppChannel(scope string, ws []Word) bool {
	if scope == "adb" || len(ws) == 0 {
		return false
	}
	v := values(ws)
	switch v[0] {
	case "content":
		all := strings.ToLower(percentDecode(strings.Join(v, " ")))
		return strings.Contains(all, ADBuddyPackage) || hasDynamic(ws[1:])
	case "am":
		sub, _ := subAt(ws)
		// A dynamic argument (am start -n $(cat f)) may name the app too.
		return (mentionsADBuddy(v) || hasDynamic(ws[1:])) && sub != "force-stop" && sub != "kill"
	}
	return false
}

// specialIdentity names the commands that have no rule of their own because
// the protection is the rule: no rules file can lower it.
func specialIdentity(scope string, v []string) (string, Text, bool) {
	switch {
	case scope == "adb":
	case v[0] == "screenrecord":
		return "screen.record", Text{EN: "Record the screen", ES: "Grabar la pantalla"}, true
	case v[0] == "dumpsys" && revealsNotifications(v):
		return "notifications.reveal", Text{EN: "Read the text of every app's notifications", ES: "Leer el texto de las notificaciones de todas las apps"}, true
	case v[0] == "content" && readsMessages(v):
		return "sms.read", Text{EN: "Read the text messages or the telephony data: {args}", ES: "Leer los mensajes de texto o los datos de telefonía: {args}"}, true
	}
	return "", Text{}, false
}

// setIdentity gives f the name and title of its fixed protection, if it has
// one.
func setIdentity(scope string, ws []Word, f *Finding) {
	if id, title, ok := specialIdentity(scope, values(ws)); ok {
		f.Rule = id
		f.Title = render(title, baseVars(ws))
	}
}

// revealsNotifications reports whether a dumpsys line asks for the text of the
// notifications unredacted: --noredact, or --reveal (AOSP's
// NotificationManagerService DumpFilter.parseFromArguments treats both the
// same), in any position. It does not need the notification service named:
// dumpsys with no service, or with another one, hands its arguments to every
// service it dumps, the notification one included.
func revealsNotifications(v []string) bool {
	for _, w := range v[1:] {
		if strings.HasPrefix(w, "--noredact") || strings.HasPrefix(w, "--reveal") {
			return true
		}
	}
	return false
}

func specialReason(scope string, ws []Word, level Level) Text {
	v := values(ws)
	name := v[0]
	if scope == "adb" {
		if r, ok := adbAlwaysRed[name]; ok {
			return r
		}
		switch {
		case strings.HasPrefix(name, "reboot"):
			return reason("restarts the phone")
		case name == "remount":
			return reason("may restart the phone")
		case name == "bugreport":
			// The report includes dumpsys activity (view tree and texts of
			// what is on screen) and notification.
			return reason("the bug report includes what is on screen and the notifications")
		case name == "reverse" && !(len(v) == 2 && v[1] == "--list"):
			return reason("may change ADBuddy's chat tunnel")
		case level != Green && mentionsADBuddy(v):
			return reason("affects the ADBuddy app")
		}
		return Text{}
	}
	switch name {
	case "settings":
		verb := strings.ToLower(subOf(ws)) // settings compares verbs ignoring case
		if verb == "reset" {
			return reason("resets settings, including the connection ones")
		}
		if verb == "put" || verb == "delete" {
			for i, w := range v[2:] {
				if why, ok := sensitiveSetting(w); ok {
					return why
				}
				if ws[2+i].Dynamic {
					// On the phone it may split or expand into a sensitive key.
					return reason("a setting only known on the phone could affect the connection, ADBuddy or the phone's security")
				}
			}
		}
	case "svc":
		if len(v) > 1 && (v[1] == "wifi" || v[1] == "power" || (v[1] == "usb" && len(v) > 2 && strings.HasPrefix(v[2], "set"))) {
			return reason("may cut the connection or turn the phone off")
		}
	case "ip", "ifconfig":
		for _, w := range v[1:] {
			if w == "down" || w == "del" || w == "delete" || w == "flush" || w == "set" {
				return reason("may cut the network")
			}
		}
	case "content":
		// ADBuddy's own channel (token, consent): even reading it is off
		// limits for the agent (its private files: see run-as in shell.go).
		// Uri.getAuthority() decodes %xx, so compare the decoded words.
		all := strings.ToLower(percentDecode(strings.Join(v, " ")))
		if strings.Contains(all, ADBuddyPackage) {
			return reason("talks to the ADBuddy app's own channel or files")
		}
		if hasDynamic(ws[1:]) {
			// $(…), $VAR, a loop variable or xargs' stdin: on the phone it
			// may become ADBuddy's authority.
			return reason("arguments only known on the phone could point at the ADBuddy app's own channel")
		}
		if readsMessages(v) {
			// Two-factor codes arrive by SMS; the shell's UID can read the
			// provider on many phones (security review 2026-10-03, A1-3).
			return reason("reads the text messages (SMS and MMS) or the telephony data, verification codes included")
		}
		if level != Green && strings.Contains(all, "settings") {
			return reason("changes system settings directly, including the connection ones")
		}
	case "am":
		if r := amSpecial(ws); !r.Empty() {
			return r
		}
	case "ime", "dpm", "locksettings":
		if len(ws) > 1 {
			if why := grantReason(name, ws[1]); !why.Empty() {
				return why
			}
		}
	case "cmd":
		if len(ws) > 2 {
			if why := grantReason(grantServices[v[1]], ws[2]); !why.Empty() {
				return why
			}
		}
		if len(v) > 1 && (v[1] == "wifi" || v[1] == "connectivity") && level != Green {
			return reason("may cut the Wi-Fi connection")
		}
		if len(v) > 2 && v[1] == "netpolicy" && (ws[2].Dynamic || !slices.Contains([]string{"get", "list", "help"}, v[2])) {
			// Its commands take UIDs (ADBuddy's is not known here) or mark
			// networks as metered.
			return reason("network policy on a UID or a network could cut ADBuddy's or the phone's connection")
		}
	case "screenrecord":
		return reason("records what is on screen")
	case "dumpsys":
		if revealsNotifications(v) {
			return reason("prints the text of every app's notifications, codes included")
		}
	case "bugreport", "bugreportz", "dumpstate":
		// The same report as adb bugreport, built on the phone.
		return reason("the bug report includes what is on screen and the notifications")
	case "appops":
		if grantsProjectMedia(ws) {
			return reason("lets the app record the screen without asking")
		}
		if sub, at := subAt(ws); sub == "set" {
			if t, ok := firstTarget(ws, at+1); ok && !ws[t].Dynamic && !packageName(ws[t].Value) {
				// AppOpsService takes a UID (or root, shell…) instead of a
				// package; ADBuddy's is not known here.
				return reason("appops set on a UID, which could be ADBuddy's")
			}
		}
	case "kill":
		if killsBeyondAPID(v) {
			return reason("kill on every process, a process group or a job could stop adbd or ADBuddy")
		}
	case "pkill", "killall":
		// Always, whatever the arguments: a pattern (whose dialect differs
		// from Go's), -f on the command line, selectors such as -U 2000 or
		// -P 1 and options in any order reach adbd in too many ways to
		// analyse, and adbd runs with the shell's UID.
		return reason("may kill adbd, which cuts the connection")
	case "reboot", "stop", "start":
		return reason("may restart the phone or adbd")
	case "setprop":
		if len(v) > 1 && (strings.HasPrefix(v[1], "service.adb") || strings.HasPrefix(v[1], "persist.adb") ||
			strings.HasPrefix(v[1], "ctl.") || strings.Contains(v[1], "usb")) {
			return reason("changes how adbd runs")
		}
	}
	if level != Green && mentionsADBuddy(v) {
		return reason("affects the ADBuddy app")
	}
	if level != Green && touchesCriticalApp(name, ws, v) {
		return reason("affects an app the phone needs to stay connected or to start")
	}
	// dumpsys options may come before deviceidle (dumpsys -t 5 deviceidle …):
	// judge it from the word deviceidle on.
	dv := v
	if name == "dumpsys" {
		for i, w := range v[1:] {
			if w == "deviceidle" {
				dv = append([]string{"dumpsys"}, v[i+1:]...)
				break
			}
		}
	}
	if level != Green && resetsEveryApp(name, dv) {
		return reason("resets the permissions, battery exemptions or app ops of every app, ADBuddy's included")
	}
	if level != Green && (name == "pm" || name == "appops" || name == "am" || (name == "dumpsys" && len(dv) > 1 && dv[1] == "deviceidle")) && hasDynamic(ws[1:]) {
		return reason("arguments only known on the phone could name ADBuddy or a critical app")
	}
	return Text{}
}

// amMemory are am subcommands that read or change an app's memory (in a
// debuggable build, ADBuddy's token included), whatever the process.
var amMemory = map[string]bool{"dumpheap": true, "attach-agent": true, "profile": true}

// amStartMemory are am start options that do the same to the app it opens.
var amStartMemory = map[string]bool{
	"--attach-agent": true, "--attach-agent-bind": true, "--start-profiler": true, "-P": true,
}

// amStartMemoryOption also catches a short option with its value attached:
// Android splits single-dash options (ShellCommand.getNextOption), so
// -P/data/local/tmp/p.trace is -P with that value.
func amStartMemoryOption(v string) bool {
	if amStartMemory[v] {
		return true
	}
	for o := range amStartMemory {
		if !strings.HasPrefix(o, "--") && strings.HasPrefix(v, o) {
			return true
		}
	}
	return false
}

func amSpecial(ws []Word) Text {
	sub, at := subAt(ws)
	if amMemory[sub] {
		return reason("reads or changes an app's memory")
	}
	switch sub {
	case "bug-report":
		return reason("the bug report includes what is on screen and the notifications")
	case "kill-all":
		return reason("kills every app in the background, ADBuddy's included")
	case "restart":
		return reason("restarts the phone's system, which cuts the connection")
	case "stop-user", "switch-user":
		return reason("stops or switches the phone's user, which can stop ADBuddy")
	case "crash":
		// am crash takes a package or a PID; ADBuddy's PID is not known here.
		if t, ok := firstTarget(ws, at+1); ok && !ws[t].Dynamic && !packageName(ws[t].Value) {
			return reason("am crash on a process ID, which could be ADBuddy's")
		}
	}
	for _, w := range ws[1:] {
		if amStartMemoryOption(w.Value) {
			return reason("reads or changes the memory of the app it opens")
		}
	}
	if sub == "instrument" && !instrumentWaits(ws[at+1:]) {
		// It returns at once and the instrumentation, which can drive the
		// screen, keeps running.
		return reason("am instrument without -w keeps running after it returns")
	}
	return Text{}
}

// firstTarget is the index of the first word from ws[from] on that is not an
// option or the value of --user.
func firstTarget(ws []Word, from int) (int, bool) {
	for i := from; i < len(ws); i++ {
		switch v := ws[i].Value; {
		case ws[i].Dynamic:
			return i, true
		case v == "--user":
			i++
		case strings.HasPrefix(v, "-"):
		default:
			return i, true
		}
	}
	return 0, false
}

// killsBeyondAPID reports whether kill signals something other than literal
// PIDs: -1 (every process the shell's UID may signal), 0 or -PGID (a process
// group) or %job. mksh's kill takes -SIGNAL (a digit or a capital after the
// dash), or -s SIGNAL and -l, then "--". On a production phone adbd runs with
// the shell's UID (2000), so kill -9 <adbd's PID> does cut the connection; a
// literal PID stays a plain 🔴 only because testdata/habitual.jsonl fixes it
// so (kill -9 1234). Accepted risk: a user rule can lower it.
func killsBeyondAPID(v []string) bool {
	args := v[1:]
	if len(args) > 0 && len(args[0]) > 1 && args[0][0] == '-' && (args[0][1] >= '0' && args[0][1] <= '9' || args[0][1] >= 'A' && args[0][1] <= 'Z') {
		args = args[1:]
	} else {
		for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "--" {
			switch a := args[0]; {
			case a == "-l":
				return false // lists signals
			case a == "-s":
				if len(args) < 2 {
					return false
				}
				args = args[2:]
			case strings.HasPrefix(a, "-s"):
				args = args[1:]
			default:
				return true // an option kill does not have: fail closed
			}
		}
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	for _, t := range args {
		if !isDigits(t) || strings.Trim(t, "0") == "" {
			return true
		}
	}
	return false
}

// instrumentValues are the am instrument options that take values. A few
// more than am documents today (-f, --abi) only make -w harder to find.
var instrumentValues = map[string]int{"-e": 2, "-p": 1, "-f": 1, "--user": 1, "--abi": 1}

// instrumentWaits reports whether am instrument's options (the words after
// "instrument") include -w as an option, not as the value of another one.
// am stops reading options at the component, at "--" and at the first word
// that does not start with "-"; a word only known on the phone is unknown.
func instrumentWaits(ws []Word) bool {
	for i := 0; i < len(ws); i++ {
		v := ws[i].Value
		if ws[i].Dynamic || v == "--" || !strings.HasPrefix(v, "-") {
			return false
		}
		if v == "-w" {
			return true
		}
		n := instrumentValues[v]
		if !strings.HasPrefix(v, "--") && len(v) > 2 {
			// -ekey: the first value is attached.
			n = max(instrumentValues[v[:2]]-1, 0)
		}
		i += n
	}
	return false
}

// mentionsADBuddy compares the words decoded and in lower case: intent: URIs
// and Uri.getAuthority() decode %xx.
func mentionsADBuddy(v []string) bool {
	for _, s := range v {
		if strings.Contains(strings.ToLower(percentDecode(s)), ADBuddyPackage) {
			return true
		}
	}
	return false
}

// criticalPackages break the connection, the shell or the phone's start when
// disabled, removed, cleared or killed.
var criticalPackages = map[string]bool{
	"android": true, "com.android.shell": true, "com.android.providers.settings": true,
	"com.android.systemui": true, "com.android.phone": true, "com.android.settings": true,
}

func criticalPackage(w string) bool {
	pkg, _, _ := strings.Cut(w, "/") // a component: pkg/.Class
	return criticalPackages[pkg] || strings.Contains(pkg, "networkstack") ||
		strings.HasPrefix(pkg, "com.android.wifi") || strings.HasPrefix(pkg, "com.google.android.wifi") ||
		strings.HasPrefix(pkg, "com.android.server.wifi")
}

// touchesCriticalApp: package management, app ops and killing apps; opening
// an app (am start) is fine.
func touchesCriticalApp(name string, ws []Word, v []string) bool {
	switch name {
	case "pm", "appops":
	case "am":
		if s := subOf(ws); s != "force-stop" && s != "kill" && s != "crash" {
			return false
		}
	default:
		return false
	}
	for _, w := range v[1:] {
		if criticalPackage(w) {
			return true
		}
	}
	return false
}

// percentDecode decodes %xx escapes, leaving malformed ones as they are.
func percentDecode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// resetsEveryApp: pm reset-permissions (it has no -p option: AOSP resets
// everything and ignores its arguments), `deviceidle whitelist reset` and
// `appops reset` (with an uninstalled package it resets the modes of every
// UID: AppOpsService.resetAllModes with reqUid -1) strip ADBuddy's app of
// WRITE_SECURE_SETTINGS (Cut access), its battery exemption or its app ops
// without naming it (spec M6a §7). cmd arrives already rewritten to
// pm/appops/dumpsys.
func resetsEveryApp(name string, v []string) bool {
	has := func(i int, s string) bool { return len(v) > i && v[i] == s }
	switch {
	case name == "pm" && has(1, "reset-permissions"):
		return true
	case name == "appops" && has(1, "reset"):
		return true
	case name == "dumpsys" && has(1, "deviceidle") && has(2, "whitelist") && has(3, "reset"):
		return true
	}
	return false
}
