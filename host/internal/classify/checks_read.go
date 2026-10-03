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
		"pgrep":            checkPgrep,
		"ss":               checkSs,
		"date":             checkDate,
		"dumpsys":          checkDumpsys,
		"ip":               checkIP,
		"ifconfig":         checkIfconfig,
		"hostname":         checkHostname,
		"tee":              checkTee,
		"no-extra-args":    checkNoExtraArgs,
		"set":              checkSet,
		"printf":           checkPrintf,
		"screencap":        checkScreencap,
		"uiautomator-dump": checkUiautomatorDump,
	})
}

func checkFind(ws []Word) checkResult {
	for _, w := range ws[1:] {
		switch w.Value {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls":
			return red(reason("find {value} changes files or runs commands", "value", w.Value))
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
			return red(reason("sed -i edits files in place"))
		case v == "-f" || strings.HasPrefix(v, "--file"):
			return red(reason("sed script from a file"))
		case v == "-e" || v == "--expression":
			if i+1 >= len(ws) || ws[i+1].Dynamic {
				return red(reason("sed script only known on the phone"))
			}
			scripts = append(scripts, ws[i+1].Value)
			i++
		case strings.HasPrefix(v, "--expression="):
			scripts = append(scripts, strings.TrimPrefix(v, "--expression="))
		case v == "-n" || v == "-E" || v == "-r" || v == "-s" || v == "-z" || v == "-u" || v == "--quiet" || v == "--silent":
		case strings.HasPrefix(v, "-") && v != "-":
			return red(reason("unknown sed option {v}", "v", v))
		default:
			if ws[i].Dynamic && len(scripts) == 0 && len(files) == 0 {
				return red(reason("sed script only known on the phone"))
			}
			files = append(files, v)
		}
	}
	if len(scripts) == 0 {
		if len(files) == 0 {
			return red(reason("sed without script"))
		}
		scripts = files[:1]
	}
	for _, s := range scripts {
		if !safeSed(s) {
			return red(reason("sed script that may write files or run commands"))
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
			return red(reason("awk program from a file"))
		case v == "-F" || v == "-v":
			i++
		case strings.HasPrefix(v, "-F") || strings.HasPrefix(v, "-v"):
		case strings.HasPrefix(v, "-"):
			return red(reason("unknown awk option {v}", "v", v))
		default:
			if ws[i].Dynamic {
				return red(reason("awk program only known on the phone"))
			}
			for _, bad := range []string{"system", "getline", ">", "|", "close(", "fflush"} {
				if strings.Contains(v, bad) {
					return red(reason("awk program that may run commands or write files"))
				}
			}
			return ok()
		}
	}
	return red(reason("awk without program"))
}

// toybox sort (confirmed with sort --help in the emulator) has no long
// options. -k, -t and -o take a value; -S and -T are accepted and take one;
// -h is not a toybox option, but it is the usual spelling of human sizes and
// only fails. Anything else is unknown, so 🔴.
var sortOptions = getoptSpec{flags: "bCcdfgiMnrsuxVzh", values: "ktoST", strict: true}

// sort writes to a file with -o, alone, grouped (-uo), glued (-ofile) or in the
// next word. -to (separator o) and -k2,2o (key modifier) are not output.
func checkSort(ws []Word) checkResult {
	r := sortOptions.parse(ws[1:])
	if r.bad != "" {
		return red(reason("sort option {v} may write a file", "v", r.bad))
	}
	if _, o := r.seen['o']; o {
		return red(reason("sort -o writes a file"))
	}
	return ok()
}

// uniq INPUT OUTPUT writes OUTPUT; -f, -s and -w take a value; after -- every
// word is a file.
var uniqOptions = getoptSpec{flags: "cduiz", values: "fsw", strict: true}

