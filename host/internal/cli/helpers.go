package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/hybriqtech/adbuddy/host/internal/daemon"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/screenshot"
	"github.com/spf13/cobra"
)

// The agent's helpers (M5 spec §3): adbuddy screen, tap, type, foreground,
// screenshot and notifications. Each one asks the running serve, which does
// it through the ADBuddy app or adb, under the same rules as an adb command.

// helperFixes are the fixes of the helpers' error codes (M5 spec §8).
var helperFixes = map[string]string{
	"UI_NOT_FOUND":   "Run `adbuddy screen` to see what is on the screen.",
	"UI_AMBIGUOUS":   "Use --id <id> or --xy <x,y> from `adbuddy screen`.",
	"UI_PROTECTED":   "Ask the user to do this part on the phone.",
	"UI_UNAVAILABLE": "Ask the user to open the ADBuddy app on the phone.",
	"UI_TIMEOUT":     "Run `adbuddy screen` to see whether it happened before trying again.",
}

// newHelperCmds builds the helpers' commands.
func newHelperCmds(app *App) []*cobra.Command {
	return []*cobra.Command{newScreenCmd(app), newTapCmd(app), newTypeCmd(app), newForegroundCmd(app), newScreenshotCmd(app), newNotificationsCmd(app)}
}

// callHelper runs a helper on the serve that runs and returns its data.
// stdout gets what the helper streams (the screenshot's bytes). The daemon
// writes the journal's command itself, from op and args. screen asks the
// daemon for the settled screen too (tap, type) or to wait for it (screen).
func callHelper(cmd *cobra.Command, app *App, op string, args any, screen bool, stdout io.Writer) ([]byte, error) {
	e, err := loadEnv(app)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	cwd, _ := os.Getwd()
	var data []byte
	req := ipc.Request{Kind: ipc.KindUI, UIOp: op, UIArgs: raw, Screen: screen, Cwd: cwd,
		Env: map[string]string{"ADBUDDY_SOURCE": app.Getenv("ADBUDDY_SOURCE")}}
	exit, err := ipc.Call(cmd.Context(), e.dirs.Data, req, nil, stdout, app.Stderr, func(b []byte) error {
		data = append([]byte{}, b...)
		return nil
	})
	if errors.Is(err, ipc.ErrNotRunning) {
		return nil, notRunning
	}
	if err != nil {
		return nil, NewError("DAEMON_UNREACHABLE", err.Error(), "")
	}
	if exit.Code == 0 {
		return data, nil
	}
	code := exit.ErrorCode
	if code == "" {
		code = "DAEMON_ERROR"
	}
	if code == "UI_INVALID" {
		return nil, UsageError(exit.Error)
	}
	return nil, &Error{Code: code, Message: exit.Error, Fix: helperFixes[code], Transient: exit.Code == ExitTransient, exit: exit.Code}
}

