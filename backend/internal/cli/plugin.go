package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	pluginmanifest "github.com/orz-i/Mender/backend/internal/plugin/manifest"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	"github.com/spf13/cobra"
)

func (a *app) newPluginCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "plugin",
		Short: "Manage typed Mender plugin packages",
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
		Short: "List installed external plugins",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				items, err := runtime.ListPluginInstallations(cmd.Context())
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
				_, _ = fmt.Fprintln(writer, "INSTALLATION	PLUGIN	KIND	VERSION	STATUS")
				for _, item := range items {
					_, _ = fmt.Fprintf(
						writer,
						"%s\t%s\t%s\t%s\t%s\n",
						item.ID(), item.PluginID(), item.Extension().Kind,
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
		Short: "Install and verify a local typed plugin package",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				raw, readErr := os.ReadFile(filepath.Join(args[0], pluginmanifest.FileName)) // #nosec G304 -- user-selected local package directory is separately validated by installer.
				if readErr != nil {
					return fmt.Errorf("read plugin manifest: %w", readErr)
				}
				manifest, parseErr := pluginmanifest.Parse(raw)
				if parseErr != nil {
					return parseErr
				}
				var item installationdomain.Installation
				var err error
				switch manifest.Extensions[0].Kind {
				case platformplugin.ProviderTools:
					item, err = runtime.InstallProviderPlugin(cmd.Context(), args[0])
				case platformplugin.SandboxBackend:
					item, err = runtime.InstallSandboxPlugin(cmd.Context(), args[0])
				case platformplugin.TunnelTransport:
					item, err = runtime.InstallTunnelPlugin(cmd.Context(), args[0])
				default:
					return fmt.Errorf("plugin extension %s not supported yet", manifest.Extensions[0].Kind)
				}
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
		Short: "Inspect one installed plugin package",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := providerplugin.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.GetPluginInstallation(cmd.Context(), id)
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
	short := "Disable an unused plugin installation"
	status := installationdomain.StatusDisabled
	if enabled {
		name = "enable"
		short = "Enable an installed plugin"
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
				current, lookupErr := runtime.GetPluginInstallation(cmd.Context(), id)
				if lookupErr != nil {
					return lookupErr
				}
				var item installationdomain.Installation
				var err error
				switch current.Extension().Kind {
				case platformplugin.ProviderTools:
					item, err = runtime.SetProviderPluginInstallationStatus(cmd.Context(), id, status)
				case platformplugin.SandboxBackend:
					item, err = runtime.SetSandboxPluginInstallationStatus(cmd.Context(), id, status)
				case platformplugin.TunnelTransport:
					item, err = runtime.SetTunnelPluginInstallationStatus(cmd.Context(), id, status)
				default:
					return fmt.Errorf("plugin kind %s cannot be enabled or disabled", current.Extension().Kind)
				}
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
		Short: "Remove an unreferenced external plugin package",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := providerplugin.ParseInstallationID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				current, lookupErr := runtime.GetPluginInstallation(cmd.Context(), id)
				if lookupErr != nil {
					return lookupErr
				}
				var err error
				switch current.Extension().Kind {
				case platformplugin.ProviderTools:
					err = runtime.RemoveProviderPluginInstallation(cmd.Context(), id)
				case platformplugin.SandboxBackend:
					err = runtime.RemoveSandboxPluginInstallation(cmd.Context(), id)
				case platformplugin.TunnelTransport:
					err = runtime.RemoveTunnelPluginInstallation(cmd.Context(), id)
				default:
					return fmt.Errorf("unsupported plugin removal kind %s", current.Extension().Kind)
				}
				if err != nil {
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
		"kind":            string(item.Extension().Kind),
		"packageVersion":  item.PackageVersion(),
		"sourceKind":      string(item.SourceKind()),
		"sourceReference": item.SourceReference(),
		"digest":          item.Digest(),
		"status":          string(item.Status()),
		"installedAt":     item.InstalledAt(),
	}
}
