// Package delivery coordinates delivery of one explicit Profile snapshot.
// Skill content, trust, digests, drift and deployment records stay Skill-owned.
package delivery

import (
	"context"

	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapp "github.com/orz-i/Mender/backend/internal/skill/application"
	skill "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// ProfileSnapshot is the exact selection and revision read for one delivery.
// Skills is a detached selection; HasTools authorizes exporting connectivity
// metadata only, without granting Tool execution or proving endpoint readiness.
type ProfileSnapshot struct {
	ID       profile.ProfileID
	Slug     profile.Slug
	Revision uint64
	Skills   []skill.InstallationID
	HasTools bool
}

type Profiles interface {
	Snapshot(context.Context, profile.Slug) (ProfileSnapshot, error)
	Revision(context.Context, profile.ProfileID) (uint64, error)
}

// SkillStatus exposes only deployment admission metadata. It does not hand
// Profile delivery Skill content, filesystem handles or trust mutation rights.
type SkillStatus struct {
	Name    string
	Trusted bool
	Enabled bool
}

type SkillSource interface {
	GetDeployable(context.Context, skill.InstallationID) (SkillStatus, error)
}

type SkillDeployments interface {
	DeployForProfile(context.Context, skill.InstallationID, skill.DeploymentTarget, string, string, uint64) (skill.Deployment, error)
	Undeploy(context.Context, skill.DeploymentID) error
}

type DeploymentRecords interface {
	GetDeployment(context.Context, skill.DeploymentID) (skill.Deployment, error)
	ListOwnedDeployments(context.Context, string) ([]skillapp.DeploymentInspection, error)
}

type AgentPluginExporter interface {
	ExportAgentPlugin(context.Context, skillapp.ExportAgentPluginRequest) error
}

type LocalCLIContext struct {
	DatabasePath string
	Platform     string
}

// CLIEnvironment supplies host-specific database path resolution and platform
// metadata. It performs no Profile lookup and never selects a fallback scope.
type CLIEnvironment interface {
	LocalContext(context.Context) (LocalCLIContext, error)
}