func checkUniq(ws []Word) checkResult {
	r := uniqOptions.parse(ws[1:])
	if r.bad != "" {
		return red(reason("uniq option {v} may write a file", "v", r.bad))
	}
	// toybox rejects a value that is not a number ("uniq: not integer: a",
	// checked in the emulator) before it acts, but a value that is not one is
	// not trusted either.
	for _, c := range "fsw" {
		if v, set := r.seen[byte(c)]; set && !isDigits(v) {
			return red(reason("uniq option {v} may write a file", "v", "-"+string(c)+v))
		}
	}
	if len(r.pos) > 1 {
		return red(reason("uniq with an output file"))
	}
	return ok()
}

func checkDate(ws []Word) checkResult {
	for _, w := range ws[1:] {
		v := w.Value
		if strings.HasPrefix(v, "+") || v == "-u" || v == "--utc" || v == "-R" || v == "-I" {
			continue
		}
		return red(reason("date with arguments can change the clock"))
	}
	return ok()
}

// dumpsys is 🟢 only in a form known to read (v1 V06): many services take
// commands among their arguments (battery set, jobscheduler run, thermalservice
// override-status, batterystats --reset…), so the arguments go through an
// allowlist. dumpsys's own options, a service alone and, for a few services,
// the arguments listed below are reading; anything else is 🔴.
//
// --noredact / --reveal (in any position, with or without a service)
// prints the text of every app's notifications: 🔴. So is any dump of the
// notification service (dumpsysNotifications): redacted, it still shows the
// start of each ticker (T2 of the manual validations, M5b-18).
func checkDumpsys(ws []Word) checkResult {
	v := values(ws)
	if revealsNotifications(v) {
		return red(reason("dumpsys notification text of every app"))
	}
	svc, args, bad := dumpsysSplit(v)
	switch {
	case bad != "":
		if strings.HasPrefix(bad, "--reset") || strings.HasPrefix(bad, "--clear") {
			return red(reason("dumpsys option that clears data"))
		}
		return red(reason("unknown dumpsys option {v}", "v", bad))
	case dumpsysNotifications(v) && svc == "notification":
		return red(reason("dumpsys notification shows the start of each notification's ticker, codes included"))
	case dumpsysNotifications(v):
		return red(reason("dumpsys of every service includes the notifications, with the start of each ticker"))
	case svc == "":
		return ok() // every service, without arguments
	case dumpsysVerbs[svc]:
		return red(reason("dumpsys command that changes state"))
	case svc == "deviceidle":
		// The deviceidle rule reads it when it comes right after dumpsys.
		return red(reason("dumpsys options before deviceidle"))
	case len(args) == 0:
		return ok()
	}
	if reads := dumpsysReads[svc]; reads != nil && reads(args) {
		return ok()
	}
	return red(reason("dumpsys {value} with arguments that are not a known way of reading it", "value", svc))
}

// dumpsysNotifications reports whether a dumpsys line, with only options
// dumpsys has, prints the notification service's dump. The redacted one keeps
// the first 8 characters of a ticker longer than 16 ("tickerText=G-482913...",
// measured on the API 34 emulator by emutest's TestNotificationReadsAndTheCodes:
// T2 of the manual validations, M5b-18 / M5c-35); a code often starts it.
// dumpsys (frameworks/native/cmds/dumpsys) reads its options up to the
// service: --help prints its usage and -l the list of services, whatever
// follows; --pid, --thread, --clients and --stability print that instead of
// the dump unless --dump asks for it too; with a service it dumps that one,
// and without one every service of --priority (all, without it) but those
// after --skip. Measured: the notification service puts its records in its
// CRITICAL dump, not in the HIGH or NORMAL ones. A --priority without a value
// or with another one counts as dumping them.
func dumpsysNotifications(v []string) bool {
	dump, info, skip := false, false, false
	priority := ""
	var rest []string
loop:
	for i := 1; i < len(v); i++ {
		switch a := v[i]; {
		case a == "--":
			rest = v[i+1:]
			break loop
		case !strings.HasPrefix(a, "-") || a == "-":
			rest = v[i:]
			break loop
		case a == "--help" || a == "-l":
			return false
		case a == "--priority":
			if i+1 < len(v) {
				priority = v[i+1]
			}
			i++
		case strings.HasPrefix(a, "--priority="):
			priority = strings.TrimPrefix(a, "--priority=")
		case a == "-t" || a == "-T":
			i++
		case a == "--skip":
			skip = true
		case a == "--dump":
			dump = true
		case a == "--pid" || a == "--thread" || a == "--clients" || a == "--stability":
			info = true
		}
	}
	switch {
	case info && !dump:
		return false
	case skip:
		if contains(rest, "notification") {
			return false
		}
	case len(rest) > 0:
		return rest[0] == "notification"
	}
	return priority != "HIGH" && priority != "NORMAL"
}

