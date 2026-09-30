package cli

import (
	"fmt"
	"os"

	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

func newEnvCmd(app *App) *cobra.Command {
	env := &cobra.Command{Use: "env", Short: "Manage the agent environment"}
	var dir string
	var force bool
	initCmd := &cobra.Command{
		Use:     "init",
		Short:   "Create or refresh the agent environment (AGENTS.md, CLAUDE.md, permissions, adb shim)",
		Example: "  adbuddy env init\n  adbuddy env init --force\n  adbuddy env init --dir ~/ADBuddy/pixel --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			exe, err := os.Executable()
			if err != nil {
				return NewError("EXECUTABLE_UNKNOWN", err.Error(), "")
			}
			res, err := workspace.Init(workspace.Options{Dir: e.workspace(dir), Exe: exe, Force: force})
			if err != nil {
				return NewError("WORKSPACE_FAILED", err.Error(), "")
			}
			if res.Created == nil {
				res.Created = []string{}
			}
			if res.Updated == nil {
				res.Updated = []string{}
			}
			if app.JSON {
				return WriteData(app.Stdout, res)
			}
			fmt.Fprintln(app.Stdout, res.Dir)
			for _, f := range res.Created {
				fmt.Fprintln(app.Stderr, "  created "+f)
			}
			for _, f := range res.Updated {
				fmt.Fprintln(app.Stderr, "  updated "+f)
			}
			fmt.Fprintln(app.Stderr, "Environment ready. Next: `adbuddy serve`, then `adbuddy run` in another terminal.")
			return nil
		},
	}
	initCmd.Flags().StringVar(&dir, "dir", "", "environment directory (default: workspace in config.toml, else ~/ADBuddy/default)")
	initCmd.Flags().BoolVar(&force, "force", false, "rewrite AGENTS.md and CLAUDE.md (NOTES.md is never touched)")
	env.AddCommand(initCmd)
	return env
}
