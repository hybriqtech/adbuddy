package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

func newEnvCmd(app *App) *cobra.Command {
	env := &cobra.Command{Use: "env", Short: "Manage the agent environment"}
	var dir string
	var force bool
	initCmd := &cobra.Command{
		Use:     "init",
		Short:   "Create or refresh the agent environment (AGENTS.md, CLAUDE.md, skills, permissions, adb shim)",
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
			for _, l := range groupSkillPaths("created", res.Created) {
				fmt.Fprintln(app.Stderr, l)
			}
			for _, l := range groupSkillPaths("updated", res.Updated) {
				fmt.Fprintln(app.Stderr, l)
			}
			fmt.Fprintln(app.Stderr, "Environment ready. Next: `adbuddy serve`, then `adbuddy run` in another terminal.")
			return nil
		},
	}
	initCmd.Flags().StringVar(&dir, "dir", "", "environment directory (default: workspace in config.toml, else ~/ADBuddy/default)")
	initCmd.Flags().BoolVar(&force, "force", false, "rewrite AGENTS.md, CLAUDE.md and the skills (NOTES.md is never touched; a skill file with <!-- adbuddy:user --> is kept)")
	env.AddCommand(initCmd)
	return env
}

// groupSkillPaths prints one line per path, except the skill files, which are
// summarised in one line per folder: "created 9 skills in .claude/skills".
func groupSkillPaths(verb string, paths []string) []string {
	var lines, order []string
	skills := map[string]map[string]bool{}
	for _, p := range paths {
		slash := filepath.ToSlash(p)
		root := ""
		for _, r := range []string{".claude/skills", ".agents/skills"} {
			if strings.HasPrefix(slash, r+"/") {
				root = r
			}
		}
		if root == "" {
			lines = append(lines, "  "+verb+" "+slash)
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(slash, root+"/"), "/")
		if skills[root] == nil {
			skills[root] = map[string]bool{}
			order = append(order, root)
		}
		skills[root][name] = true
	}
	for _, r := range order {
		lines = append(lines, fmt.Sprintf("  %s %d skills in %s", verb, len(skills[r]), r))
	}
	return lines
}
