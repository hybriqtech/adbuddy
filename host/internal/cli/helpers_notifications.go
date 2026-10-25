package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
	"github.com/hybriqtech/adbuddy/host/internal/classify"
	"github.com/spf13/cobra"
)

// newNotificationsCmd is `adbuddy notifications` (M5 spec §3.5). The app does
// it through its notification listener, once the user has allowed it; there
// is no adb fallback, because adb would show the text of every app.
func newNotificationsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "notifications",
		Short:   app.T("help.notifications.short"),
		Long:    app.T("help.notifications.long"),
		Example: "  adbuddy notifications\n  adbuddy notifications --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n, err := callHelperInto[bridge.UINotifications](cmd, app, bridge.UIOpNotifications, bridge.Empty{}, false, app.Stderr)
			if err != nil {
				return err
			}
			if n.Notifications == nil {
				n.Notifications = []bridge.UINotification{}
			}
			if app.JSON {
				return WriteData(app.Stdout, n)
			}
			printNotifications(app.Stdout, n)
			return nil
		},
	}
}

// printNotifications writes the notifications for the agent, one per line.
// Everything comes from the phone: escaped, and the text cut to 120
// characters (--json has it whole).
func printNotifications(w io.Writer, n bridge.UINotifications) {
	if len(n.Notifications) == 0 {
		fmt.Fprintln(w, "No notifications.")
		return
	}
	fmt.Fprintf(w, "%d notifications, newest first\n", len(n.Notifications))
	cut := false
	for _, x := range n.Notifications {
		var b strings.Builder
		pkg := classify.Printable(x.Package)
		if pkg == "" {
			pkg = "unknown app"
		}
		b.WriteString(pkg)
		if !x.PostedAt.IsZero() {
			b.WriteString(" " + x.PostedAt.UTC().Format(time.RFC3339))
		}
		if x.Ongoing {
			b.WriteString(" ongoing")
		}
		if x.Title == "" && x.Text == "" {
			b.WriteString(" (no text: empty, or hidden: a protected app or a notification marked private)")
		}
		if x.Title != "" {
			b.WriteString(" title=" + quoteScreen(x.Title))
		}
		if x.Text != "" {
			b.WriteString(" text=" + quoteScreen(x.Text))
		}
		cut = cut || len([]rune(x.Title)) > 120 || len([]rune(x.Text)) > 120
		fmt.Fprintln(w, b.String())
	}
	if n.Truncated {
		fmt.Fprintln(w, "… older notifications not shown")
	}
	if cut {
		fmt.Fprintln(w, "Some text was cut: use --json for the whole text.")
	}
}
