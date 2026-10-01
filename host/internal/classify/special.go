package classify

import (
	"strconv"
	"strings"
)

// ADBuddyPackage is the Android app's package name.
const ADBuddyPackage = "tech.hybriq.adbuddy"

// Fixed protections (spec §4.2 "Siempre 🔴"). They are applied after the
// rules and user rules cannot lower them.

// adbAlwaysRed are adb commands that cut the connection or change how the
// phone listens; remote mode belongs to the host, not the agent.
var adbAlwaysRed = map[string]string{
	"kill-server":       "stops the adb server",
	"disconnect":        "disconnects the phone",
	"usb":               "restarts adbd",
	"tcpip":             "opens a network port on the phone",
	"reconnect":         "drops the connection",
	"root":              "restarts adbd",
	"unroot":            "restarts adbd",
	"reboot":            "restarts the phone",
	"reboot-bootloader": "restarts the phone",
}

// sensitiveSettings would cut the connection, disable ADBuddy's service or
// weaken the phone's security.
var sensitiveSettings = map[string]bool{
	"adb_enabled": true, "adb_wifi_enabled": true, "development_settings_enabled": true,
	"adb_allowed_connection_time": true, "enabled_accessibility_services": true,
	"accessibility_enabled": true, "wifi_on": true, "airplane_mode_on": true,
	"package_verifier_enable": true, "verifier_verify_adb_installs": true,
	"install_non_market_apps": true,
}

func applySpecial(scope string, ws []Word, f *Finding) {
	if reason := specialReason(scope, ws, f.Level); reason != "" {
		f.Level = Red
		f.Special = true
		f.Op = nil
		setIdentity(scope, ws, f)
		if f.Reason == "" {
			f.Reason = reason
		} else {
			f.Reason = reason + "; " + f.Reason
		}
	}
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

func specialReason(scope string, ws []Word, level Level) string {
	v := values(ws)
	name := v[0]
	if scope == "adb" {
		if r, ok := adbAlwaysRed[name]; ok {
			return r
		}
		switch {
		case strings.HasPrefix(name, "reboot"):
			return "restarts the phone"
		case name == "remount":
			return "may restart the phone"
		case name == "bugreport":
			// The report includes dumpsys activity (view tree and texts of
			// what is on screen) and notification.
			return "the bug report includes what is on screen and the notifications"
		case name == "reverse" && !(len(v) == 2 && v[1] == "--list"):
			return "may change ADBuddy's chat tunnel"
		case level != Green && mentionsADBuddy(v):
			return "affects the ADBuddy app"
		}
		return ""
	}
	switch name {
	case "settings":
		verb := strings.ToLower(subOf(ws)) // settings compares verbs ignoring case
		if verb == "reset" {
			return "resets settings, including the connection ones"
		}
		if verb == "put" || verb == "delete" {
			for i, w := range v[2:] {
				if sensitiveSettings[w] {
					return "setting " + w + " affects the connection, ADBuddy or the phone's security"
				}
				if ws[2+i].Dynamic {
					// On the phone it may split or expand into a sensitive key.
					return "a setting only known on the phone could affect the connection, ADBuddy or the phone's security"
				}
			}
		}
	case "svc":
		if len(v) > 1 && (v[1] == "wifi" || v[1] == "power" || (v[1] == "usb" && len(v) > 2 && strings.HasPrefix(v[2], "set"))) {
			return "may cut the connection or turn the phone off"
		}
	case "ip", "ifconfig":
		for _, w := range v[1:] {
			if w == "down" || w == "del" || w == "delete" || w == "flush" || w == "set" {
				return "may cut the network"
			}
		}
	case "content":
		// ADBuddy's own channel (token, consent): even reading it is off
		// limits for the agent (its private files: see run-as in shell.go).
		// Uri.getAuthority() decodes %xx, so compare the decoded words.
		all := strings.ToLower(percentDecode(strings.Join(v, " ")))
		if strings.Contains(all, ADBuddyPackage) {
			return "talks to the ADBuddy app's own channel or files"
		}
		if hasDynamic(ws[1:]) {
			// $(…), $VAR, a loop variable or xargs' stdin: on the phone it
			// may become ADBuddy's authority.
			return "arguments only known on the phone could point at the ADBuddy app's own channel"
		}
		if level != Green && strings.Contains(all, "settings") {
			return "changes system settings directly, including the connection ones"
		}
	case "am":
		if r := amSpecial(ws); r != "" {
			return r
		}
	case "cmd":
		if len(v) > 1 && (v[1] == "wifi" || v[1] == "connectivity") && level != Green {
			return "may cut the Wi-Fi connection"
		}
	case "screenrecord":
		return "records what is on screen"
	case "dumpsys":
		if revealsNotifications(v) {
			return "prints the text of every app's notifications, codes included"
		}
	case "reboot", "stop", "start":
		return "may restart the phone or adbd"
	case "setprop":
		if len(v) > 1 && (strings.HasPrefix(v[1], "service.adb") || strings.HasPrefix(v[1], "persist.adb") ||
			strings.HasPrefix(v[1], "ctl.") || strings.Contains(v[1], "usb")) {
			return "changes how adbd runs"
		}
	}
	if level != Green && mentionsADBuddy(v) {
		return "affects the ADBuddy app"
	}
	if level != Green && touchesCriticalApp(name, ws, v) {
		return "affects an app the phone needs to stay connected or to start"
	}
	if level != Green && resetsEveryApp(name, v) {
		return "resets the permissions, battery exemptions or app ops of every app, ADBuddy's included"
	}
	if level != Green && (name == "pm" || name == "appops" || name == "am" || (name == "dumpsys" && len(v) > 1 && v[1] == "deviceidle")) && hasDynamic(ws[1:]) {
		return "arguments only known on the phone could name ADBuddy or a critical app"
	}
	return ""
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

func amSpecial(ws []Word) string {
	sub, at := subAt(ws)
	if amMemory[sub] {
		return "reads or changes an app's memory"
	}
	for _, w := range ws[1:] {
		if amStartMemoryOption(w.Value) {
			return "reads or changes the memory of the app it opens"
		}
	}
	if sub == "instrument" && !instrumentWaits(ws[at+1:]) {
		// It returns at once and the instrumentation, which can drive the
		// screen, keeps running.
		return "am instrument without -w keeps running after it returns"
	}
	return ""
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
