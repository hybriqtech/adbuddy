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
	})
}

// settings [--user U] put [--user U] NAMESPACE KEY VALUE
func checkSettingsPut(ws []Word) checkResult {
	put := indexOf(ws, "put")
	if put < 0 {
		return red("cannot read settings arguments")
	}
	pos, user, good := positional(ws, put+1)
	if pre, u, _ := positional(ws[:put], 1); len(pre) == 0 && u != "" {
		user = u
	}
	if !good || len(pos) != 3 {
		return red("settings put with extra or missing arguments")
	}
	if pos[0] != "system" && pos[0] != "secure" && pos[0] != "global" {
		return red("unknown settings namespace " + pos[0])
	}
	op := newOp("settings-put", "ns", pos[0], "key", pos[1], "value", pos[2])
	if user != "" {
		op.Args["user"] = user
	}
	return change(op)
}

// pm disable-user|disable [--user U] PACKAGE
func checkPmDisable(ws []Word) checkResult {
	pos, user, good := positional(ws, 2)
	if !good || len(pos) != 1 {
		return red("pm disable with unexpected arguments")
	}
	op := newOp("pm-disable", "pkg", pos[0])
	if user != "" {
		op.Args["user"] = user
	}
	return change(op)
}

// pm uninstall [-k] --user U PACKAGE. Without --user it removes the app for
// every user: never reversible.
func checkPmUninstallUser(ws []Word) checkResult {
	pos, user, good := positional(ws, 2, "-k")
	if !good || len(pos) != 1 {
		return red("pm uninstall with unexpected arguments")
	}
	if user == "" {
		return red("uninstalls the app for every user")
	}
	return change(newOp("pm-uninstall-user", "pkg", pos[0], "user", user))
}

// appops set [--user U] PACKAGE OP MODE
func checkAppopsSet(ws []Word) checkResult {
	pos, user, good := positional(ws, 2)
	if !good || len(pos) != 3 {
		return red("appops set with unexpected arguments")
	}
	op := newOp("appops-set", "pkg", pos[0], "op", pos[1], "mode", pos[2])
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
	return red("changes the battery optimisation state")
}

// mv SRC DST with absolute paths and no options.
func checkMv(ws []Word) checkResult {
	pos, user, good := positional(ws, 1)
	if !good || user != "" || len(pos) != 2 {
		return red("mv with options or several sources")
	}
	if !strings.HasPrefix(pos[0], "/") || !strings.HasPrefix(pos[1], "/") {
		return red("mv with relative paths")
	}
	return change(newOp("mv", "src", pos[0], "dst", pos[1]))
}

// mkdir [-p] DIR with an absolute path.
func checkMkdir(ws []Word) checkResult {
	pos, user, good := positional(ws, 1, "-p")
	if !good || user != "" || len(pos) != 1 || !strings.HasPrefix(pos[0], "/") {
		return red("mkdir with options, several folders or a relative path")
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
	return red("monkey sending random events")
}
