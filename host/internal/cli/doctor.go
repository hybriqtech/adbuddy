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
type check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Run      string `json:"run,omitempty"`
	Guidance string `json:"guidance,omitempty"`
}

// doctorReport is the output of `adbuddy doctor`.
type doctorReport struct {
	Verdict string   `json:"verdict"` // ready, degraded or broken
	Fixed   []string `json:"fixed,omitempty"`
	Checks  []check  `json:"checks"`
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
		Use:   "doctor",
		Short: "Check that ADBuddy, adb, the phone and the app are ready, and say how to fix what is not",
		Long: `Check, in order, everything ADBuddy needs: configuration, ADBuddy's adb and its
private server, ` + "`adbuddy serve`" + `, the phone (connected, the one you set up, no open
network port), the phone app and the agent environment. Every problem comes with
a "Run:" line (a safe command) or a "Guidance:" line (something only you can do);
in the agent's session, what only you can run becomes "Ask the user to run …".
It only reads: nothing on the phone changes. With --fix it also restarts
ADBuddy's private adb server when that is what fails and serve is not running.
Doctor answers within 60 seconds.
Exit code: 0 ready or degraded, 1 broken, 130 stopped with Ctrl+C.`,
		Example: "  adbuddy doctor\n  adbuddy doctor --json\n  adbuddy doctor --fix",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			agent := app.Getenv("ADBUDDY_SOURCE") == "agent"
			if fix && agent {
				return NewError("AGENT_SESSION", "`adbuddy doctor --fix` cannot be run from the agent's session", "Run `adbuddy doctor` (without --fix) and follow its Run: lines, or ask the user to run --fix.")
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
				rep.Fixed = out.fixed
				if out.note != "" {
					noteOnServer(&rep, out.note)
				}
			}
			if agent {
				askTheUser(&rep)
			}
			broken := rep.Verdict == "broken"
			if app.JSON {
				if broken {
					return writeFailureData(app.Stdout, NewError("DOCTOR_BROKEN", "ADBuddy cannot work until the failed checks are fixed",
						"Follow the run and guidance of the failed checks, from the top."), rep)
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
	cmd.Flags().BoolVar(&fix, "fix", false, "also restart ADBuddy's private adb server when it is what fails (never while serve runs)")
	return cmd
}

func printReport(app *App, rep doctorReport) {
	icon := map[string]string{checkOK: "✓", checkWarn: "⚠", checkFail: "✗", checkSkip: "·"}
	for _, f := range rep.Fixed {
		fmt.Fprintf(app.Stdout, "↻ %s\n", f)
	}
	for _, c := range rep.Checks {
		fmt.Fprintf(app.Stdout, "%s %s: %s\n", icon[c.Status], c.Name, classify.Printable(c.Detail))
		if c.Run != "" {
			fmt.Fprintf(app.Stdout, "    Run: %s\n", classify.Printable(c.Run))
		}
		if c.Guidance != "" {
			fmt.Fprintf(app.Stdout, "    Guidance: %s\n", classify.Printable(c.Guidance))
		}
	}
	switch rep.Verdict {
	case "ready":
		fmt.Fprintln(app.Stdout, "Verdict: ready.")
	case "degraded":
		fmt.Fprintln(app.Stdout, "Verdict: degraded — ADBuddy works; the ⚠ lines are optional or can wait.")
	default:
		fmt.Fprintln(app.Stdout, "Verdict: broken — fix the ✗ lines first, from the top.")
	}
}

// runDoctor runs every check, in this order: config, adb, adb server, serve,
// phone, identity, network port, phone app, environment. A check that
// depends on one that failed is reported as skipped. It never changes
// anything, on the PC or on the phone (starting ADBuddy's private adb server
// aside, as any adb call does).
func runDoctor(ctx context.Context, app *App) (rep doctorReport) {
	add := func(c check) { rep.Checks = append(rep.Checks, c) }
	skip := func(why string, names ...string) {
		for _, n := range names {
			add(check{Name: n, Status: checkSkip, Detail: "not checked: " + why})
		}
	}
	defer func() { rep.Verdict = verdict(rep.Checks) }()

	e, err := loadEnv(app)
	if err != nil {
		var ce *Error
		fix := "Run `adbuddy setup`."
		if errors.As(err, &ce) && ce.Fix != "" {
			fix = ce.Fix
		}
		add(check{Name: "config", Status: checkFail, Detail: err.Error(), Guidance: fix})
		skip("no valid configuration", "adb", "adb server", "serve", "phone", "identity", "network port", "phone app", "environment")
		return rep
	}
	configured := e.cfg.Serial != "" || e.cfg.Address != ""
	if configured {
		add(check{Name: "config", Status: checkOK, Detail: e.dirs.ConfigFile()})
	} else {
		add(check{Name: "config", Status: checkFail, Detail: "no phone set up yet (" + e.dirs.ConfigFile() + ")", Run: "adbuddy setup"})
	}

	// serve first: whether doctor may offer to restart the adb server
	// depends on it. It is reported after the adb server, its dependency.
	running, ping, perr := servePing(ctx, e.dirs.Data)

	adbPath := configuredADB(e, "")
	runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
	adbCheck := check{Name: "adb", Status: checkFail, Detail: "ADBuddy's adb is not installed", Run: "adbuddy setup"}
	if adbPath != "" {
		adbCheck = checkADB(ctx, runner, adbPath)
	}
	add(adbCheck)

	var online []adb.Device
	serverUp := false
	if adbCheck.Status == checkFail {
		skip("ADBuddy's adb does not work", "adb server")
	} else if online, err = callWithin(ctx, func(c context.Context) ([]adb.Device, error) { return runner.Online(c) }); err != nil {
		add(serverDown(e.cfg.ADBServerPort, err, running, perr))
	} else {
		serverUp = true
		add(check{Name: "adb server", Status: checkOK, Detail: fmt.Sprintf("port %d, %d phone(s) connected", e.cfg.ADBServerPort, len(online))})
	}
	add(checkServe(running, ping, perr, e.dirs.Data))

	switch active, guid := findConfiguredPhone(online, e.cfg.Serial, e.cfg.Address); {
	case !serverUp:
		skip("adb server down", "phone", "identity", "network port", "phone app")
	case !configured:
		skip("no phone set up", "phone", "identity", "network port", "phone app")
	case active == "":
		how := "Turn on Wireless debugging on the phone and keep it on the same Wi-Fi as this PC."
		if e.cfg.Address != "" {
			how = "Turn on Wireless debugging on the phone. On another network (Tailscale), run `adbuddy connect <ip:port>` with the address shown on that screen."
		}
		add(check{Name: "phone", Status: checkFail, Detail: "the phone you set up is not connected to ADBuddy's adb server", Guidance: how})
		skip("phone not connected", "identity", "network port", "phone app")
	default:
		add(check{Name: "phone", Status: checkOK, Detail: active})
		phone := runner
		phone.Serial = active
		add(checkIdentity(ctx, phone, guid, savedName(e)))
		add(checkRemotePort(ctx, phone, e.cfg.RemoteUntilTime(), running))
		add(checkApp(ctx, phone, e, running))
	}

	ws := e.workspace("")
	if w := serveWorkspace(ping); running && w != "" {
		ws = w
	}
	add(checkWorkspace(ws))
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
			err = fmt.Errorf("doctor's time limit (%s) ran out", doctorTotalWait)
		case ctx.Err() == nil && errors.Is(c.Err(), context.DeadlineExceeded):
			err = fmt.Errorf("no answer within %s", doctorCallWait)
		}
	}
	return v, err
}