func newScreenCmd(app *App) *cobra.Command {
	var all, settle bool
	cmd := &cobra.Command{
		Use:     "screen",
		Short:   app.T("help.screen.short"),
		Long:    app.T("help.screen.long"),
		Example: "  adbuddy screen\n  adbuddy screen --settle\n  adbuddy screen --all --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := callHelper(cmd, app, bridge.UIOpDump, bridge.UIDumpArgs{All: all}, settle, app.Stderr)
			if err != nil {
				return err
			}
			// settled is a *bool: an old daemon does not say, and then there is
			// nothing to warn about. changed too: absent means not known, and
			// --json keeps it only when the daemon sent it.
			var d struct {
				bridge.UIDump
				Settled *bool `json:"settled"`
				Changed *bool `json:"changed"`
			}
			if err := json.Unmarshal(data, &d); err != nil {
				return NewError("UI_FAILED", "unreadable answer", "")
			}
			if app.JSON {
				if d.Settled != nil {
					return WriteData(app.Stdout, daemon.ScreenSettled{UIDump: d.UIDump, Settled: *d.Settled, Changed: d.Changed})
				}
				return WriteData(app.Stdout, d.UIDump)
			}
			printScreen(app.Stdout, d.UIDump)
			if d.Settled != nil && !*d.Settled {
				fmt.Fprintln(app.Stdout, stillChanging)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, app.T("help.screen.flag.all"))
	cmd.Flags().BoolVar(&settle, "settle", false, app.T("help.screen.flag.settle"))
	return cmd
}

// noticeInImage is what the agent reads on stderr when ADBuddy's on-screen
// notice could not be taken off the screen for a screenshot. Fixed English,
// for the agent.
const noticeInImage = "(ADBuddy's on-screen notice, a thin frame round the screen and a pill near the top, may be in this image because it could not be taken off the screen for the capture: ignore it)"

// stillChanging is what the agent reads when the screen did not settle in time.
const stillChanging = "(the screen was still changing: run adbuddy screen again if you need it steady)"

// noChange is what the agent reads after tap or type --screen when the screen
// settled and is the one from before the act: the act did nothing visible.
const noChange = "(the screen did not change in about 2 s after the action: run adbuddy screen before trying again)"

// printAfterAct writes what tap and type print after the act itself, with
// --screen: the settled screen, the daemon's note (the act was done and the
// read was refused or failed: exit 0, so nothing retries the act) or, from a
// daemon that ignored the flag, where to look. A settled screen that did not
// change says so; an unknown change (no changed) says nothing. Fixed English,
// for the agent.
func printAfterAct(w io.Writer, screen *daemon.ScreenSettled, note string) {
	switch {
	case screen != nil:
		fmt.Fprintln(w)
		printScreen(w, screen.UIDump)
		switch {
		case !screen.Settled:
			fmt.Fprintln(w, stillChanging)
		case screen.Changed != nil && !*screen.Changed:
			fmt.Fprintln(w, noChange)
		}
	case note != "":
		fmt.Fprintln(w, note)
	default:
		fmt.Fprintln(w, "This ADBuddy did not return the screen: run adbuddy screen.")
	}
}

// undecodedAfterAct is for an answer to tap/type --screen that came with exit
// 0 but cannot be decoded: the act was done, so say so and exit 0 (an error
// code would make the agent repeat it).
func undecodedAfterAct(app *App, done string) error {
	const note = "(the screen could not be decoded: run adbuddy screen)"
	if app.JSON {
		return WriteData(app.Stdout, map[string]string{"screenNote": note})
	}
	fmt.Fprintln(app.Stdout, done)
	fmt.Fprintln(app.Stdout, note)
	return nil
}

// printScreen writes a dump for the agent, one element per line. Everything
// comes from the phone: escaped and cut.
func printScreen(w io.Writer, d bridge.UIDump) {
	if d.Locked {
		fmt.Fprintln(w, "The phone is locked: ask the user to unlock it.")
		return
	}
	head := classify.Printable(d.Package)
	if head == "" {
		head = "unknown app"
	}
	fmt.Fprintf(w, "%s · %d×%d\n", head, d.Width, d.Height)
	for _, e := range d.Elements {
		var b strings.Builder
		if e.ID != "" {
			b.WriteString("[" + classify.Printable(e.ID) + "]")
		} else {
			b.WriteString("-")
		}
		if e.Text != "" {
			b.WriteString(" " + quoteScreen(e.Text))
		}
		if e.Desc != "" {
			b.WriteString(" desc=" + quoteScreen(e.Desc))
		}
		if e.Hint != "" {
			b.WriteString(" hint=" + quoteScreen(e.Hint))
		}
		b.WriteString(" " + classify.Printable(e.Class) + " " + fmt.Sprintf("(%d,%d)-(%d,%d)", e.Bounds[0], e.Bounds[1], e.Bounds[2], e.Bounds[3]))
		for _, f := range []struct {
			on   bool
			name string
		}{{e.Click, "click"}, {e.Edit, "edit"}, {e.Scroll, "scroll"}, {e.Focused, "focused"}, {e.Password, "password"}, {!e.Enabled, "disabled"}} {
			if f.on {
				b.WriteString(" " + f.name)
			}
		}
		if e.Checked != nil {
			b.WriteString(" checked=" + strconv.FormatBool(*e.Checked))
		}
		if e.Window != "" && e.Window != "app" {
			b.WriteString(" (" + classify.Printable(e.Window) + ")")
		}
		fmt.Fprintln(w, b.String())
	}
	if d.Truncated {
		fmt.Fprintln(w, "… more elements not shown")
	}
}

// quoteScreen quotes a text from the screen, cut to 120 characters.
func quoteScreen(s string) string {
	if utf8.RuneCountInString(s) > 120 {
		s = string([]rune(s)[:120]) + "…"
	}
	// Quote and backslash escaped, so a text cannot close its own quotes and
	// fake a field of the line. Before Printable, whose own escapes (\n,
	// ‮) must stay single: a backslash of the phone's is then told apart.
	s = classify.Printable(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s))
	return `"` + s + `"`
}