// dumpsysFlags are dumpsys's own options without a value; dumpsysValued take
// one. dumpsys reads them with getopt_long("+t:T:l"), so they end at the first
// word that is not an option: the service. A long option abbreviated, which
// getopt_long would also accept, is not in the list and is 🔴.
var (
	dumpsysFlags = map[string]bool{
		"-l": true, "--help": true, "--proto": true, "--skip": true, "--clients": true,
		"--dump": true, "--pid": true, "--thread": true, "--stability": true,
	}
	dumpsysValued = map[string]bool{"-t": true, "-T": true, "--priority": true}
)

// dumpsysSplit separates dumpsys's own options from the service and its
// arguments. bad is the first option dumpsys does not have. With --skip the
// words after the options are services to leave out: no service is dumped
// with arguments.
func dumpsysSplit(v []string) (svc string, args []string, bad string) {
	skip := false
	for i := 1; i < len(v); i++ {
		a := v[i]
		switch {
		case a == "--":
			if skip || i+1 >= len(v) {
				return "", nil, ""
			}
			return v[i+1], v[i+2:], ""
		case !strings.HasPrefix(a, "-") || a == "-":
			if skip {
				return "", nil, ""
			}
			return a, v[i+1:], ""
		case dumpsysValued[a]:
			i++
		case (strings.HasPrefix(a, "-t") || strings.HasPrefix(a, "-T")) && isDigits(a[2:]),
			strings.HasPrefix(a, "--priority="):
		case dumpsysFlags[a]:
			skip = skip || a == "--skip"
		default:
			return "", nil, a
		}
	}
	return "", nil, ""
}

// dumpsysVerbs are words that name a command, not a service.
var dumpsysVerbs = map[string]bool{
	"set": true, "reset": true, "unplug": true, "enable": true, "disable": true, "clear": true,
	"put": true, "delete": true, "remove": true, "add": true, "force": true, "force-idle": true,
	"force-inactive": true, "unforce": true, "step": true, "whitelist": true, "tempwhitelist": true,
	"sys-whitelist": true, "except-idle-whitelist": true, "suspend": true, "kill": true, "stop": true,
	"start": true,
}

// dumpsysReads lists, for the services whose arguments are commonly used to
// read, the arguments known to read. Any other service is 🟢 only alone.
// --checkin is left out of procstats, batterystats and activity: there it
// consumes state (procstats renames its saved files, batterystats deletes its
// checkin file, activity clears the broadcast statistics); -c only prints.
var dumpsysReads = map[string]func(args []string) bool{
	"activity": dumpsysActivity,
	"package": wordsOrNames("packages", "p", "permissions", "perm", "permission", "features", "f",
		"libraries", "l", "preferred", "pref", "preferred-xml", "providers", "prov", "messages", "m",
		"verifiers", "v", "version", "installs", "shared-users", "s", "queries", "q",
		"domain-preferred-apps", "d", "keysets", "k", "frozen", "volumes", "dexopt", "compiler-stats",
		"changes", "service-permissions", "known-packages", "-f", "-a", "-h", "--checkin", "--all-components"),
	"meminfo": wordsOrNames("-a", "-d", "-c", "-s", "-S", "-h", "--oom", "--local", "--package",
		"--checkin", "--proto", "--unreachable"),
	"gfxinfo":      wordsOrNames("framestats"),
	"procstats":    wordsOrNames("--hours", "--last", "--current", "--details", "--full-details", "--active", "-a", "-c", "-h"),
	"batterystats": wordsOrNames("--charged", "-c", "--history", "--daily", "--proto", "-h"),
	"window": wordsOrNames("windows", "w", "displays", "d", "tokens", "t", "policy", "p", "animator", "a",
		"sessions", "s", "lastanr", "l", "input", "visible", "visible-apps", "containers", "-a", "-h"),
}

