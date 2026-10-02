package classify

import "strings"

// Checks for commands that change the phone: they recognise the change
// (Op) so package precheck can check the phone and compute the inverse.

func init() {
	register(map[string]checkFunc{
		"settings-put":      checkSettingsPut,
		"pm-disable":        checkPmDisable,
		"pm-uninstall-user": checkPmUninstallUser,
		"appops-set":        checkAppopsSet,
		"deviceidle":        checkDeviceidle,
		"mv":                checkMv,
		"mkdir":             checkMkdir,
		"monkey":            checkMonkey,
		"am-start":          checkAmStart,
	})
}

// validUser accepts the user ids a change can be undone for: a number or
// "current". "all" and "-1" mean every user and are never reversible.
func validUser(u string) bool {
	return u == "current" || u == "cur" || isDigits(u)
}

// packageName rejects UIDs ("10234", "u0a234") and special user names that
// appops and pm also accept: the change would hit every package of a UID.
func packageName(s string) bool {
	if !strings.Contains(s, ".") || strings.HasPrefix(s, "-") || isDigits(s) {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_') {
			return false
		}
	}
	return true
}

// userOpt reads an optional "--user U" at ws[i]. Android reads --user only
// in fixed positions, so any other place makes the command 🔴.
func userOpt(ws []Word, i int) (user string, next int, valid bool) {
	if i < len(ws) && ws[i].Value == "--user" {
		if i+1 >= len(ws) || !validUser(ws[i+1].Value) {
			return "", i, false
		}
		return ws[i+1].Value, i + 2, true
	}
	return "", i, true
}

// settings [--user U] put [--user U] NAMESPACE KEY VALUE (one --user at most)
func checkSettingsPut(ws []Word) checkResult {
	user1, i, valid := userOpt(ws, 1)
	if !valid || i >= len(ws) || !strings.EqualFold(ws[i].Value, "put") {
		return red(reason("settings put with --user in an unexpected place"))
	}
	user2, i, valid := userOpt(ws, i+1)
	if !valid || (user1 != "" && user2 != "") {
		return red(reason("settings put with --user in an unexpected place"))
	}
	rest := values(ws[i:])
	if len(rest) != 3 {
		return red(reason("settings put with extra or missing arguments"))
	}
	for _, v := range rest {
		if strings.HasPrefix(v, "-") {
			return red(reason("settings put with options after the namespace"))
		}
	}
	if rest[0] != "system" && rest[0] != "secure" && rest[0] != "global" {
		return red(reason("unknown settings namespace {arg}", "arg", rest[0]))
	}
	op := newOp("settings-put", "ns", rest[0], "key", rest[1], "value", rest[2])
	if u := user1 + user2; u != "" {
		op.Args["user"] = u
	}
	return change(op)
}

// pm disable-user|disable [--user U] PACKAGE: pm reads --user only as the
// first option and ignores words after the package.
func checkPmDisable(ws []Word) checkResult {
	user, i, valid := userOpt(ws, 2)
	if !valid || len(ws) != i+1 || !packageName(ws[i].Value) {
		return red(reason("pm disable with unexpected arguments"))
	}
	op := newOp("pm-disable", "pkg", ws[i].Value)
	if user != "" {
		op.Args["user"] = user
	}
	return change(op)
}

// pm uninstall [-k] --user U PACKAGE: options before the package, the
// package last. Without --user it removes the app for every user.
func checkPmUninstallUser(ws []Word) checkResult {
	user, keep := "", false
	i := 2
	for i < len(ws) && strings.HasPrefix(ws[i].Value, "-") {
		switch ws[i].Value {
		case "-k":
			keep = true
			i++
		case "--user":
			if user != "" || i+1 >= len(ws) || !validUser(ws[i+1].Value) {
				return red(reason("pm uninstall for every user or with an unexpected --user"))
			}
			user = ws[i+1].Value
			i += 2
		default:
			return red(reason("pm uninstall with unexpected options"))
		}
	}
	if len(ws) != i+1 || !packageName(ws[i].Value) {
		return red(reason("pm uninstall with unexpected arguments"))
	}
	if user == "" {
		return red(reason("uninstalls the app for every user"))
	}
	op := newOp("pm-uninstall-user", "pkg", ws[i].Value, "user", user)
	if keep {
		op.Args["keep"] = "1"
	}
	return change(op)
}

