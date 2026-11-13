package domain

import "errors"

var (
	// ErrInvalidSignature indicates Ed25519 signature verification failure.
	ErrInvalidSignature = errors.New("registry: invalid signature")

	// ErrNoTrustedSigner indicates that no signatures matched the configured trust store.
	ErrNoTrustedSigner = errors.New("registry: no trusted signer found")

	// ErrAntiFreeze indicates that the metadata index is stale or expired.
	ErrAntiFreeze = errors.New("registry: anti-freeze validation failed")

	// ErrAntiRollback indicates that the metadata index sequence number went backward.
	ErrAntiRollback = errors.New("registry: anti-rollback validation failed")

	// ErrInvalidIndex indicates that the catalog index metadata structure is malformed.
	ErrInvalidIndex = errors.New("registry: invalid catalog index")

	// ErrTargetNotFound indicates no compatible artifact target exists for the host platform.
	ErrTargetNotFound = errors.New("registry: artifact target not found for platform")

	// ErrPluginNotFound indicates the requested plugin identity is not in the catalog.
	ErrPluginNotFound = errors.New("registry: plugin not found in catalog")

	// ErrInstallationInUse indicates active resource references block an in-place upgrade.
	ErrInstallationInUse = errors.New("registry: plugin is currently in use and cannot be upgraded in-place")
)
