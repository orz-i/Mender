package domain

import (
	"strings"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

func TestTunnelTypedBindingValidation(t *testing.T) {
	t.Parallel()
	if err := OpenAIBinding().ValidateProvider(ProviderOpenAI); err != nil {
		t.Fatal(err)
	}
	external := Binding{PluginID: "dev.example.transport", ExtensionID: "main",
		InstallationID: platformplugin.InstallationID("pinst_" + strings.Repeat("a", 32))}
	if err := external.ValidateProvider(ProviderExternal); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		provider Provider
		binding  Binding
	}{
		{ProviderExternal, OpenAIBinding()}, {ProviderOpenAI, external},
		{ProviderExternal, Binding{PluginID: "dev.example.transport", ExtensionID: "main"}},
		{ProviderExternal, Binding{PluginID: "dev.example.transport", ExtensionID: "wrong", InstallationID: external.InstallationID}},
		{Provider("unknown"), OpenAIBinding()},
	} {
		if err := tc.binding.ValidateProvider(tc.provider); err == nil {
			t.Fatalf("invalid provider/binding accepted: %s %+v", tc.provider, tc.binding)
		}
	}
}
