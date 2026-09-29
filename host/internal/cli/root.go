package cli

import (
	"io"
	"os"

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
	root.AddCommand(newVersionCmd(app), newServeCmd(app), newRunCmd(app), newJournalCmd(app), newUndoCmd(app), newEnvCmd(app))
	return root
}

// Main runs the CLI and returns the process exit code.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	app := &App{Stdin: stdin, Stdout: stdout, Stderr: stderr, Getenv: os.Getenv}
	root := NewRoot(app)
	app.JSON = wantsJSON(args) // errors raised before cobra reads --json still use the envelope
	root.SetArgs(args)
	err := root.Execute()
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
