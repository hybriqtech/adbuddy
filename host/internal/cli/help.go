package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// localizeHelp puts into the language of this run what cobra writes by
// itself: the headers of the usage template ("Usage:", "Flags:"…), the
// "help for …" and "version for …" flags of every command, and the listing of
// its own `help` and `completion` commands. It runs once, after every command
// has been added. The Short, Long and Example of each command and the help of
// each flag are the commands' own (app.T where each is built).
//
// What stays in English on purpose (spec M6c ruling 19): cobra's errors
// ("unknown command", "unknown flag"), which USAGE errors wrap with a fix line
// that is translated; and the text of `completion <shell>`, which is
// instructions for a shell.
func localizeHelp(app *App, root *cobra.Command) {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	// Longer headers first: "Global Flags:" before "Flags:".
	root.SetUsageTemplate(strings.NewReplacer(
		"Additional help topics:", app.T("help.topics"),
		"Additional Commands:", app.T("help.additional"),
		"Available Commands:", app.T("help.commands"),
		"Global Flags:", app.T("help.global_flags"),
		"Examples:", app.T("help.examples"),
		"Aliases:", app.T("help.aliases"),
		"Usage:", app.T("help.usage"),
		"Flags:", app.T("help.flags"),
		`Use "{{.CommandPath}} [command] --help" for more information about a command.`, app.T("help.more"),
	).Replace(root.UsageTemplate()))
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		if f := c.Flags().Lookup("help"); f != nil {
			f.Usage = app.T("help.flag", c.DisplayName())
		}
		if c.Version != "" {
			c.InitDefaultVersionFlag()
			if f := c.Flags().Lookup("version"); f != nil {
				f.Usage = app.T("help.version_flag", c.DisplayName())
			}
		}
		switch c.Name() {
		case "help":
			if c.Parent() == root {
				c.Short = app.T("help.cmd.short")
				c.Long = app.T("help.cmd.long", root.DisplayName())
			}
		case "completion":
			if c.Parent() == root {
				c.Short = app.T("help.completion.short")
				c.Long = app.T("help.completion.long", root.DisplayName())
			}
		case "bash", "zsh", "fish", "powershell":
			if p := c.Parent(); p != nil && p.Name() == "completion" && p.Parent() == root {
				c.Short = app.T("help.completion.sub", c.Name())
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
