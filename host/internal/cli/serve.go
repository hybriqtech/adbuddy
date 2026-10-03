package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/remote"
	"github.com/hybriqtech/adbuddy/host/internal/session"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// deviceWait is how long serve waits for the phone: ADBuddy's private adb
// server needs some seconds to reconnect a paired phone found by mDNS (a
// variable so tests can shorten it).
var deviceWait = 45 * time.Second

func newServeCmd(app *App) *cobra.Command {
	var adbPath, serial, wsDir, confirmMode string
	var noAgent bool
	var model, effort string
	cmd := &cobra.Command{
		Use:     "serve",
		Short:   app.T("help.serve.short"),
		Long:    app.T("help.serve.long"),
		Example: "  adbuddy serve\n  adbuddy serve --adb ~/platform-tools/adb --serial adb-XXXX-yyyy._adb-tls-connect._tcp\n  adbuddy serve --json > journal.ndjson",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Ctrl+C at any step, from the first check on, exits with 130.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			// The agent must not be able to start its own host and answer
			// its own confirmations.
			if app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewErrorT("AGENT_SESSION", "err.serve.agent_session", "err.serve.agent_session.fix")
			}
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			if model == "" {
				model = e.cfg.AgentModel
			}
			if effort == "" {
				effort = e.cfg.AgentEffort
			}
			if model != "" && !agent.ValidModel(model) {
				return UsageErrorT("err.serve.usage_model_invalid")
			}
			if effort != "" && !agent.ValidEffort(effort) {
				return UsageErrorT("err.serve.usage_effort_invalid")
			}
			adbPath = configuredADB(e, adbPath)
			if adbPath == "" {
				return ConfigErrorT("ADB_NOT_CONFIGURED", "err.serve.adb_not_configured", "err.serve.adb_not_configured.fix")
			}
			if serial == "" {
				serial = e.cfg.Serial
			}
			// Another serve would get its workspace rewritten under its agent:
			// find it before touching anything or waiting for the phone.
			if running, err := serveRunning(ctx, e.dirs.Data); ctx.Err() != nil {
				return Interrupted()
			} else if err != nil {
				return err
			} else if running {
				return alreadyRunning
			}
			// One serve per data folder from here on (ipc.Lock), taken before
			// the workspace is touched and before the watchers of the phone
			// and of the remote mode start: a second serve stops here. The
			// server releases it on Close; the defer covers every other way
			// out.
			lock, err := ipc.Lock(e.dirs.Data)
			if errors.Is(err, ipc.ErrAlreadyRunning) {
				return alreadyRunning
			}
			if err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			defer func() { stop(); lock.Close() }() // watchers first, then the lock
			ws, err := filepath.Abs(e.workspace(wsDir))
			if err != nil {
				return NewErrorT("WORKSPACE_FAILED", "error.cause", "err.serve.workspace_failed.fix", err.Error())
			}
			exe, err := os.Executable()
			if err != nil {
				return NewError("EXECUTABLE_UNKNOWN", err.Error(), "")
			}
			if _, err := workspace.Init(workspace.Options{Dir: ws, Exe: exe}); err != nil {
				return NewErrorT("WORKSPACE_FAILED", "error.cause", "error.hint", err.Error(), workspaceHintMsg(err, ws))
			}
			cls, err := loadClassifier(e)
			if err != nil {
				return err
			}
			if n := cls.UserRuleCount(); n > 0 {
				fmt.Fprintln(app.Stderr, app.T("serve.using_rules_of_yours_on", n, rulesPath(e)))
			}
			if ctx.Err() != nil {
				return Interrupted()
			}

			// The one writer of config.toml from here on (chat port, the phone's
			// Tailscale address, the remote mode's expiry).
			store := newCfgStore(e.dirs.ConfigFile(), e.cfg)
			runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
			fmt.Fprintln(app.Stderr, app.T("serve.waiting_for_the_phone_on", e.cfg.ADBServerPort))
			// The phone that mDNS does not find (Tailscale, another network) is
			// tried at the address that worked last time.
			// The addresses are those of the phone in config.toml. The remote
			// mode counts as on while an expiry is saved, even a past one: its
			// controller does not exist yet (and Closing is still on).
			var addrs []string
			if serial == e.cfg.Serial {
				addrs = candidateAddrs(store.Config(), !store.Until().IsZero())
			}
			reconnect := i18n.M("serve.reconnect") // a piece of the fix: it goes in the language of the error
			k, err := findKnownPhone(ctx, runner, serial, addrs, deviceWait, app.Log())
			switch {
			case err != nil && ctx.Err() != nil:
				return Interrupted()
			case hasCode(err, "WRONG_PHONE") || hasCode(err, "DEVICE_UNREADABLE"):
				return err
			case errors.Is(err, adb.ErrNoDevice) && k.AddrErr != nil:
				return TransientErrorT("DEVICE_NOT_FOUND", "err.serve.device_not_found", "err.serve.device_not_found.fix", strings.Join(addrs, ", "), AsError(k.AddrErr).Message, reconnect)
			case errors.Is(err, adb.ErrNoDevice):
				return TransientErrorT("DEVICE_NOT_FOUND", "err.serve.device_not_found_2", "err.serve.device_not_found_2.fix", strconv.Itoa(e.cfg.ADBServerPort))
			case errors.Is(err, adb.ErrManyDevices):
				return NewErrorT("MULTIPLE_DEVICES", "err.serve.multiple_devices", "err.serve.multiple_devices.fix", strconv.Itoa(e.cfg.ADBServerPort), e.dirs.ConfigFile())
			case err != nil:
				return NewErrorT("ADB_FAILED", "error.cause", "err.serve.adb_failed.fix", err.Error(), adbPath)
			}
			activeSerial := k.Active
			// The journal and undo know the phone by its stable name only when
			// its identity was checked: otherwise by the serial of this session,
			// so undo of older changes stops at OTHER_DEVICE.
			identity := activeSerial
			if k.Verified {
				identity = phoneIdentity(serial, activeSerial)
			} else if _, byGuid := pairing.GuidOf(serial); byGuid || k.Used != "" {
				fmt.Fprintln(app.Stderr, app.T("serve.note_could_not_check_that", activeSerial, unverifiedWhy(serial)))
			}
			hostRunner := runner // for host commands: the watcher of the connection has no serial
			runner.Serial = activeSerial
			// From here every command to the agent's phone asks `active` for its
			// serial (it can change); `runner` is not used again.
			active := newActivePhone(runner)
			// The remote mode (M5b): what a previous serve left open is looked at
			// before the invariant, which keeps port 5555 only while it is on.
			ctl := remote.New(active, store, time.Now)
			keep := ""
			rs, err := ctl.Reconcile(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return Interrupted()
				}
				fmt.Fprintln(app.Stderr, app.T("serve.could_not_check_the_remote", classify.Printable(err.Error())))
			}
			if rs.Enabled {
				keep = remote.Port
				fmt.Fprintln(app.Stderr, remoteOnNoticeIn(app.speaks(), rs))
			}
			// Host invariant (spec §3.3): no adb network port while remote mode is off.
			tcp, err := invariantTCP(ctx, active.Runner(), deviceWait, app.Log(), "adbuddy serve", keep)
			if err != nil {
				if ctx.Err() != nil {
					return Interrupted()
				}
				return err
			}
			// VB-41: `adbuddy remote status` says what persist.adb.tcp.port
			// holds from the start, not only after the first "off".
			ctl.Note(tcp)

			secret, err := ipc.EnsureSecret(e.dirs.Data)
			if err != nil {
				return NewErrorT("SECRET_FAILED", "error.cause", "err.serve.secret_failed.fix", err.Error(), e.dirs.Data)
			}
			var confirmer confirm.Confirmer
			switch confirmMode {
			case "auto":
				confirmer = confirm.DenyAll{Out: app.Stderr, Lang: app.Lang}
				if isTerminal(app.Stdin) {
					confirmer = confirm.NewTerminal(app.Stdin, app.Stderr).WithLang(app.Lang)
				}
			case "terminal":
				confirmer = confirm.NewTerminal(app.Stdin, app.Stderr).WithLang(app.Lang)
			case "deny":
				confirmer = confirm.DenyAll{Out: app.Stderr, Lang: app.Lang}
			default:
				return UsageErrorT("err.serve.usage_confirm_must_be_auto_terminal")
			}

			agentName := e.cfg.Agent
			if agentName == "" {
				agentName = agent.Known[0]
			}
			var outMu sync.Mutex
			session := journal.NewID("s_", time.Now())
			// Without the app, or if its channel fails, confirmations stay
			// here: only Ctrl+C stops serve at this step.
			link, err := startPhoneLink(ctx, app, &e, store, ctl, active, session, agentName, confirmer)
			if err != nil {
				return err
			}
			// Read here, before the goroutine: a test that sets remoteTick for its
			// own serve must not race with the goroutine of a previous one.
			every, opTimeout := remoteTick, remoteOpTimeout
			go watchRemote(ctx, app.Log(), ctl, link.remote, every, opTimeout)
			// Same here for the watcher of the phone's connection. It never
			// touches the adb port: only the remote mode does.
			go watchPhone(ctx, phoneWatch{
				Active: active, Runner: hostRunner, Serial: serial,
				Cfg: store.Config, RemoteOn: func() bool { return !store.Until().IsZero() }, Log: app.Log(),
				Every: watchEvery, Misses: watchMisses, TryWait: watchTryWait,
				Port: tcp, Seen: ctl.Note, ReadWait: device.ReadTCPWait,
			})
			ui, protected := link.helpers()
			d := daemon.New(daemon.Options{
				Classifier: cls, ADB: active, Journal: journal.OpenSigned(workspace.JournalDir(ws), []byte(secret)), Confirmer: link.Confirmer(),
				Paused: link.Paused, RemoteActive: remoteActive(ctl),
				UI: ui, Protected: protected,
				Serials: active.Serials, CurrentSerial: active.Serial,
				Serial: activeSerial, Identity: identity, Workspace: ws, Session: session, Lang: e.cfg.Lang(app.Getenv), ConfirmTimeout: e.cfg.ConfirmTimeout.Duration, Version: Version,
				UndoneLog: undoneLogPath(e.dirs.Data, workspace.JournalDir(ws)),
				OnEntry: func(j journal.Entry) {
					outMu.Lock()
					defer outMu.Unlock()
					if app.JSON {
						_ = writeJSON(app.Stdout, j)
					} else {
						fmt.Fprintln(app.Stdout, formatEntryIn(app.speaks(), j))
					}
					link.Entry(j)
				},
				OnWarning: func(w string) { fmt.Fprintln(app.Stderr, "⚠ "+w) },
				OnNotice:  func(n string) { fmt.Fprintln(app.Stderr, "⏳ "+n) },
			})
			link.OnPause(d.CancelConfirmations)
			sess := newSession(app, ws, activeSerial, link.sessionInfo, model, effort)
			defer sess.Stop()
			link.useSession(sess)
			link.useWorkspace(ws)
			srv, err := ipc.ListenLocked(lock, secret, serveHandler(link, e.cfg, app.Getenv, ws, agentName, sess, d))
			if errors.Is(err, ipc.ErrAlreadyRunning) {
				return alreadyRunning
			}
			if err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			fmt.Fprintln(app.Stderr, app.T("serve.adbuddy_is_serving_session_journal", activeSerial, session, workspace.JournalDir(ws)))
			if noAgent {
				fmt.Fprintln(app.Stderr, app.T("serve.in_another_terminal", agentName))
				fmt.Fprintln(app.Stderr)
			} else {
				if model != "" || effort != "" {
					if _, unused := agent.Args(agentName, model, effort); len(unused) > 0 {
						fmt.Fprintln(app.Stderr, app.T("serve.model_not_applied", agentName))
					} else {
						fmt.Fprintln(app.Stderr, app.T("serve.model_applied", agent.Describe(model, effort)))
					}
				}
				startAgent(app, sess, agentName)
				fmt.Fprintln(app.Stderr)
			}
			if err := serveUntil(ctx, stop, srv); err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			if s := ctl.State(); s.Enabled {
				fmt.Fprintln(app.Stderr, stillOnNoticeIn(app.speaks(), s))
			}
			fmt.Fprintln(app.Stderr, app.T("serve.adbuddy_stopped"))
			return nil
		},
	}
	cmd.Flags().StringVar(&adbPath, "adb", "", app.T("help.serve.flag.adb"))
	cmd.Flags().StringVar(&serial, "serial", "", app.T("help.serve.flag.serial"))
	cmd.Flags().StringVar(&wsDir, "workspace", "", app.T("help.serve.flag.workspace"))
	cmd.Flags().BoolVar(&noAgent, "no-agent", false, app.T("help.serve.flag.no_agent"))
	cmd.Flags().StringVar(&model, "model", "", app.T("help.serve.flag.model"))
	cmd.Flags().StringVar(&effort, "effort", "", app.T("help.serve.flag.effort"))
	cmd.Flags().StringVar(&confirmMode, "confirm", "auto", app.T("help.serve.flag.confirm"))
	return cmd
}

