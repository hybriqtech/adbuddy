package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// Check statuses. A failed check stops ADBuddy from working; a warning
// leaves it working with something missing (degraded); a skipped check
// could not run because an earlier one failed.
const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
	checkSkip = "skip"
)

// check is one line of `adbuddy doctor`. Run is a single safe local command
// the person or the agent may run; Guidance is something only the person can
// do (on the phone, in another terminal); they are never both empty on a
// failure or a warning.
//
// Detail and Guidance are English: they are what --json, the agent and the
// tests read. They are computed, with the checks' own texts as messages
// (detail and guidance), by mk and withGuide; the human report writes those
// messages again in the language of the run. Name and Run are not translated:
// the name is an identifier of the JSON (the report shows it in the language
// of the run) and Run is a command.
type check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Run      string `json:"run,omitempty"`
	Guidance string `json:"guidance,omitempty"`

	detail, guidance i18n.Msg
}

// mk is a check whose detail is a message.
func mk(name, status string, detail i18n.Msg) check {
	return check{Name: name, Status: status, Detail: detail.String(), detail: detail}
}

// withRun adds the command to run.
func (c check) withRun(cmd string) check {
	c.Run = cmd
	return c
}

// withGuide adds what only the person can do, as a message.
func (c check) withGuide(m i18n.Msg) check {
	c.Guidance, c.guidance = m.String(), m
	return c
}

// detailMsg and guidanceMsg are the texts of c as messages. A check that was
// not built with mk (a test's) has only its English, which is its message.
func (c check) detailMsg() i18n.Msg {
	if c.detail.Key == "" {
		return i18n.M("doctor.plain", c.Detail)
	}
	return c.detail
}

func (c check) guidanceMsg() i18n.Msg {
	if c.guidance.Key == "" {
		return i18n.M("doctor.plain", c.Guidance)
	}
	return c.guidance
}

// doctorReport is the output of `adbuddy doctor`. fixed is Fixed as messages.
type doctorReport struct {
	Verdict string   `json:"verdict"` // ready, degraded or broken
	Fixed   []string `json:"fixed,omitempty"`
	Checks  []check  `json:"checks"`

	fixed []i18n.Msg
}

// msgError is an error whose text is a message of the catalog: doctor writes
// it in the language of the run, and Error is its English (what a log or a
// test reads). causeMsg is how a check takes an error as the cause in its text.
type msgError struct{ msg i18n.Msg }

func (e msgError) Error() string { return e.msg.String() }

func failf(key string, args ...any) error { return msgError{i18n.M(key, args...)} }

// causeMsg is err as a message: its own when it is a msgError, else its text
// as it is (a cause of the system or of another package is English and is not
// translated, spec ruling 10).
func causeMsg(err error) i18n.Msg {
	var me msgError
	if errors.As(err, &me) {
		return me.msg
	}
	return i18n.M("doctor.plain", err.Error())
}

// listMsg is a list of up to three items, written in the language of the run.
func listMsg(items ...i18n.Msg) i18n.Msg {
	args := make([]any, len(items))
	for i, m := range items {
		args[i] = m
	}
	return i18n.M("doctor.list."+strconv.Itoa(len(items)), args...)
}

// checkName is the name of a check for people, in lang (the JSON keeps the identifier).
func checkName(lang i18n.Lang, name string) string {
	id := strings.ReplaceAll(name, " ", "_")
	if i18n.Has("doctor.name." + id) {
		return lang.T("doctor.name." + id)
	}
	return name
}

// Doctor time limits: every call is bounded, and so is the whole of doctor
// (--fix included), so doctor always answers.
var (
	doctorCallWait  = 10 * time.Second // one adb call
	doctorPingWait  = 3 * time.Second  // serve's answer on its socket
	doctorTotalWait = 60 * time.Second // everything
	adbVersionRe    = regexp.MustCompile(`Version (\d+)\.(\d+)\.(\d+)`)
)

