package domain

import (
	"fmt"
	"strings"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

// Binding identifies one exact Tunnel transport extension. A missing
// InstallationID is reserved for a host-trusted built-in extension.
type Binding struct {
	PluginID       platformplugin.ID
	ExtensionID    platformplugin.ExtensionID
	InstallationID platformplugin.InstallationID
}

func OpenAIBinding() Binding {
	return Binding{PluginID: "io.mender.tunnel.openai", ExtensionID: "main"}
}

func (b Binding) Validate() error {
	if _, err := platformplugin.ParseID(b.PluginID.String()); err != nil {
		return err
	}
	if _, err := platformplugin.ParseExtensionID(b.ExtensionID.String()); err != nil {
		return err
	}
	if b.InstallationID != "" {
		if _, err := platformplugin.ParseInstallationID(b.InstallationID.String()); err != nil {
			return err
		}
	}
	if b.ExtensionID != "main" {
		return fmt.Errorf("tunnel extension binding must target main")
	}
	return nil
}

func (b Binding) ValidateProvider(provider Provider) error {
	if err := b.Validate(); err != nil {
		return err
	}
	switch provider {
	case ProviderOpenAI:
		if b != OpenAIBinding() {
			return fmt.Errorf("openai Tunnel must use the trusted built-in transport")
		}
	case ProviderExternal:
		if b.InstallationID == "" || strings.HasPrefix(b.PluginID.String(), "io.mender.") {
			return fmt.Errorf("external Tunnel requires an exact third-party installation")
		}
	default:
		return fmt.Errorf("unsupported Tunnel provider %s", provider)
	}
	return nil
}
