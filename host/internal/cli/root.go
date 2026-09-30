package cli

import (
	"context"
	"fmt"
	"io"
	"os"

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
}

const exitCodesHelp = `Exit codes:
  0    success
  1    failure (see the error code)
  2    invalid usage
  75   temporary failure, retrying may help
  78   configuration error
  130  interrupted (Ctrl+C)
`

// NewRoot builds the command tree.
func NewRoot(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "adbuddy",
		Short:         "Let any coding agent manage your Android phone over Wi-Fi adb, safely",
		Long:          "ADBuddy classifies every adb command, journals every change so it can be undone,\nand asks before anything irreversible.\n\n" + exitCodesHelp,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetVersionTemplate("adbuddy {{.Version}}\n")
	root.PersistentFlags().BoolVar(&app.JSON, "json", false, "print machine-readable JSON on stdout")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return UsageError(err.Error()) })
	setup := newSetupCmd(app)
	root.AddCommand(newVersionCmd(app), newServeCmd(app), newRunCmd(app), newJournalCmd(app), newUndoCmd(app), newEnvCmd(app), newConnectCmd(app), newDoctorCmd(app), setup)
	// `adbuddy` alone starts the setup the first time (spec §3.1), but only
	// for a person at a terminal: a script, a CI job or an agent would get a
	// download, a background adb server and a long wait for a QR code.
	root.RunE = func(cmd *cobra.Command, _ []string) error {
		if dirs, err := paths.Default(app.Getenv); err == nil && !fileExists(dirs.ConfigFile()) {
			if app.JSON || !isTerminal(app.Stdin) || !isTerminal(app.Stderr) {
				return ConfigError("NOT_CONFIGURED", "ADBuddy is not set up yet", "Run `adbuddy setup`.")
			}
			fmt.Fprintln(app.Stderr, "ADBuddy is not set up yet: starting `adbuddy setup`.")
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
	app := &App{Stdin: stdin, Stdout: stdout, Stderr: stderr, Getenv: os.Getenv}
	root := NewRoot(app)
	app.JSON = wantsJSON(args) // errors raised before cobra reads --json still use the envelope
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err != nil {
		WriteFailure(stdout, stderr, app.JSON, err)
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
