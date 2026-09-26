package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/buildinfo"
	"github.com/spf13/cobra"
)

func (a *app) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show Mender build version",
		Args:  exactArgs(0),
		RunE: func(*cobra.Command, []string) error {
			info := buildinfo.Current()
			return a.writeValue(
				fmt.Sprintf(
					"Mender %s (commit %s, built %s)",
					info.Version,
					info.Commit,
					info.BuildDate,
				),
				map[string]any{
					"version":   info.Version,
					"commit":    info.Commit,
					"buildDate": info.BuildDate,
				},
			)
		},
	}
}
