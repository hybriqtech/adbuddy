package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/adb"
	"github.com/hybriqtech/adbuddy/host/internal/agent"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/config"
	"github.com/hybriqtech/adbuddy/host/internal/device"
	"github.com/hybriqtech/adbuddy/host/internal/pairing"
	"github.com/hybriqtech/adbuddy/host/internal/platformtools"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// HelpURL is the step-by-step guide with pictures (GitHub Pages, docs/setup).
const HelpURL = "https://hybriqtech.github.io/adbuddy/setup/"

// Waiting times of setup (variables so tests can shorten them).
var (
	knownPhoneWait  = 25 * time.Second // a paired phone reconnects to a fresh adb server in 15-20 s
	pairingWait     = 3 * time.Minute
	connectWait     = time.Minute
	autoConnectWait = 20 * time.Second
	startServerWait = 30 * time.Second // bounds `adb start-server`
	// With --code, mDNS is given less time before the address is asked for or
	// --connect is used: over Tailscale it never finds the phone.
	codeConnectWait     = 10 * time.Second
	codeAutoConnectWait = 5 * time.Second
	qrHintDelay         = 10 * time.Second // when to say that no phone is visible while a QR code waits
)

// phoneGuide is the phone preparation text; step 3 depends on how it is paired.
func phoneGuide(useCode bool) string {
	network := "Keep the phone on the same Wi-Fi network as this PC."
	if useCode {
		network = "Keep the phone on the same Wi-Fi network as this PC, or reachable over Tailscale: then use the IP address and port the pairing popup shows."
	}
	return `Prepare your phone (only once):
  1. Settings → About phone → tap "Build number" 7 times to turn on Developer options.
  2. Developer options (Settings → System on a Pixel) → turn on "Wireless debugging".
     When the phone asks, tick "Always allow on this network".
  3. ` + network + `
Step-by-step guide with pictures (scan it with the phone's camera):
`
}

type setupResult struct {
	Device    device.Profile `json:"device"`
	Serial    string         `json:"serial"` // saved in config.toml; "" when not saved (an address)
	Active    string         `json:"active"` // the serial adb lists the phone with this session
	Workspace string         `json:"workspace"`
	Agent     string         `json:"agent"`
	Address   string         `json:"address"` // saved in config.toml: the last IP:port that worked, "" if none
	ADB       string         `json:"adb"`
	App       appResult      `json:"app"`
}

