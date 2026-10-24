package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapp "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skillapp "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func pluginTestTool(t *testing.T, r *Runtime, ctx context.Context) catalogdomain.ToolID {
	t.Helper()
	provider, err := r.Catalog.RegisterProvider.Execute(ctx,
		catalogapp.RegisterProviderRequest{Namespace: "pluginfixture", DisplayName: "Plugin Fixture"})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	id := mustBootstrapToolID(t, "tool_00000000000000000000000000000001")
	tool, err := catalogdomain.NewTool(id, provider.ID(), catalogdomain.ToolSpec{
		SourceKey: "plugin", Name: "plugin", InputSchema: schema,
	}, systemClock{}.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Catalog.store.ReplaceProviderTools(ctx, provider.ID(), []catalogdomain.Tool{tool}); err != nil {
		t.Fatal(err)
	}
	return tool.ID()
}

func addPluginTestTool(t *testing.T, r *Runtime, ctx context.Context,
	slug profiledomain.Slug, toolID catalogdomain.ToolID,
) {
	t.Helper()
	if _, err := r.Profiles.AddTool.Execute(ctx, profileapp.AddToolRequest{
		ProfileSlug: slug, ToolID: toolID, Direct: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExportProfileAgentPluginPinsExactSkillsAndOnlyExplicitHTTPSMCP(t *testing.T) {
	r, ctx, root, profile := deliveryFixture(t)
	available, err := r.Skills.List(ctx)
	if err != nil || len(available) == 0 {
		t.Fatalf("available Skills: %v %v", available, err)
	}
	builtin := available[0]
	toolID := pluginTestTool(t, r, ctx)
	profile = selectTestSkills(t, r, ctx, profile.Slug(), builtin.InstallationID)
	addPluginTestTool(t, r, ctx, profile.Slug(), toolID)
	profile, err = r.Profiles.Get.Execute(ctx, profile.Slug())
	if err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(root, "plugin-combined")
	other := skilldomain.InstallationID("sinst_00000000000000000000000000000009")
	revision, err := r.ExportProfileAgentPlugin(ctx, profile.Slug(), skillapp.ExportAgentPluginRequest{
		Name: "profile.bundle", Version: "1.0.0", Destination: output,
		InstallationIDs: []skilldomain.InstallationID{other}, // caller cannot inject IDs
		MCP:             &skillapp.AgentPluginMCP{URL: "https://mcp.example.com/mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if revision != profile.Revision() {
		t.Fatalf("export revision = %d, want %d", revision, profile.Revision())
	}
	if _, err := os.Stat(filepath.Join(output, "skills", builtin.Name, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	var mcp struct {
		Schema  string                       `json:"$schema"`
		Servers map[string]map[string]string `json:"mcpServers"`
	}
	//nolint:gosec // Read only from an isolated TempDir fixture.
	data, err := os.ReadFile(filepath.Join(output, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &mcp); err != nil {
		t.Fatal(err)
	}
	if mcp.Schema != "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json" ||
		mcp.Servers["profile.bundle"]["type"] != "streamable-http" ||
		mcp.Servers["profile.bundle"]["url"] != "https://mcp.example.com/mcp" {
		t.Fatalf("portable MCP configuration = %+v", mcp)
	}

	// Skills-only does not publish an MCP connection.
	skillsOnly := filepath.Join(root, "plugin-skills")
	if _, err := r.ExportProfileAgentPlugin(ctx, profile.Slug(), skillapp.ExportAgentPluginRequest{
		Name: "skills.only", Destination: skillsOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(skillsOnly, "mcp.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("skills-only exported unwanted MCP: %v", err)
	}

	// A Profile with Tools but no Skills can publish an MCP-only dependency.
	another, err := r.Profiles.Create.Execute(ctx, profileapp.CreateProfileRequest{
		Slug: "just-tools", DisplayName: "Tools", Mode: profiledomain.ExposureHybrid,
	})
	if err != nil {
		t.Fatal(err)
	}
	addPluginTestTool(t, r, ctx, another.Slug(), toolID)
	mcpOnly := filepath.Join(root, "plugin-mcp")
	if _, err := r.ExportProfileAgentPlugin(ctx, another.Slug(), skillapp.ExportAgentPluginRequest{
		Name: "mcp.only", Destination: mcpOnly,
		MCP: &skillapp.AgentPluginMCP{URL: "https://mcp.example.com/mcp"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mcpOnly, "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("MCP-only unexpectedly exported Skills: %v", err)
	}
}

func TestExportProfileAgentPluginRejectsUnsafeModeAndEmptyProfile(t *testing.T) {
	r, ctx, root, profile := deliveryFixture(t)
	dest := filepath.Join(root, "should-not-exist")
	if _, err := r.ExportProfileAgentPlugin(ctx, profile.Slug(), skillapp.ExportAgentPluginRequest{
		Name: "no.components", Destination: dest,
	}); err == nil {
		t.Fatal("empty Profile exported an empty Plugin")
	}
	if _, err := r.ExportProfileAgentPlugin(ctx, profile.Slug(), skillapp.ExportAgentPluginRequest{
		Name: "not.selected", Destination: dest,
		MCP: &skillapp.AgentPluginMCP{URL: "https://mcp.example.com/mcp"},
	}); err == nil {
		t.Fatal("Profile with no Tools exported MCP")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed export wrote directory: %v", err)
	}
}
