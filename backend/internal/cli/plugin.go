package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	"github.com/spf13/cobra"
)

func (a *app) newPluginCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "plugin",
		Short: "Manage external Provider plugin packages",
	}
	command.AddCommand(a.newPluginListCommand())
	command.AddCommand(a.newPluginInstallCommand())
	command.AddCommand(a.newPluginInspectCommand())
	command.AddCommand(a.newPluginStatusCommand(true))
	command.AddCommand(a.newPluginStatusCommand(false))
	command.AddCommand(a.newPluginRemoveCommand())
	return command
}

func (a *app) newPluginListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed external Provider plugins",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				items, err := runtime.ListProviderPluginInstallations(cmd.Context())
				if err != nil {
					return err
				}
				if a.output == "json" {
					projected := make([]map[string]any, 0, len(items))
					for _, item := range items {
						projected = append(projected, pluginInstallationProjection(item))
					}
					return a.writeValue("", map[string]any{"installations": projected})
				}
				writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
				_, _ = fmt.Fprintln(writer, "INSTALLATION	PLUGIN	VERSION	STATUS")
				for _, item := range items {
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%s\t%s\n",
						item.ID(),
						item.PluginID(),
						item.PackageVersion(),
						item.Status(),
					)
				}
				return writer.Flush()
			})
		},
	}
}

func (a *app) newPluginInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install <package-directory>",
		Short: "Install and verify a local Provider plugin package",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.InstallProviderPlugin(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf(
						"installed %s %s as %s",
						item.PluginID(),
						item.PackageVersion(),
						item.ID(),
					),
					map[string]any{"installation": pluginInstallationProjection(item)},
				)
			})
		},
	}
}

func (a *app) newPluginInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <installation>",
		Short: "Inspect one installed Provider plugin package",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := providerplugin.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.GetProviderPluginInstallation(cmd.Context(), id)
				if err != nil {
					return err
				}
				projected := pluginInstallationProjection(item)
				if a.output == "json" {
					return a.writeValue("", map[string]any{"installation": projected})
				}
				_, err = fmt.Fprintf(
					a.options.Stdout,
					"Installation: %s\nPlugin: %s\nVersion: %s\nStatus: %s\nDigest: %s\nSource: %s\n",
					item.ID(),
					item.PluginID(),
					item.PackageVersion(),
					item.Status(),
					item.Digest(),
					item.SourceReference(),
				)
				return err
			})
		},
	}
}

func (a *app) newPluginStatusCommand(enabled bool) *cobra.Command {
	name := "disable"
	short := "Disable an installed Provider plugin and close its active Providers"
	status := installationdomain.StatusDisabled
	if enabled {
		name = "enable"
		short = "Enable an installed Provider plugin"
		status = installationdomain.StatusEnabled
	}
	return &cobra.Command{
		Use:   name + " <installation>",
		Short: short,
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := providerplugin.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.SetProviderPluginInstallationStatus(
					cmd.Context(),
					id,
					status,
				)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("%s %s", name+"d", item.ID()),
					map[string]any{"installation": pluginInstallationProjection(item)},
				)
			})
		},
	}
}

func (a *app) newPluginRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <installation>",
		Short: "Remove an unreferenced external Provider plugin package",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := providerplugin.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				if err := runtime.RemoveProviderPluginInstallation(
					cmd.Context(),
					id,
				); err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("removed %s", id),
					map[string]any{"installationId": id.String()},
				)
			})
		},
	}
}

func pluginInstallationProjection(
	item installationdomain.Installation,
) map[string]any {
	return map[string]any{
		"installationId":  item.ID().String(),
		"pluginId":        item.PluginID().String(),
		"packageVersion":  item.PackageVersion(),
		"sourceKind":      string(item.SourceKind()),
		"sourceReference": item.SourceReference(),
		"digest":          item.Digest(),
		"status":          string(item.Status()),
		"installedAt":     item.InstalledAt(),
	}
}