func newSetupCmd(app *App) *cobra.Command {
	var adbFlag, agentFlag, wsFlag, connectFlag, apkFlag string
	var useCode, yes, noApp bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up ADBuddy: get adb, pair the phone over Wi-Fi and prepare the agent environment",
		Long: `Set up ADBuddy step by step: download Android platform-tools (checked against
Google's checksum), pair the phone over wireless debugging with a QR code (or
with --code), connect, read the phone's profile, pick the agent and create the
environment. Run it again at any time; it skips what is already done.

Over Tailscale or another network without mDNS, pair with --code and give the
connection address with --connect (or type it when asked).`,
		Example: "  adbuddy setup\n  adbuddy setup --code\n  adbuddy setup --code --connect 100.64.0.5:41234\n  adbuddy setup --agent codex --yes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Ctrl+C at any step (a download, a wait for the phone, a
			// question) exits with 130. After the first one, the signals go
			// back to their default, so a second Ctrl+C stops setup at once.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			go func() { <-ctx.Done(); stop() }()
			err := func() error {
				if app.Getenv("ADBUDDY_SOURCE") == "agent" {
					return NewError("AGENT_SESSION", "`adbuddy setup` cannot be run from the agent's session", "Run it yourself in a terminal.")
				}
				if apkFlag != "" && noApp {
					return UsageError("--apk and --no-app contradict each other: use only one")
				}
				if connectFlag != "" && !validPairAddr(connectFlag) {
					return UsageError(fmt.Sprintf("--connect: %q is not an IP address and port: type them as the phone shows them on its Wireless debugging screen (e.g. 100.64.0.5:41234)", classify.Printable(connectFlag)))
				}
				e, err := loadEnv(app)
				if err != nil {
					return err
				}
				say := func(format string, a ...any) { fmt.Fprintf(app.Stderr, format, a...) }
				in := bufio.NewReader(app.Stdin)
				type answer struct {
					line string
					err  error
				}
				ask := func(prompt string) (string, error) {
					say("%s", prompt)
					ch := make(chan answer, 1)
					go func() {
						line, err := in.ReadString('\n')
						ch <- answer{line, err}
					}()
					select {
					case <-ctx.Done():
						return "", ctx.Err()
					case a := <-ch:
						if a.err != nil && a.line == "" {
							return "", UsageError("no answer on stdin")
						}
						return strings.TrimSpace(a.line), nil
					}
				}

				// setup rewrites the environment and config.toml, and may run
				// `adb usb`: never under a running serve and its agent.
				if running, err := serveRunning(ctx, e.dirs.Data); err != nil {
					return err
				} else if running {
					return NewError("ALREADY_RUNNING", "`adbuddy serve` is running", "Stop `adbuddy serve` first (Ctrl+C in its terminal), then run `adbuddy setup` again.")
				}

				// serve, run and journal read config.toml from any folder:
				// relative paths are saved as absolute ones.
				adbArg, wsDir := adbFlag, wsFlag
				if adbArg != "" {
					if adbArg, err = absExecutable(adbArg); err != nil {
						return UsageError("--adb: " + err.Error())
					}
				}
				if wsDir != "" {
					if wsDir, err = filepath.Abs(wsDir); err != nil {
						return UsageError("--workspace: " + err.Error())
					}
				}

				// 1. Tools.
				adbPath := adbArg
				if adbPath == "" {
					adbPath = e.cfg.ADBPath
				}
				if adbPath == "" {
					say("Getting Android platform-tools…\n")
					res, err := platformtools.Installer{DataDir: e.dirs.Data, BaseURL: repositoryURL(app), GOOS: runtime.GOOS, Log: app.Stderr}.Ensure(ctx)
					switch {
					case errors.Is(err, platformtools.ErrChecksum):
						// Not a network hiccup: the same file would come again.
						return NewError("TOOLS_CHECKSUM_MISMATCH", err.Error(), "Run `adbuddy setup` again later; if it keeps failing, pass --adb <path to adb 37 or newer> and report it at https://github.com/hybriqtech/adbuddy/issues.")
					case err != nil:
						return TransientError("TOOLS_DOWNLOAD_FAILED", err.Error(), "Check the internet connection and run `adbuddy setup` again, or pass --adb <path to adb 37 or newer>.")
					}
					adbPath = res.ADB
				}
				runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
				if err := startServer(ctx, runner); err != nil {
					return err
				}

				// 2. A phone paired before comes back on its own, maybe under
				// another serial than the saved one (see findPhone).
				// When mDNS does not find it, it is tried at --connect or at
				// the address saved last time.
				serial, guid, usedAddr := "", "", ""
				savedFailed := false // the saved address answered with an error: not kept
				if e.cfg.Serial != "" {
					known := connectFlag
					if known == "" {
						known = e.cfg.Address
					}
					say("Looking for your phone…\n")
					k, err := findKnownPhone(ctx, runner, e.cfg.Serial, known, knownPhoneWait, app.Stderr)
					if ctx.Err() != nil {
						return ctx.Err() // Ctrl+C: no guide nor QR code
					}
					savedFailed = k.AddrErr != nil
					// Something else answering as the phone: never set it up as it.
					if hasCode(err, "WRONG_PHONE") || hasCode(err, "DEVICE_UNREADABLE") {
						return err
					}
					if err == nil {
						serial, usedAddr = k.Active, k.Used
						guid, _ = pairing.GuidOf(e.cfg.Serial) // keeps the saved name
						if !k.Verified && (guid != "" || k.Used != "") {
							say("Note: could not check that the phone at %s is the one you set up (%s).\n", serial, unverifiedWhy(e.cfg.Serial))
						}
						say("Found it: already paired.\n")
					}
				}

				// 3-4. Guide and pairing.
				if serial == "" {
					guid, err = pair(ctx, app, runner, useCode, ask, say)
					if err != nil {
						return err
					}
					say("Paired. Connecting…\n")
					// With --code the phone may be out of mDNS's reach
					// (Tailscale): if an address can be had, it is asked for sooner.
					wait, auto := connectWait, autoConnectWait
					canAsk := isTerminal(app.Stdin)
					if useCode && (connectFlag != "" || canAsk) {
						wait, auto = codeConnectWait, codeAutoConnectWait
					}
					serial, err = pairing.Connect(ctx, runner, guid, wait, auto, time.Second)
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if err == nil {
						// Found by mDNS: anything on the network can announce the guid.
						k, err := checkFound(ctx, runner, serial, guid, true)
						if err != nil {
							return err
						}
						if !k.Verified {
							say("Note: could not check that the phone at %s is the one just paired.\n", serial)
						}
					}
					if err != nil {
						addr := connectFlag
						if addr == "" && canAsk {
							say("The phone did not show up on this network by itself (mDNS does not reach it, e.g. over Tailscale).\n")
							if addr, err = ask("Connection address (IP:port on the Wireless debugging screen): "); err != nil {
								return err
							}
							if !validPairAddr(addr) {
								return UsageError(fmt.Sprintf("%q is not an IP address and port: type them as the phone shows them on its Wireless debugging screen", classify.Printable(addr)))
							}
						}
						if addr == "" {
							return TransientError("DEVICE_NOT_FOUND", "the paired phone did not connect: "+err.Error(),
								"Check that wireless debugging is still on, then run `adbuddy setup` again; if the phone is on another network (e.g. Tailscale), add --connect <ip:port> with the IP address and port on its Wireless debugging screen.")
						}
						say("Connecting to %s…\n", addr)
						s, verified, err := connectAddr(ctx, runner, addr, guid, connectAddrWait)
						if err != nil {
							return err
						}
						if !verified {
							say("Note: could not check that the phone at %s is the one just paired.\n", addr)
						}
						serial, usedAddr = s, addr
					}
				}
				runner.Serial = serial

				// Host invariant: no open adb network port unless remote mode is on.
				if err := applyInvariant(ctx, runner, deviceWait, app.Stderr, "adbuddy setup", ""); err != nil {
					return err // Ctrl+C becomes INTERRUPTED below
				}

				// Profile of the phone.
				prof, err := device.Collect(ctx, runner)
				if err != nil {
					return NewError("DEVICE_UNREADABLE", err.Error(), "Check that the phone is unlocked and still connected.")
				}
				say("Phone: %s, Android %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android))

				// 5-6. The phone app.
				appRes := appResult{Consent: "skipped", Note: noAppNote}
				if !noApp {
					appRes = setupApp(ctx, app, &e, runner, apkFlag, say)
					if err := ctx.Err(); err != nil {
						return err // Ctrl+C while waiting for the phone: INTERRUPTED below
					}
				}

				// Agent.
				chosen, err := chooseAgent(app, agentFlag, e.cfg.Agent, yes, ask, say)
				if err != nil {
					return err
				}

				// Environment and configuration. The saved serial is the
				// phone's stable name, not the address it used this time.
				saved := stableSerial(guid, serial)
				if saved == "" {
					say("Note: not saving the serial %s in the configuration: it is an address that changes when wireless debugging restarts. Pair again with `adbuddy setup` to save the phone's name.\n", classify.Printable(serial))
				}
				// The environment chosen before is kept for the same phone, and
				// when config.toml does not say which phone it was for.
				samePhone := e.cfg.Serial == "" || e.cfg.Serial == saved || e.cfg.Serial == serial
				ws := wsDir
				if ws == "" && samePhone {
					ws = e.cfg.Workspace
				}
				if ws == "" {
					ws = filepath.Join(e.home, "ADBuddy", prof.Name)
				}
				exe, err := os.Executable()
				if err != nil {
					return NewError("EXECUTABLE_UNKNOWN", err.Error(), "")
				}
				if _, err := workspace.Init(workspace.Options{Dir: ws, Exe: exe, Profile: prof.Markdown(), Force: true}); err != nil {
					return NewError("WORKSPACE_FAILED", err.Error(), "Check that "+ws+" is writable.")
				}
				if err := device.WriteJSON(ws, prof); err != nil {
					return NewError("WORKSPACE_FAILED", err.Error(), "")
				}
				cfg := e.cfg
				cfg.Serial, cfg.Workspace, cfg.Agent = saved, ws, chosen
				// The address that worked this time; else the one saved for this
				// phone, which may still work (a Tailscale IP while on Wi-Fi).
				cfg.Address = usedAddr
				if cfg.Address == "" && isAddress(serial) {
					cfg.Address = serial
				}
				if cfg.Address == "" && samePhone && !savedFailed {
					cfg.Address = e.cfg.Address
				}
				if adbArg != "" {
					cfg.ADBPath = adbArg
				}
				if cfg.RemoteUntil != "" {
					// applyInvariant closed port 5555 above: setup always ends remote mode.
					if samePhone {
						fmt.Fprintln(app.Stderr, "Remote mode was on: closing it.")
					} else {
						// The `adb usb` went to another phone.
						fmt.Fprintln(app.Stderr, "Remote mode was on for the phone you had set up before: its adb port 5555 may stay open until that phone restarts. Turn remote mode off from its app or restart it.")
					}
					cfg.RemoteUntil = ""
				}
				if err := config.Save(e.dirs.ConfigFile(), cfg); err != nil {
					return ConfigError("CONFIG_UNWRITABLE", err.Error(), "Check that "+e.dirs.Config+" is writable.")
				}

				res := setupResult{Device: prof, Serial: saved, Active: serial, Address: cfg.Address, Workspace: ws, Agent: chosen, ADB: adbPath, App: appRes}
				if app.JSON {
					return WriteData(app.Stdout, res)
				}
				fmt.Fprintf(app.Stdout, "Ready: %s (Android %s), environment %s, agent %s, %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android), ws, chosen, fmtApp(appRes))
				say("Next: run `adbuddy serve` and keep it open, then `adbuddy run` in another terminal.\n")
				return nil
			}()
			if err != nil && ctx.Err() != nil {
				return Interrupted()
			}
			return err
		},
	}
	cmd.Flags().StringVar(&adbFlag, "adb", "", "use this adb (37 or newer) instead of downloading platform-tools")
	cmd.Flags().StringVar(&agentFlag, "agent", "", "agent to use: claude, codex, gemini, opencode…")
	cmd.Flags().StringVar(&wsFlag, "workspace", "", "agent environment (default: ~/ADBuddy/<phone model>)")
	cmd.Flags().BoolVar(&useCode, "code", false, "pair with IP:port and the 6-digit code instead of a QR code")
	cmd.Flags().StringVar(&connectFlag, "connect", "", "connect by this IP:port when mDNS does not find the phone (Tailscale or another network), as its Wireless debugging screen shows it")
	cmd.Flags().BoolVar(&yes, "yes", false, "accept the recommended choices without asking")
	cmd.Flags().StringVar(&apkFlag, "apk", "", "install this ADBuddy app APK instead of the latest release (a file you choose: if hybriqtech's release certificate did not sign it, exactly this APK becomes the trusted development build)")
	cmd.Flags().BoolVar(&noApp, "no-app", false, "do not install or set up the phone app")
	return cmd
}