// personOnly are the Run commands the agent's session cannot run: they
// refuse ADBUDDY_SOURCE=agent or need the person in front of the phone. For
// the agent they become "Ask the user to run …".
var personOnly = []string{"adbuddy setup", "adbuddy serve", "adbuddy connect", "adbuddy doctor --fix"}

func newDoctorCmd(app *App) *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   app.T("help.doctor.short"),
		Long:    app.T("help.doctor.long"),
		Example: "  adbuddy doctor\n  adbuddy doctor --json\n  adbuddy doctor --fix",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			agent := app.Getenv("ADBUDDY_SOURCE") == "agent"
			if fix && agent {
				return NewErrorT("AGENT_SESSION", "err.doctor.agent_session", "err.doctor.agent_session.fix")
			}
			// stop is Ctrl+C; ctx also ends when doctor's time is up, and
			// then the checks still left say so instead of stopping.
			stop := cmd.Context()
			ctx, cancel := context.WithTimeout(stop, doctorTotalWait)
			defer cancel()

			rep := runDoctor(ctx, app)
			if stop.Err() != nil {
				return Interrupted()
			}
			if fix {
				out := doctorFix(ctx, app, rep)
				if stop.Err() != nil {
					return Interrupted()
				}
				if out.restarted {
					rep = runDoctor(ctx, app)
					if stop.Err() != nil {
						return Interrupted()
					}
				}
				rep.fixed = out.fixed
				for _, m := range out.fixed {
					rep.Fixed = append(rep.Fixed, m.String())
				}
				if out.note.Key != "" {
					noteOnServer(&rep, out.note)
				}
			}
			if agent {
				askTheUser(&rep)
			}
			broken := rep.Verdict == "broken"
			if app.JSON {
				if broken {
					return writeFailureData(app.Stdout, NewErrorT("DOCTOR_BROKEN", "err.doctor.doctor_broken", "err.doctor.doctor_broken.fix"), rep)
				}
				return WriteData(app.Stdout, rep)
			}
			printReport(app, rep)
			if broken {
				return ExitWith(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, app.T("help.doctor.flag.fix"))
	return cmd
}

func printReport(app *App, rep doctorReport) {
	lang := app.speaks()
	icon := map[string]string{checkOK: "✓", checkWarn: "⚠", checkFail: "✗", checkSkip: "·"}
	for _, f := range rep.fixed {
		fmt.Fprintf(app.Stdout, "↻ %s\n", classify.Printable(f.In(lang)))
	}
	for _, c := range rep.Checks {
		fmt.Fprintf(app.Stdout, "%s %s: %s\n", icon[c.Status], checkName(lang, c.Name), classify.Printable(c.detailMsg().In(lang)))
		if c.Run != "" {
			fmt.Fprintf(app.Stdout, "    %s %s\n", lang.T("doctor.label.run"), classify.Printable(c.Run))
		}
		if c.Guidance != "" {
			fmt.Fprintf(app.Stdout, "    %s %s\n", lang.T("doctor.label.guidance"), classify.Printable(c.guidanceMsg().In(lang)))
		}
	}
	switch rep.Verdict {
	case "ready":
		fmt.Fprintln(app.Stdout, lang.T("doctor.verdict.ready"))
	case "degraded":
		fmt.Fprintln(app.Stdout, lang.T("doctor.verdict.degraded"))
	default:
		fmt.Fprintln(app.Stdout, lang.T("doctor.verdict.broken"))
	}
}

