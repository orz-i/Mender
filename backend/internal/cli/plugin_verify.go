package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	"github.com/orz-i/Mender/backend/internal/providerplugin"
	"github.com/spf13/cobra"
)

func (a *app) newPluginVerifyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <installation>",
		Short: "Verify a missing Provider plugin descriptor snapshot",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := providerplugin.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.VerifyProviderPluginInstallation(cmd.Context(), id)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("verified descriptor for %s", item.ID()),
					map[string]any{"installation": pluginInstallationProjection(item)})
			})
		},
	}
}
