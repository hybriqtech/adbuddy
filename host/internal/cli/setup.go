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
)

const phoneGuide = `Prepare your phone (only once):
  1. Settings → About phone → tap "Build number" 7 times to turn on Developer options.
  2. Developer options (Settings → System on a Pixel) → turn on "Wireless debugging".
     When the phone asks, tick "Always allow on this network".
  3. Keep the phone on the same Wi-Fi network as this PC.
Step-by-step guide with pictures (scan it with the phone's camera):
`

type setupResult struct {
	Device    device.Profile `json:"device"`
	Serial    string         `json:"serial"`
	Workspace string         `json:"workspace"`
	Agent     string         `json:"agent"`
	ADB       string         `json:"adb"`
}

func newSetupCmd(app *App) *cobra.Command {
	var adbFlag, agentFlag, wsFlag string
	var useCode, yes bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up ADBuddy: get adb, pair the phone over Wi-Fi and prepare the agent environment",
		Long: `Set up ADBuddy step by step: download Android platform-tools (checked against
Google's checksum), pair the phone over wireless debugging with a QR code (or
with --code), connect, read the phone's profile, pick the agent and create the
environment. Run it again at any time; it skips what is already done.`,
		Example: "  adbuddy setup\n  adbuddy setup --code\n  adbuddy setup --agent codex --yes",
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

				// serve, run and journal read config.toml from any folder:
				// relative paths are saved as absolute ones.
				if adbFlag != "" {
					if adbFlag, err = absExecutable(adbFlag); err != nil {
						return UsageError("--adb: " + err.Error())
					}
				}
				wsDir := wsFlag
				if wsDir != "" {
					if wsDir, err = filepath.Abs(wsDir); err != nil {
						return UsageError("--workspace: " + err.Error())
					}
				}

				// 1. Tools.
				adbPath := adbFlag
				if adbPath == "" {
					adbPath = e.cfg.ADBPath
				}
				if adbPath == "" {
					say("Getting Android platform-tools…\n")
					res, err := platformtools.Installer{DataDir: e.dirs.Data, BaseURL: repositoryURL(app), GOOS: runtime.GOOS, Log: app.Stderr}.Ensure(ctx)
					if err != nil {
						return TransientError("TOOLS_DOWNLOAD_FAILED", err.Error(), "Check the internet connection and run `adbuddy setup` again, or pass --adb <path to adb 37 or newer>.")
					}
					adbPath = res.ADB
				}
				runner := adb.Runner{Path: adbPath, ServerPort: e.cfg.ADBServerPort}
				if _, errOut, code, err := runner.Output(ctx, "start-server"); err != nil || code != 0 {
					return NewError("ADB_FAILED", fmt.Sprintf("cannot start ADBuddy's adb server: %v %s", err, strings.TrimSpace(errOut)), "Check "+adbPath+".")
				}

				// 2. A phone paired before comes back on its own.
				serial := ""
				if e.cfg.Serial != "" {
					say("Looking for your phone…\n")
					s, err := runner.WaitSerial(ctx, e.cfg.Serial, knownPhoneWait)
					if ctx.Err() != nil {
						return ctx.Err() // Ctrl+C: no guide nor QR code
					}
					if err == nil {
						serial = s
						say("Found it: already paired.\n")
					}
				}

				// 3-4. Guide and pairing.
				guid := ""
				if serial == "" {
					guid, err = pair(ctx, app, runner, useCode, ask, say)
					if err != nil {
						return err
					}
					say("Paired. Connecting…\n")
					serial, err = pairing.Connect(ctx, runner, guid, connectWait, autoConnectWait, time.Second)
					if err != nil {
						return TransientError("DEVICE_NOT_FOUND", "the paired phone did not connect: "+err.Error(), "Check that wireless debugging is still on, then run `adbuddy setup` again.")
					}
				}
				runner.Serial = serial

				// Host invariant: no open adb network port unless remote mode is on.
				if err := applyInvariant(ctx, runner, deviceWait, app.Stderr, "adbuddy setup"); err != nil {
					return err // Ctrl+C becomes INTERRUPTED below
				}

				// 5. Profile.
				prof, err := device.Collect(ctx, runner)
				if err != nil {
					return NewError("DEVICE_UNREADABLE", err.Error(), "Check that the phone is unlocked and still connected.")
				}
				say("Phone: %s, Android %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android))

				// 6. Agent.
				chosen, err := chooseAgent(app, agentFlag, e.cfg.Agent, yes, ask, say)
				if err != nil {
					return err
				}

				// 7. Environment and configuration. The saved serial is the
				// phone's stable name, not the address it used this time.
				saved := stableSerial(guid, serial)
				if saved == "" {
					say("Note: not saving the serial %s in the configuration: it is an address that changes when wireless debugging restarts. Pair again with `adbuddy setup` to save the phone's name.\n", classify.Printable(serial))
				}
				samePhone := e.cfg.Serial != "" && (e.cfg.Serial == saved || e.cfg.Serial == serial)
				ws := wsDir
				if ws == "" && samePhone {
					ws = e.cfg.Workspace // the environment chosen before
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
				if adbFlag != "" {
					cfg.ADBPath = adbFlag
				}
				if err := config.Save(e.dirs.ConfigFile(), cfg); err != nil {
					return ConfigError("CONFIG_UNWRITABLE", err.Error(), "Check that "+e.dirs.Config+" is writable.")
				}

				res := setupResult{Device: prof, Serial: saved, Workspace: ws, Agent: chosen, ADB: adbPath}
				if res.Serial == "" {
					res.Serial = serial
				}
				if app.JSON {
					return WriteData(app.Stdout, res)
				}
				fmt.Fprintf(app.Stdout, "Ready: %s (Android %s), environment %s, agent %s.\n", classify.Printable(prof.Model), classify.Printable(prof.Android), ws, chosen)
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
	cmd.Flags().BoolVar(&yes, "yes", false, "accept the recommended choices without asking")
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
	say("%s", phoneGuide)
	if art, err := pairing.RenderQR(HelpURL, colorOK(app)); err == nil {
		say("%s", art)
	}
	say("%s\n\n", HelpURL)
	if useCode {
		addr, err := ask("In Wireless debugging → Pair device with pairing code, type the IP address and port: ")
		if err != nil {
			return "", err
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
			return "", NewError("PAIRING_FAILED", err.Error(), "Check the address and the code (they change every time) and try again.")
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
	guid, err := pairing.PairQR(ctx, runner, q, pairingWait)
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

// isAddress reports whether serial is IP:port (a TCP connection).
func isAddress(serial string) bool {
	host, _, err := net.SplitHostPort(serial)
	if err != nil {
		return false
	}
	_, err = netip.ParseAddr(host)
	return err == nil
}

// absExecutable makes an --adb path absolute; a bare name ("adb") is looked
// up in PATH first.
func absExecutable(p string) (string, error) {
	if !strings.ContainsAny(p, `/\`) {
		if found, err := exec.LookPath(p); err == nil {
			p = found
		}
	}
	return filepath.Abs(p)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
