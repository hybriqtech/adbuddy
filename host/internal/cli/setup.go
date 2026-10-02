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
	"github.com/hybriqtech/adbuddy/host/internal/i18n"
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

// phoneGuide is the phone preparation text in English; step 3 depends on how
// it is paired.
func phoneGuide(useCode bool) string { return phoneGuideIn(i18n.EN, useCode) }

// phoneGuideIn is phoneGuide in lang.
func phoneGuideIn(lang i18n.Lang, useCode bool) string {
	network := lang.T("setup.guide.network")
	if useCode {
		network = lang.T("setup.guide.network_code")
	}
	return lang.T("setup.guide", network)
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
		Use:     "setup",
		Short:   app.T("help.setup.short"),
		Long:    app.T("help.setup.long"),
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
					return NewErrorT("AGENT_SESSION", "err.setup.agent_session", "err.setup.agent_session.fix")
				}
				if apkFlag != "" && noApp {
					return UsageErrorT("err.setup.usage_apk_and_no_app_contradict")
				}
				if connectFlag != "" && !validPairAddr(connectFlag) {
					return UsageErrorT("err.setup.usage_connect_is_not_an_ip", classify.Printable(connectFlag))
				}
				e, err := loadEnv(app)
				if err != nil {
					return err
				}
				// say writes the message key (with its arguments) and a line break in the language of this run.
				say := func(key string, a ...any) { fmt.Fprintln(app.Stderr, app.T(key, a...)) }
				in := bufio.NewReader(app.Stdin)
				type answer struct {
					line string
					err  error
				}
				ask := func(prompt string) (string, error) {
					fmt.Fprint(app.Stderr, prompt)
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
							return "", UsageErrorT("err.setup.usage_no_answer_on_stdin")
						}
						return strings.TrimSpace(a.line), nil
					}
				}

				// setup rewrites the environment and config.toml, and may run
				// `adb usb`: never under a running serve and its agent.
				if running, err := serveRunning(ctx, e.dirs.Data); err != nil {
					return err
				} else if running {
					return NewErrorT("ALREADY_RUNNING", "err.setup.already_running", "err.setup.already_running.fix")
				}

				// serve, run and journal read config.toml from any folder:
				// relative paths are saved as absolute ones.
				adbArg, wsDir := adbFlag, wsFlag
				if adbArg != "" {
					if adbArg, err = absExecutable(adbArg); err != nil {
						return UsageErrorT("err.setup.usage_adb", err.Error())
					}
				}
				if wsDir != "" {
					if wsDir, err = filepath.Abs(wsDir); err != nil {
						return UsageErrorT("err.setup.usage_workspace", err.Error())
					}
				}

				// 1. Tools.
				adbPath := adbArg
				if adbPath == "" {
					adbPath = e.cfg.ADBPath
				}
				if adbPath == "" {
					say("setup.getting_android_platform_tools")
					res, err := platformtools.Installer{DataDir: e.dirs.Data, BaseURL: repositoryURL(app), GOOS: runtime.GOOS, Log: app.Stderr}.Ensure(ctx)
					switch {
					case errors.Is(err, platformtools.ErrChecksum):
						// Not a network hiccup: the same file would come again.
						return NewErrorT("TOOLS_CHECKSUM_MISMATCH", "error.cause", "err.setup.tools_checksum_mismatch.fix", err.Error())
					case err != nil:
						return TransientErrorT("TOOLS_DOWNLOAD_FAILED", "error.cause", "err.setup.tools_download_failed.fix", err.Error())
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
					say("setup.looking_for_your_phone")
					var knownAddrs []string
					if known != "" {
						knownAddrs = []string{known}
					}
					k, err := findKnownPhone(ctx, runner, e.cfg.Serial, knownAddrs, knownPhoneWait, app.Log())
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
							say("setup.note_could_not_check_that", serial, unverifiedWhy(e.cfg.Serial))
						}
						say("setup.found_it_already_paired")
					}
				}

				// 3-4. Guide and pairing.
				if serial == "" {
					guid, err = pair(ctx, app, runner, useCode, ask, say)
					if err != nil {
						return err
					}
					say("setup.paired_connecting")
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
							say("setup.note_could_not_check_that_2", serial)
						}
					}
					if err != nil {
						addr := connectFlag
						if addr == "" && canAsk {
							say("setup.the_phone_did_not_show")
							if addr, err = ask(app.T("setup.prompt.connection_address")); err != nil {
								return err
							}
							if !validPairAddr(addr) {
								return UsageErrorT("err.setup.usage_is_not_an_ip_address", classify.Printable(addr))
							}
						}
						if addr == "" {
							return TransientErrorT("DEVICE_NOT_FOUND", "err.setup.device_not_found", "err.setup.device_not_found.fix", err.Error())
						}
						say("setup.connecting_to", addr)
						s, verified, err := connectAddr(ctx, runner, addr, guid, connectAddrWait)
						if err != nil {
							return err
						}
						if !verified {
							say("setup.note_could_not_check_that_3", addr)
						}
						serial, usedAddr = s, addr
					}
				}
				runner.Serial = serial

				// Host invariant: no open adb network port unless remote mode is on.
				if err := applyInvariant(ctx, runner, deviceWait, app.Log(), "adbuddy setup", ""); err != nil {
					return err // Ctrl+C becomes INTERRUPTED below
				}

				// Profile of the phone.
				prof, err := device.Collect(ctx, runner)
				if err != nil {
					return NewErrorT("DEVICE_UNREADABLE", "error.cause", "err.setup.device_unreadable.fix", err.Error())
				}
				say("setup.phone_android", classify.Printable(prof.Model), classify.Printable(prof.Android))

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
					say("setup.note_not_saving_the_serial", classify.Printable(serial))
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
					return NewErrorT("WORKSPACE_FAILED", "error.cause", "error.hint", err.Error(), workspaceHintMsg(err, ws))
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
						fmt.Fprintln(app.Stderr, app.T("setup.remote_mode_was_on_closing"))
					} else {
						// The `adb usb` went to another phone.
						fmt.Fprintln(app.Stderr, app.T("setup.remote_mode_was_on_for"))
					}
					cfg.RemoteUntil = ""
				}
				if err := config.Save(e.dirs.ConfigFile(), cfg); err != nil {
					return ConfigErrorT("CONFIG_UNWRITABLE", "error.cause", "err.setup.config_unwritable.fix", err.Error(), e.dirs.Config)
				}

				res := setupResult{Device: prof, Serial: saved, Active: serial, Address: cfg.Address, Workspace: ws, Agent: chosen, ADB: adbPath, App: appRes}
				if app.JSON {
					return WriteData(app.Stdout, res)
				}
				fmt.Fprintln(app.Stdout, app.T("setup.ready_android_environment_agent", classify.Printable(prof.Model), classify.Printable(prof.Android), ws, chosen, fmtAppMsg(appRes)))
				say("setup.next_run_adbuddy_serve_and")
				return nil
			}()
			if err != nil && ctx.Err() != nil {
				return Interrupted()
			}
			return err
		},
	}
	cmd.Flags().StringVar(&adbFlag, "adb", "", app.T("help.setup.flag.adb"))
	cmd.Flags().StringVar(&agentFlag, "agent", "", app.T("help.setup.flag.agent"))
	cmd.Flags().StringVar(&wsFlag, "workspace", "", app.T("help.setup.flag.workspace"))
	cmd.Flags().BoolVar(&useCode, "code", false, app.T("help.setup.flag.code"))
	cmd.Flags().StringVar(&connectFlag, "connect", "", app.T("help.setup.flag.connect"))
	cmd.Flags().BoolVar(&yes, "yes", false, app.T("help.setup.flag.yes"))
	cmd.Flags().StringVar(&apkFlag, "apk", "", app.T("help.setup.flag.apk"))
	cmd.Flags().BoolVar(&noApp, "no-app", false, app.T("help.setup.flag.no_app"))
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
	fmt.Fprint(app.Stderr, phoneGuideIn(app.speaks(), useCode))
	if art, err := pairing.RenderQR(HelpURL, colorOK(app)); err == nil {
		fmt.Fprint(app.Stderr, art)
	}
	fmt.Fprintf(app.Stderr, "%s\n\n", HelpURL)
	if useCode {
		addr, err := ask(app.T("setup.prompt.pairing_address"))
		if err != nil {
			return "", err
		}
		if !validPairAddr(addr) {
			return "", UsageErrorT("err.setup.usage_is_not_an_ip_address_2", classify.Printable(addr))
		}
		code, err := ask(app.T("setup.prompt.pairing_code"))
		if err != nil {
			return "", err
		}
		if !pairingCodeRe.MatchString(code) {
			return "", UsageErrorT("err.setup.usage_the_wi_fi_pairing_code")
		}
		guid, err := pairing.Pair(ctx, runner, addr, code)
		if err != nil {
			return "", NewErrorT("PAIRING_FAILED", "error.cause", "err.setup.pairing_failed.fix", err.Error())
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
	fmt.Fprint(app.Stderr, app.T("setup.on_the_phone_qr", art))
	say("setup.waiting_for_the_phone_up", pairingWait)
	stopHint := qrHint(ctx, runner, say)
	guid, err := pairing.PairQR(ctx, runner, q, pairingWait)
	stopHint()
	if errors.Is(err, pairing.ErrTimeout) {
		return "", TransientErrorT("PAIRING_TIMEOUT", "err.setup.pairing_timeout", "err.setup.pairing_timeout.fix")
	}
	if err != nil {
		return "", NewErrorT("PAIRING_FAILED", "error.cause", "err.setup.pairing_failed.fix_2", err.Error())
	}
	return guid, nil
}

// chooseAgent picks the agent: the flag, else the installed ones (Claude
// Code recommended), asking only on an interactive terminal.
func chooseAgent(app *App, flag, current string, yes bool, ask func(string) (string, error), say func(string, ...any)) (string, error) {
	found := agent.Detect(exec.LookPath)
	if flag != "" {
		if len(found) == 0 || !containsAgent(found, flag) {
			say("setup.note_is_not_in_path", flag, agent.InstallHint(flag))
		}
		return flag, nil
	}
	if len(found) == 0 {
		say("setup.no_coding_agent_found_in", agent.InstallHint(agent.Known[0]))
		if current != "" {
			return current, nil
		}
		return agent.Known[0], nil
	}
	if current != "" && !containsAgent(found, current) {
		// The user chose it before: never swap it silently.
		say("setup.note_is_not_in_path_2", current, agent.InstallHint(current))
		return current, nil
	}
	def := found[0].Name
	if current != "" && containsAgent(found, current) {
		def = current
	}
	if yes || len(found) == 1 || !isTerminal(app.Stdin) {
		say("setup.agent", def)
		return def, nil
	}
	names := make([]string, len(found))
	for i, f := range found {
		names[i] = f.Name
	}
	for {
		answer, err := ask(app.T("setup.prompt.agent", strings.Join(names, ", "), def))
		if err != nil {
			return "", err
		}
		if answer == "" {
			return def, nil
		}
		if containsAgent(found, answer) {
			return answer, nil
		}
		say("setup.please_choose_one_of", strings.Join(names, ", "))
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
		return NewErrorT("ADB_FAILED", "err.setup.adb_failed", "err.setup.adb_failed.fix", startServerWait, runner.Path)
	case err != nil:
		return NewErrorT("ADB_FAILED", "err.setup.adb_failed_2", "err.setup.adb_failed_2.fix", err.Error(), runner.Path)
	case code != 0:
		msg := classify.Printable(strings.TrimSpace(errOut))
		if msg == "" {
			msg = "no message"
		}
		return NewErrorT("ADB_FAILED", "err.setup.adb_failed_3", "err.setup.adb_failed_3.fix", code, msg, runner.Path)
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

// qrHint looks at the network once, after qrHintDelay, and says the hint
// (setup.no_phone_hint: a QR code needs mDNS) if no `_adb-tls-*` service (of
// this phone or any other) shows up. Setup
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
		say("setup.no_phone_hint")
	}()
	return func() { cancel(); <-done }
}

// workspaceHint is the guidance for a workspace.Init failure, in English.
func workspaceHint(err error, ws string) string { return workspaceHintMsg(err, ws).String() }

// workspaceHintMsg is workspaceHint as a message of its own, to be the fix of
// an error: NewErrorT(code, "error.cause", "error.hint", cause, workspaceHintMsg(err, ws)).
// An unsafe path is not a permissions problem.
func workspaceHintMsg(err error, ws string) i18n.Msg {
	if errors.Is(err, workspace.ErrUnsafeHookPath) {
		return i18n.M("setup.workspace_unsafe_path")
	}
	return i18n.M("setup.workspace_not_writable", ws)
}