// runDoctor runs every check, in this order: config, adb, adb server, serve,
// phone, identity, network port, phone app, environment. A check that
// depends on one that failed is reported as skipped. It never changes
// anything, on the PC or on the phone (starting ADBuddy's private adb server
// aside, as any adb call does).
func runDoctor(ctx context.Context, app *App) (rep doctorReport) {
	add := func(c check) { rep.Checks = append(rep.Checks, c) }
	skip := func(why i18n.Msg, names ...string) {
		for _, n := range names {
			add(mk(n, checkSkip, i18n.M("doctor.skipped", why)))
		}
	}
	defer func() { rep.Verdict = verdict(rep.Checks) }()

	e, err := loadEnv(app)
	if err != nil {
		// The message and the fix of the error, as messages: the report writes
		// them in the language of the run (the JSON keeps the English).
		detail, fix := i18n.M("doctor.plain", err.Error()), i18n.M("doctor.config.run_setup")
		var ce *Error
		if errors.As(err, &ce) {
			message, ceFix := ce.msgs()
			detail = i18n.M("doctor.error", ce.Code, message)
			if ce.Fix != "" {
				fix = ceFix
			}
		}
		add(mk("config", checkFail, detail).withGuide(fix))
		skip(i18n.M("doctor.skip.no_config"), "adb", "adb server", "serve", "phone", "identity", "network port", "phone app", "environment")
		return rep
	}
	configured := e.cfg.Serial != "" || e.cfg.Address != ""
	if configured {
		add(mk("config", checkOK, i18n.M("doctor.plain", e.dirs.ConfigFile())))
	} else {
		add(mk("config", checkFail, i18n.M("doctor.config.no_phone", e.dirs.ConfigFile())).withRun("adbuddy setup"))
	}

	// serve first: whether doctor may offer to restart the adb server
	// depends on it. It is reported after the adb server, its dependency.
	running, ping, perr := servePing(ctx, e.dirs.Data)

	adbPath := configuredADB(e, "")
	runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
	adbCheck := mk("adb", checkFail, i18n.M("doctor.adb.not_installed")).withRun("adbuddy setup")
	if adbPath != "" {
		adbCheck = checkADB(ctx, runner, adbPath)
	}
	add(adbCheck)

	var online []adb.Device
	serverUp := false
	if adbCheck.Status == checkFail {
		skip(i18n.M("doctor.skip.adb_broken"), "adb server")
	} else if online, err = callWithin(ctx, func(c context.Context) ([]adb.Device, error) { return runner.Online(c) }); err != nil {
		add(serverDown(e.cfg.ADBServerPort, err, running, perr))
	} else {
		serverUp = true
		// Plural by count (TN's convention: the count is %[1]d); the English is the same in both forms.
		if n := len(online); n == 1 {
			add(mk("adb server", checkOK, i18n.M("doctor.server.ok.one", n, e.cfg.ADBServerPort)))
		} else {
			add(mk("adb server", checkOK, i18n.M("doctor.server.ok.other", n, e.cfg.ADBServerPort)))
		}
	}
	add(checkServe(running, ping, perr, e.dirs.Data))

	switch active, guid := findConfiguredPhone(online, e.cfg.Serial, e.cfg.Address); {
	case !serverUp:
		skip(i18n.M("doctor.skip.server_down"), "phone", "identity", "network port", "phone app")
	case !configured:
		skip(i18n.M("doctor.skip.no_phone"), "phone", "identity", "network port", "phone app")
	case active == "":
		how := i18n.M("doctor.phone.how_wifi")
		if e.cfg.Address != "" {
			how = i18n.M("doctor.phone.how_tailscale")
		}
		add(mk("phone", checkFail, i18n.M("doctor.phone.not_connected")).withGuide(how))
		skip(i18n.M("doctor.skip.not_connected"), "identity", "network port", "phone app")
	default:
		add(mk("phone", checkOK, i18n.M("doctor.plain", active)))
		phone := runner
		phone.Serial = active
		add(checkIdentity(ctx, phone, guid, savedName(e)))
		add(checkRemotePort(ctx, phone, remoteStatusWithoutServe(e), running))
		add(checkApp(ctx, phone, e, running))
	}

	ws := e.workspace("")
	if w := serveWorkspace(ping); running && w != "" {
		ws = w
	}
	add(checkWorkspace(ws))
	add(checkClaudeHooks())
	return rep
}

func verdict(cs []check) string {
	v := "ready"
	for _, c := range cs {
		switch c.Status {
		case checkFail:
			return "broken"
		case checkWarn:
			v = "degraded"
		}
	}
	return v
}

