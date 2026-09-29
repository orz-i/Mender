package application

import "errors"

var (
	ErrInstallationNotFound    = errors.New("skill installation not found")
	ErrInstallationConflict    = errors.New("skill installation conflict")
	ErrInstallationInUse       = errors.New("skill installation is in use")
	ErrPackageAlreadyInstalled = errors.New("skill package is already installed")
)