// wordsOrNames accepts arguments that are all in words or are names: a
// package, a process, a component or a number (a PID, a UID, hours…).
func wordsOrNames(words ...string) func([]string) bool {
	return func(args []string) bool {
		for _, a := range args {
			if !contains(words, a) && !dumpsysName(a) {
				return false
			}
		}
		return true
	}
}

// dumpsysName is a package or process name (com.x, com.x:remote), a component
// (com.x/.Main) or a number.
func dumpsysName(s string) bool {
	if isDigits(s) {
		return true
	}
	name, class, isComponent := strings.Cut(s, "/")
	if strings.HasPrefix(name, "-") || !strings.Contains(name, ".") || !nameChars(name, ":") {
		return false
	}
	return !isComponent || (class != "" && nameChars(class, "$"))
}

func nameChars(s, extra string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || strings.ContainsRune(extra, r)) {
			return false
		}
	}
	return true
}

// activitySections are the parts of dumpsys activity that only print.
var activitySections = []string{
	"activities", "a", "recents", "r", "broadcasts", "b", "broadcast-stats", "intents", "i",
	"permissions", "perm", "processes", "p", "oom", "o", "lmk", "lru", "services", "s", "providers",
	"prov", "top", "starter", "containers", "lastanr", "lastanr-traces", "associations", "as",
	"exit-info", "users", "settings", "binder-proxies",
	"-a", "-c", "-v", "-f", "-h", "--proto",
}

// dumpsysActivity: sections and names; `service NAME` and `provider NAME`
// hand the rest of the line to that service's or provider's own dump, so
// nothing may follow the name.
func dumpsysActivity(args []string) bool {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "service" || a == "provider":
			return i == len(args)-2 && dumpsysName(args[i+1])
		case a == "-p":
			if i+1 >= len(args) || !dumpsysName(args[i+1]) {
				return false
			}
			i++
		case !contains(activitySections, a) && !dumpsysName(a):
			return false
		}
	}
	return true
}

// toybox pgrep (confirmed with pgrep --help in the emulator): -L SIGNAL sends
// the signal to what it finds ("pgrep -L 9 -x sleep" killed the sleep), so L is
// 🔴 whatever the way it is written. The other options only select or print.
// It has no long options.
var pgrepOptions = getoptSpec{flags: "aclfnovx", values: "dLGgPstUu", strict: true}

func checkPgrep(ws []Word) checkResult {
	r := pgrepOptions.parse(ws[1:])
	if r.bad != "" {
		return red(reason("pgrep option {v} may send a signal", "v", r.bad))
	}
	if _, l := r.seen['L']; l {
		return red(reason("pgrep -L sends a signal to the processes"))
	}
	return ok()
}

// ss (iproute2, confirmed with ss --help in the emulator): -D FILE (--diag)
// writes a file, -K (--kill) closes sockets, -F FILE reads the filter from a
// file and -N switches namespace. They are not listed, so they are unknown
// and 🔴; so is -E, which never ends. -A and -f take a value.
var ssOptions = getoptSpec{
	flags:  "hVnrlaoempisbZz460tSudwxH",
	values: "Af",
	longFlags: map[string]bool{
		"--help": true, "--version": true, "--numeric": true, "--resolve": true, "--all": true, "--listening": true,
		"--options": true, "--extended": true, "--memory": true, "--processes": true, "--info": true, "--summary": true,
		"--bpf": true, "--context": true, "--contexts": true, "--ipv4": true, "--ipv6": true, "--packet": true,
		"--tcp": true, "--sctp": true, "--udp": true, "--dccp": true, "--raw": true, "--unix": true, "--no-header": true,
	},
	longValues: map[string]bool{"--query": true, "--socket": true, "--family": true},
	strict:     true,
}

func checkSs(ws []Word) checkResult {
	if r := ssOptions.parse(ws[1:]); r.bad != "" {
		return red(reason("ss option {v} may write a file or close sockets", "v", r.bad))
	}
	return ok()
}

