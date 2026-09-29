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
		switch {
		case strings.HasPrefix(name, "reboot"):
			return "restarts the phone"
		case name == "remount":
			return "may restart the phone"
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
			for _, w := range v[2:] {
				if sensitiveSettings[w] {
					return "setting " + w + " affects the connection, ADBuddy or the phone's security"
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
		// Uri.getAuthority() decodes %xx, so compare the decoded words.
		if level != Green && strings.Contains(strings.ToLower(percentDecode(strings.Join(v, " "))), "settings") {
			return "changes system settings directly, including the connection ones"
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
	if level != Green && touchesCriticalApp(name, ws, v) {
		return "affects an app the phone needs to stay connected or to start"
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
