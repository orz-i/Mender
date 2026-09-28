package application

import "errors"

var (
	// ErrInstallationNotFound indicates no installation exists for the requested identity.
	ErrInstallationNotFound = errors.New("provider plugin installation not found")
	// ErrInstallationConflict indicates an InstallationID or immutable package digest already exists.
	ErrInstallationConflict = errors.New("provider plugin installation conflict")
	// ErrInstallationInUse prevents destructive removal while Providers reference an installation.
	ErrInstallationInUse = errors.New("provider plugin installation is in use")
	// ErrPackageAlreadyInstalled indicates the exact package digest is already installed.
	ErrPackageAlreadyInstalled = errors.New("provider plugin package is already installed")
)
