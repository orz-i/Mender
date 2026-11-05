package cli

import (
	"fmt"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpdelivery "github.com/orz-i/Mender/backend/internal/mcp/application/delivery"
	mcp "github.com/orz-i/Mender/backend/internal/mcp/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunnel "github.com/orz-i/Mender/backend/internal/tunnel/domain"
	"github.com/spf13/cobra"
)

func (a *app) newProfileMCPDeliveryCommand() *cobra.Command {
	command := &cobra.Command{Use: "mcp-delivery", Short: "Inspect or configure a Profile's desired MCP delivery"}
	command.AddCommand(a.newProfileMCPDeliveryShowCommand(), a.newProfileMCPDeliveryConfigureCommand())
	return command
}

func (a *app) newProfileMCPDeliveryShowCommand() *cobra.Command {
	return &cobra.Command{
		Use: "show <profile>", Short: "Show desired MCP delivery without starting a server", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := profile.NewSlug(args[0])
			if err != nil {
				return usageError{err: err}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				snapshot, err := runtime.GetProfileMCPDelivery(cmd.Context(), slug)
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("%s MCP mode=%s revision=%d profileRevision=%d", slug, snapshot.Config.Mode(), snapshot.Config.Revision(), snapshot.Profile.Revision()), profileMCPDeliveryProjection(snapshot))
			})
		},
	}
}

func (a *app) newProfileMCPDeliveryConfigureCommand() *cobra.Command {
	var modeValue, tunnelValue string
	var directValues []string
	var profileRevision, revision uint64
	command := &cobra.Command{
		Use: "configure <profile>", Short: "Configure desired MCP delivery without starting a server", Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := profile.NewSlug(args[0])
			if err != nil {
				return usageError{err: err}
			}
			mode, err := parseMCPDeliveryMode(modeValue)
			if err != nil {
				return usageError{err: err}
			}
			var tunnelID tunnel.TunnelID
			if tunnelValue != "" {
				tunnelID, err = tunnel.ParseTunnelID(tunnelValue)
				if err != nil {
					return usageError{err: err}
				}
			}
			direct := make([]catalog.ToolID, 0, len(directValues))
			for _, raw := range directValues {
				id, err := catalog.ParseToolID(raw)
				if err != nil {
					return usageError{err: err}
				}
				direct = append(direct, id)
			}
			var expected *uint64
			if cmd.Flags().Changed("revision") {
				expected = &revision
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				snapshot, err := runtime.ConfigureProfileMCPDelivery(cmd.Context(), slug, mcpdelivery.ConfigureRequest{
					Mode: mode, TunnelID: tunnelID, DirectToolIDs: direct,
					ExpectedProfileRevision: profileRevision, ExpectedRevision: expected,
				})
				if err != nil {
					return err
				}
				return a.writeValue(fmt.Sprintf("configured MCP delivery for Profile %s", slug), profileMCPDeliveryProjection(snapshot))
			})
		},
	}
	command.Flags().StringVar(&modeValue, "mode", "", "direct|dynamic|hybrid")
	command.Flags().StringVar(&tunnelValue, "tunnel", "", "desired Tunnel ID; omit to clear")
	command.Flags().StringArrayVar(&directValues, "direct-tool", nil, "selected Tool ID to expose directly in hybrid mode; may be repeated")
	command.Flags().Uint64Var(&profileRevision, "profile-revision", 0, "current Profile selection revision (required)")
	command.Flags().Uint64Var(&revision, "revision", 0, "expected MCP delivery revision; omit to create only")
	_ = command.MarkFlagRequired("mode")
	_ = command.MarkFlagRequired("profile-revision")
	return command
}

func parseMCPDeliveryMode(value string) (mcp.Mode, error) {
	mode := mcp.Mode(value)
	switch mode {
	case mcp.ModeDirect, mcp.ModeDynamic, mcp.ModeHybrid:
		return mode, nil
	default:
		return "", fmt.Errorf("mode must be direct, dynamic, or hybrid")
	}
}

func profileMCPDeliveryProjection(snapshot bootstrap.ProfileMCPDelivery) map[string]any {
	direct := make([]string, 0, len(snapshot.Config.DirectToolIDs()))
	for _, id := range snapshot.Config.DirectToolIDs() {
		direct = append(direct, id.String())
	}
	result := map[string]any{
		"profileId": snapshot.Profile.ID().String(), "profileRevision": snapshot.Profile.Revision(),
		"revision": snapshot.Config.Revision(), "mode": string(snapshot.Config.Mode()), "directToolIds": direct,
	}
	if id, ok := snapshot.Config.TunnelID(); ok {
		result["tunnelId"] = id.String()
	}
	return result
}