func findCheck(rep doctorReport, name string) check {
	for _, c := range rep.Checks {
		if c.Name == name {
			return c
		}
	}
	return check{}
}

// callWithin bounds one call, and says in its error which limit ran out.
func callWithin[T any](ctx context.Context, f func(context.Context) (T, error)) (T, error) {
	c, cancel := context.WithTimeout(ctx, doctorCallWait)
	defer cancel()
	v, err := f(c)
	if err != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			err = failf("doctor.err.limit", doctorTotalWait.String())
		case ctx.Err() == nil && errors.Is(c.Err(), context.DeadlineExceeded):
			err = failf("doctor.err.no_answer", doctorCallWait.String())
		}
	}
	return v, err
}

func checkADB(ctx context.Context, runner adb.Runner, path string) check {
	out, err := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, code, err := runner.Output(c, "version")
		if err == nil && code != 0 {
			err = failf("doctor.err.exit", code)
		}
		return o, err
	})
	if err != nil {
		return mk("adb", checkFail, i18n.M("doctor.adb.error", path, causeMsg(err))).withRun("adbuddy setup")
	}
	m := adbVersionRe.FindStringSubmatch(out)
	if m == nil {
		return mk("adb", checkWarn, i18n.M("doctor.adb.unknown_version", path)).withRun("adbuddy setup")
	}
	major, _ := strconv.Atoi(m[1])
	version := m[1] + "." + m[2] + "." + m[3]
	if major < 37 {
		return mk("adb", checkFail, i18n.M("doctor.adb.too_old", path, version)).withRun("adbuddy setup")
	}
	return mk("adb", checkOK, i18n.M("doctor.adb.ok", version, path))
}

// serverDown is the adb server check when it does not answer. Restarting it
// (--fix) is offered only when no serve can be using it.
func serverDown(port int, err error, serveRunning bool, serveErr error) check {
	c := mk("adb server", checkFail, i18n.M("doctor.server.down", port, causeMsg(err)))
	switch {
	case serveRunning:
		c = c.withGuide(i18n.M("doctor.server.serve_running"))
	case serveErr != nil:
		c = c.withGuide(i18n.M("doctor.server.socket_stuck"))
	default:
		c = c.withRun("adbuddy doctor --fix").withGuide(i18n.M("doctor.server.other_tools", port))
	}
	return c
}

func checkServe(running bool, ping ipc.Ping, perr error, dataDir string) check {
	switch {
	case perr != nil:
		return mk("serve", checkWarn, causeMsg(perr)).withGuide(i18n.M("doctor.serve.stuck", ipc.SocketPath(dataDir)))
	case running:
		return mk("serve", checkOK, i18n.M("doctor.serve.running", ping.Session, ping.Serial))
	}
	return mk("serve", checkWarn, i18n.M("doctor.serve.not_running")).withRun("adbuddy serve").withGuide(i18n.M("doctor.serve.keep_open"))
}

func servePing(ctx context.Context, dataDir string) (bool, ipc.Ping, error) {
	pctx, cancel := context.WithTimeout(ctx, doctorPingWait)
	defer cancel()
	p, err := ipc.PingDaemon(pctx, dataDir)
	switch {
	case err == nil:
		return true, p, nil
	case errors.Is(err, ipc.ErrNotRunning):
		return false, ipc.Ping{}, nil
	}
	return false, ipc.Ping{}, failf("doctor.serve.no_answer", doctorPingWait.String(), err.Error())
}

// findConfiguredPhone finds the phone of config.toml among the connected
// ones, by its stable name (with or without " (N)") or by its saved address.
// It never connects anything.
func findConfiguredPhone(online []adb.Device, serial, addr string) (active, guid string) {
	guid, _ = pairing.GuidOf(serial)
	for _, d := range online {
		if (guid != "" && pairing.IsGuidSerial(guid, d.Serial)) || (guid == "" && serial != "" && d.Serial == serial) {
			return d.Serial, guid
		}
	}
	for _, d := range online {
		if addr != "" && d.Serial == addr {
			return d.Serial, guid
		}
	}
	return "", guid
}

