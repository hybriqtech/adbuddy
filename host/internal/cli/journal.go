package cli

import (
	"errors"
	"fmt"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

func newJournalCmd(app *App) *cobra.Command {
	var session, wsDir string
	var limit int
	cmd := &cobra.Command{
		Use:     "journal",
		Short:   "Show the journal of adb commands and their inverses",
		Example: "  adbuddy journal\n  adbuddy journal --session\n  adbuddy journal --session s_01J… --json\n  adbuddy journal --limit 0 --json | jq '.data.entries[] | select(.level==\"red\")'",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			if session == "current" {
				p, err := ipc.PingDaemon(cmd.Context(), e.dirs.Data)
				if errors.Is(err, ipc.ErrNotRunning) {
					return NewError("DAEMON_NOT_RUNNING", "the current session is only known while `adbuddy serve` runs", "Pass --session <id> (ids start with s_).")
				}
				if err != nil {
					return NewError("DAEMON_UNREACHABLE", err.Error(), "")
				}
				session = p.Session
			}
			store := journal.Open(workspace.JournalDir(e.workspace(wsDir)))
			if secret, err := ipc.ReadSecret(e.dirs.Data); err == nil {
				store = journal.OpenSigned(workspace.JournalDir(e.workspace(wsDir)), []byte(secret))
			}
			entries, skipped, err := store.Load()
			if err != nil {
				return NewError("JOURNAL_UNREADABLE", err.Error(), "")
			}
			var out []journal.Entry
			for _, j := range entries {
				if session == "" || j.Session == session {
					out = append(out, j)
				}
			}
			if limit > 0 && len(out) > limit {
				out = out[len(out)-limit:]
			}
			if out == nil {
				out = []journal.Entry{}
			}
			if app.JSON {
				return WriteData(app.Stdout, map[string]any{"entries": out, "skippedLines": skipped})
			}
			for _, j := range out {
				fmt.Fprintln(app.Stdout, formatEntry(j))
			}
			if len(out) == 0 {
				fmt.Fprintln(app.Stderr, "The journal is empty.")
			}
			if skipped > 0 {
				fmt.Fprintf(app.Stderr, "⚠ %d damaged journal lines were skipped.\n", skipped)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "only this session (without a value: the current one)")
	cmd.Flags().Lookup("session").NoOptDefVal = "current"
	cmd.Flags().IntVar(&limit, "limit", 50, "show only the last N entries (0 = all)")
	cmd.Flags().StringVar(&wsDir, "workspace", "", "agent environment (default: ~/ADBuddy/default)")
	return cmd
}
