package classify

import "strings"

// Checks for read-only commands: they stay 🟢 unless their arguments make
// them write files, run commands or change state.

func init() {
	register(map[string]checkFunc{
		"find":             checkFind,
		"sed":              checkSed,
		"awk":              checkAwk,
		"sort":             checkSort,
		"uniq":             checkUniq,
		"date":             checkDate,
		"dumpsys":          checkDumpsys,
		"ip":               checkIP,
		"ifconfig":         checkIfconfig,
		"hostname":         checkHostname,
		"tee":              checkTee,
		"no-extra-args":    checkNoExtraArgs,
		"set":              checkSet,
		"screencap":        checkScreencap,
		"uiautomator-dump": checkUiautomatorDump,
	})
}

func checkFind(ws []Word) checkResult {
	for _, w := range ws[1:] {
		switch w.Value {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls":
			return red("find " + w.Value + " changes files or runs commands")
		}
	}
	return ok()
}

// sed is 🟢 only when it cannot write files or run commands.
func checkSed(ws []Word) checkResult {
	var scripts []string
	var files []string
	for i := 1; i < len(ws); i++ {
		v := ws[i].Value
		switch {
		case v == "-i" || strings.HasPrefix(v, "-i") || strings.HasPrefix(v, "--in-place"):
			return red("sed -i edits files in place")
		case v == "-f" || strings.HasPrefix(v, "--file"):
			return red("sed script from a file")
		case v == "-e" || v == "--expression":
			if i+1 >= len(ws) || ws[i+1].Dynamic {
				return red("sed script only known on the phone")
			}
			scripts = append(scripts, ws[i+1].Value)
			i++
		case strings.HasPrefix(v, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(v, "--expression="))
		case v == "-n" || v == "-E" || v == "-r" || v == "-s" || v == "-z" || v == "-u" || v == "--quiet" || v == "--silent":
		case strings.HasPrefix(v, "-") && v != "-":
			return red("unknown sed option " + v)
		default:
			if ws[i].Dynamic && len(scripts) == 0 && len(files) == 0 {
				return red("sed script only known on the phone")
			}
			files = append(files, v)
		}
	}
	if len(scripts) == 0 {
		if len(files) == 0 {
			return red("sed without script")
		}
		scripts = files[:1]
	}
	for _, s := range scripts {
		if !safeSed(s) {
			return red("sed script that may write files or run commands")
		}
	}
	return ok()
}

// safeSed accepts scripts made of print/delete/quit/substitute commands and
// friends; w, W, r, R, e and the s///w and s///e flags are rejected.
func safeSed(s string) bool {
	i := 0
	skipSpace := func() {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == ';') {
			i++
		}
	}
	readRegex := func(delim byte) bool {
		for i < len(s) {
			switch s[i] {
			case '\\':
				i += 2
				continue
			case delim:
				i++
				return true
			}
			i++
		}
		return false
	}
	readAddr := func() bool {
		switch {
		case i < len(s) && s[i] == '$':
			i++
		case i < len(s) && s[i] >= '0' && s[i] <= '9':
			for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '~') {
				i++
			}
		case i < len(s) && s[i] == '/':
			i++
			if !readRegex('/') {
				return false
			}
			for i < len(s) && (s[i] == 'I' || s[i] == 'M') {
				i++
			}
		}
		return true
	}
	for {
		skipSpace()
		if i >= len(s) {
			return true
		}
		if !readAddr() {
			return false
		}
		if i < len(s) && s[i] == ',' {
			i++
			if !readAddr() {
				return false
			}
		}
		for i < len(s) && (s[i] == ' ' || s[i] == '!') {
			i++
		}
		if i >= len(s) {
			return false
		}
		cmd := s[i]
		i++
		switch cmd {
		case 'p', 'd', 'q', 'Q', '=', 'n', 'N', 'P', 'D', 'h', 'H', 'g', 'G', 'x', 'l', '{', '}':
		case 's', 'y':
			if i >= len(s) {
				return false
			}
			delim := s[i]
			if delim == '\\' || delim == '\n' {
				return false
			}
			i++
			if !readRegex(delim) || !readRegex(delim) {
				return false
			}
			if cmd == 's' {
				for i < len(s) && strings.IndexByte("gipI0123456789", s[i]) >= 0 {
					i++
				}
			}
		default:
			return false
		}
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i < len(s) && s[i] != ';' && s[i] != '\n' && s[i] != '}' {
			return false
		}
	}
}

// awk is 🟢 unless the program can run commands or write files.
func checkAwk(ws []Word) checkResult {
	for i := 1; i < len(ws); i++ {
		v := ws[i].Value
		switch {
		case v == "-f":
			return red("awk program from a file")
		case v == "-F" || v == "-v":
			i++
		case strings.HasPrefix(v, "-F") || strings.HasPrefix(v, "-v"):
		case strings.HasPrefix(v, "-"):
			return red("unknown awk option " + v)
		default:
			if ws[i].Dynamic {
				return red("awk program only known on the phone")
			}
			for _, bad := range []string{"system", "getline", ">", "|", "close(", "fflush"} {
				if strings.Contains(v, bad) {
					return red("awk program that may run commands or write files")
				}
			}
			return ok()
		}
	}
	return red("awk without program")
}