// serveHandler is what serve's IPC socket answers: each kind goes to the part
// that owns it. It also tells the link how to ask the user about `remote on`
// (their language and confirm_timeout, as the daemon has them).
func serveHandler(link *phoneLink, cfg config.Config, getenv func(string) string, ws, agentName string, sess *session.Manager, d *daemon.Daemon) ipc.Handler {
	lang := cfg.Lang(getenv)
	link.useRemoteAsk(lang, cfg.ConfirmTimeout.Duration)
	return func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
		if req.Kind == ipc.KindSend {
			return link.handleSend(ctx, ws, req, c)
		}
		if req.Kind == ipc.KindHook {
			return link.handleHook(ctx, ws, req, c)
		}
		if req.Kind == ipc.KindRemote {
			return link.handleRemote(ctx, req, c, d, sess)
		}
		if req.Kind == ipc.KindAttach || req.Kind == ipc.KindSession {
			return handleSessionIn(i18n.Parse(lang), ctx, sess, agentName, req, c)
		}
		return d.Handle(ctx, req, c)
	}
}

var alreadyRunning = NewErrorT("ALREADY_RUNNING", "err.serve.already_running", "err.serve.already_running.fix")

// serveRunning reports whether a serve answers on the data folder's socket.
// A socket owned by something that does not answer as a host (another
// secret, a timeout…) is an error: it must not be taken over.
func serveRunning(ctx context.Context, dataDir string) (bool, error) {
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := ipc.PingDaemon(pctx, dataDir)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ipc.ErrNotRunning):
		return false, nil
	}
	return false, NewErrorT("DAEMON_UNREACHABLE", "err.serve.daemon_unreachable", "err.serve.daemon_unreachable.fix", err.Error(), ipc.SocketPath(dataDir))
}

