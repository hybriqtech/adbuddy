package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/confirm"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// deviceWait is how long serve waits for the phone: ADBuddy's private adb
// server needs some seconds to reconnect a paired phone found by mDNS.
const deviceWait = 45 * time.Second

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
			if adbPath == "" {
				return ConfigError("ADB_NOT_CONFIGURED", "no adb binary configured", "Pass --adb <path to adb> or set adb_path in "+e.dirs.ConfigFile()+".")
			}
			if serial == "" {
				serial = e.cfg.Serial
			}
			ws := e.workspace(wsDir)
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
			case errors.Is(err, adb.ErrNoDevice):
				return TransientError("DEVICE_NOT_FOUND", "no phone in state `device` on adb server port "+strconv.Itoa(e.cfg.ADBServerPort),
					"Turn on wireless debugging on the phone (it must be paired with this PC), then run `adbuddy serve` again.")
			case errors.Is(err, adb.ErrManyDevices):
				return NewError("MULTIPLE_DEVICES", "more than one phone is connected", "Pass --serial <serial> (see `adb devices`) or set serial in "+e.dirs.ConfigFile()+".")
			case err != nil:
				return NewError("ADB_FAILED", err.Error(), "Check the adb path ("+adbPath+").")
			}
			runner.Serial = active

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
				Serial: active, Session: session, Lang: e.cfg.Lang(app.Getenv), ConfirmTimeout: e.cfg.ConfirmTimeout.Duration, Version: Version,
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
				return NewError("ALREADY_RUNNING", "another `adbuddy serve` is already running", "Use that terminal, or stop it (Ctrl+C) first.")
			}
			if err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			fmt.Fprintf(app.Stderr, "ADBuddy is serving %s (session %s).\nJournal: %s\nIn another terminal run: adbuddy run claude\nPress Ctrl+C to stop.\n\n",
				active, session, workspace.JournalDir(ws))
			go func() {
				<-ctx.Done()
				srv.Close()
			}()
			if err := srv.Serve(); err != nil {
				return NewError("SOCKET_FAILED", err.Error(), "")
			}
			fmt.Fprintln(app.Stderr, "ADBuddy stopped.")
			return nil
		},
	}
	cmd.Flags().StringVar(&adbPath, "adb", "", "path to the real adb (default: adb_path in config.toml)")
	cmd.Flags().StringVar(&serial, "serial", "", "the phone to use when several are connected")
	cmd.Flags().StringVar(&wsDir, "workspace", "", "agent environment (default: ~/ADBuddy/default)")
	cmd.Flags().StringVar(&confirmMode, "confirm", "auto", "where to confirm 🔴 commands: auto (this terminal if interactive, else deny), terminal or deny")
	return cmd
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
