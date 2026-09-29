package application

import (
	"context"
	"time"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

// Repository persists managed Skill installation metadata.
type Repository interface {
	Create(context.Context, skilldomain.Installation) error
	Get(context.Context, skilldomain.InstallationID) (skilldomain.Installation, error)
	GetBySkillDigest(
		context.Context,
		skilldomain.SkillID,
		string,
	) (skilldomain.Installation, error)
	List(context.Context) ([]skilldomain.Installation, error)
	SetState(context.Context, skilldomain.InstallationID, bool, bool) error
	Delete(context.Context, skilldomain.InstallationID) error
}

// IDGenerator allocates exact installation identities.
type IDGenerator interface {
	NewInstallationID() (skilldomain.InstallationID, error)
}

// ManagedPackage is one validated Skill copied into Mender-managed storage.
type ManagedPackage struct {
	Snapshot        skilldomain.Snapshot
	SourceReference string
}

// PackageStore owns immutable managed Skill files.
type PackageStore interface {
	InstallLocal(
		context.Context,
		string,
		skilldomain.InstallationID,
	) (ManagedPackage, error)
	Resolve(
		context.Context,
		skilldomain.Installation,
	) (skilldomain.Snapshot, error)
	Remove(context.Context, skilldomain.InstallationID) error
}

// Usage describes one deployment/reference that blocks removal.
type Usage struct {
	Kind        string
	Description string
}

// UsageChecker reports references to one exact Skill installation.
type UsageChecker interface {
	Usages(context.Context, skilldomain.InstallationID) ([]Usage, error)
}

// DeployableInstallation is the minimum exact Skill metadata required to
// authorize and materialize a deployment. Implementations may resolve built-in
// or managed installations behind this port.
type DeployableInstallation struct {
	InstallationID skilldomain.InstallationID
	Name           string
	Digest         string
	Trusted        bool
	Enabled        bool
}

// DeploymentSource resolves and materializes exact Skill content.
type DeploymentSource interface {
	GetDeployable(
		context.Context,
		skilldomain.InstallationID,
	) (DeployableInstallation, error)
	Materialize(
		context.Context,
		skilldomain.InstallationID,
		string,
	) error
}

// DeploymentRepository persists exact deployment metadata.
type DeploymentRepository interface {
	CreateDeployment(context.Context, skilldomain.Deployment) error
	GetDeployment(
		context.Context,
		skilldomain.DeploymentID,
	) (skilldomain.Deployment, error)
	ListDeployments(context.Context) ([]skilldomain.Deployment, error)
	ListDeploymentsByInstallation(
		context.Context,
		skilldomain.InstallationID,
	) ([]skilldomain.Deployment, error)
	DeleteDeployment(context.Context, skilldomain.DeploymentID) error
}

// DeploymentFilesystem owns host-specific deployment path resolution,
// observation, and cleanup. It never grants Tool execution authority.
type DeploymentFilesystem interface {
	ResolveDestination(
		skilldomain.DeploymentTarget,
		string,
		string,
	) (string, error)
	Observe(context.Context, string) (bool, string, error)
	Remove(context.Context, string, string) error
}

// DeploymentIDGenerator allocates stable deployment identities.
type DeploymentIDGenerator interface {
	NewDeploymentID() (skilldomain.DeploymentID, error)
}

// AgentPluginSkill is one exact Skill selected for portable Agent Plugin
// export.
type AgentPluginSkill struct {
	InstallationID skilldomain.InstallationID
	Name           string
	Digest         string
}

// AgentPluginPackage is the portable manifest metadata and exact Skill set to
// export. Destination is a host filesystem directory that must not already
// contain a package.
type AgentPluginPackage struct {
	Name        string
	Version     string
	Description string
	Destination string
	Skills      []AgentPluginSkill
}

// AgentPluginExporter writes one portable Agent Plugins package.
type AgentPluginExporter interface {
	ExportAgentPlugin(context.Context, AgentPluginPackage) error
}

// Clock supplies installation/deployment timestamps.
type Clock interface {
	Now() time.Time
}