// savedName is how config.toml names the phone.
func savedName(e env) string {
	if e.cfg.Serial != "" {
		return e.cfg.Serial
	}
	return e.cfg.Address
}

func checkIdentity(ctx context.Context, phone adb.Runner, guid, saved string) check {
	if _, ok := pairing.GuidSerialNo(guid); !ok {
		var c check
		if guid == "" {
			c = mk("identity", checkWarn, i18n.M("doctor.identity.cant_name", saved))
		} else {
			c = mk("identity", checkWarn, i18n.M("doctor.identity.cant_why", unverifiedWhy(guid)))
		}
		if _, _, err := net.SplitHostPort(phone.Serial); err == nil {
			c = c.withGuide(i18n.M("doctor.identity.check_address", phone.Serial))
		} else {
			c = c.withGuide(i18n.M("doctor.identity.no_other_phone"))
		}
		return c
	}
	_, err := callWithin(ctx, func(c context.Context) (struct{}, error) { return struct{}{}, pairing.VerifyIdentity(c, phone, guid) })
	switch {
	case errors.Is(err, pairing.ErrWrongPhone):
		return mk("identity", checkFail, i18n.M("doctor.identity.wrong", phone.Serial)).withGuide(i18n.M("doctor.identity.wrong.fix"))
	case err != nil:
		return mk("identity", checkWarn, i18n.M("doctor.identity.read_failed", causeMsg(err))).withGuide(unlockAndRetry)
	}
	return mk("identity", checkOK, i18n.M("doctor.identity.ok"))
}

// unlockAndRetry is the guidance when the phone did not answer a read.
var unlockAndRetry = i18n.M("doctor.unlock")

// checkRemotePort reads whether adbd listens on a TCP port. With the remote
// mode on (mode: remote_until in config.toml as the controller reads it, so
// an expiry beyond its lifetime counts as expired) that is expected, and it
// says until when; with serve stopped nothing closes it, even after that.
func checkRemotePort(ctx context.Context, phone adb.Runner, mode remoteInfo, serveRunning bool) check {
	port, err := callWithin(ctx, func(c context.Context) (string, error) { return device.TCPPort(c, phone) })
	switch {
	case err != nil:
		return mk("network port", checkWarn, i18n.M("doctor.port.read_failed", causeMsg(err))).withGuide(unlockAndRetry)
	case port == "" || port == "0":
		return mk("network port", checkOK, i18n.M("doctor.port.closed"))
	}
	if mode.Enabled && mode.ExpiresAt != nil {
		hm := mode.ExpiresAt.Local().Format("15:04")
		c := mk("network port", checkWarn, i18n.M("doctor.port.remote_on", port, hm))
		if mode.Closing {
			c = mk("network port", checkWarn, i18n.M("doctor.port.remote_expired", port, hm))
		}
		if serveRunning {
			return c.withGuide(i18n.M("doctor.port.stays_open"))
		}
		return c.withGuide(i18n.M("doctor.port.serve_stopped", hm))
	}
	return mk("network port", checkWarn, i18n.M("doctor.port.open", port)).withGuide(i18n.M("doctor.port.nothing_to_do"))
}

