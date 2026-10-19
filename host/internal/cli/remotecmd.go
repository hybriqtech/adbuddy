package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
	"github.com/hybriqtech/adbuddy/host/internal/session"
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
// (otherwise it was read from config.toml). PersistPort is
// persist.adb.tcp.port when the last read found it naming a port (VB-41: adbd
// listens on it again when the phone restarts), and PersistUnknown says that
// read failed: an "off" with either is never a plain "closed".
type remoteInfo struct {
	Enabled        bool       `json:"enabled"`
	Closing        bool       `json:"closing,omitempty"`
	Address        string     `json:"address,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	Serve          bool       `json:"serve"`
	PersistPort    string     `json:"persistPort,omitempty"`
	PersistUnknown bool       `json:"persistUnknown,omitempty"`
}

func infoOf(s remote.State, serve bool) remoteInfo {
	i := remoteInfo{Enabled: s.Enabled, Closing: s.Closing, Address: s.Address, Serve: serve, PersistPort: s.Persist, PersistUnknown: s.PersistUnknown}
	if s.Enabled {
		t := s.ExpiresAt.UTC()
		i.ExpiresAt = &t
	}
	return i
}

// remoteFixKeys are the catalog keys of the fix of each code the daemon can
// answer `adbuddy remote` with; a code that is not here has no fix.
var remoteFixKeys = map[string]string{
	"AGENT_SESSION":      "err.remotecmd.fix.agent_session",
	"REMOTE_UNAVAILABLE": "err.remotecmd.fix.remote_unavailable",
	"REMOTE_DENIED":      "err.remotecmd.fix.remote_denied",
	"REMOTE_FAILED":      "err.remotecmd.fix.remote_failed",
	"REMOTE_BUSY":        "err.remotecmd.fix.remote_busy",
}

func newRemoteCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:       "remote on|off|status",
		Short:     app.T("help.remote.short"),
		Long:      app.T("help.remote.long"),
		Example:   "  adbuddy remote on\n  adbuddy remote status --json\n  adbuddy remote off",
		Args:      cobra.ArbitraryArgs,
		ValidArgs: []string{"on", "off", "status"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewErrorT("AGENT_SESSION", "err.remotecmd.agent_session", remoteFixKeys["AGENT_SESSION"])
			}
			if len(args) != 1 || args[0] != "on" && args[0] != "off" && args[0] != "status" {
				return UsageErrorT("err.remotecmd.usage_use_adbuddy_remote_on_off")
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
					// Already off in config.toml: nothing to close, and `adb usb`
					// would restart adbd for nothing.
					if e.cfg.RemoteUntil == "" {
						if app.JSON {
							return WriteData(app.Stdout, remoteInfo{})
						}
						fmt.Fprintln(app.Stdout, app.T("remotecmd.serve_is_not_running_remote"))
						return nil
					}
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
				// The daemon's text is English and for the agent: it is the cause; the fix is ours.
				return errorT(exit.Code, false, code, "error.cause", remoteFixKeys[code], []any{exit.Error})
			}
			if app.JSON {
				return WriteData(app.Stdout, info)
			}
			fmt.Fprintln(app.Stdout, remoteTextIn(app.speaks(), op, info))
			return nil
		},
	}
}

// remoteText is what the person reads, in English.
func remoteText(op string, i remoteInfo) string { return remoteTextIn(i18n.EN, op, i) }

// remoteTextIn is remoteText in lang. A sentence that follows "serve is not
// running; " keeps its first letter as the catalog has it (lower case); alone,
// it starts with a capital.
func remoteTextIn(lang i18n.Lang, op string, i remoteInfo) string {
	prefix := ""
	if !i.Serve {
		prefix = lang.T("remote.text.no_serve")
	}
	sentence := func(s string) string {
		if prefix == "" && s != "" {
			r, n := utf8.DecodeRuneInString(s)
			return string(unicode.ToUpper(r)) + s[n:]
		}
		return prefix + s
	}
	var until string
	if i.ExpiresAt != nil {
		until = i.ExpiresAt.Local().Format("15:04")
	}
	// VB-41: off with a persist port, or with persist unread, is not closed
	// for good.
	var persist string
	switch {
	case i.PersistPort != "":
		persist = lang.T("remote.text.persist", i.PersistPort)
	case i.PersistUnknown:
		persist = lang.T("remote.text.persist_unknown")
	}
	switch {
	case !i.Enabled && op == "off" && persist != "":
		return lang.T("remote.text.off_mode") + " " + persist
	case !i.Enabled && op == "off":
		return lang.T("remote.text.off_closed")
	case !i.Enabled && persist != "":
		return sentence(lang.T("remote.text.off")) + ". " + persist
	case !i.Enabled:
		return sentence(lang.T("remote.text.off"))
	case i.Closing && i.Serve:
		return sentence(lang.T("remote.text.expired_serve", until))
	case i.Closing:
		return sentence(lang.T("remote.text.expired", until))
	}
	left := lang.T("remote.text.about", remoteLeft(i))
	reach := lang.T("remote.text.reach_tailscale")
	if i.Address != "" {
		reach = lang.T("remote.text.reach", i.Address)
	}
	switch {
	case op == "on":
		return lang.T("remote.text.on", until, left, reach)
	case !i.Serve:
		return sentence(lang.T("remote.text.status_on", until, left, reach)) + " " + lang.T("remote.text.no_serve_rest")
	}
	return sentence(lang.T("remote.text.status_on", until, left, reach))
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
// There is no serve to hold config.toml, so this is its one writer now. It
// reads persist.adb.tcp.port first (VB-41: `adb usb` does not clear it, and
// right after it adbd restarts) and says what it holds.
func remoteOffWithoutServe(ctx context.Context, app *App, e env) (remoteInfo, error) {
	adbPath := configuredADB(e, "")
	if adbPath == "" {
		return remoteInfo{}, ConfigErrorT("ADB_NOT_CONFIGURED", "err.remotecmd.adb_not_configured", "err.remotecmd.adb_not_configured.fix")
	}
	runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
	failed := func(why i18n.Msg) error {
		return NewErrorT("REMOTE_OFF_FAILED", "err.remotecmd.remote_off_failed", "err.remotecmd.remote_off_failed.fix", why)
	}
	if err := startServer(ctx, runner); err != nil {
		return remoteInfo{}, err
	}
	k, err := findKnownPhone(ctx, runner, e.cfg.Serial, candidateAddrs(e.cfg, true), deviceWait, app.Log())
	switch {
	case ctx.Err() != nil:
		return remoteInfo{}, Interrupted()
	case hasCode(err, "WRONG_PHONE") || hasCode(err, "DEVICE_UNREADABLE"):
		return remoteInfo{}, err
	case err != nil:
		return remoteInfo{}, failed(i18n.M("remote.off.unreachable", classify.Printable(err.Error())))
	}
	runner.Serial = k.Active
	octx, cancel := context.WithTimeout(ctx, remoteOpTimeout)
	defer cancel()
	// Its own short deadline: a hung read must not eat the budget of adb usb.
	rctx, rcancel := context.WithTimeout(ctx, device.ReadTCPWait)
	t, rerr := device.ReadTCP(rctx, runner)
	rcancel()
	var out, errb strings.Builder
	code, err := runner.Run(octx, "", []string{"usb"}, nil, &out, &errb)
	switch {
	case ctx.Err() != nil:
		return remoteInfo{}, Interrupted()
	case err != nil:
		return remoteInfo{}, failed(i18n.M("remote.off.usb_failed", classify.Printable(err.Error())))
	case code != 0:
		return remoteInfo{}, failed(i18n.M("remote.off.usb_failed", classify.Printable(strings.TrimSpace(errb.String()+" "+out.String()))))
	}
	cfg, err := config.Load(e.dirs.ConfigFile())
	if err != nil {
		return remoteInfo{}, ConfigErrorT("CONFIG_INVALID", "error.cause", "err.remotecmd.config_invalid.fix", err.Error(), e.dirs.ConfigFile())
	}
	cfg.RemoteUntil = ""
	if err := config.Save(e.dirs.ConfigFile(), cfg); err != nil {
		return remoteInfo{}, NewErrorT("CONFIG_UNWRITABLE", "err.remotecmd.config_unwritable", "err.remotecmd.config_unwritable.fix", err.Error(), e.dirs.ConfigFile())
	}
	return remoteInfo{PersistPort: t.Persist, PersistUnknown: rerr != nil}, nil
}

// remoteConfirmTitle is the question's title, in the user's language.
func remoteConfirmTitle(lang string) string { return i18n.Lang(lang).T("remote.confirm.title") }

func remoteConfirmDetail(lang string) string { return i18n.Lang(lang).T("remote.confirm.detail") }

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

// useRemoteAsk sets the language and the time `adbuddy remote on` asks the
// user in. Call it before the IPC server starts.
func (l *phoneLink) useRemoteAsk(lang string, timeout time.Duration) {
	l.askLang, l.askTimeout = lang, timeout
}

func remoteExit(code int, errCode, format string, a ...any) ipc.Exit {
	return ipc.Exit{Code: code, ErrorCode: errCode, Error: fmt.Sprintf(format, a...)}
}

// handleRemote answers `adbuddy remote`. The agent is refused (fromAgent: its
// process tree, as serve sees it, or its declaration; sess may be nil), but a
// process it detaches from its tree is not seen, so the real defence of `on`
// is the question: the user answers it (app or terminal), every time, and
// only for this request.
// It is asked through d, serve's daemon, so that while it is open every
// command of the agent is refused (an `input tap` could press Allow). The
// change goes through serve's controller and its queue, behind the changes
// asked before (the app's included); config.toml is only written by serve's
// own store.
func (l *phoneLink) handleRemote(ctx context.Context, req ipc.Request, c *ipc.Conn, d *daemon.Daemon, sess *session.Manager) ipc.Exit {
	if fromAgent(sess, req, c) {
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
		if exit := l.confirmRemoteOn(ctx, d); exit.Code != 0 {
			return exit
		}
		return l.changeRemote(ctx, true, reply)
	}
	return l.changeRemote(ctx, false, reply)
}

// confirmRemoteOn asks the user to open the port, through the daemon's door
// for questions (Daemon.AskUser): one question at a time with the 🔴 ones,
// and the agent blocked while it is open. The request has no Key and is
// Special so that "allow during the session" never covers it, and a fresh
// ID so that the answer is for this question only. Without a daemon nobody
// can block the agent: it is not asked, and nothing opens.
func (l *phoneLink) confirmRemoteOn(ctx context.Context, d *daemon.Daemon) ipc.Exit {
	denied := func(why string) ipc.Exit {
		return remoteExit(exitRemoteDenied, "REMOTE_DENIED", "%s: the remote mode was not turned on", why)
	}
	if d == nil {
		return remoteExit(1, "REMOTE_UNAVAILABLE", "the remote mode is not available on this host")
	}
	if l.Paused() {
		return denied("the agent is paused from the phone")
	}
	timeout := l.askTimeout
	if timeout <= 0 {
		timeout = config.DefaultConfirmTimeout
	}
	qctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// One question of `on` at a time (the daemon also refuses one while any
	// other question is open); a pause from the phone withdraws it.
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
	dec, err := d.AskUser(qctx, confirm.Request{
		ID: journal.NewID("r_", time.Now()), Title: remoteConfirmTitle(l.askLang), Detail: remoteConfirmDetail(l.askLang),
		RawCommand: "adbuddy remote on", Special: true, Key: "",
	}, timeout)
	switch {
	case errors.Is(err, daemon.ErrAskBusy):
		return remoteExit(1, "REMOTE_BUSY", "%s", classify.Printable(err.Error()))
	case errors.Is(err, daemon.ErrAskPaused) || err != nil && l.Paused():
		return denied("the agent was paused from the phone")
	case errors.Is(err, daemon.ErrAskYolo):
		// YOLO was turned on while it waited: the question was withdrawn (nothing
		// answered after that counts) and the remote mode stays off; ask again.
		return denied(daemon.YoloWithdrawnMessage)
	case err != nil:
		return denied("the question was abandoned before it was asked")
	}
	if l.Paused() { // an answer that comes after a pause counts for nothing
		return denied("the agent was paused from the phone")
	}
	if dec == confirm.Allow && l.remote.offCount() != offs {
		return denied("the remote mode was turned off while asking; run it again")
	}
	switch dec {
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
		case res.err != nil:
			// No phone: `on` put back the previous expiry (the order never
			// reached adbd); whatever stays on (Closing included) is said.
			msg := classify.Printable(res.err.Error())
			if s := r.ctl.State(); s.Enabled {
				msg += " (the remote mode is still on)"
			}
			if remote.NoPhone(res.err) {
				return remoteExit(1, "REMOTE_UNAVAILABLE", "no phone connected: %s", msg)
			}
			return remoteExit(1, "REMOTE_FAILED", "%s", msg)
		}
		return reply(res.state)
	case <-ctx.Done():
		return remoteExit(1, "INTERRUPTED", "the request was abandoned; the change may still run")
	}
}