func repositoryURL(app *App) string {
	if u := app.Getenv("ADBUDDY_REPOSITORY_URL"); u != "" {
		return u
	}
	return platformtools.RepositoryURL
}

func colorOK(app *App) bool {
	return app.Getenv("NO_COLOR") == "" && app.Getenv("TERM") != "dumb"
}

// pairingCodeRe is the code the phone shows in "Pair device with pairing code".
var pairingCodeRe = regexp.MustCompile(`^[0-9]{6}$`)

// pair shows the guide and pairs by QR code or by code.
func pair(ctx context.Context, app *App, runner adb.Runner, useCode bool, ask func(string) (string, error), say func(string, ...any)) (string, error) {
	say("%s", phoneGuide(useCode))
	if art, err := pairing.RenderQR(HelpURL, colorOK(app)); err == nil {
		say("%s", art)
	}
	say("%s\n\n", HelpURL)
	if useCode {
		addr, err := ask("In Wireless debugging → Pair device with pairing code, type the IP address and port: ")
		if err != nil {
			return "", err
		}
		if !validPairAddr(addr) {
			return "", UsageError(fmt.Sprintf("%q is not an IP address and port: type them as the phone shows them under Pair device with pairing code", classify.Printable(addr)))
		}
		code, err := ask("Wi-Fi pairing code: ")
		if err != nil {
			return "", err
		}
		if !pairingCodeRe.MatchString(code) {
			return "", UsageError("the Wi-Fi pairing code is 6 digits")
		}
		guid, err := pairing.Pair(ctx, runner, addr, code)
		if err != nil {
			return "", NewError("PAIRING_FAILED", err.Error(), "Keep the pairing popup open until setup says Paired: the code stops working as soon as it closes, also when you switch to another app to copy it. Open it again and use the new address and code.")
		}
		return guid, nil
	}
	q, err := pairing.NewQR()
	if err != nil {
		return "", NewError("PAIRING_FAILED", err.Error(), "")
	}
	art, err := pairing.RenderQR(q.Payload(), colorOK(app))
	if err != nil {
		return "", NewError("PAIRING_FAILED", err.Error(), "")
	}
	say("On the phone: Wireless debugging → Pair device with QR code, and scan:\n%s", art)
	say("Waiting for the phone (up to %s)…\n", pairingWait)
	stopHint := qrHint(ctx, runner, say)
	guid, err := pairing.PairQR(ctx, runner, q, pairingWait)
	stopHint()
	if errors.Is(err, pairing.ErrTimeout) {
		return "", TransientError("PAIRING_TIMEOUT", "the phone did not answer the QR code in time", "Check that phone and PC are on the same Wi-Fi and run `adbuddy setup` again, or use `adbuddy setup --code`.")
	}
	if err != nil {
		return "", NewError("PAIRING_FAILED", err.Error(), "Try again, or use `adbuddy setup --code`.")
	}
	return guid, nil
}

