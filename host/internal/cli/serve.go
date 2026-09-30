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
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/platformtools"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// deviceWait is how long serve waits for the phone: ADBuddy's private adb
// server needs some seconds to reconnect a paired phone found by mDNS (a
// variable so tests can shorten it).
var deviceWait = 45 * time.Second

func newServeCmd(app *App) *cobra.Command {
	var adbPath, serial, wsDir, confirmMode string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the ADBuddy host: live journal and confirmations for the agent's adb commands",
		Long: `Run the ADBuddy host in this terminal. It classifies every adb command the agent
runs, journals it, shows it here and asks you before anything irreversible.
Keep it open and start the agent in another terminal with ` + "`adbuddy run`" + `.`,
		Example: "  adbuddy serve\n  adbuddy serve --adb ~/platform-tools/adb --serial adb-XXXX-yyyy._adb-tls-connect._tcp\n  adbuddy serve --json > journal.ndjson",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// The agent must not be able to start its own host and answer
			// its own confirmations.
			if app.Getenv("ADBUDDY_SOURCE") == "agent" {
				return NewError("AGENT_SESSION", "`adbuddy serve` cannot be started from the agent's session", "Run it yourself in a separate terminal.")
			}
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			if adbPath == "" {
				adbPath = e.cfg.ADBPath
			}
			if adbPath == "" && fileExists(platformtools.ADBPath(e.dirs.Data, runtime.GOOS)) {
				adbPath = platformtools.ADBPath(e.dirs.Data, runtime.GOOS)
			}
			if adbPath == "" {
				return ConfigError("ADB_NOT_CONFIGURED", "ADBuddy's adb is not installed", "Run `adbuddy setup` (or pass --adb <path to adb 37 or newer>).")
			}
			if serial == "" {
				serial = e.cfg.Serial
			}
			// Another serve would get its workspace rewritten under its agent:
			// find it before touching anything or waiting for the phone.
			if running, err := serveRunning(cmd.Context(), e.dirs.Data); err != nil {
				return err
			} else if running {
				return alreadyRunning
			}
			ws, err := filepath.Abs(e.workspace(wsDir))
			if err != nil {
				return NewError("WORKSPACE_FAILED", err.Error(), "Pass --workspace with an absolute path.")
			}
			exe, err := os.Executable()
			if err != nil {
				return NewError("EXECUTABLE_UNKNOWN", err.Error(), "")
			}
			if _, err := workspace.Init(workspace.Options{Dir: ws, Exe: exe}); err != nil {
				return NewError("WORKSPACE_FAILED", err.Error(), "Check that "+ws+" is writable.")
			}
			cls, err := loadClassifier(e)
			if err != nil {
				return err
			}
			if n := cls.UserRuleCount(); n > 0 {
				fmt.Fprintf(app.Stderr, "Using %d rules of yours on top of the built-in ones (%s).\n", n, rulesPath(e))
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
			fmt.Fprintf(app.Stderr, "Waiting for the phone on ADBuddy's adb server (port %d)…\n", e.cfg.ADBServerPort)
			active, err := runner.WaitSerial(ctx, serial, deviceWait)
			switch {
			case err != nil && ctx.Err() != nil:
				return Interrupted()
			case errors.Is(err, adb.ErrNoDevice):
				return TransientError("DEVICE_NOT_FOUND", "no phone in state `device` on adb server port "+strconv.Itoa(e.cfg.ADBServerPort),
					"Turn on wireless debugging on the phone and keep it on the same Wi-Fi as this PC; if it was never paired, run `adbuddy setup`.")
			case errors.Is(err, adb.ErrManyDevices):
				return NewError("MULTIPLE_DEVICES", "more than one phone is connected", "Pass --serial <serial> (see `adb -P "+strconv.Itoa(e.cfg.ADBServerPort)+" devices`) or set serial in "+e.dirs.ConfigFile()+".")
			case err != nil:
				return NewError("ADB_FAILED", err.Error(), "Check the adb path ("+adbPath+").")
			}
			runner.Serial = active
			// Host invariant (spec §3.3): no adb network port while remote mode is off.
			if err := applyInvariant(ctx, runner, deviceWait, app.Stderr, "adbuddy serve"); err != nil {
				if ctx.Err() != nil {
					return Interrupted()
				}
				return err
			}

			secret, err := ipc.EnsureSecret(e.dirs.Data)
			if err != nil {
				return NewError("SECRET_FAILED", err.Error(), "Check that "+e.dirs.Data+" is writable.")
			}
			var confirmer confirm.Confirmer
			switch confirmMode {
			case "auto":
				confirmer = confirm.DenyAll{Out: app.Stderr}
				if isTerminal(app.Stdin) {
					confirmer = confirm.NewTerminal(app.Stdin, app.Stderr)
				}
			case "terminal":
				confirmer = confirm.NewTerminal(app.Stdin, app.Stderr)
			case "deny":
				confirmer = confirm.DenyAll{Out: app.Stderr}
			default:
				return UsageError("--confirm must be auto, terminal or deny")
			}

			var outMu sync.Mutex
			session := journal.NewID("s_", time.Now())
			d := daemon.New(daemon.Options{
				Classifier: cls, ADB: runner, Journal: journal.OpenSigned(workspace.JournalDir(ws), []byte(secret)), Confirmer: confirmer,
				Serial: active, Workspace: ws, Session: session, Lang: e.cfg.Lang(app.Getenv), ConfirmTimeout: e.cfg.ConfirmTimeout.Duration, Version: Version,
				UndoneLog: undoneLogPath(e.dirs.Data, workspace.JournalDir(ws)),
				OnEntry: func(j journal.Entry) {
					outMu.Lock()
					defer outMu.Unlock()
					if app.JSON {
						_ = writeJSON(app.Stdout, j)
					} else {
						fmt.Fprintln(app.Stdout, formatEntry(j))
					}
				},
				OnWarning: func(w string) { fmt.Fprintln(app.Stderr, "⚠ "+w) },
			})
			srv, err := ipc.Listen(e.dirs.Data, secret, func(ctx context.Context, req ipc.Request, c *ipc.Conn) ipc.Exit {
				return d.Handle(ctx, req, c)
			})
			if errors.Is(err, ipc.ErrAlreadyRunning) {
				return alreadyRunning
			}
			if err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			agentName := e.cfg.Agent
			if agentName == "" {
				agentName = agent.Known[0]
			}
			fmt.Fprintf(app.Stderr, "ADBuddy is serving %s (session %s).\nJournal: %s\nIn another terminal run: adbuddy run %s\nPress Ctrl+C to stop.\n\n",
				active, session, workspace.JournalDir(ws), agentName)
			if err := serveUntil(ctx, stop, srv); err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			fmt.Fprintln(app.Stderr, "ADBuddy stopped.")
			return nil
		},
	}
	cmd.Flags().StringVar(&adbPath, "adb", "", "path to the real adb (default: adb_path in config.toml, else the adb installed by adbuddy setup)")
	cmd.Flags().StringVar(&serial, "serial", "", "the phone to use when several are connected")
	cmd.Flags().StringVar(&wsDir, "workspace", "", "agent environment (default: workspace in config.toml, else ~/ADBuddy/default)")
	cmd.Flags().StringVar(&confirmMode, "confirm", "auto", "where to confirm 🔴 commands: auto (this terminal if interactive, else deny), terminal (read the answers from stdin even when it is not interactive, e.g. a pipe) or deny")
	return cmd
}

var alreadyRunning = NewError("ALREADY_RUNNING", "another `adbuddy serve` is already running", "Use that terminal, or stop it (Ctrl+C) first.")

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
	return false, NewError("DAEMON_UNREACHABLE", "the ADBuddy socket does not answer: "+err.Error(),
		"Stop the other `adbuddy serve` or delete "+ipc.SocketPath(dataDir)+" if none is running.")
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
		return nil, ConfigError("RULES_UNREADABLE", err.Error(), "Check "+path+".")
	}
	cls, err := classify.New(data)
	if err != nil {
		return nil, ConfigError("RULES_INVALID", path+": "+err.Error(), "Fix the rule or remove it.")
	}
	return cls, nil
}

func rulesPath(e env) string {
	if e.cfg.Rules != "" {
		return e.cfg.Rules
	}
	return e.dirs.RulesFile()
}
