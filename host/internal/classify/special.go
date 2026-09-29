package classify

import "strings"

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
		if f.Reason == "" {
			f.Reason = reason
		} else {
			f.Reason = reason + "; " + f.Reason
		}
	}
}

func specialReason(scope string, ws []Word, level Level) string {
	v := values(ws)
	name := v[0]
	if scope == "adb" {
		if r, ok := adbAlwaysRed[name]; ok {
			return r
		}
		if name == "reverse" && len(v) > 1 && strings.HasPrefix(v[1], "--remove") {
			return "may remove ADBuddy's chat tunnel"
		}
		if name == "uninstall" && mentionsADBuddy(v) {
			return "affects the ADBuddy app"
		}
		return ""
	}
	switch name {
	case "settings":
		verb := subOf(ws)
		if verb == "reset" {
			return "resets settings, including the connection ones"
		}
		if verb == "put" || verb == "delete" {
			for _, w := range v[2:] {
				if sensitiveSettings[w] {
					return "setting " + w + " affects the connection, ADBuddy or the phone's security"
				}
			}
		}
	case "svc":
		if len(v) > 1 && (v[1] == "wifi" || v[1] == "power") {
			return "may cut the Wi-Fi connection or turn the phone off"
		}
	case "cmd":
		if len(v) > 1 && (v[1] == "wifi" || v[1] == "connectivity") && level != Green {
			return "may cut the Wi-Fi connection"
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
	return ""
}

func mentionsADBuddy(v []string) bool {
	for _, s := range v {
		if strings.Contains(s, ADBuddyPackage) {
			return true
		}
	}
	return false
}