func checkSort(ws []Word) checkResult {
	for _, w := range ws[1:] {
		if w.Value == "-o" || strings.HasPrefix(w.Value, "--output") || (strings.HasPrefix(w.Value, "-o") && !strings.HasPrefix(w.Value, "--")) {
			return red("sort -o writes a file")
		}
	}
	return ok()
}

func checkUniq(ws []Word) checkResult {
	n := 0
	for _, w := range ws[1:] {
		if !strings.HasPrefix(w.Value, "-") || w.Value == "-" {
			n++
		}
	}
	if n > 1 {
		return red("uniq with an output file")
	}
	return ok()
}

func checkDate(ws []Word) checkResult {
	for _, w := range ws[1:] {
		v := w.Value
		if strings.HasPrefix(v, "+") || v == "-u" || v == "--utc" || v == "-R" || v == "-I" {
			continue
		}
		return red("date with arguments can change the clock")
	}
	return ok()
}

// dumpsys is 🟢 unless it passes a command that changes state.
func checkDumpsys(ws []Word) checkResult {
	for _, w := range ws[2:] {
		switch w.Value {
		case "set", "reset", "unplug", "enable", "disable", "clear", "put", "delete", "remove", "add",
			"force-idle", "force-inactive", "unforce", "step", "whitelist", "tempwhitelist",
			"sys-whitelist", "except-idle-whitelist", "--reset", "suspend", "kill", "stop", "start":
			return red("dumpsys command that changes state")
		}
	}
	return ok()
}

func checkIP(ws []Word) checkResult {
	for _, w := range ws[1:] {
		switch w.Value {
		case "add", "del", "delete", "change", "replace", "set", "flush", "append", "prepend", "save", "restore", "exec", "up", "down":
			return red("ip command that changes the network")
		}
	}
	return ok()
}

func checkIfconfig(ws []Word) checkResult {
	if len(ws) > 2 {
		return red("ifconfig changing an interface")
	}
	return ok()
}

func checkHostname(ws []Word) checkResult {
	for _, w := range ws[1:] {
		if !contains([]string{"-s", "-d", "-f", "-i", "-I"}, w.Value) {
			return red("hostname changing the name")
		}
	}
	return ok()
}

func checkTee(ws []Word) checkResult {
	for _, w := range ws[1:] {
		if w.Value != "-a" && w.Value != "/dev/null" {
			return red("tee writes files")
		}
	}
	return ok()
}

// checkNoExtraArgs allows only `<cmd> <sub>`: e.g. `wm size` reads, `wm size 1080x1920` writes.
func checkNoExtraArgs(ws []Word) checkResult {
	if len(ws) > 2 {
		return red("changes the value instead of reading it")
	}
	return ok()
}

// newFile handles tools that write their output to an optional file argument.
func newFile(pos []string) checkResult {
	switch {
	case len(pos) == 0:
		return ok()
	case len(pos) == 1 && strings.HasPrefix(pos[0], "/"):
		return change(newOp("new-file", "path", pos[0]))
	}
	return red("writes to a relative path or several files")
}

// screencap [-p] [-j] [-d ID] [FILE]
func checkScreencap(ws []Word) checkResult {
	var pos []string
	for i := 1; i < len(ws); i++ {
		switch v := ws[i].Value; {
		case v == "-d":
			i++
		case v == "-p" || v == "-j" || v == "-a":
		case strings.HasPrefix(v, "-"):
			return red("unknown screencap option")
		default:
			pos = append(pos, v)
		}
	}
	return newFile(pos)
}

// uiautomator dump [--compressed] [FILE]
func checkUiautomatorDump(ws []Word) checkResult {
	pos, user, good := positional(ws, 2, "--compressed")
	if !good || user != "" {
		return red("unknown uiautomator option")
	}
	return newFile(pos)
}

// checkSet refuses the shell options that turn brace expansion on: with it a
// word such as {5,'reboot'} becomes two words on the phone.
func checkSet(ws []Word) checkResult {
	for i := 1; i < len(ws); i++ {
		v := ws[i].Value
		if v == "--" || v == "-" || len(v) < 2 || (v[0] != '-' && v[0] != '+') {
			return ok() // the rest are positional parameters
		}
		on, flags := v[0] == '-', v[1:]
		if on && strings.Contains(flags, "B") {
			return red("set -B turns brace expansion on")
		}
		k := strings.IndexByte(flags, 'o')
		if k < 0 {
			continue
		}
		name := flags[k+1:]
		if name == "" {
			if i+1 >= len(ws) {
				return ok() // set -o lists the options
			}
			i++
			name = ws[i].Value
		}
		if (on && name == "braceexpand") || (!on && (name == "posix" || name == "sh")) {
			return red("set " + v[:1] + "o " + name + " turns brace expansion on")
		}
	}
	return ok()
}
