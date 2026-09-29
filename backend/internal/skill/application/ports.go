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

// Clock supplies installation timestamps.
type Clock interface {
	Now() time.Time
}