// serveUntil serves requests until ctx ends, then waits for the ones in
// flight: a pending confirmation is journaled as denied and its shim gets
// its answer before serve exits. Stopping also cancels a 🟡 change that is
// running: its adb is killed and the entry ends as `error`, so it counts as
// uncertain and is never undone automatically; check the phone by hand.
func serveUntil(ctx context.Context, stop context.CancelFunc, srv *ipc.Server) error {
	closed := make(chan struct{})
	go func() {
		<-ctx.Done()
		srv.Close() // cancels the requests and waits for them
		close(closed)
	}()
	err := srv.Serve()
	stop() // if Serve failed by itself; a second Ctrl+C then kills the process
	<-closed
	return err
}

// undoneLogPath is the log of undone ids for one journal. It lives in the data
// folder, out of the agent's reach, and is named after the journal's absolute
// path so each environment keeps its own. Like the journal's signature, the
// name depends on how that path is written (case, links): another spelling
// finds another log, but it also changes the signature's scope, so the old
// records stop verifying and undo refuses them instead of repeating them.
func undoneLogPath(dataDir, journalDir string) string {
	if abs, err := filepath.Abs(journalDir); err == nil {
		journalDir = abs
	}
	sum := sha256.Sum256([]byte(filepath.Clean(journalDir)))
	return filepath.Join(dataDir, "undone-"+hex.EncodeToString(sum[:])[:16]+".log")
}

// loadClassifier reads the user's rules.yaml (if any) on top of the built-in rules.
func loadClassifier(e env) (*classify.Classifier, error) {
	path := rulesPath(e)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, ConfigErrorT("RULES_UNREADABLE", "error.cause", "err.serve.rules_unreadable.fix", err.Error(), path)
	}
	cls, err := classify.New(data)
	if err != nil {
		return nil, ConfigErrorT("RULES_INVALID", "err.serve.rules_invalid", "err.serve.rules_invalid.fix", path, err.Error())
	}
	return cls, nil
}

func rulesPath(e env) string {
	if e.cfg.Rules != "" {
		return e.cfg.Rules
	}
	return e.dirs.RulesFile()
}

// remoteActive is what the daemon asks to know whether the remote mode is on:
// Enabled, which includes a pending close.
func remoteActive(ctl *remote.Controller) func() bool {
	return func() bool { return ctl.State().Enabled }
}