// ip objects that only have a reading side. iproute2 accepts any prefix of an
// object (ip a, ip l, ip r), so a word is an object if it is one.
var ipObjects = []string{"address", "link", "route", "rule", "neighbor", "neighbour", "netconf", "maddress", "mroute"}

// ip options that only change how the answer is printed. -b/-batch (read
// commands from a file or stdin), -force, -n/-netns, -f and the rest are 🔴.
var ipOptions = []string{
	"-4", "-6", "-s", "-d", "-o", "-j", "-p", "-br", "-c",
	"-stats", "-statistics", "-details", "-oneline", "-json", "-pretty", "-brief", "-color",
}

// ipReadVerbs: the verb is read exactly (an abbreviation such as s can be set,
// save or show depending on the object). No verb shows.
var ipReadVerbs = []string{"show", "list", "get"}

// checkIP: ip [read-only options] OBJECT [show|list|get ...]; everything else
// is 🔴, whether it is an unknown option, an abbreviated or unlisted object or
// any verb that is not reading.
func checkIP(ws []Word) checkResult {
	i := 1
	for ; i < len(ws) && strings.HasPrefix(ws[i].Value, "-"); i++ {
		if !contains(ipOptions, ws[i].Value) {
			return red(reason("ip option {v} is not a known way of reading the network", "v", ws[i].Value))
		}
	}
	if i == len(ws) {
		return ok()
	}
	if !isIPObject(ws[i].Value) {
		return red(reason("ip {v} is not a known way of reading the network", "v", ws[i].Value))
	}
	i++
	if i == len(ws) || contains(ipReadVerbs, ws[i].Value) {
		return ok()
	}
	return red(reason("ip command that changes the network"))
}

func isIPObject(w string) bool {
	if w == "" || strings.HasPrefix(w, "-") {
		return false
	}
	for _, o := range ipObjects {
		if strings.HasPrefix(o, w) {
			return true
		}
	}
	return false
}

func checkIfconfig(ws []Word) checkResult {
	if len(ws) > 2 {
		return red(reason("ifconfig changing an interface"))
	}
	return ok()
}

func checkHostname(ws []Word) checkResult {
	for _, w := range ws[1:] {
		if !contains([]string{"-s", "-d", "-f", "-i", "-I"}, w.Value) {
			return red(reason("hostname changing the name"))
		}
	}
	return ok()
}

func checkTee(ws []Word) checkResult {
	for _, w := range ws[1:] {
		if w.Value != "-a" && w.Value != "/dev/null" {
			return red(reason("tee writes files"))
		}
	}
	return ok()
}

// checkNoExtraArgs allows only `<cmd> <sub>`: e.g. `wm size` reads, `wm size 1080x1920` writes.
func checkNoExtraArgs(ws []Word) checkResult {
	if len(ws) > 2 {
		return red(reason("changes the value instead of reading it"))
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
	return red(reason("writes to a relative path or several files"))
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
			return red(reason("unknown screencap option"))
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
		return red(reason("unknown uiautomator option"))
	}
	return newFile(pos)
}

// checkPrintf refuses `printf -v VAR …`, which stores the output in a shell
// variable instead of printing it and can therefore set PATH and friends.
func checkPrintf(ws []Word) checkResult {
	for i := 1; i < len(ws); i++ {
		v := ws[i].Value
		if v == "--" {
			break
		}
		if v == "-v" {
			return red(reason("printf -v writes to a shell variable"))
		}
		if !strings.HasPrefix(v, "-") {
			break // the format string; the rest are arguments
		}
	}
	return ok()
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
		if strings.Contains(flags, "A") {
			// set -A / +A assigns to an array variable, e.g. set -A PATH ….
			return red(reason("set {sign}A assigns to a variable", "sign", v[:1]))
		}
		if on && strings.Contains(flags, "B") {
			return red(reason("set -B turns brace expansion on"))
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
			return red(reason("set {sign}o {name} turns brace expansion on", "sign", v[:1], "name", name))
		}
	}
	return ok()
}
