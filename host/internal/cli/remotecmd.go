package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
	"github.com/spf13/cobra"
)

// remoteNow is the clock of the texts `adbuddy remote` prints (a variable so a
// test can fix it).
var remoteNow = time.Now

// exitRemoteDenied is the exit code of a remote mode nobody confirmed: the
// shell's "found but not allowed", as for a 🔴 command that was denied.
const exitRemoteDenied = 126

// remoteInfo is the remote mode as `adbuddy remote` reports it. Closing
// means the expiry passed and the port is not known to be closed yet: it is
// still enabled, never "off". Serve says whether `adbuddy serve` answered
// (otherwise it was read from config.toml).
type remoteInfo struct {
	Enabled   bool       `json:"enabled"`
	Closing   bool       `json:"closing,omitempty"`
	Address   string     `json:"address,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Serve     bool       `json:"serve"`
}

func infoOf(s remote.State, serve bool) remoteInfo {
	i := remoteInfo{Enabled: s.Enabled, Closing: s.Closing, Address: s.Address, Serve: serve}
	if s.Enabled {
		t := s.ExpiresAt.UTC()
		i.ExpiresAt = &t
	}
	return i
}

var remoteFixes = map[string]string{
	"AGENT_SESSION":      "Run it yourself in a terminal; only the user can change the remote mode.",
	"REMOTE_UNAVAILABLE": "Open the ADBuddy app on the phone with Tailscale on, then run it again.",
	"REMOTE_DENIED":      "Run `adbuddy remote on` again and allow the question (in the ADBuddy app or the serve terminal).",
	"REMOTE_FAILED":      "Check the phone is connected and run it again; `adbuddy remote status` says what the host believes.",
	"REMOTE_BUSY":        "Try again in a moment.",
}

func newRemoteCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "remote on|off|status",
		Short: "Open adb's network port 5555 for 12 hours (remote mode), close it, or see its state",
		Long: `Remote mode opens adb's port 5555 on every network for 12 hours, so the host can
reach the phone over Tailscale with only mobile data. ` + "`on`" + ` asks you first (in the
ADBuddy app, or in the ` + "`adbuddy serve`" + ` terminal) and needs serve running;
` + "`off`" + ` closes the port (even with serve stopped) and ` + "`status`" + ` says whether it is
open. Not available to the agent.`,
		Example:   "  adbuddy remote on\n  adbuddy remote status --json\n  adbuddy remote off",
		Args:      cobra.ArbitraryArgs,
		ValidArgs: []string{"on", "off", "status"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewError("AGENT_SESSION", "`adbuddy remote` cannot be run from the agent's session", remoteFixes["AGENT_SESSION"])
			}
			if len(args) != 1 || args[0] != "on" && args[0] != "off" && args[0] != "status" {
				return UsageError("use `adbuddy remote on`, `off` or `status`")
			}
			op := args[0]
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			var info remoteInfo
			exit, err := ipc.Call(cmd.Context(), e.dirs.Data, ipc.Request{Kind: ipc.KindRemote, Argv: []string{op}, Env: map[string]string{"ADBUDDY_SOURCE": app.Getenv("ADBUDDY_SOURCE")}},
				nil, app.Stderr, app.Stderr, func(b []byte) error { return json.Unmarshal(b, &info) })
			switch {
			case errors.Is(err, ipc.ErrNotRunning):
				switch op {
				case "on":
					return notRunning
				case "off":
					var offErr error
					if info, offErr = remoteOffWithoutServe(cmd.Context(), app, e); offErr != nil {
						return offErr
					}
				default:
					info = remoteStatusWithoutServe(e)
				}
			case err != nil:
				if cmd.Context().Err() != nil {
					return Interrupted()
				}
				return NewError("DAEMON_UNREACHABLE", err.Error(), "")
			case exit.Code != 0:
				code := orDefault(exit.ErrorCode, "DAEMON_ERROR") // e.g. "unauthorized" after the secret changed
				return &Error{Code: code, Message: exit.Error, Fix: remoteFixes[code], exit: exit.Code}
			}
			if app.JSON {
				return WriteData(app.Stdout, info)
			}
			fmt.Fprintln(app.Stdout, remoteText(op, info))
			return nil
		},
	}
}

// remoteText is what the person reads.
func remoteText(op string, i remoteInfo) string {
	prefix := ""
	if !i.Serve {
		prefix = "serve is not running; "
	}
	sentence := func(s string) string {
		if prefix == "" && s != "" {
			return strings.ToUpper(s[:1]) + s[1:]
		}
		return prefix + s
	}
	var until string
	if i.ExpiresAt != nil {
		until = i.ExpiresAt.Local().Format("15:04")
	}
	switch {
	case !i.Enabled && op == "off":
		return "Remote mode is off: port 5555 is closed."
	case !i.Enabled:
		return sentence("remote mode: off")
	case i.Closing && i.Serve:
		return sentence("remote mode: expired at " + until + ", but port 5555 is not closed yet. Serve keeps trying; you can also run 'adbuddy remote off'.")
	case i.Closing:
		return sentence("remote mode: expired at " + until + ", but port 5555 may still be open. Run 'adbuddy remote off' to close it.")
	}
	left := "about " + remoteLeft(i)
	reach := "Reach the phone at its Tailscale address on port 5555."
	if i.Address != "" {
		reach = "Reach the phone at " + i.Address + "."
	}
	switch {
	case op == "on":
		return "Remote mode is on until " + until + " (" + left + "). " + reach +
			" It stays open on any network until then: turn it off with 'adbuddy remote off' when you are home."
	case !i.Serve:
		return sentence("remote mode: on until "+until+" ("+left+"). "+reach) +
			" Nothing closes it while serve is stopped: run 'adbuddy remote off'."
	}
	return sentence("remote mode: on until " + until + " (" + left + "). " + reach)
}

// remoteLeft is the time to the expiry, rounded to hours (minutes when under one).
func remoteLeft(i remoteInfo) string {
	d := i.ExpiresAt.Sub(remoteNow())
	if d >= 30*time.Minute {
		return fmt.Sprintf("%d h", int((d+30*time.Minute)/time.Hour))
	}
	return fmt.Sprintf("%d min", max(1, int((d+30*time.Second)/time.Minute)))
}

// cfgView is config.toml as a read-only remote.Store.
type cfgView struct{ cfg config.Config }

func (v cfgView) TailscaleIP() string      { return v.cfg.TailscaleIP }
func (v cfgView) Until() time.Time         { return v.cfg.RemoteUntilTime() }
func (v cfgView) SetUntil(time.Time) error { return errors.New("read-only") }

// remoteStatusWithoutServe reads the mode from config.toml. An expiry that
// has passed shows as expired and not closed: with serve stopped nothing has
// closed the port.
func remoteStatusWithoutServe(e env) remoteInfo {
	return infoOf(remote.New(nil, cfgView{e.cfg}, remoteNow).State(), false)
}

// remoteOffWithoutServe closes the port directly: `adb usb` on the phone, and
// only when it worked the expiry is cleared (if it fails the mode stays on).
// There is no serve to hold config.toml, so this is its one writer now.
func remoteOffWithoutServe(ctx context.Context, app *App, e env) (remoteInfo, error) {
	adbPath := configuredADB(e, "")
	if adbPath == "" {
		return remoteInfo{}, ConfigError("ADB_NOT_CONFIGURED", "ADBuddy's adb is not installed", "Run `adbuddy setup`.")
	}
	runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
	failed := func(why string) error {
		return NewError("REMOTE_OFF_FAILED", why+"; the remote mode stays on in config.toml",
			"Connect the phone (wireless debugging on, same network or Tailscale) and run `adbuddy remote off` again; restarting the phone also closes the port.")
	}
	if err := startServer(ctx, runner); err != nil {
		return remoteInfo{}, err
	}
	k, err := findKnownPhone(ctx, runner, e.cfg.Serial, candidateAddrs(e.cfg, true), deviceWait, app.Stderr)
	switch {
	case ctx.Err() != nil:
		return remoteInfo{}, Interrupted()
	case hasCode(err, "WRONG_PHONE") || hasCode(err, "DEVICE_UNREADABLE"):
		return remoteInfo{}, err
	case err != nil:
		return remoteInfo{}, failed("could not reach the phone (" + classify.Printable(err.Error()) + ")")
	}
	runner.Serial = k.Active
	octx, cancel := context.WithTimeout(ctx, remoteOpTimeout)
	defer cancel()
	var out, errb strings.Builder
	code, err := runner.Run(octx, "", []string{"usb"}, nil, &out, &errb)
	switch {
	case ctx.Err() != nil:
		return remoteInfo{}, Interrupted()
	case err != nil:
		return remoteInfo{}, failed("adb usb failed (" + classify.Printable(err.Error()) + ")")
	case code != 0:
		return remoteInfo{}, failed("adb usb failed (" + classify.Printable(strings.TrimSpace(errb.String()+" "+out.String())) + ")")
	}
	cfg, err := config.Load(e.dirs.ConfigFile())
	if err != nil {
		return remoteInfo{}, ConfigError("CONFIG_INVALID", err.Error(), "Fix or delete "+e.dirs.ConfigFile()+".")
	}
	cfg.RemoteUntil = ""
	if err := config.Save(e.dirs.ConfigFile(), cfg); err != nil {
		return remoteInfo{}, NewError("CONFIG_UNWRITABLE", "port 5555 is closed but remote_until could not be cleared: "+err.Error(),
			"Delete the remote_until line in "+e.dirs.ConfigFile()+".")
	}
	return remoteInfo{}, nil
}

// remoteConfirmTitle is the question's title, in the user's language.
func remoteConfirmTitle(lang string) string {
	if lang == "es" {
		return "Abrir el puerto de red 5555 de adb durante 12 horas"
	}
	return "Open adb's network port 5555 for 12 hours"
}

func remoteConfirmDetail(lang string) string {
	if lang == "es" {
		return "adb escuchará en el puerto 5555 de cualquier red hasta que lo cierres con `adbuddy remote off` o pasen 12 horas."
	}
	return "adb will listen on port 5555 on any network until you close it with `adbuddy remote off` or 12 hours pass."
}

// withdrawRemoteAsk cancels the question of `remote on` that is waiting, if
// any (a pause from the phone).
func (l *phoneLink) withdrawRemoteAsk() {
	l.mu.Lock()
	cancel := l.askCancel
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// noPhone reports whether adb failed because no phone is connected or it is
// offline: the spec's REMOTE_UNAVAILABLE. Matched on adb's own messages.
func noPhone(err error) bool {
	if errors.Is(err, adb.ErrNoDevice) {
		return true
	}
	m := strings.ToLower(err.Error())
	for _, s := range []string{"no devices/emulators found", "device offline", "no devices", "no phone connected"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// useRemoteAsk sets the language and the time `adbuddy remote on` asks the
// user in. Call it before the IPC server starts.
func (l *phoneLink) useRemoteAsk(lang string, timeout time.Duration) {
	l.askLang, l.askTimeout = lang, timeout
}

func remoteExit(code int, errCode, format string, a ...any) ipc.Exit {
	return ipc.Exit{Code: code, ErrorCode: errCode, Error: fmt.Sprintf(format, a...)}
}

// handleRemote answers `adbuddy remote`. The agent is refused, but it declares
// where it comes from itself, so the real defence of `on` is the question: the
// user answers it (app or terminal), every time, and only for this request.
// The change goes through serve's controller and its queue, behind the changes
// asked before (the app's included); config.toml is only written by serve's
// own store.
func (l *phoneLink) handleRemote(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
	if req.Env["ADBUDDY_SOURCE"] == "agent" {
		return remoteExit(1, "AGENT_SESSION", "the agent cannot change the remote mode")
	}
	if len(req.Argv) != 1 || req.Argv[0] != "on" && req.Argv[0] != "off" && req.Argv[0] != "status" {
		return remoteExit(2, "USAGE", "use on, off or status")
	}
	r := l.remote
	if r == nil || r.ctl == nil {
		return remoteExit(1, "REMOTE_UNAVAILABLE", "the remote mode is not available on this host")
	}
	reply := func(s remote.State) ipc.Exit {
		if c != nil {
			_ = c.Data(infoOf(s, true))
		}
		return ipc.Exit{}
	}
	switch req.Argv[0] {
	case "status":
		return reply(r.ctl.State())
	case "on":
		if s := r.ctl.State(); s.Enabled && !s.Closing {
			return reply(s) // nothing opens: nothing to ask
		}
		if r.store.TailscaleIP() == "" {
			return remoteExit(1, "REMOTE_UNAVAILABLE", "%s", remote.ErrNoTailscale.Error())
		}
		if exit := l.confirmRemoteOn(ctx); exit.Code != 0 {
			return exit
		}
		return l.changeRemote(ctx, true, reply)
	}
	return l.changeRemote(ctx, false, reply)
}

// confirmRemoteOn asks the user to open the port. The request has no Key and
// is Special so that "allow during the session" never covers it, and a fresh
// ID so that the answer is for this question only.
func (l *phoneLink) confirmRemoteOn(ctx context.Context) ipc.Exit {
	denied := func(why string) ipc.Exit {
		return remoteExit(exitRemoteDenied, "REMOTE_DENIED", "%s: the remote mode was not turned on", why)
	}
	if l.Paused() {
		return denied("the agent is paused from the phone")
	}
	timeout := l.askTimeout
	if timeout <= 0 {
		timeout = config.DefaultConfirmTimeout
	}
	qctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// One question at a time; a pause from the phone withdraws it.
	l.mu.Lock()
	if l.askBusy {
		l.mu.Unlock()
		return remoteExit(1, "REMOTE_BUSY", "another question to turn the remote mode on is waiting for an answer")
	}
	l.askBusy, l.askCancel = true, cancel
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.askBusy, l.askCancel = false, nil
		l.mu.Unlock()
	}()
	offs := l.remote.offCount()
	now := time.Now()
	d := l.Confirmer().Confirm(qctx, confirm.Request{
		ID: journal.NewID("r_", now), Title: remoteConfirmTitle(l.askLang), Detail: remoteConfirmDetail(l.askLang),
		RawCommand: "adbuddy remote on", ExpiresAt: now.UTC().Add(timeout), Special: true, Key: "",
	})
	if l.Paused() { // an answer that comes after a pause counts for nothing
		return denied("the agent was paused from the phone")
	}
	if d == confirm.Allow && l.remote.offCount() != offs {
		return denied("the remote mode was turned off while asking; run it again")
	}
	switch d {
	case confirm.Allow:
		return ipc.Exit{}
	case confirm.Timeout:
		return denied("the confirmation expired")
	case confirm.Nobody:
		return denied("nobody could confirm (serve --confirm deny or no terminal)")
	}
	return denied("denied by the user")
}

// changeRemote queues the change behind the earlier ones and waits for it.
// The adb calls run on serve's context, not on the request's: a client that
// goes away must not cut `adb tcpip` or `adb usb` half way.
func (l *phoneLink) changeRemote(ctx context.Context, on bool, reply func(remote.State) ipc.Exit) ipc.Exit {
	r := l.remote
	done := make(chan remoteResult, 1)
	if !r.enqueue(remoteJob{on: on, timeout: remoteOpTimeout, reply: done}) {
		return remoteExit(1, "REMOTE_BUSY", "too many remote mode requests are waiting")
	}
	select {
	case res := <-done:
		switch {
		case errors.Is(res.err, remote.ErrNoTailscale):
			return remoteExit(1, "REMOTE_UNAVAILABLE", "%s", res.err.Error())
		case res.err != nil && noPhone(res.err):
			return remoteExit(1, "REMOTE_UNAVAILABLE", "no phone connected: %s", classify.Printable(res.err.Error()))
		case res.err != nil:
			msg := classify.Printable(res.err.Error())
			if s := r.ctl.State(); s.Enabled {
				msg += " (the remote mode is still on)"
			}
			return remoteExit(1, "REMOTE_FAILED", "%s", msg)
		}
		return reply(res.state)
	case <-ctx.Done():
		return remoteExit(1, "INTERRUPTED", "the request was abandoned; the change may still run")
	}
}
