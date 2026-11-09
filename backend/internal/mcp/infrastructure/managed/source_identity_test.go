package managed

import (
	"context"
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestManagedMCPMetadataFingerprintFencesExactSourceRef(t *testing.T) {
	ctx := context.Background()
	id, err := catalog.ParseToolID("tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := catalog.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 11, 8, 9, 0, 0, 0, time.UTC)
	makeTool := func(source catalog.SourceRef) catalog.Tool {
		t.Helper()
		tool, e := catalog.NewToolFromSource(id, source, catalog.ToolSpec{
			SourceKey: "agent.status", Name: "status", Title: "Agent status",
			InputSchema: schema, Description: "Read task status",
		}, when)
		if e != nil {
			t.Fatal(e)
		}
		return tool
	}
	firstSource, _ := catalog.AgentSource("agent_11111111111111111111111111111111")
	secondSource, _ := catalog.AgentSource("agent_22222222222222222222222222222222")
	first := makeTool(firstSource)
	second := makeTool(secondSource)
	profileID, _ := profile.ParseProfileID("prof_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	slug, _ := profile.NewSlug("typed-refresh")
	binding, err := profile.NewToolBinding(profile.ToolBindingSpec{ToolID: id})
	if err != nil {
		t.Fatal(err)
	}
	p, err := profile.NewProfileWithBindings(profileID, slug, "Typed refresh", []profile.ToolBinding{binding})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{Profile: p}
	current := first
	manager := &Manager{deps: Dependencies{DescribeTool: func(context.Context, catalog.ToolID) (catalog.Tool, error) { return current, nil }}}
	firstDigest, err := manager.fingerprint(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	current = second
	secondDigest, err := manager.fingerprint(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatal("same ToolID metadata under different Agent SourceRef produced the same Managed MCP identity fingerprint")
	}
	current = first
	again, err := manager.fingerprint(ctx, snapshot)
	if err != nil || again != firstDigest {
		t.Fatalf("same source identity should be stable: %v %s != %s", err, again, firstDigest)
	}
	current, err = catalog.NewToolFromSource("tool_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", firstSource,
		catalog.ToolSpec{SourceKey: "agent.status", Name: "status", InputSchema: schema}, when)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.fingerprint(ctx, snapshot); err == nil {
		t.Fatal("Managed MCP accepted Tool lookup with a mismatched canonical ToolID")
	}
}