// chooseAgent picks the agent: the flag, else the installed ones (Claude
// Code recommended), asking only on an interactive terminal.
func chooseAgent(app *App, flag, current string, yes bool, ask func(string) (string, error), say func(string, ...any)) (string, error) {
	found := agent.Detect(exec.LookPath)
	if flag != "" {
		if len(found) == 0 || !containsAgent(found, flag) {
			say("Note: %s is not in PATH yet. %s\n", flag, agent.InstallHint(flag))
		}
		return flag, nil
	}
	if len(found) == 0 {
		say("No coding agent found in PATH. %s\n", agent.InstallHint(agent.Known[0]))
		if current != "" {
			return current, nil
		}
		return agent.Known[0], nil
	}
	if current != "" && !containsAgent(found, current) {
		// The user chose it before: never swap it silently.
		say("Note: %s is not in PATH; keeping it as the agent (%s). Pass --agent to use another one.\n", current, agent.InstallHint(current))
		return current, nil
	}
	def := found[0].Name
	if current != "" && containsAgent(found, current) {
		def = current
	}
	if yes || len(found) == 1 || !isTerminal(app.Stdin) {
		say("Agent: %s\n", def)
		return def, nil
	}
	names := make([]string, len(found))
	for i, f := range found {
		names[i] = f.Name
	}
	for {
		answer, err := ask(fmt.Sprintf("Agent to use (%s) [%s]: ", strings.Join(names, ", "), def))
		if err != nil {
			return "", err
		}
		if answer == "" {
			return def, nil
		}
		if containsAgent(found, answer) {
			return answer, nil
		}
		say("Please choose one of: %s\n", strings.Join(names, ", "))
	}
}