func checkADB(ctx context.Context, runner adb.Runner, path string) check {
	out, err := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, code, err := runner.Output(c, "version")
		if err == nil && code != 0 {
			err = fmt.Errorf("exit %d", code)
		}
		return o, err
	})
	if err != nil {
		return check{Name: "adb", Status: checkFail, Detail: path + ": " + err.Error(), Run: "adbuddy setup"}
	}
	m := adbVersionRe.FindStringSubmatch(out)
	if m == nil {
		return check{Name: "adb", Status: checkWarn, Detail: path + ": unknown version", Run: "adbuddy setup"}
	}
	major, _ := strconv.Atoi(m[1])
	version := m[1] + "." + m[2] + "." + m[3]
	if major < 37 {
		return check{Name: "adb", Status: checkFail, Detail: path + ": version " + version + ", ADBuddy needs 37 or newer (adb Wi-Fi 2.0)", Run: "adbuddy setup"}
	}
	return check{Name: "adb", Status: checkOK, Detail: version + " (" + path + ")"}
}

// serverDown is the adb server check when it does not answer. Restarting it
// (--fix) is offered only when no serve can be using it.
func serverDown(port int, err error, serveRunning bool, serveErr error) check {
	c := check{Name: "adb server", Status: checkFail, Detail: fmt.Sprintf("ADBuddy's adb server (port %d) does not answer: %v", port, err)}
	switch {
	case serveRunning:
		c.Guidance = "`adbuddy serve` is running and uses this server, so doctor does not restart it. Stop serve (Ctrl+C in its terminal) and run adbuddy doctor again."
	case serveErr != nil:
		c.Guidance = "The ADBuddy socket does not answer, so a serve may be using this server. Stop any `adbuddy serve` (Ctrl+C in its terminal) and run adbuddy doctor again."
	default:
		c.Run = "adbuddy doctor --fix"
		c.Guidance = "If it keeps failing, stop other adb tools using port " + strconv.Itoa(port) + "."
	}
	return c
}

