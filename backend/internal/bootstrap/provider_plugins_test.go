package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	mcpplugin "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/plugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

// Canonical Provider CRUD alone owns MCP plugin configuration. There is no
// adapter-specific config table, import path, or side-channel delete.
func TestMCPProviderCanonicalLifecycleIsSingleSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	for _, tc := range []struct {
		label  string
		config mcpplugin.Config
	}{
		{"http", mcpplugin.Config{Transport: mcpplugin.TransportConfig{Kind: mcpplugin.TransportStreamableHTTP, Endpoint: "https://example.test/mcp"}}},
		{"stdio", mcpplugin.Config{Transport: mcpplugin.TransportConfig{Kind: mcpplugin.TransportStdio, Executable: "server-v1"}}},
	} {
		raw, err := json.Marshal(tc.config)
		if err != nil {
			t.Fatal(err)
		}
		provider, err := registerConfiguredProviderTest(t, runtime, ctx, catalogapplication.RegisterProviderRequest{
			Namespace: "canonical-" + tc.label, DisplayName: "Canonical " + tc.label,
		}, mcpplugin.ID, raw)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := runtime.ProviderConfig(ctx, provider.ID())
		if err != nil {
			t.Fatal(err)
		}
		if stored.ConfigVersion != mcpplugin.ConfigVersion || stored.PluginID != mcpplugin.ID {
			t.Fatalf("MCP identity changed: %+v", stored)
		}
		next := tc.config
		if tc.label == "stdio" {
			next.Transport.Executable = "server-v2"
		} else {
			next.Transport.Endpoint = "https://example.test/new"
		}
		nextRaw, err := json.Marshal(next)
		if err != nil {
			t.Fatal(err)
		}
		_, err = updateConfiguredProviderTest(t, runtime, ctx, provider.ID(), catalogapplication.UpdateProviderRequest{
			DisplayName: "Updated " + tc.label,
		}, nextRaw)
		if err != nil {
			t.Fatal(err)
		}
		reloaded, err := runtime.ProviderConfig(ctx, provider.ID())
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := mcpplugin.DecodeConfig(reloaded.Config)
		if err != nil {
			t.Fatal(err)
		}
		if string(reloaded.Config) != string(nextRaw) {
			// canonical encoding can normalize ordering, so compare decoded data.
			gotJSON, err := json.Marshal(parsed)
			if err != nil {
				t.Fatal(err)
			}
			canonicalJSON, err := json.Marshal(next)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(canonicalJSON) {
				t.Fatalf("config mismatch: got=%s want=%s", gotJSON, canonicalJSON)
			}
		}
		if err := runtime.DeleteConfiguredProvider(ctx, provider.ID()); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.ProviderConfig(ctx, provider.ID()); !errors.Is(err, providerconfigstore.ErrNotFound) {
			t.Fatalf("canonical MCP deletion failed: %v", err)
		}
	}
	// Schema v4 tests assert retired tables are absent; the Runtime never recreates them.
}