// checkApp: the phone app is optional; everything here is a warning at most.
// A read that fails is "could not check", never a missing setting.
func checkApp(ctx context.Context, phone adb.Runner, e env, serveRunning bool) check {
	couldNot := func(err error) check {
		return mk("phone app", checkWarn, i18n.M("doctor.app.could_not", causeMsg(err))).withGuide(unlockAndRetry)
	}
	version, err := callWithin(ctx, func(c context.Context) (string, error) { return phoneapp.InstalledVersion(c, phone) })
	if err != nil {
		return couldNot(err)
	}
	if version == "" {
		return mk("phone app", checkWarn, i18n.M("doctor.app.not_installed")).withRun("adbuddy setup")
	}
	pkg, err := readShell(ctx, phone, "dumpsys package "+phoneapp.Package)
	if err != nil {
		return couldNot(err)
	}
	a11y, err := readShell(ctx, phone, "settings get secure enabled_accessibility_services")
	if err != nil {
		return couldNot(err)
	}
	idle, err := readShell(ctx, phone, "dumpsys deviceidle whitelist")
	if err != nil {
		return couldNot(err)
	}
	var missing []i18n.Msg
	if !strings.Contains(pkg, phoneapp.SecureWrites+": granted=true") {
		missing = append(missing, i18n.M("doctor.app.missing.permission"))
	}
	if !phoneapp.A11yEnabled(strings.TrimSpace(a11y)) {
		missing = append(missing, i18n.M("doctor.app.missing.accessibility"))
	}
	if !batteryExempt(idle, phoneapp.Package) {
		missing = append(missing, i18n.M("doctor.app.missing.battery"))
	}
	if len(missing) > 0 {
		return mk("phone app", checkWarn, i18n.M("doctor.app.missing", version, listMsg(missing...))).
			withRun("adbuddy setup").withGuide(i18n.M("doctor.app.accept_consent"))
	}
	if serveRunning && e.cfg.ChatPort != 0 {
		open, err := callWithin(ctx, func(c context.Context) (bool, error) { return phoneapp.TunnelOpen(c, phone, e.cfg.ChatPort) })
		switch {
		case err != nil:
			return mk("phone app", checkWarn, i18n.M("doctor.app.channel_unknown", version, causeMsg(err))).withGuide(unlockAndRetry)
		case !open:
			return mk("phone app", checkWarn, i18n.M("doctor.app.channel_closed", version)).withGuide(i18n.M("doctor.app.wait"))
		}
	}
	return mk("phone app", checkOK, i18n.M("doctor.app.ok", version))
}

// readShell runs one read-only shell command on the phone; an exit code
// other than 0 is an error.
func readShell(ctx context.Context, phone adb.Runner, command string) (string, error) {
	return callWithin(ctx, func(c context.Context) (string, error) {
		out, errOut, code, err := phone.Output(c, "shell", command)
		if err == nil && code != 0 {
			err = failf("doctor.err.shell_exit", command, code, strings.TrimSpace(errOut))
		}
		return out, err
	})
}

// batteryExempt reads `dumpsys deviceidle whitelist`, one "<list>,<package>,<uid>"
// line per exempt app: the exemption setup asks for is on the "user" list.
func batteryExempt(whitelist, pkg string) bool {
	for _, line := range strings.Split(whitelist, "\n") {
		f := strings.Split(strings.TrimSpace(line), ",")
		if len(f) == 3 && f[0] == "user" && f[1] == pkg {
			return true
		}
	}
	return false
}

func checkWorkspace(ws string) check {
	var missing []string
	for _, f := range []string{"AGENTS.md", filepath.Join(".adbuddy", "bin", exeName("adb"))} {
		if _, err := os.Stat(filepath.Join(ws, f)); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return mk("environment", checkFail, i18n.M("doctor.env.missing", ws, strings.Join(missing, ", "))).withRun("adbuddy env init --dir " + quoteArg(ws))
	}
	jdir := workspace.JournalDir(ws)
	firstUse, err := journalWritable(jdir)
	if err != nil {
		return mk("environment", checkFail, i18n.M("doctor.env.journal_unwritable", causeMsg(err))).withGuide(i18n.M("doctor.env.make_writable", jdir))
	}
	if firstUse {
		return mk("environment", checkOK, i18n.M("doctor.env.first_use", ws))
	}
	return mk("environment", checkOK, i18n.M("doctor.plain", ws))
}

