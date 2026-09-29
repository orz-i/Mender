package application

import "errors"

var (
	ErrInstallationNotFound      = errors.New("skill installation not found")
	ErrInstallationConflict      = errors.New("skill installation conflict")
	ErrInstallationInUse         = errors.New("skill installation is in use")
	ErrPackageAlreadyInstalled   = errors.New("skill package is already installed")
	ErrInstallationUntrusted     = errors.New("skill installation is not trusted")
	ErrInstallationDisabled      = errors.New("skill installation is disabled")
	ErrDeploymentNotFound        = errors.New("skill deployment not found")
	ErrDeploymentConflict        = errors.New("skill deployment conflict")
	ErrDeploymentTargetExists    = errors.New("skill deployment target already exists")
	ErrInvalidAgentPluginPackage = errors.New("invalid Agent Plugin package")
	ErrAgentPluginTargetExists   = errors.New("agent plugin export target already exists")
)
