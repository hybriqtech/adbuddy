package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

type versionInfo struct {
	Version string `json:"version"`
	Go      string `json:"go"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: app.T("help.version.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := versionInfo{Version: Version, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
			if app.JSON {
				return WriteData(app.Stdout, info)
			}
			_, err := fmt.Fprintf(app.Stdout, "adbuddy %s (%s, %s/%s)\n", info.Version, info.Go, info.OS, info.Arch)
			return err
		},
	}
}
