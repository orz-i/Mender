package domain

import (
	"fmt"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

// BackendBinding is the immutable software identity of the exact extension
// selected by trusted Sandbox management. BackendKind remains resource metadata.
// Empty InstallationID denotes a statically linked, host-trusted built-in.
type BackendBinding struct {
	PluginID       platformplugin.ID
	ExtensionID    platformplugin.ExtensionID
	InstallationID platformplugin.InstallationID
}

func (b BackendBinding) Validate() error {
	if _, err := platformplugin.ParseID(b.PluginID.String()); err != nil {
		return fmt.Errorf("sandbox backend binding plugin: %w", err)
	}
	if _, err := platformplugin.ParseExtensionID(b.ExtensionID.String()); err != nil {
		return fmt.Errorf("sandbox backend binding extension: %w", err)
	}
	if b.InstallationID != "" {
		if _, err := platformplugin.ParseInstallationID(b.InstallationID.String()); err != nil {
			return fmt.Errorf("sandbox backend binding installation: %w", err)
		}
	}
	return nil
}

// BuiltinBinding explicitly selects one trusted, shipped Sandbox backend.
// It is not an implicit resource loading fallback: callers must provide it.
func BuiltinBinding(kind BackendKind) BackendBinding {
	switch kind {
	case BackendLocalWorkspace:
		return BackendBinding{PluginID: "io.mender.sandbox.local-workspace", ExtensionID: "main"}
	case BackendDocker:
		return BackendBinding{PluginID: "io.mender.sandbox.docker", ExtensionID: "main"}
	case BackendDaytona:
		return BackendBinding{PluginID: "io.mender.sandbox.daytona", ExtensionID: "main"}
	case BackendSSHRemote:
		return BackendBinding{PluginID: "io.mender.sandbox.ssh-remote", ExtensionID: "main"}
	default:
		return BackendBinding{}
	}
}

// RequireBuiltinBinding rejects an installation pin or a different plugin
// hidden behind a familiar built-in backend kind.
func RequireBuiltinBinding(kind BackendKind, binding BackendBinding) error {
	expected := BuiltinBinding(kind)
	if expected.PluginID == "" {
		return fmt.Errorf("sandbox backend %s has no implemented built-in extension", kind)
	}
	if binding != expected {
		return fmt.Errorf("sandbox backend %s must use exactly %s/%s without external installation", kind, expected.PluginID, expected.ExtensionID)
	}
	return nil
}
