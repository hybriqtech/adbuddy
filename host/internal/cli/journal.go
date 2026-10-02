package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/hybriqtech/adbuddy/host/internal/journal"
	"github.com/hybriqtech/adbuddy/host/internal/workspace"
	"github.com/spf13/cobra"
)

// journalEntry is an entry as `journal --json` prints it: Verified is true
// only when its signature was checked and is good.
type journalEntry struct {
	journal.Entry
	Verified bool `json:"verified"`
}

func newJournalCmd(app *App) *cobra.Command {
	var session, wsDir string
	var limit int
	cmd := &cobra.Command{
		Use:     "journal",
		Short:   app.T("help.journal.short"),
		Example: "  adbuddy journal\n  adbuddy journal --session current\n  adbuddy journal --session s_01J… --json\n  adbuddy journal --limit 0 --json | jq '.data.entries[] | select(.level==\"red\")'",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit < 0 {
				return UsageErrorT("err.journal.usage_limit_must_be_0_all")
			}
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			ws := e.workspace(wsDir)
			if session == "current" || wsDir == "" {
				pctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
				p, err := ipc.PingDaemon(pctx, e.dirs.Data)
				cancel()
				if session == "current" {
					if errors.Is(err, ipc.ErrNotRunning) {
						return NewErrorT("DAEMON_NOT_RUNNING", "err.journal.daemon_not_running", "err.journal.daemon_not_running.fix")
					}
					if err != nil {
						return NewError("DAEMON_UNREACHABLE", err.Error(), "")
					}
					session = p.Session
				}
				// Without --workspace, the journal of the serve that runs.
				if err == nil && wsDir == "" && serveWorkspace(p) != "" {
					ws = serveWorkspace(p)
				}
			}
			dir := workspace.JournalDir(ws)
			store := journal.Open(dir)
			secret, err := ipc.ReadSecret(e.dirs.Data)
			signed := err == nil
			if signed {
				store = journal.OpenSigned(dir, []byte(secret))
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
			if !signed && len(out) > 0 {
				fmt.Fprintln(app.Stderr, app.T("journal.the_journal_signatures_could_not"))
			}
			if app.JSON {
				list := make([]journalEntry, 0, len(out))
				for _, j := range out {
					list = append(list, journalEntry{Entry: j, Verified: signed && !j.Tampered})
				}
				return WriteData(app.Stdout, map[string]any{"entries": list, "skippedLines": skipped})
			}
			for _, j := range out {
				fmt.Fprintln(app.Stdout, formatEntryIn(app.speaks(), j))
			}
			if len(out) == 0 {
				fmt.Fprintln(app.Stderr, app.T("journal.the_journal_is_empty"))
			}
			if skipped > 0 {
				fmt.Fprintln(app.Stderr, app.TN("journal.damaged_journal_lines_were_skipped", skipped))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&session, "session", "", app.T("help.journal.flag.session"))
	cmd.Flags().IntVar(&limit, "limit", 50, app.T("help.journal.flag.limit"))
	cmd.Flags().StringVar(&wsDir, "workspace", "", app.T("help.journal.flag.workspace"))
	return cmd
}