func newTapCmd(app *App) *cobra.Command {
	var id, at string
	var screen bool
	cmd := &cobra.Command{
		Use:     `tap ["<text>"] [--id <id>] [--xy <x,y>]`,
		Short:   app.T("help.tap.short"),
		Long:    app.T("help.tap.long"),
		Example: "  adbuddy tap \"Network & internet\"\n  adbuddy tap --id com.android.settings:id/search_action_bar\n  adbuddy tap --xy 540,1200",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var a bridge.UIClickArgs
			n := 0
			if len(args) == 1 {
				a.Text, n = args[0], n+1
			}
			if id != "" {
				a.ID, n = id, n+1
			}
			if at != "" {
				n++
				x, y, ok := parseXY(at)
				if !ok {
					return UsageError("--xy needs two numbers of 0 or more, like 540,1200")
				}
				a.X, a.Y = &x, &y
			}
			if n != 1 {
				return UsageError("tap needs exactly one of a text, --id or --xy")
			}
			data, err := callHelper(cmd, app, bridge.UIOpClick, a, screen, app.Stderr)
			if err != nil {
				return err
			}
			var c daemon.ClickedScreen
			if err := json.Unmarshal(data, &c); err != nil {
				if screen {
					return undecodedAfterAct(app, "Tapped.")
				}
				return NewError("UI_FAILED", "unreadable answer", "")
			}
			if app.JSON {
				if screen {
					return WriteData(app.Stdout, c)
				}
				return WriteData(app.Stdout, c.UIClicked)
			}
			fmt.Fprintf(app.Stdout, "Tapped (%d,%d)-(%d,%d).\n", c.Bounds[0], c.Bounds[1], c.Bounds[2], c.Bounds[3])
			if screen {
				printAfterAct(app.Stdout, c.Screen, c.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", app.T("help.tap.flag.id"))
	cmd.Flags().StringVar(&at, "xy", "", app.T("help.tap.flag.xy"))
	cmd.Flags().BoolVar(&screen, "screen", false, app.T("help.tap.flag.screen"))
	return cmd
}

func parseXY(s string) (x, y int, ok bool) {
	a, b, found := strings.Cut(s, ",")
	if !found {
		return 0, 0, false
	}
	x, err1 := strconv.Atoi(strings.TrimSpace(a))
	y, err2 := strconv.Atoi(strings.TrimSpace(b))
	return x, y, err1 == nil && err2 == nil && x >= 0 && y >= 0
}

func newTypeCmd(app *App) *cobra.Command {
	var screen bool
	cmd := &cobra.Command{
		Use:     `type "<text>"`,
		Short:   app.T("help.type.short"),
		Long:    app.T("help.type.long"),
		Example: "  adbuddy tap \"Search settings\"\n  adbuddy type \"Wi-Fi\"",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if n := utf8.RuneCountInString(args[0]); n == 0 || n > bridge.MaxSetText {
				return UsageError(fmt.Sprintf("type needs a text of 1 to %d characters", bridge.MaxSetText))
			}
			data, err := callHelper(cmd, app, bridge.UIOpSetText, bridge.UISetTextArgs{Text: args[0]}, screen, app.Stderr)
			if err != nil {
				return err
			}
			var r daemon.TextSetScreen
			if screen {
				if err := json.Unmarshal(data, &r); err != nil {
					return undecodedAfterAct(app, "Typed.")
				}
			}
			if app.JSON {
				if screen {
					return WriteData(app.Stdout, r)
				}
				return WriteData(app.Stdout, bridge.UITextSet{Set: true})
			}
			fmt.Fprintln(app.Stdout, "Typed.")
			if screen {
				printAfterAct(app.Stdout, r.Screen, r.Note)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&screen, "screen", false, app.T("help.type.flag.screen"))
	return cmd
}

func newForegroundCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "foreground",
		Short:   app.T("help.foreground.short"),
		Long:    app.T("help.foreground.long"),
		Example: "  adbuddy foreground\n  adbuddy foreground --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := callHelper(cmd, app, bridge.UIOpForeground, bridge.Empty{}, false, app.Stderr)
			if err != nil {
				return err
			}
			var fg bridge.UIForeground
			if err := json.Unmarshal(data, &fg); err != nil {
				return NewError("UI_FAILED", "unreadable answer", "")
			}
			if app.JSON {
				return WriteData(app.Stdout, fg)
			}
			switch {
			case fg.Package == "":
				fmt.Fprintln(app.Stdout, "No app is in the foreground (the screen may be off).")
			case fg.Activity == "":
				fmt.Fprintln(app.Stdout, classify.Printable(fg.Package))
			default:
				fmt.Fprintln(app.Stdout, classify.Printable(fg.Package)+"/"+classify.Printable(fg.Activity))
			}
			if fg.Protected {
				fmt.Fprintln(app.Stdout, "Something on the screen is protected (a protected app, a notification or a window ADBuddy cannot read): ADBuddy does not touch or read it.")
			}
			return nil
		},
	}
}

// now is the clock of the default screenshot name; tests fix it.
var now = time.Now

// screenshotWrite writes the image into the file just created; tests make it fail.
var screenshotWrite = func(f *os.File, b []byte) error {
	_, err := f.Write(b)
	return err
}

// saveNewScreenshot writes data to dir/YYYYMMDD-HHMMSS.mmm<ext>, or with -2,
// -3... before the extension if that name exists. The file is created with
// O_EXCL, so it never overwrites another; if writing fails it is removed.
func saveNewScreenshot(dir, ext string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stem := now().Format("20060102-150405.000")
	var f *os.File
	name, err := candidates(stem, ext, func(name string) (err error) {
		f, err = os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		return err
	})
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	werr := screenshotWrite(f, data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(path)
		return "", werr
	}
	return path, nil
}

func newScreenshotCmd(app *App) *cobra.Command {
	var scale float64
	var format, out string
	cmd := &cobra.Command{
		Use:     "screenshot",
		Short:   app.T("help.screenshot.short"),
		Long:    app.T("help.screenshot.long"),
		Example: "  adbuddy screenshot\n  adbuddy screenshot --scale 1 -o screen.png\n  adbuddy screenshot --format jpeg --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out != "" && format == "" {
				if ext := strings.ToLower(filepath.Ext(out)); ext == ".jpg" || ext == ".jpeg" {
					format = screenshot.JPEG
				}
			}
			if out != "" && format == "" && strings.ToLower(filepath.Ext(out)) == ".png" {
				format = screenshot.PNG
			}
			if format != "" && format != screenshot.PNG && format != screenshot.JPEG {
				return UsageError("--format must be png or jpeg")
			}
			// Without --scale (0) or --format ("") the daemon decides.
			if cmd.Flags().Changed("scale") && !(scale > 0 && scale <= 1) {
				return UsageError("--scale must be more than 0 and at most 1")
			}
			var img bytes.Buffer
			data, err := callHelper(cmd, app, daemon.OpScreenshot, daemon.ScreenshotArgs{Scale: scale, Format: format}, false, &img)
			if err != nil {
				return err
			}
			var r daemon.ScreenshotResult
			if err := json.Unmarshal(data, &r); err != nil || r.Bytes != img.Len() {
				return NewError("UI_FAILED", "the screenshot did not arrive whole", "Try again.")
			}
			if out == "" { // named after what the daemon made; never overwrites
				ext := map[string]string{screenshot.PNG: ".png", screenshot.JPEG: ".jpg"}[r.Format]
				if ext == "" {
					ext = ".png"
				}
				path, err := saveNewScreenshot("screenshots", ext, img.Bytes())
				if err != nil {
					return NewError("WRITE_FAILED", err.Error(), "")
				}
				out = path
			} else {
				if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
					return NewError("WRITE_FAILED", err.Error(), "")
				}
				if err := os.WriteFile(out, img.Bytes(), 0o644); err != nil {
					return NewError("WRITE_FAILED", err.Error(), "")
				}
			}
			if r.NoticeInImage && !app.JSON {
				fmt.Fprintln(app.Stderr, noticeInImage)
			}
			if app.JSON {
				data := map[string]any{"path": out, "format": r.Format, "bytes": img.Len()}
				if r.NoticeInImage {
					data["noticeInImage"] = true
				}
				return WriteData(app.Stdout, data)
			}
			fmt.Fprintln(app.Stdout, out)
			return nil
		},
	}
	cmd.Flags().Float64Var(&scale, "scale", 0, app.T("help.screenshot.flag.scale"))
	cmd.Flags().StringVar(&format, "format", "", app.T("help.screenshot.flag.format"))
	cmd.Flags().StringVarP(&out, "output", "o", "", app.T("help.screenshot.flag.output"))
	return cmd
}
