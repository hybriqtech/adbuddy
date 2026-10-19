package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/i18n"
	"github.com/hybriqtech/adbuddy/host/internal/ipc"
	"github.com/spf13/cobra"
)

// yoloInfo is the YOLO mode as `adbuddy yolo` reports it. Serve says whether
// `adbuddy serve` answered: YOLO lives in its memory, so with it stopped the
// mode is off.
type yoloInfo struct {
	Active  bool       `json:"active"`
	Session bool       `json:"session,omitempty"`
	Until   *time.Time `json:"until,omitempty"`
	Serve   bool       `json:"serve"`
}

// newYoloCmd is `adbuddy yolo off|status`. There is no `on`: only the phone
// app turns the mode on, and the agent may run this command (turning it off is
// always safe).
func newYoloCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:       "yolo off|status",
		Short:     app.T("help.yolo.short"),
		Long:      app.T("help.yolo.long"),
		Example:   "  adbuddy yolo status\n  adbuddy yolo off",
		Args:      cobra.ArbitraryArgs,
		ValidArgs: []string{"off", "status"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || args[0] != "off" && args[0] != "status" {
				return UsageErrorT("err.yolocmd.usage")
			}
			op := args[0]
			e, err := loadEnv(app)
			if err != nil {
				return err
			}
			var wire ipc.YoloStatus
			exit, err := ipc.Call(cmd.Context(), e.dirs.Data, ipc.Request{Kind: ipc.KindYolo, Argv: []string{op}},
				nil, app.Stderr, app.Stderr, func(b []byte) error { return json.Unmarshal(b, &wire) })
			info := yoloInfo{Serve: true}
			switch {
			case errors.Is(err, ipc.ErrNotRunning):
				info = yoloInfo{}
			case err != nil:
				if cmd.Context().Err() != nil {
					return Interrupted()
				}
				return NewError("DAEMON_UNREACHABLE", err.Error(), "")
			case exit.Code != 0:
				return NewError(orDefault(exit.ErrorCode, "DAEMON_ERROR"), exit.Error, "")
			default:
				info.Active, info.Session = wire.Active, wire.Session
				if wire.Active && !wire.Session {
					t := time.UnixMilli(wire.Until).UTC()
					info.Until = &t
				}
			}
			if app.JSON {
				return WriteData(app.Stdout, info)
			}
			fmt.Fprintln(app.Stdout, yoloTextIn(app.speaks(), info))
			return nil
		},
	}
}

// yoloTextIn is what the person reads, in lang.
func yoloTextIn(lang i18n.Lang, i yoloInfo) string {
	switch {
	case !i.Serve:
		return lang.T("yolocmd.serve_not_running")
	case !i.Active:
		return lang.T("yolocmd.off")
	case i.Session:
		return lang.T("yolocmd.on_session")
	}
	return lang.T("yolocmd.on_until", i.Until.Local().Format("15:04"))
}