func containsAgent(found []agent.Found, name string) bool {
	for _, f := range found {
		if f.Name == name {
			return true
		}
	}
	return false
}

// stableSerial is the serial to save for the phone: its mDNS name
// (<guid>._adb-tls-connect._tcp), which survives a new wireless debugging
// port, even when this time it connected with `adb connect ip:port`. An
// ip:port without a known guid yields "": that serial would stop working.
func stableSerial(guid, active string) string {
	if guid != "" {
		return guid + "." + pairing.ConnectType
	}
	if isAddress(active) {
		return ""
	}
	return active
}

// phoneIdentity is the name the journal and undo know the phone by (see
// daemon.Options.Identity): its mDNS name when adb lists it by its guid, or
// when findPhone found it by the guid of the saved serial (then active is
// that phone, even as ip:port); else the active serial itself.
func phoneIdentity(saved, active string) string {
	guid, ok := pairing.GuidOf(active)
	if !ok {
		guid, ok = pairing.GuidOf(saved)
	}
	if ok {
		return guid + "." + pairing.ConnectType
	}
	return active
}

// isAddress reports whether serial is IP:port (a TCP connection).
func isAddress(serial string) bool {
	host, _, err := net.SplitHostPort(serial)
	if err != nil {
		return false
	}
	_, err = netip.ParseAddr(host)
	return err == nil
}

