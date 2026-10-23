package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapp "github.com/orz-i/Mender/backend/internal/skill/application"
	"github.com/spf13/cobra"
)

// Export is a Profile delivery operation, not a free-form Skill Library
// operation. The Runtime derives all exact Skill IDs from this Profile.
func (a *app) newProfileExportPluginCommand() *cobra.Command {
	var name, version, description, outputDirectory, remoteURL string
	command := &cobra.Command{
		Use:   "export-plugin <profile>",
		Short: "Package this Profile as portable Agent Plugins 1.0",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := profiledomain.NewSlug(args[0])
			if err != nil {
				return usageError{err: err}
			}
			if strings.TrimSpace(name) == "" || strings.TrimSpace(outputDirectory) == "" {
				return usageError{err: fmt.Errorf("--name and --output-dir are required")}
			}
			destination, err := filepath.Abs(outputDirectory)
			if err != nil {
				return usageError{err: fmt.Errorf("resolve output directory: %w", err)}
			}
			request := skillapp.ExportAgentPluginRequest{
				Name: name, Version: version, Description: description, Destination: destination,
			}
			if remoteURL != "" {
				request.MCP = &skillapp.AgentPluginMCP{URL: remoteURL}
			}
			return a.withRuntime(cmd.Context(), func(runtime *bootstrap.Runtime) error {
				revision, err := runtime.ExportProfileAgentPlugin(cmd.Context(), slug, request)
				if err != nil {
					return err
				}
				return a.writeValue(
					fmt.Sprintf("exported Profile %s Agent Plugin to %s", slug, destination),
					map[string]any{
						"profile": slug.String(), "profileRevision": revision,
						"name": name, "version": version,
						"format": "agent-plugin", "outputDirectory": destination,
						"hasMCP": request.MCP != nil,
					},
				)
			})
		},
	}
	command.Flags().StringVar(&name, "name", "", "portable Agent Plugin name")
	command.Flags().StringVar(&version, "version", "", "Agent Plugin version")
	command.Flags().StringVar(&description, "description", "", "Agent Plugin description")
	command.Flags().StringVar(&outputDirectory, "output-dir", "", "new package directory; must not already exist")
	command.Flags().StringVar(&remoteURL, "mcp-url", "", "optional public/authenticated HTTPS MCP URL (never embed secrets)")
	_ = command.MarkFlagRequired("name")
	_ = command.MarkFlagRequired("output-dir")
	return command
}
