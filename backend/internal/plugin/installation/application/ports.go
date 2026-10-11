package application

import (
	"context"
	"time"

	installationdomain "github.com/orz-i/Mender/backend/internal/plugin/installation/domain"
	"github.com/orz-i/Mender/backend/internal/plugin/manifest"
)

// Repository persists plugin installation metadata.
type Repository interface {
	Create(context.Context, installationdomain.Installation) error
	Get(context.Context, installationdomain.InstallationID) (installationdomain.Installation, error)
	GetByDigest(context.Context, string) (installationdomain.Installation, error)
	List(context.Context) ([]installationdomain.Installation, error)
	SetStatus(context.Context, installationdomain.InstallationID, installationdomain.Status) error
	Delete(context.Context, installationdomain.InstallationID) error
}

// IDGenerator allocates stable installation identities.
type IDGenerator interface {
	NewInstallationID() (installationdomain.InstallationID, error)
}

// ManagedPackage is one validated package copied into Mender-managed storage.
type ManagedPackage struct {
	Manifest        manifest.Manifest
	ManifestJSON    []byte
	Digest          string
	SourceReference string
}

// ResolvedPackage is a verified immutable package ready for process launch.
type ResolvedPackage struct {
	Manifest           manifest.Manifest
	EntrypointPath     string
	WorkingDirectory   string
	ExecutableChecksum []byte
}

// PackageStore owns managed plugin package files.
type PackageStore interface {
	InstallLocal(
		context.Context,
		string,
		installationdomain.InstallationID,
	) (ManagedPackage, error)
	Resolve(context.Context, installationdomain.Installation) (ResolvedPackage, error)
	Remove(context.Context, installationdomain.InstallationID) error
}

// Usage describes one reference that blocks uninstall.
type Usage struct {
	Kind        string
	Description string
}

// UsageChecker reports configured Provider or runtime references to an installation.
type UsageChecker interface {
	Usages(context.Context, installationdomain.InstallationID) ([]Usage, error)
}

// Clock supplies installation timestamps.
type Clock interface {
	Now() time.Time
}