// validPairAddr accepts only IP:port with a port in 1-65535.
func validPairAddr(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if _, err := netip.ParseAddr(host); err != nil {
		return false
	}
	p, err := strconv.ParseUint(port, 10, 16)
	return err == nil && p >= 1
}

// startServer starts ADBuddy's adb server, waiting at most startServerWait.
func startServer(ctx context.Context, runner adb.Runner) error {
	sctx, cancel := context.WithTimeout(ctx, startServerWait)
	defer cancel()
	_, errOut, code, err := runner.Output(sctx, "start-server")
	switch {
	case ctx.Err() != nil:
		return ctx.Err() // Ctrl+C becomes INTERRUPTED
	case sctx.Err() != nil:
		return NewError("ADB_FAILED", fmt.Sprintf("adb start-server did not answer in %s", startServerWait), "Check "+runner.Path+", or stop other adb servers and try again.")
	case err != nil:
		return NewError("ADB_FAILED", "cannot start ADBuddy's adb server: "+err.Error(), "Check "+runner.Path+".")
	case code != 0:
		msg := classify.Printable(strings.TrimSpace(errOut))
		if msg == "" {
			msg = "no message"
		}
		return NewError("ADB_FAILED", fmt.Sprintf("cannot start ADBuddy's adb server: exit %d: %s", code, msg), "Check "+runner.Path+".")
	}
	return nil
}

// absExecutable makes an --adb path absolute; a bare name ("adb") must be
// in PATH.
func absExecutable(p string) (string, error) {
	if !strings.ContainsAny(p, `/\`) {
		found, err := exec.LookPath(p)
		if err != nil {
			name := classify.Printable(p)
			return "", fmt.Errorf("%s not found in PATH (pass its path, e.g. ./%s)", name, name)
		}
		p = found
	}
	return filepath.Abs(p)
}

// findPhone waits for the phone saved as serial and returns the serial adb
// lists it with now. A phone saved by its mDNS name is looked up by its
// guid: adb may list it renamed by mDNS ("<guid> (2)._adb-tls-connect._tcp")
// or, after `adb connect`, as its address; pairing.Connect accepts both and
// runs `adb connect` itself when the phone does not show up. Any other
// serial (or "" for the only phone) is waited for as is. A phone that does
// not show up in time is adb.ErrNoDevice.
func findPhone(ctx context.Context, runner adb.Runner, serial string, wait time.Duration) (string, error) {
	runner.Serial = "" // host commands: devices, mdns, connect
	guid, ok := pairing.GuidOf(serial)
	if !ok {
		return runner.WaitSerial(ctx, serial, wait)
	}
	s, err := pairing.Connect(ctx, runner, guid, wait, autoConnectWait, time.Second)
	if errors.Is(err, pairing.ErrTimeout) {
		return "", adb.ErrNoDevice
	}
	return s, err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// noPhoneHint is said once when a QR code waits and no phone announces
// wireless debugging on the network: the QR code needs mDNS.
const noPhoneHint = "I can't see any phone with wireless debugging on this network. If the phone is on another network (for example Tailscale) or this Wi-Fi blocks mDNS, press Ctrl+C and run `adbuddy setup --code`.\n"

// qrHint looks at the network once, after qrHintDelay, and says noPhoneHint
// if no `_adb-tls-*` service (of this phone or any other) shows up. Setup
// keeps waiting either way. The returned stop ends the look and waits for
// it, so nothing is said after the wait for the QR code is over.
func qrHint(ctx context.Context, runner adb.Runner, say func(string, ...any)) (stop func()) {
	hctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTimer(qrHintDelay)
		defer t.Stop()
		select {
		case <-hctx.Done():
			return
		case <-t.C:
		}
		ss, err := pairing.Services(hctx, runner)
		if err != nil || hctx.Err() != nil {
			return
		}
		for _, s := range ss {
			if strings.HasPrefix(s.Type, "_adb-tls-") {
				return
			}
		}
		say("%s", noPhoneHint)
	}()
	return func() { cancel(); <-done }
}
