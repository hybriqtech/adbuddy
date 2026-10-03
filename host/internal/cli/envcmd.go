package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

func newEnvCmd(app *App) *cobra.Command {
	env := &cobra.Command{Use: "env", Short: app.T("help.env.short")}
	var dir string
	var force bool
	initCmd := &cobra.Command{
		Use:     "init",
		Short:   app.T("help.init.short"),
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
			for _, l := range groupSkillLines(app.speaks(), "created", res.Created) {
				fmt.Fprintln(app.Stderr, l)
			}
			for _, l := range groupSkillLines(app.speaks(), "updated", res.Updated) {
				fmt.Fprintln(app.Stderr, l)
			}
			fmt.Fprintln(app.Stderr, app.T("envcmd.environment_ready_next_adbuddy_serve"))
			return nil
		},
	}
	initCmd.Flags().StringVar(&dir, "dir", "", app.T("help.init.flag.dir"))
	initCmd.Flags().BoolVar(&force, "force", false, app.T("help.init.flag.force"))
	env.AddCommand(initCmd)
	return env
}

// groupSkillPaths is groupSkillLines in English (what the tests read).
func groupSkillPaths(verb string, paths []string) []string {
	return groupSkillLines(i18n.EN, verb, paths)
}

// groupSkillLines prints one line per path, except the skill files, which are
// summarised in one line per folder: "created 9 skills in .claude/skills". verb
// is "created" or "updated". English and Spanish both have their singular;
// the count picks the form.
func groupSkillLines(lang i18n.Lang, verb string, paths []string) []string {
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
			if verb == "created" {
				lines = append(lines, lang.T("envcmd.created", slash))
			} else {
				lines = append(lines, lang.T("envcmd.updated", slash))
			}
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
		if verb == "created" {
			lines = append(lines, lang.TN("envcmd.skills_created", len(skills[r]), r))
		} else {
			lines = append(lines, lang.TN("envcmd.skills_updated", len(skills[r]), r))
		}
	}
	return lines
}
