package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	tunnelapp "github.com/orz-i/Mender/backend/internal/tunnel/application"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	"github.com/spf13/cobra"
)

func (a *app) newTunnelCommand() *cobra.Command {
	c := &cobra.Command{Use: "tunnel", Short: "Manage Tunnel Transport resources"}
	c.AddCommand(a.newTunnelListCommand())
	create := &cobra.Command{Use: "add", Short: "Create one governed reachability resource"}
	create.AddCommand(a.newTunnelAddExternalCommand())
	create.AddCommand(a.newTunnelAddOpenAICommand())
	c.AddCommand(create)
	c.AddCommand(a.newTunnelDeleteCommand())
	return c
}
func (a *app) newTunnelListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List configured Tunnels", Args: exactArgs(0), RunE: func(cmd *cobra.Command, _ []string) error {
		return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
			items, err := runtime.Tunnels.List.Execute(cmd.Context())
			if err != nil {
				return err
			}
			projected := make([]map[string]any, 0, len(items))
			for _, item := range items {
				projected = append(projected, tunnelProjectionCLI(item))
			}
			if a.output == "json" {
				return a.writeValue("", map[string]any{"tunnels": projected})
			}
			writer := tabwriter.NewWriter(a.options.Stdout, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(writer, "TUNNEL ID\tPROVIDER\tPLUGIN\tNAME")
			for _, item := range items {
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
					item.ID(), item.Provider(), item.Binding().PluginID, item.DisplayName())
			}
			return writer.Flush()
		})
	}}
}
func (a *app) newTunnelAddExternalCommand() *cobra.Command {
	var name, installation, externalID, credential string
	c := &cobra.Command{Use: "external", Short: "Create Tunnel using exact installed tunnel.transport package", Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || installation == "" || externalID == "" || credential == "" {
				return usageError{err: fmt.Errorf("--name, --installation, --external-id and --credential-ref are required")}
			}
			id, err := platformplugin.ParseInstallationID(installation)
			if err != nil {
				return usageError{err: err}
			}
			ref, err := credentialdomain.ParseReference(credential)
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Tunnels.CreateExternal(cmd.Context(), bootstrap.CreateExternalTunnelRequest{
					DisplayName: name, InstallationID: id, ExternalTunnelID: externalID, RuntimeCredentialRef: ref,
				})
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Tunnel %s created", item.ID()), map[string]any{"tunnel": tunnelProjectionCLI(item)})
			})
		},
	}
	c.Flags().StringVar(&name, "name", "", "Tunnel display name")
	c.Flags().StringVar(&installation, "installation", "", "exact installed tunnel.transport InstallationID")
	c.Flags().StringVar(&externalID, "external-id", "", "provider-defined remote transport ID")
	c.Flags().StringVar(&credential, "credential-ref", "", "existing Mender Credential Reference")
	return c
}
func (a *app) newTunnelAddOpenAICommand() *cobra.Command {
	var name, externalID, credential string
	c := &cobra.Command{Use: "openai", Short: "Create trusted OpenAI Secure MCP Tunnel", Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" || externalID == "" || credential == "" {
				return usageError{err: fmt.Errorf("--name, --external-id and --credential-ref required")}
			}
			ref, err := credentialdomain.ParseReference(credential)
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				item, err := runtime.Tunnels.Create.Execute(cmd.Context(), tunnelapp.CreateOpenAITunnelRequest{
					DisplayName: name, ExternalTunnelID: externalID, RuntimeCredentialRef: ref,
				})
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("Tunnel %s created", item.ID()), map[string]any{"tunnel": tunnelProjectionCLI(item)})
			})
		},
	}
	c.Flags().StringVar(&name, "name", "", "Tunnel display name")
	c.Flags().StringVar(&externalID, "external-id", "", "OpenAI control-plane tunnel ID")
	c.Flags().StringVar(&credential, "credential-ref", "", "OpenAI Tunnel Credential Reference")
	return c
}
func (a *app) newTunnelDeleteCommand() *cobra.Command {
	return &cobra.Command{Use: "delete <tunnel-id>", Short: "Delete an unreferenced Tunnel", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := tunneldomain.ParseTunnelID(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				if err := runtime.Tunnels.Delete.Execute(cmd.Context(), id); err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("deleted Tunnel %s", id), map[string]any{"tunnelId": id.String()})
			})
		}}
}
func tunnelProjectionCLI(item tunneldomain.Tunnel) map[string]any {
	return map[string]any{
		"tunnelId": item.ID().String(), "displayName": item.DisplayName(),
		"provider": string(item.Provider()), "externalTunnelId": item.ExternalTunnelID(),
		"runtimeCredentialRef": item.RuntimeCredentialRef().String(),
		"pluginId":             item.Binding().PluginID.String(), "extensionId": item.Binding().ExtensionID.String(),
		"installationId": item.Binding().InstallationID.String(),
	}
}
