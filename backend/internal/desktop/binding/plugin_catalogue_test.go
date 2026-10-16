package binding

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

// Keep the Wails projection in sync with the exact typed Runtime catalogue:
// a newly introduced backend kind must not disappear from the desktop app.
func TestWailsPluginCatalogueExposesAllFourExtensionKinds(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "plugin-catalogue.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	service := NewService(ctx, runtime)
	result, err := service.ListPluginCatalogue()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	agents := map[string]bool{}
	for _, entry := range result.Plugins {
		kinds[entry.Kind] = true
		if entry.MarketplaceAction == "" {
			t.Fatalf("Wails catalogue omitted host-owned Marketplace eligibility: %+v", entry)
		}
		if entry.PluginID == "io.mender.provider.workspace" && entry.MarketplaceAction != "none" {
			t.Fatalf("private Workspace facade advertised as independent offering: %+v", entry)
		}
		if entry.Kind != "agent.backend" {
			continue
		}
		if entry.SourceKind != "builtin" || entry.ExtensionID != "main" ||
			entry.PackageVersion == "" || entry.UsageCount != 0 {
			t.Fatalf("invalid Wails Agent descriptor: %+v", entry)
		}
		if entry.Status != "enabled" && entry.Status != "unavailable" {
			t.Fatalf("invalid Wails Agent Backend status: %+v", entry)
		}
		if entry.Status == "unavailable" && (entry.Enabled || entry.Issue == "") {
			t.Fatalf("unavailable Agent Backend claims it is enabled: %+v", entry)
		}
		agents[entry.PluginID] = true
	}
	for _, kind := range []string{"provider.tools", "sandbox.backend", "tunnel.transport", "agent.backend"} {
		if !kinds[kind] {
			t.Fatalf("Wails catalogue omitted %s: %+v", kind, result.Plugins)
		}
	}
	for _, id := range []string{"io.mender.agent.codex", "io.mender.agent.grok", "io.mender.agent.opencode"} {
		if !agents[id] {
			t.Fatalf("Wails catalogue omitted built-in Agent %s", id)
		}
	}
	if len(agents) != 3 {
		t.Fatalf("unexpected Agent Backend inventory: %+v", agents)
	}
}
