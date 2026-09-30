package cli

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// Check statuses. A failed check stops ADBuddy from working; a warning
// leaves it working with something missing (degraded).
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

// Doctor time limits: every adb call is bounded, so doctor always answers.
var (
	doctorCallWait = 10 * time.Second
	adbVersionRe   = regexp.MustCompile(`Version (\d+)\.(\d+)\.(\d+)`)
)

func newDoctorCmd(app *App) *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that ADBuddy, adb, the phone and the app are ready, and say how to fix what is not",
		Long: `Check, in order, everything ADBuddy needs: configuration, ADBuddy's adb and its
private server, the phone (connected, the one you set up, no open network port),
the agent environment, ` + "`adbuddy serve`" + ` and the phone app. Every problem comes with
a "Run:" line (a safe command) or a "Guidance:" line (something only you can do).
It only reads: nothing on the phone changes. With --fix it also restarts
ADBuddy's private adb server when that is what fails and serve is not running.
Exit code: 0 ready or degraded, 1 broken.`,
		Example: "  adbuddy doctor\n  adbuddy doctor --json\n  adbuddy doctor --fix",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if fix && app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewError("AGENT_SESSION", "`adbuddy doctor --fix` cannot be run from the agent's session", "Run `adbuddy doctor` (without --fix) and follow its Run: lines, or ask the person to run --fix.")
			}
			ctx := cmd.Context()
			rep := runDoctor(ctx, app)
			if fix {
				if fixed := doctorFix(ctx, app, rep); len(fixed) > 0 {
					rep = runDoctor(ctx, app)
					rep.Fixed = fixed
				}
			}
			if app.JSON {
				if err := WriteData(app.Stdout, rep); err != nil {
					return err
				}
			} else {
				printReport(app, rep)
			}
			if rep.Verdict == "broken" {
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

// runDoctor runs every check. It never changes anything, on the PC or on the
// phone (starting ADBuddy's private adb server aside, as any adb call does).
func runDoctor(ctx context.Context, app *App) (rep doctorReport) {
	add := func(c check) { rep.Checks = append(rep.Checks, c) }
	defer func() { rep.Verdict = verdict(rep.Checks) }()

	e, err := loadEnv(app)
	if err != nil {
		var ce *Error
		fix := "Run `adbuddy setup`."
		if errors.As(err, &ce) && ce.Fix != "" {
			fix = ce.Fix
		}
		add(check{Name: "config", Status: checkFail, Detail: err.Error(), Guidance: fix})
		return rep
	}
	if e.cfg.Serial == "" && e.cfg.Address == "" {
		add(check{Name: "config", Status: checkFail, Detail: "no phone set up yet (" + e.dirs.ConfigFile() + ")", Run: "adbuddy setup"})
	} else {
		add(check{Name: "config", Status: checkOK, Detail: e.dirs.ConfigFile()})
	}

	adbPath := configuredADB(e, "")
	if adbPath == "" {
		add(check{Name: "adb", Status: checkFail, Detail: "ADBuddy's adb is not installed", Run: "adbuddy setup"})
		return rep
	}
	runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
	add(checkADB(ctx, runner, adbPath))

	online, err := callWithin(ctx, func(c context.Context) ([]adb.Device, error) { return runner.Online(c) })
	if err != nil {
		add(check{Name: "adb server", Status: checkFail,
			Detail:   fmt.Sprintf("ADBuddy's adb server (port %d) does not answer: %v", e.cfg.ADBServerPort, err),
			Run:      "adbuddy doctor --fix",
			Guidance: "If it keeps failing, stop other adb tools using port " + strconv.Itoa(e.cfg.ADBServerPort) + "."})
		return rep
	}
	add(check{Name: "adb server", Status: checkOK, Detail: fmt.Sprintf("port %d, %d phone(s) connected", e.cfg.ADBServerPort, len(online))})

	running, ping, perr := servePing(ctx, e.dirs.Data)
	switch {
	case perr != nil:
		add(check{Name: "serve", Status: checkFail, Detail: perr.Error(), Guidance: "Stop the other `adbuddy serve`, or delete " + ipc.SocketPath(e.dirs.Data) + " if none is running."})
	case running:
		add(check{Name: "serve", Status: checkOK, Detail: "running, session " + ping.Session + ", phone " + ping.Serial})
	default:
		add(check{Name: "serve", Status: checkWarn, Detail: "not running: the agent cannot use the phone", Guidance: "Start `adbuddy serve` in a terminal and keep it open."})
	}

	active, guid := findConfiguredPhone(online, e.cfg.Serial, e.cfg.Address)
	if active == "" {
		how := "Turn on Wireless debugging on the phone and keep it on the same Wi-Fi as this PC."
		if e.cfg.Address != "" {
			how = "Turn on Wireless debugging on the phone. On another network (Tailscale), run `adbuddy connect <ip:port>` with the address shown on that screen."
		}
		add(check{Name: "phone", Status: checkFail, Detail: "the phone you set up is not connected to ADBuddy's adb server", Guidance: how})
	} else {
		add(check{Name: "phone", Status: checkOK, Detail: active})
		phone := runner
		phone.Serial = active
		add(checkIdentity(ctx, phone, guid))
		add(checkRemotePort(ctx, phone))
		add(checkApp(ctx, phone, e, running))
	}

	ws := e.workspace("")
	if running {
		ws = serveWorkspace(ping)
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

// callWithin bounds one adb call.
func callWithin[T any](ctx context.Context, f func(context.Context) (T, error)) (T, error) {
	c, cancel := context.WithTimeout(ctx, doctorCallWait)
	defer cancel()
	return f(c)
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

func servePing(ctx context.Context, dataDir string) (bool, ipc.Ping, error) {
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	p, err := ipc.PingDaemon(pctx, dataDir)
	switch {
	case err == nil:
		return true, p, nil
	case errors.Is(err, ipc.ErrNotRunning):
		return false, ipc.Ping{}, nil
	}
	return false, ipc.Ping{}, fmt.Errorf("the ADBuddy socket does not answer: %v", err)
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

func checkIdentity(ctx context.Context, phone adb.Runner, guid string) check {
	if _, ok := pairing.GuidSerialNo(guid); !ok {
		return check{Name: "identity", Status: checkWarn, Detail: "cannot be checked: " + unverifiedWhy(guid),
			Guidance: "Nothing to do unless you have several phones; `adbuddy undo` will not mix them up."}
	}
	_, err := callWithin(ctx, func(c context.Context) (struct{}, error) { return struct{}{}, pairing.VerifyIdentity(c, phone, guid) })
	switch {
	case errors.Is(err, pairing.ErrWrongPhone):
		return check{Name: "identity", Status: checkFail, Detail: "the phone at " + phone.Serial + " is not the one you set up",
			Guidance: "Check the IP and port on the phone you set up; run `adbuddy connect <ip:port>` with them."}
	case err != nil:
		return check{Name: "identity", Status: checkWarn, Detail: "could not read the phone's serial number: " + err.Error(), Guidance: "Unlock the phone and run `adbuddy doctor` again."}
	}
	return check{Name: "identity", Status: checkOK, Detail: "it is the phone you set up"}
}

func checkRemotePort(ctx context.Context, phone adb.Runner) check {
	port, err := callWithin(ctx, func(c context.Context) (string, error) { return device.TCPPort(c, phone) })
	switch {
	case err != nil:
		return check{Name: "network port", Status: checkWarn, Detail: "could not read service.adb.tcp.port: " + err.Error(), Guidance: "Unlock the phone and run `adbuddy doctor` again."}
	case port == "" || port == "0":
		return check{Name: "network port", Status: checkOK, Detail: "closed (no adb tcpip mode)"}
	}
	return check{Name: "network port", Status: checkWarn, Detail: "adb listens on port " + port + " (tcpip mode): `adbuddy serve` closes it with `adb usb` when it starts",
		Guidance: "Nothing to do: serve closes it. Remote mode (Tailscale over mobile data) arrives in a later version."}
}

// checkApp: the phone app is optional; everything here is a warning at most.
func checkApp(ctx context.Context, phone adb.Runner, e env, serveRunning bool) check {
	version, err := callWithin(ctx, func(c context.Context) (string, error) { return phoneapp.InstalledVersion(c, phone) })
	if err != nil {
		return check{Name: "phone app", Status: checkWarn, Detail: "could not check: " + err.Error(), Guidance: "Unlock the phone and run `adbuddy doctor` again."}
	}
	if version == "" {
		return check{Name: "phone app", Status: checkWarn, Detail: "not installed: 🔴 confirmations happen in the terminal running serve", Run: "adbuddy setup"}
	}
	var missing []string
	pkg, _ := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, _, err := phone.Output(c, "shell", "dumpsys package "+phoneapp.Package)
		return o, err
	})
	if !strings.Contains(pkg, phoneapp.SecureWrites+": granted=true") {
		missing = append(missing, "permission for Cut access")
	}
	a11y, _ := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, _, err := phone.Output(c, "shell", "settings get secure enabled_accessibility_services")
		return o, err
	})
	if !phoneapp.A11yEnabled(strings.TrimSpace(a11y)) {
		missing = append(missing, "accessibility service")
	}
	idle, _ := callWithin(ctx, func(c context.Context) (string, error) {
		o, _, _, err := phone.Output(c, "shell", "dumpsys deviceidle whitelist")
		return o, err
	})
	if !strings.Contains(idle, phoneapp.Package) {
		missing = append(missing, "battery exemption")
	}
	if len(missing) > 0 {
		return check{Name: "phone app", Status: checkWarn, Detail: version + " installed, but without: " + strings.Join(missing, ", "),
			Run: "adbuddy setup", Guidance: "Accept the consent screen in the ADBuddy app when setup asks."}
	}
	if serveRunning && e.cfg.ChatPort != 0 {
		open, err := callWithin(ctx, func(c context.Context) (bool, error) { return phoneapp.TunnelOpen(c, phone, e.cfg.ChatPort) })
		if err == nil && !open {
			return check{Name: "phone app", Status: checkWarn, Detail: version + " installed, but the channel with serve is not open",
				Guidance: "Open the ADBuddy app on the phone; serve reopens the channel within 15 seconds."}
		}
	}
	return check{Name: "phone app", Status: checkOK, Detail: version + " installed and set up"}
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
	if err := journal.Open(workspace.JournalDir(ws)).Writable(); err != nil {
		return check{Name: "environment", Status: checkFail, Detail: "the journal cannot be written (" + err.Error() + "): nothing 🟡 or 🔴 will run",
			Guidance: "Check the permissions and free space of " + workspace.JournalDir(ws) + "."}
	}
	return check{Name: "environment", Status: checkOK, Detail: ws}
}

// doctorFix applies the only automatic fix: restarting ADBuddy's private adb
// server when it does not answer and no serve is using it.
func doctorFix(ctx context.Context, app *App, rep doctorReport) []string {
	var failing bool
	for _, c := range rep.Checks {
		if c.Name == "adb server" && c.Status == checkFail {
			failing = true
		}
		if c.Name == "serve" && c.Status == checkOK {
			return nil // never under a running serve
		}
	}
	if !failing {
		return nil
	}
	e, err := loadEnv(app)
	if err != nil {
		return nil
	}
	runner := adb.Runner{Path: configuredADB(e, ""), ServerPort: e.cfg.ADBServerPort}
	_, _ = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, _, _, err := runner.Output(c, "kill-server")
		return struct{}{}, err
	})
	_, _ = callWithin(ctx, func(c context.Context) (struct{}, error) {
		_, _, _, err := runner.Output(c, "start-server")
		return struct{}{}, err
	})
	return []string{fmt.Sprintf("restarted ADBuddy's adb server (port %d)", e.cfg.ADBServerPort)}
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