// journalWritable checks, without creating or writing anything, that the
// journal could be appended to: its folder, when there, must be a writable
// folder, and this month's file, when there, must not be read-only. A
// missing folder is fine: the journal creates it on first use.
func journalWritable(dir string) (firstUse bool, err error) {
	fi, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, nil
	case err != nil:
		return false, err
	case !fi.IsDir():
		return false, failf("doctor.err.not_folder", dir)
	case runtime.GOOS != "windows" && fi.Mode().Perm()&0o200 == 0:
		// On Windows the read-only attribute of a folder does not stop
		// new files in it.
		return false, failf("doctor.err.read_only", dir)
	}
	// The file journal.Store appends to this month.
	f := filepath.Join(dir, time.Now().UTC().Format("2006-01")+".jsonl")
	if fi, err := os.Stat(f); err == nil && fi.Mode().Perm()&0o200 == 0 {
		return false, failf("doctor.err.read_only", f)
	}
	return false, nil
}

// fixOutcome is what --fix did.
type fixOutcome struct {
	fixed     []i18n.Msg // what it fixed, checked afterwards
	note      i18n.Msg   // why the adb server is not fixed, when --fix left it failing (empty: nothing to say)
	restarted bool       // it touched the server: every check runs again
}

// doctorFix applies the only automatic fix: restarting ADBuddy's private adb
// server when it does not answer and no serve is using it. It reports a fix
// only when start-server succeeded and the server answers afterwards.
func doctorFix(ctx context.Context, app *App, rep doctorReport) fixOutcome {
	if findCheck(rep, "adb server").Status != checkFail {
		return fixOutcome{} // nothing to fix, or no adb to fix it with
	}
	e, err := loadEnv(app)
	if err != nil {
		return fixOutcome{}
	}
	// Its own look at serve, now: never under a running serve, even one
	// started after the checks.
	switch running, _, perr := servePing(ctx, e.dirs.Data); {
	case running:
		return fixOutcome{note: i18n.M("doctor.fix.serve_running")}
	case perr != nil:
		return fixOutcome{note: i18n.M("doctor.fix.socket_stuck")}
	}
	runner := adb.Runner{Path: configuredADB(e, ""), ServerPort: e.cfg.ADBServerPort}
	_, _ = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, _, _, err := runner.Output(c, "kill-server")
		return struct{}{}, err
	})
	_, err = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, errOut, code, err := runner.Output(c, "start-server")
		if err == nil && code != 0 {
			err = failf("doctor.err.exit_output", code, strings.TrimSpace(errOut))
		}
		return struct{}{}, err
	})
	if err != nil {
		return fixOutcome{restarted: true, note: i18n.M("doctor.fix.start_failed", causeMsg(err))}
	}
	if _, err := callWithin(ctx, func(c context.Context) ([]adb.Device, error) { return runner.Online(c) }); err != nil {
		return fixOutcome{restarted: true, note: i18n.M("doctor.fix.still_down")}
	}
	return fixOutcome{restarted: true, fixed: []i18n.Msg{i18n.M("doctor.fix.restarted", e.cfg.ADBServerPort)}}
}

// noteOnServer tells, on a failing adb server check, what --fix did about
// it; --fix is not offered again.
func noteOnServer(rep *doctorReport, note i18n.Msg) {
	for i, c := range rep.Checks {
		if c.Name == "adb server" && c.Status == checkFail {
			rep.Checks[i].detail = i18n.M("doctor.with_note", c.detailMsg(), note)
			rep.Checks[i].Detail = rep.Checks[i].detail.String()
			if c.Run == "adbuddy doctor --fix" {
				rep.Checks[i].Run = ""
			}
		}
	}
}

// askTheUser rewrites, for the agent's session, the Run lines it cannot run
// as something to ask the user.
func askTheUser(rep *doctorReport) {
	for i, c := range rep.Checks {
		if !isPersonOnly(c.Run) {
			continue
		}
		g := i18n.M("doctor.ask_user", c.Run)
		if c.Guidance != "" {
			g = i18n.M("doctor.ask_user_guidance", c.Run, c.guidanceMsg())
		}
		rep.Checks[i].Run, rep.Checks[i].guidance, rep.Checks[i].Guidance = "", g, g.String()
	}
}

func isPersonOnly(run string) bool {
	for _, p := range personOnly {
		if run == p || strings.HasPrefix(run, p+" ") {
			return true
		}
	}
	return false
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// quoteArg quotes a path for the Run: line when it has spaces.
func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}
