package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

func TestCanonicalPluginCatalogueAllTypesInertAndStable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r, err := NewRuntime(ctx, RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "mender.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	first, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.ListPluginCatalogue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) || len(first) < 6 {
		t.Fatalf("missing typed built-ins: %+v", first)
	}
	seen := map[platformplugin.Kind]int{}
	keys := map[string]bool{}
	for _, plugin := range first {
		seen[plugin.Kind]++
		if keys[plugin.Key] {
			t.Fatalf("duplicate catalogue key %s", plugin.Key)
		}
		keys[plugin.Key] = true
		if plugin.InstalledAt != "" || plugin.InstallationID != "" || plugin.SourceKind != "builtin" ||
			!plugin.Enabled || plugin.Status != "enabled" || plugin.UsageCount != 0 {
			t.Fatalf("untrusted process or fake installation on builtin: %+v", plugin)
		}
	}
	for _, kind := range []platformplugin.Kind{platformplugin.ProviderTools, platformplugin.SandboxBackend, platformplugin.TunnelTransport} {
		if seen[kind] == 0 {
			t.Fatalf("catalogue lacks extension kind %s", kind)
		}
	}
	if len(r.ListMCPServers()) != 0 {
		t.Fatal("catalogue started MCP server")
	}
}