func checkServe(running bool, ping ipc.Ping, perr error, dataDir string) check {
	switch {
	case perr != nil:
		return check{Name: "serve", Status: checkWarn, Detail: perr.Error(),
			Guidance: "If `adbuddy serve` is running, it is stuck: stop it (Ctrl+C in its terminal) and start it again. If none is running, delete " + ipc.SocketPath(dataDir) + "."}
	case running:
		return check{Name: "serve", Status: checkOK, Detail: "running, session " + ping.Session + ", phone " + ping.Serial}
	}
	return check{Name: "serve", Status: checkWarn, Detail: "not running: the agent cannot use the phone",
		Run: "adbuddy serve", Guidance: "Keep it open in its own terminal while the agent works."}
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
	return false, ipc.Ping{}, fmt.Errorf("the ADBuddy socket does not answer within %s: %v", doctorPingWait, err)
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
		c := check{Name: "identity", Status: checkWarn}
		if guid == "" {
			c.Detail = "cannot be checked: config.toml keeps this phone as " + saved + ", not by the name it paired with"
		} else {
			c.Detail = "cannot be checked: " + unverifiedWhy(guid)
		}
		if _, _, err := net.SplitHostPort(phone.Serial); err == nil {
			c.Guidance = "Make sure the IP address and port under Wireless debugging on your phone are " + phone.Serial + "."
		} else {
			c.Guidance = "Make sure no other phone paired with this PC has Wireless debugging on at the same time."
		}
		return c
	}
	_, err := callWithin(ctx, func(c context.Context) (struct{}, error) { return struct{}{}, pairing.VerifyIdentity(c, phone, guid) })
	switch {
	case errors.Is(err, pairing.ErrWrongPhone):
		return check{Name: "identity", Status: checkFail, Detail: "the phone at " + phone.Serial + " is not the one you set up",
			Guidance: "Check the IP and port on the phone you set up; run `adbuddy connect <ip:port>` with them."}
	case err != nil:
		return check{Name: "identity", Status: checkWarn, Detail: "could not read the phone's serial number: " + err.Error(), Guidance: unlockAndRetry}
	}
	return check{Name: "identity", Status: checkOK, Detail: "it is the phone you set up"}
}

