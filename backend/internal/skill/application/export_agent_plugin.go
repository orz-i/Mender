package application

import (
	"context"
	"fmt"
	"slices"
	"strings"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// ExportAgentPlugin validates exact Skill selection/trust and delegates
// portable Agent Plugins package writing to an infrastructure adapter.
type ExportAgentPlugin struct {
	source   DeploymentSource
	exporter AgentPluginExporter
}

func NewExportAgentPlugin(
	source DeploymentSource,
	exporter AgentPluginExporter,
) ExportAgentPlugin {
	return ExportAgentPlugin{
		source:   source,
		exporter: exporter,
	}
}

type ExportAgentPluginRequest struct {
	Name            string
	Version         string
	Description     string
	Destination     string
	InstallationIDs []skilldomain.InstallationID
}

func (uc ExportAgentPlugin) Execute(
	ctx context.Context,
	request ExportAgentPluginRequest,
) error {
	if strings.TrimSpace(request.Name) == "" {
		return fmt.Errorf("export agent plugin: name must not be empty")
	}
	if strings.TrimSpace(request.Destination) == "" {
		return fmt.Errorf("export agent plugin: destination must not be empty")
	}
	if len(request.InstallationIDs) == 0 {
		return fmt.Errorf("export agent plugin: at least one Skill installation is required")
	}

	seenIDs := make(map[skilldomain.InstallationID]struct{}, len(request.InstallationIDs))
	seenNames := make(map[string]skilldomain.InstallationID, len(request.InstallationIDs))
	skills := make([]AgentPluginSkill, 0, len(request.InstallationIDs))
	for _, id := range request.InstallationIDs {
		if _, duplicate := seenIDs[id]; duplicate {
			return fmt.Errorf("export agent plugin: duplicate installation %s", id)
		}
		seenIDs[id] = struct{}{}

		item, err := uc.source.GetDeployable(ctx, id)
		if err != nil {
			return fmt.Errorf("export agent plugin: resolve installation %s: %w", id, err)
		}
		if !item.Trusted {
			return fmt.Errorf(
				"export agent plugin: %w: %s",
				ErrInstallationUntrusted,
				id,
			)
		}
		if !item.Enabled {
			return fmt.Errorf(
				"export agent plugin: %w: %s",
				ErrInstallationDisabled,
				id,
			)
		}
		if existing, duplicate := seenNames[item.Name]; duplicate {
			return fmt.Errorf(
				"export agent plugin: Skill name %q is selected by both %s and %s",
				item.Name,
				existing,
				id,
			)
		}
		seenNames[item.Name] = id
		skills = append(skills, AgentPluginSkill{
			InstallationID: id,
			Name:           item.Name,
			Digest:         item.Digest,
		})
	}
	slices.SortFunc(skills, func(a, b AgentPluginSkill) int {
		if comparison := strings.Compare(a.Name, b.Name); comparison != 0 {
			return comparison
		}
		return strings.Compare(a.InstallationID.String(), b.InstallationID.String())
	})

	if err := uc.exporter.ExportAgentPlugin(ctx, AgentPluginPackage{
		Name:        strings.TrimSpace(request.Name),
		Version:     strings.TrimSpace(request.Version),
		Description: strings.TrimSpace(request.Description),
		Destination: request.Destination,
		Skills:      skills,
	}); err != nil {
		return fmt.Errorf("export agent plugin: %w", err)
	}
	return nil
}
