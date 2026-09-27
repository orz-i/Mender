package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

func TestRuntimeExposesBuiltInProviderPluginDescriptors(t *testing.T) {
	t.Parallel()

	runtime := newProviderTestRuntime(t)
	descriptors := runtime.ProviderPluginDescriptors()
	if len(descriptors) != 1 {
		t.Fatalf("ProviderPluginDescriptors() len = %d, want 1", len(descriptors))
	}
	if descriptors[0].ID != mcpplugin.ID {
		t.Fatalf(
			"ProviderPluginDescriptors()[0].ID = %s, want %s",
			descriptors[0].ID,
			mcpplugin.ID,
		)
	}
	if descriptors[0].ConfigVersion != mcpplugin.ConfigVersion {
		t.Fatalf(
			"ProviderPluginDescriptors()[0].ConfigVersion = %d, want %d",
			descriptors[0].ConfigVersion,
			mcpplugin.ConfigVersion,
		)
	}
}

func TestRegisterAndUpdateConfiguredProviderPreservePluginIdentity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)

	stdioConfig := mustProviderConfigJSON(t, mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{
			Kind:       mcpplugin.TransportStdio,
			Executable: "mcp-server",
			Args:       []string{"--stdio"},
		},
	})
	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "generic",
			DisplayName: "Generic MCP",
		},
		mcpplugin.ID,
		stdioConfig,
	)
	if err != nil {
		t.Fatalf("RegisterConfiguredProvider() error = %v", err)
	}

	stored, err := runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatalf("ProviderConfig() error = %v", err)
	}
	if stored.PluginID != mcpplugin.ID {
		t.Fatalf("PluginID = %s, want %s", stored.PluginID, mcpplugin.ID)
	}
	firstRevision := stored.Revision

	httpConfig := mustProviderConfigJSON(t, mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{
			Kind:     mcpplugin.TransportStreamableHTTP,
			Endpoint: "https://example.test/mcp",
		},
	})
	updated, err := runtime.UpdateConfiguredProvider(
		ctx,
		provider.ID(),
		catalogapplication.UpdateProviderRequest{
			DisplayName: "Generic MCP edited",
			Description: "same plugin, different transport",
		},
		httpConfig,
	)
	if err != nil {
		t.Fatalf("UpdateConfiguredProvider() error = %v", err)
	}
	if updated.ID() != provider.ID() {
		t.Fatalf("ProviderID changed: %s -> %s", provider.ID(), updated.ID())
	}

	stored, err = runtime.ProviderConfig(ctx, provider.ID())
	if err != nil {
		t.Fatalf("ProviderConfig(after update) error = %v", err)
	}
	if stored.PluginID != mcpplugin.ID {
		t.Fatalf("PluginID after update = %s, want %s", stored.PluginID, mcpplugin.ID)
	}
	if stored.Revision == firstRevision {
		t.Fatal("provider config revision did not change")
	}
	decoded, err := mcpplugin.DecodeConfig(stored.Config)
	if err != nil {
		t.Fatalf("DecodeConfig() error = %v", err)
	}
	if decoded.Transport.Kind != mcpplugin.TransportStreamableHTTP {
		t.Fatalf(
			"transport kind = %q, want %q",
			decoded.Transport.Kind,
			mcpplugin.TransportStreamableHTTP,
		)
	}
}

func TestRegisterConfiguredProviderRejectsUnknownPlugin(t *testing.T) {
	t.Parallel()

	runtime := newProviderTestRuntime(t)
	unknown, err := providerplugin.ParseID("com.example.provider.unknown")
	if err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RegisterConfiguredProvider(
		context.Background(),
		catalogapplication.RegisterProviderRequest{
			Namespace:   "unknown",
			DisplayName: "Unknown",
		},
		unknown,
		json.RawMessage(`{"value":true}`),
	)
	if !errors.Is(err, providerplugin.ErrNotFound) {
		t.Fatalf("RegisterConfiguredProvider() error = %v, want ErrNotFound", err)
	}

	providers, listErr := runtime.Catalog.ListProviders.Execute(context.Background())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(providers) != 0 {
		t.Fatalf("providers = %+v, want none after rejected plugin", providers)
	}
}

func TestDeleteConfiguredProviderRemovesGenericState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newProviderTestRuntime(t)
	raw := mustProviderConfigJSON(t, mcpplugin.Config{
		Transport: mcpplugin.TransportConfig{
			Kind:       mcpplugin.TransportStdio,
			Executable: "mcp-server",
		},
	})
	provider, err := runtime.RegisterConfiguredProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "delete-me",
			DisplayName: "Delete Me",
		},
		mcpplugin.ID,
		raw,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := runtime.DeleteConfiguredProvider(ctx, provider.ID()); err != nil {
		t.Fatalf("DeleteConfiguredProvider() error = %v", err)
	}
	if _, err := runtime.ProviderConfig(ctx, provider.ID()); !errors.Is(err, providerconfigstore.ErrNotFound) {
		t.Fatalf("ProviderConfig(after delete) error = %v, want ErrNotFound", err)
	}
	if _, err := runtime.Catalog.GetProvider.Execute(ctx, provider.ID()); !errors.Is(
		err,
		catalogapplication.ErrProviderNotFound,
	) {
		t.Fatalf("GetProvider(after delete) error = %v, want ErrProviderNotFound", err)
	}
}

func newProviderTestRuntime(t *testing.T) *Runtime {
	t.Helper()

	runtime, err := NewRuntime(context.Background(), RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	return runtime
}

func mustProviderConfigJSON(t *testing.T, config mcpplugin.Config) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
