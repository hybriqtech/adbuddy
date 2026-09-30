package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/platformtools"
	"github.com/spf13/cobra"
)

// Waiting times of a connection by address (variables so tests can shorten them).
var (
	connectAddrWait = 20 * time.Second // `adb connect` plus the wait for the phone to be online
	disconnectWait  = 5 * time.Second  // bounds `adb disconnect` of another phone
)

// wrongPhoneFix is the fix of WRONG_PHONE.
const wrongPhoneFix = "that address is another phone; check the IP and port on the phone you set up"

type connectResult struct {
	Serial   string `json:"serial"` // the serial adb lists the phone with now
	Address  string `json:"address"`
	Verified bool   `json:"verified"` // the phone is the one in config.toml
}

func newConnectCmd(app *App) *cobra.Command {
	var adbFlag string
	cmd := &cobra.Command{
		Use:   "connect <ip:port>",
		Short: "Connect to the phone by IP address and port (Tailscale or another network without mDNS)",
		Long: `Connect to the phone by the IP address and port shown on its Wireless debugging
screen, when ADBuddy cannot find it by itself: over Tailscale, or on a network
that blocks mDNS. The phone must be paired with this PC first (` + "`adbuddy setup --code`" + `).
The address is checked against the phone you set up and saved in config.toml, so
` + "`adbuddy serve`" + ` uses it again. The port changes each time wireless debugging is
turned on: run this command again with the new one.`,
		Example: "  adbuddy connect 100.64.0.5:41234\n  adbuddy connect 100.64.0.5:41234 --json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() { <-ctx.Done(); stop() }()
			err := func() error {
				if app.Getenv("ADBUDDY_SOURCE") == "agent" {
					return NewError("AGENT_SESSION", "`adbuddy connect` cannot be run from the agent's session", "Run it yourself in a terminal.")
				}
				addr := args[0]
				if !validPairAddr(addr) {
					return UsageError(fmt.Sprintf("%q is not an IP address and port: type them as the phone shows them on its Wireless debugging screen (e.g. 100.64.0.5:41234)", classify.Printable(addr)))
				}
				e, err := loadEnv(app)
				if err != nil {
					return err
				}
				// It changes config.toml and the phones on ADBuddy's adb
				// server: never under a running serve and its agent.
				if running, err := serveRunning(ctx, e.dirs.Data); err != nil {
					return err
				} else if running {
					return NewError("ALREADY_RUNNING", "`adbuddy serve` is running", "Stop `adbuddy serve` first (Ctrl+C in its terminal), then run `adbuddy connect` again.")
				}
				adbPath := configuredADB(e, adbFlag)
				if adbPath == "" {
					return ConfigError("ADB_NOT_CONFIGURED", "ADBuddy's adb is not installed", "Run `adbuddy setup` (or pass --adb <path to adb 37 or newer>).")
				}
				runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
				if err := startServer(ctx, runner); err != nil {
					return err
				}
				fmt.Fprintf(app.Stderr, "Connecting to %s…\n", addr)
				guid, _ := pairing.GuidOf(e.cfg.Serial)
				active, verified, err := connectAddr(ctx, runner, addr, guid)
				if err != nil {
					return err
				}
				if !verified {
					fmt.Fprintf(app.Stderr, "Note: could not check that this is the phone you set up (%s).\n", unverifiedWhy(e.cfg.Serial))
				}
				cfg := e.cfg
				cfg.Address = addr
				if err := config.Save(e.dirs.ConfigFile(), cfg); err != nil {
					return ConfigError("CONFIG_UNWRITABLE", err.Error(), "Check that "+e.dirs.Config+" is writable.")
				}
				res := connectResult{Serial: active, Address: addr, Verified: verified}
				if app.JSON {
					return WriteData(app.Stdout, res)
				}
				fmt.Fprintf(app.Stdout, "Connected to %s.\n", addr)
				fmt.Fprintf(app.Stderr, "Saved the address in %s. Next: run `adbuddy serve`.\n", e.dirs.ConfigFile())
				return nil
			}()
			if err != nil && ctx.Err() != nil {
				return Interrupted()
			}
			return err
		},
	}
	cmd.Flags().StringVar(&adbFlag, "adb", "", "path to the real adb (default: adb_path in config.toml, else the adb installed by adbuddy setup)")
	return cmd
}

// configuredADB is the adb to use: the flag, else adb_path in config.toml,
// else the one `adbuddy setup` installed ("" if none).
func configuredADB(e env, flag string) string {
	if flag != "" {
		return flag
	}
	if e.cfg.ADBPath != "" {
		return e.cfg.ADBPath
	}
	if p := platformtools.ADBPath(e.dirs.Data, runtime.GOOS); fileExists(p) {
		return p
	}
	return ""
}

// unverifiedWhy says why the phone at an address could not be checked.
func unverifiedWhy(saved string) string {
	if saved == "" {
		return "no phone is set up in config.toml yet"
	}
	return "its saved name does not carry its serial number"
}

// connectAddr connects runner's adb server to the phone at addr and, when
// guid names the paired phone, checks that it is that phone (verified).
// Another phone there is disconnected again and is WRONG_PHONE. When ctx
// ends, the error is ctx's: callers turn it into Interrupted.
func connectAddr(ctx context.Context, runner adb.Runner, addr, guid string) (active string, verified bool, err error) {
	host := runner
	host.Serial = "" // connect and devices are host commands
	active, err = pairing.ConnectAddr(ctx, host, addr, connectAddrWait)
	switch {
	case ctx.Err() != nil:
		return "", false, ctx.Err()
	case errors.Is(err, pairing.ErrTimeout):
		return "", false, TransientError("DEVICE_NOT_FOUND", err.Error(), "Unlock the phone and check that wireless debugging is on and that this PC is paired with it (`adbuddy setup --code`), then try again.")
	case err != nil:
		return "", false, TransientError("CONNECT_FAILED", err.Error(), "Check the IP address and port on the phone's Wireless debugging screen (the port changes each time wireless debugging is turned on) and that the phone and this PC can reach each other.")
	}
	if _, ok := pairing.GuidSerialNo(guid); !ok {
		return active, false, nil
	}
	phone := runner
	phone.Serial = active
	err = pairing.VerifyIdentity(ctx, phone, guid)
	switch {
	case ctx.Err() != nil:
		return "", false, ctx.Err()
	case errors.Is(err, pairing.ErrWrongPhone):
		dctx, cancel := context.WithTimeout(ctx, disconnectWait)
		_, _, _, _ = host.Output(dctx, "disconnect", addr)
		cancel()
		return "", false, NewError("WRONG_PHONE", "the phone at "+addr+" is not the phone you set up", wrongPhoneFix)
	case err != nil:
		return "", false, NewError("DEVICE_UNREADABLE", err.Error(), "Check that the phone is unlocked and still connected.")
	}
	return active, true, nil
}
