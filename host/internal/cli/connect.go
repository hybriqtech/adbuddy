package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	knownAddrWait   = 5 * time.Second  // the try of the saved address before the wait for mDNS
	identityWait    = 8 * time.Second  // bounds the check of the phone's identity (getprop ro.serialno)
)

type connectResult struct {
	Serial   string `json:"serial"` // the serial adb lists the phone with now
	Address  string `json:"address"`
	Verified bool   `json:"verified"` // the phone is the one in config.toml
}

func newConnectCmd(app *App) *cobra.Command {
	var adbFlag string
	cmd := &cobra.Command{
		Use:     "connect <ip:port>",
		Short:   app.T("help.connect.short"),
		Long:    app.T("help.connect.long"),
		Example: "  adbuddy connect 100.64.0.5:41234\n  adbuddy connect 100.64.0.5:41234 --json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() { <-ctx.Done(); stop() }()
			err := func() error {
				if app.Getenv("ADBUDDY_SOURCE") == "agent" {
					return NewErrorT("AGENT_SESSION", "err.connect.agent_session", "err.connect.agent_session.fix")
				}
				addr := args[0]
				if !validPairAddr(addr) {
					return UsageErrorT("err.connect.usage_is_not_an_ip_address", classify.Printable(addr))
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
					return NewErrorT("ALREADY_RUNNING", "err.connect.already_running", "err.connect.already_running.fix")
				}
				adbPath := configuredADB(e, adbFlag)
				if adbPath == "" {
					return ConfigErrorT("ADB_NOT_CONFIGURED", "err.connect.adb_not_configured", "err.connect.adb_not_configured.fix")
				}
				runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
				if err := startServer(ctx, runner); err != nil {
					return err
				}
				fmt.Fprintf(app.Stderr, "Connecting to %s…\n", addr)
				guid, _ := pairing.GuidOf(e.cfg.Serial)
				active, verified, err := connectAddr(ctx, runner, addr, guid, connectAddrWait)
				if err != nil {
					return err
				}
				if !verified {
					fmt.Fprintf(app.Stderr, "Note: could not check that this is the phone you set up (%s).\n", unverifiedWhy(e.cfg.Serial))
				}
				cfg := e.cfg
				cfg.Address = addr
				if err := config.Save(e.dirs.ConfigFile(), cfg); err != nil {
					return ConfigErrorT("CONFIG_UNWRITABLE", "error.cause", "err.connect.config_unwritable.fix", err.Error(), e.dirs.Config)
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
	cmd.Flags().StringVar(&adbFlag, "adb", "", app.T("help.connect.flag.adb"))
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
	return "its saved name does not carry a serial number that tells phones apart"
}

// connectAddr connects runner's adb server to the phone at addr (waiting at
// most timeout) and, when guid names the paired phone, checks that it is
// that phone (see verifyAt). When ctx ends, the error is ctx's: callers turn
// it into Interrupted.
func connectAddr(ctx context.Context, runner adb.Runner, addr, guid string, timeout time.Duration) (active string, verified bool, err error) {
	host := runner
	host.Serial = "" // connect and devices are host commands
	active, err = pairing.ConnectAddr(ctx, host, addr, timeout)
	switch {
	case ctx.Err() != nil:
		return "", false, ctx.Err()
	case errors.Is(err, pairing.ErrTimeout):
		return "", false, TransientErrorT("DEVICE_NOT_FOUND", "error.cause", "err.connect.device_not_found.fix", err.Error())
	case err != nil:
		return "", false, TransientErrorT("CONNECT_FAILED", "error.cause", "err.connect.connect_failed.fix", err.Error())
	}
	verified, err = verifyAt(ctx, runner, active, guid)
	if err != nil {
		return "", false, err
	}
	return active, verified, nil
}

// verifyAt checks that the phone online as active (an address, or the name
// mDNS gave it) is the one with guid, waiting at most identityWait for it
// to answer. verified is false when guid carries no usable serial number
// (see pairing.GuidSerialNo). Another phone is disconnected from ADBuddy's
// adb server and is WRONG_PHONE; a phone that does not answer in time is a
// transient DEVICE_UNREADABLE.
func verifyAt(ctx context.Context, runner adb.Runner, active, guid string) (verified bool, err error) {
	if _, ok := pairing.GuidSerialNo(guid); !ok {
		return false, nil
	}
	phone := runner
	phone.Serial = active
	vctx, cancel := context.WithTimeout(ctx, identityWait)
	err = pairing.VerifyIdentity(vctx, phone, guid)
	timedOut := vctx.Err() != nil
	cancel()
	switch {
	case ctx.Err() != nil:
		return false, ctx.Err()
	case err != nil && timedOut:
		return false, TransientErrorT("DEVICE_UNREADABLE", "err.connect.device_unreadable", "err.connect.device_unreadable.fix", classify.Printable(active), identityWait)
	case errors.Is(err, pairing.ErrWrongPhone):
		host := runner
		host.Serial = ""
		dctx, cancel := context.WithTimeout(ctx, disconnectWait)
		_, _, _, _ = host.Output(dctx, "disconnect", active)
		cancel()
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return false, NewErrorT("WRONG_PHONE", "err.connect.wrong_phone", "err.connect.wrong_phone.fix", classify.Printable(active))
	case err != nil:
		return false, NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.connect.device_unreadable.fix_2", err.Error())
	}
	return true, nil
}

// knownPhone is where findKnownPhone found the phone.
type knownPhone struct {
	Active   string // the serial adb lists it with
	Used     string // the saved address it was reached at; "" when not by address
	Verified bool   // its identity was checked (ro.serialno against the guid)
	AddrErr  error  // why the saved address was not used; nil if it was not tried
}

// findKnownPhone finds the phone saved as serial, trying the quickest ways
// first when there are addresses to try (without one, only step 3):
//  1. adb already lists it by its guid (renamed by mDNS or not);
//  2. each of addrs, in order (the address that worked last time, --connect,
//     the phone's Tailscale address, see candidateAddrs): already online on
//     ADBuddy's adb server, or `adb connect` works within knownAddrWait, and
//     it is that phone (see tryAddr);
//  3. the usual wait for it by mDNS (findPhone), at most wait.
//
// Whatever the step, a phone saved by its guid is checked to be that phone
// (anything on the network can announce the guid): another one is
// WRONG_PHONE. Another phone at an address is disconnected, said on log and
// skipped instead. A phone that does not show up is adb.ErrNoDevice;
// AddrErr then tells why the last address tried did not help. When ctx ends,
// the error is ctx's.
func findKnownPhone(ctx context.Context, runner adb.Runner, serial string, addrs []string, wait time.Duration, log io.Writer) (knownPhone, error) {
	var k knownPhone
	guid, byGuid := pairing.GuidOf(serial)
	if len(addrs) > 0 {
		online, err := runner.Online(ctx) // host command: Online drops the serial
		if ctx.Err() != nil {
			return knownPhone{}, ctx.Err()
		}
		if err == nil {
			for _, d := range online {
				if byGuid && pairing.IsGuidSerial(guid, d.Serial) || !byGuid && serial != "" && d.Serial == serial {
					return checkFound(ctx, runner, d.Serial, guid, byGuid)
				}
			}
		}
		for _, addr := range addrs {
			kp, ok, addrErr := tryAddr(ctx, runner, online, addr, guid, log)
			if ctx.Err() != nil {
				return knownPhone{}, ctx.Err()
			}
			if ok {
				return kp, nil
			}
			if addrErr != nil {
				k.AddrErr = addrErr
			}
		}
	}
	active, err := findPhone(ctx, runner, serial, wait)
	if err != nil {
		return k, err
	}
	found, err := checkFound(ctx, runner, active, guid, byGuid)
	found.AddrErr = k.AddrErr
	return found, err
}

// tryAddr tries one saved address: the phone is already online at it (online
// is what `adb devices` listed, nil if that failed) or `adb connect` reaches
// it within knownAddrWait, and it is the phone with guid. ok is false when
// the address does not serve; addrErr then says why (said on log too).
func tryAddr(ctx context.Context, runner adb.Runner, online []adb.Device, addr, guid string, log io.Writer) (kp knownPhone, ok bool, addrErr error) {
	if hasSerial(online, addr) {
		fmt.Fprintf(log, "Checking the phone at %s (a saved address)…\n", addr)
		verified, err := verifyAt(ctx, runner, addr, guid)
		if ctx.Err() != nil {
			return knownPhone{}, false, ctx.Err()
		}
		if err == nil {
			return knownPhone{Active: addr, Used: addr, Verified: verified}, true, nil
		}
		skipAddr(log, addr, err)
		return knownPhone{}, false, err
	}
	fmt.Fprintf(log, "Connecting to %s (a saved address)…\n", addr)
	active, verified, err := connectAddr(ctx, runner, addr, guid, knownAddrWait)
	if ctx.Err() != nil {
		return knownPhone{}, false, ctx.Err()
	}
	if err != nil {
		skipAddr(log, addr, err)
		return knownPhone{}, false, err
	}
	return knownPhone{Active: active, Used: addr, Verified: verified}, true, nil
}

// checkFound checks the phone found under active by its guid (or by mDNS).
func checkFound(ctx context.Context, runner adb.Runner, active, guid string, byGuid bool) (knownPhone, error) {
	if !byGuid {
		return knownPhone{Active: active}, nil
	}
	verified, err := verifyAt(ctx, runner, active, guid)
	if hasCode(err, "WRONG_PHONE") {
		err = NewErrorT("WRONG_PHONE", "err.connect.wrong_phone_mdns", "err.connect.wrong_phone_mdns.fix", classify.Printable(active))
	}
	if err != nil {
		return knownPhone{}, err
	}
	return knownPhone{Active: active, Verified: verified}, nil
}

// skipAddr says why the saved address addr is not used.
func skipAddr(log io.Writer, addr string, err error) {
	if hasCode(err, "WRONG_PHONE") {
		fmt.Fprintf(log, "Warning: the phone at %s %s.\n", addr, wrongPhoneWarningText)
		return
	}
	fmt.Fprintf(log, "The address %s did not work (%s); waiting for the phone on this network…\n", addr, AsError(err).Message)
}

// wrongPhoneWarningText follows "the phone at <addr>" when it is another phone.
const wrongPhoneWarningText = "is not the one you set up; not using that address"

// hasCode reports whether err is an *Error with that code.
func hasCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}