// unlockAndRetry is the guidance when the phone did not answer a read.
const unlockAndRetry = "Unlock the phone and run adbuddy doctor again."

// checkRemotePort reads whether adbd listens on a TCP port. With the remote
// mode on (remote_until in config.toml) that is expected, and it says until
// when; with serve stopped nothing closes it, even after that.
func checkRemotePort(ctx context.Context, phone adb.Runner, until time.Time, serveRunning bool) check {
	port, err := callWithin(ctx, func(c context.Context) (string, error) { return device.TCPPort(c, phone) })
	switch {
	case err != nil:
		return check{Name: "network port", Status: checkWarn, Detail: "could not read service.adb.tcp.port: " + err.Error(), Guidance: unlockAndRetry}
	case port == "" || port == "0":
		return check{Name: "network port", Status: checkOK, Detail: "closed (no adb tcpip mode)"}
	}
	if !until.IsZero() {
		hm := until.Local().Format("15:04")
		c := check{Name: "network port", Status: checkWarn, Detail: "adb listens on port " + port + " (tcpip mode): remote mode on until " + hm}
		if !until.After(remoteNow()) {
			c.Detail = "adb listens on port " + port + " (tcpip mode): remote mode expired at " + hm + " but the port is still open"
		}
		if serveRunning {
			c.Guidance = "It stays open on any network until serve closes it when the remote mode expires; run `adbuddy remote off` to close it now."
		} else {
			c.Guidance = "Nothing closes it while serve is stopped, even after " + hm + ": run `adbuddy remote off` to close it."
		}
		return c
	}
	return check{Name: "network port", Status: checkWarn, Detail: "adb listens on port " + port + " (tcpip mode): `adbuddy serve` closes it with `adb usb` when it starts",
		Guidance: "Nothing to do: serve closes it (remote mode is off)."}
}

