package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/paths"
	"github.com/spf13/cobra"
)

// Version is set at build time with -ldflags "-X github.com/hybriqtech/adbuddy/host/internal/cli.Version=…".
var Version = "dev"

// App holds the process streams and environment so commands stay testable.
type App struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
	JSON   bool
	// Lang is the language of what this run writes for people (lang.go).
	// The zero value, an App built by hand in a test, is English.
	Lang i18n.Lang
}

// NewRoot builds the command tree.
func NewRoot(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "adbuddy",
		Short:         app.T("help.root.short"),
		Long:          app.T("help.root.long"),
		Example:       app.T("help.root.example"),
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetVersionTemplate("adbuddy {{.Version}}\n")
	root.PersistentFlags().BoolVar(&app.JSON, "json", false, app.T("help.root.flag.json"))
	// cobra's "Did you mean this?" lists commands that do not look alike; ours is suggestCommand.
	root.DisableSuggestions = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		if t := translateFlagError(err); t != nil {
			return t
		}
		return UsageError(err.Error())
	})
	root.AddGroup(
		&cobra.Group{ID: "start", Title: app.T("help.group.start")},
		&cobra.Group{ID: "daily", Title: app.T("help.group.daily")},
		&cobra.Group{ID: "agent", Title: app.T("help.group.agent")},
	)
	setup := newSetupCmd(app)
	root.AddCommand(newVersionCmd(app), newServeCmd(app), newRunCmd(app), newJournalCmd(app), newUndoCmd(app), newEnvCmd(app), newConnectCmd(app), newDoctorCmd(app), setup)
	root.AddCommand(newAttachCmd(app), newNewCmd(app), newStopCmd(app))
	root.AddCommand(newSendCmd(app), newRemoteCmd(app))
	root.AddCommand(newHookCmd(app))
	root.AddCommand(newHelperCmds(app)...)
	groupOf := map[string]string{
		"setup": "start", "doctor": "start", "serve": "start",
		"run": "daily", "attach": "daily", "new": "daily", "stop": "daily", "journal": "daily",
		"undo": "daily", "connect": "daily", "remote": "daily", "env": "daily", "version": "daily",
		"screen": "agent", "tap": "agent", "type": "agent", "screenshot": "agent",
		"foreground": "agent", "notifications": "agent", "send": "agent",
	}
	for _, c := range root.Commands() {
		c.GroupID = groupOf[c.Name()]
	}
	root.SetHelpCommand(&cobra.Command{
		Use: "help [command]",
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			var completions []cobra.Completion
			cmd, _, e := c.Root().Find(args)
			if e != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			if cmd == nil {
				cmd = c.Root()
			}
			for _, sub := range cmd.Commands() {
				if sub.IsAvailableCommand() || sub.Name() == "help" {
					if strings.HasPrefix(sub.Name(), toComplete) {
						completions = append(completions, cobra.CompletionWithDesc(sub.Name(), sub.Short))
					}
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := c.Root().Find(args)
			if target == nil || err != nil {
				topic := strings.Join(args, " ")
				if len(args) == 1 {
					if s := suggestCommand(c.Root(), args[0]); s != "" {
						return UsageErrorT("err.help.unknown_topic_suggest", topic, s)
					}
				}
				return UsageErrorT("err.help.unknown_topic", topic)
			}
			target.InitDefaultHelpFlag()
			target.InitDefaultVersionFlag()
			return target.Help()
		},
	})
	localizeHelp(app, root)
	// `adbuddy` alone starts the setup the first time (spec §3.1), but only
	// for a person at a terminal: a script, a CI job or an agent would get a
	// download, a background adb server and a long wait for a QR code.
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		if dirs, err := paths.Default(app.Getenv); err == nil && !fileExists(dirs.ConfigFile()) {
			if app.JSON || !isTerminal(app.Stdin) || !isTerminal(app.Stderr) {
				return ConfigErrorT("NOT_CONFIGURED", "err.root.not_configured", "err.root.not_configured.fix")
			}
			fmt.Fprintln(app.Stderr, app.T("root.adbuddy_is_not_set_up"))
			setup.SetContext(cmd.Context())
			return setup.RunE(setup, nil)
		}
		return cmd.Help()
	}
	return root
}

// Main runs the CLI and returns the process exit code.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return mainContext(context.Background(), args, stdin, stdout, stderr)
}

// mainContext is Main with a context that tests can cancel (Ctrl+C).
func mainContext(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	defer utf8Console(asFile(stdout), asFile(stderr))()
	lang := detectLang(os.Getenv)
	if wantsJSON(args) {
		lang = i18n.EN // machine output, help included, is English
	}
	app := &App{Stdin: stdin, Stdout: stdout, Stderr: stderr, Getenv: withLang(os.Getenv, lang), Lang: lang}
	root := NewRoot(app)
	app.JSON = wantsJSON(args) // errors raised before cobra reads --json still use the envelope
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(ctx)
	if err != nil {
		var code *exitCodeError
		var typed *Error
		if !errors.As(err, &code) && !errors.As(err, &typed) {
			if t := translateArgsError(root, cmd, err); t != nil {
				err = t
			}
		}
		if !errors.As(err, &code) && cmd != nil {
			name := cmd.CommandPath()
			// help's own usage error points at the root, not at `adbuddy help --help`.
			if cmd.Name() == "help" && cmd.Parent() == root {
				name = root.CommandPath()
			}
			err = AsError(err).forCommand(name)
		}
		WriteFailureIn(stdout, stderr, app.JSON, app.speaks(), err)
	}
	return ExitCode(err)
}

// wantsJSON reports whether --json appears before "--".
func wantsJSON(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}