// appops set [--user U] PACKAGE OP MODE: --user only before the package, and
// a package name (not a UID).
func checkAppopsSet(ws []Word) checkResult {
	user, i, valid := userOpt(ws, 2)
	rest := values(ws[i:])
	if !valid || len(rest) != 3 {
		return red(reason("appops set with unexpected arguments"))
	}
	if !packageName(rest[0]) {
		return red(reason("appops set on a UID changes every app that shares it"))
	}
	for _, v := range rest[1:] {
		if strings.HasPrefix(v, "-") {
			return red(reason("appops set with unexpected options"))
		}
	}
	op := newOp("appops-set", "pkg", rest[0], "op", rest[1], "mode", rest[2])
	if user != "" {
		op.Args["user"] = user
	}
	return change(op)
}

// dumpsys deviceidle [whitelist [+pkg|-pkg]]
func checkDeviceidle(ws []Word) checkResult {
	rest := values(ws[2:])
	switch {
	case len(rest) == 0, len(rest) == 1 && rest[0] == "whitelist":
		return ok()
	case len(rest) == 2 && rest[0] == "whitelist" && len(rest[1]) > 1 && (rest[1][0] == '+' || rest[1][0] == '-'):
		return change(newOp("deviceidle", "sign", rest[1][:1], "pkg", rest[1][1:]))
	}
	return red(reason("changes the battery optimisation state"))
}

// mv SRC DST with absolute paths and no options.
func checkMv(ws []Word) checkResult {
	pos, user, good := positional(ws, 1)
	if !good || user != "" || len(pos) != 2 {
		return red(reason("mv with options or several sources"))
	}
	if !strings.HasPrefix(pos[0], "/") || !strings.HasPrefix(pos[1], "/") {
		return red(reason("mv with relative paths"))
	}
	return change(newOp("mv", "src", pos[0], "dst", pos[1]))
}

// mkdir [-p] DIR with an absolute path.
func checkMkdir(ws []Word) checkResult {
	pos, user, good := positional(ws, 1, "-p")
	if !good || user != "" || len(pos) != 1 || !strings.HasPrefix(pos[0], "/") {
		return red(reason("mkdir with options, several folders or a relative path"))
	}
	return change(newOp("mkdir", "path", pos[0]))
}

// monkey -p PACKAGE [-c CATEGORY] 1: the usual way to open an app.
func checkMonkey(ws []Word) checkResult {
	v := values(ws)
	if len(v) == 4 && v[1] == "-p" && v[3] == "1" {
		return checkResult{level: Green, vars: map[string]string{"pkg": v[2]}}
	}
	if len(v) == 6 && v[1] == "-p" && v[3] == "-c" && v[5] == "1" {
		return checkResult{level: Green, vars: map[string]string{"pkg": v[2]}}
	}
	return red(reason("monkey sending random events"))
}

// am start is a harmless interface action, except for intents that place a
// call, restart or wipe the phone, or uninstall an app.
func checkAmStart(ws []Word) checkResult {
	for _, w := range ws[2:] {
		u := strings.ToUpper(w.Value)
		for _, bad := range []string{"ACTION.CALL", "CALL_PRIVILEGED", "CALL_EMERGENCY", "REBOOT", "SHUTDOWN", "MASTER_CLEAR", "FACTORY_RESET", "ACTION.DELETE", "UNINSTALL_PACKAGE"} {
			if strings.Contains(u, bad) {
				return red(reason("the intent {value} can call, restart, wipe or uninstall", "value", w.Value))
			}
		}
	}
	return ok()
}