// checkApp: the phone app is optional; everything here is a warning at most.
// A read that fails is "could not check", never a missing setting.
func checkApp(ctx context.Context, phone adb.Runner, e env, serveRunning bool) check {
	couldNot := func(err error) check {
		return check{Name: "phone app", Status: checkWarn, Detail: "could not check: " + err.Error(), Guidance: unlockAndRetry}
	}
	version, err := callWithin(ctx, func(c context.Context) (string, error) { return phoneapp.InstalledVersion(c, phone) })
	if err != nil {
		return couldNot(err)
	}
	if version == "" {
		return check{Name: "phone app", Status: checkWarn, Detail: "not installed: 🔴 confirmations happen in the terminal running serve", Run: "adbuddy setup"}
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
	var missing []string
	if !strings.Contains(pkg, phoneapp.SecureWrites+": granted=true") {
		missing = append(missing, "permission for Cut access")
	}
	if !phoneapp.A11yEnabled(strings.TrimSpace(a11y)) {
		missing = append(missing, "accessibility service")
	}
	if !batteryExempt(idle, phoneapp.Package) {
		missing = append(missing, "battery exemption")
	}
	if len(missing) > 0 {
		return check{Name: "phone app", Status: checkWarn, Detail: version + " installed, but without: " + strings.Join(missing, ", "),
			Run: "adbuddy setup", Guidance: "Accept the consent screen in the ADBuddy app when setup asks."}
	}
	if serveRunning && e.cfg.ChatPort != 0 {
		open, err := callWithin(ctx, func(c context.Context) (bool, error) { return phoneapp.TunnelOpen(c, phone, e.cfg.ChatPort) })
		switch {
		case err != nil:
			return check{Name: "phone app", Status: checkWarn, Detail: version + " installed, but the channel with serve could not be checked: " + err.Error(), Guidance: unlockAndRetry}
		case !open:
			return check{Name: "phone app", Status: checkWarn, Detail: version + " installed, but the channel with serve is not open",
				Guidance: "Wait 15 seconds and run adbuddy doctor again; if it stays closed, open the ADBuddy app."}
		}
	}
	return check{Name: "phone app", Status: checkOK, Detail: version + " installed and set up"}
}

// readShell runs one read-only shell command on the phone; an exit code
// other than 0 is an error.
func readShell(ctx context.Context, phone adb.Runner, command string) (string, error) {
	return callWithin(ctx, func(c context.Context) (string, error) {
		out, errOut, code, err := phone.Output(c, "shell", command)
		if err == nil && code != 0 {
			err = fmt.Errorf("adb shell %s: exit %d: %s", command, code, strings.TrimSpace(errOut))
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
		return check{Name: "environment", Status: checkFail, Detail: ws + ": missing " + strings.Join(missing, ", "), Run: "adbuddy env init --dir " + quoteArg(ws)}
	}
	jdir := workspace.JournalDir(ws)
	firstUse, err := journalWritable(jdir)
	if err != nil {
		return check{Name: "environment", Status: checkFail, Detail: "the journal cannot be written (" + err.Error() + "): nothing 🟡 or 🔴 will run",
			Guidance: "Make " + jdir + " and its files writable for your user, and free some disk space."}
	}
	if firstUse {
		return check{Name: "environment", Status: checkOK, Detail: ws + " (journal created on first use)"}
	}
	return check{Name: "environment", Status: checkOK, Detail: ws}
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
		return false, errors.New(dir + " is not a folder")
	case runtime.GOOS != "windows" && fi.Mode().Perm()&0o200 == 0:
		// On Windows the read-only attribute of a folder does not stop
		// new files in it.
		return false, errors.New(dir + " is read-only")
	}
	// The file journal.Store appends to this month.
	f := filepath.Join(dir, time.Now().UTC().Format("2006-01")+".jsonl")
	if fi, err := os.Stat(f); err == nil && fi.Mode().Perm()&0o200 == 0 {
		return false, errors.New(f + " is read-only")
	}
	return false, nil
}

// fixOutcome is what --fix did.
type fixOutcome struct {
	fixed     []string // what it fixed, checked afterwards
	note      string   // why the adb server is not fixed, when --fix left it failing
	restarted bool     // it touched the server: every check runs again
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
		return fixOutcome{note: "--fix did not restart it: adbuddy serve is running and uses it"}
	case perr != nil:
		return fixOutcome{note: "--fix did not restart it: the ADBuddy socket does not answer, so a serve may be using it"}
	}
	runner := adb.Runner{Path: configuredADB(e, ""), ServerPort: e.cfg.ADBServerPort}
	_, _ = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, _, _, err := runner.Output(c, "kill-server")
		return struct{}{}, err
	})
	_, err = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, errOut, code, err := runner.Output(c, "start-server")
		if err == nil && code != 0 {
			err = fmt.Errorf("exit %d: %s", code, strings.TrimSpace(errOut))
		}
		return struct{}{}, err
	})
	if err != nil {
		return fixOutcome{restarted: true, note: "--fix could not restart it: start-server: " + err.Error()}
	}
	if _, err := callWithin(ctx, func(c context.Context) ([]adb.Device, error) { return runner.Online(c) }); err != nil {
		return fixOutcome{restarted: true, note: "--fix restarted it and it still does not answer"}
	}
	return fixOutcome{restarted: true, fixed: []string{fmt.Sprintf("restarted ADBuddy's adb server (port %d)", e.cfg.ADBServerPort)}}
}

// noteOnServer tells, on a failing adb server check, what --fix did about
// it; --fix is not offered again.
func noteOnServer(rep *doctorReport, note string) {
	for i, c := range rep.Checks {
		if c.Name == "adb server" && c.Status == checkFail {
			rep.Checks[i].Detail += " (" + note + ")"
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
		g := "Ask the user to run " + c.Run + "."
		if c.Guidance != "" {
			g += " " + c.Guidance
		}
		rep.Checks[i].Run, rep.Checks[i].Guidance = "", g
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
