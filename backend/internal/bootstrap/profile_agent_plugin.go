package bootstrap

import (
	"context"
	"fmt"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
)

// ExportProfileAgentPlugin derives all portable package components from
// an explicit Profile. The caller cannot smuggle arbitrary Skill IDs into a
// package or read tools across Profile boundaries.
func (r *Runtime) ExportProfileAgentPlugin(
	ctx context.Context,
	slug profiledomain.Slug,
	request skillapplication.ExportAgentPluginRequest,
) (uint64, error) {
	profile, err := r.Profiles.Get.Execute(ctx, slug)
	if err != nil {
		return 0, fmt.Errorf("export Profile Agent Plugin: %w", err)
	}
	if request.MCP != nil && len(profile.ToolIDs()) == 0 {
		return 0, fmt.Errorf("export Profile Agent Plugin: Profile must select Tools to export MCP connectivity")
	}
	request.InstallationIDs = profile.Skills()
	if err := r.Skills.ExportAgentPlugin(ctx, request); err != nil {
		return 0, fmt.Errorf("export Profile Agent Plugin: %w", err)
	}
	// This revision denotes the snapshot exported; it is not a claim of
	// continuing package freshness or remote MCP endpoint readiness.
	return profile.Revision(), nil
}
