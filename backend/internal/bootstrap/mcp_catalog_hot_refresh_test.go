package bootstrap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	catalogsqlite "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sqlite"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestManagedMCPHTTPRefreshesCatalogMetadataWithoutChangingProfile(t *testing.T) {
	for _, mode := range []profiledomain.ExposureMode{profiledomain.ExposureDirect, profiledomain.ExposureDynamic, profiledomain.ExposureHybrid} {
		t.Run(string(mode), func(t *testing.T) {
			f := newLiveMCPFixture(t, mode)
			ctx := context.Background()
			pid := mustHotRefreshProvider(t, f)
			beforeProfile, err := f.runtime.Profiles.Get.Execute(ctx, f.profile.Slug())
			if err != nil {
				t.Fatal(err)
			}
			if mode != profiledomain.ExposureDirect {
				initialDesc := readDescribedTool(t, f, f.ids["echo_alpha"])
				if initialDesc["title"] != "echo_alpha" {
					t.Fatalf("unexpected initial title: %#v", initialDesc)
				}
			}
			// A separate SQLite writer simulates an external CLI/process syncing
			// the Tool source while the already-connected HTTP MCP server runs.
			writer, err := catalogsqlite.Open(ctx, f.dbPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			tools, err := writer.ListProviderTools(ctx, pid)
			if err != nil {
				t.Fatal(err)
			}
			var changed []catalogdomain.Tool
			for _, tool := range tools {
				if tool.ID() != f.ids["echo_alpha"] {
					changed = append(changed, tool)
					continue
				}
				input, err := catalogdomain.NewSchema([]byte(`{"type":"object","properties":{"fresh":{"type":"string"}}}`))
				if err != nil {
					t.Fatal(err)
				}
				output, err := catalogdomain.NewSchema([]byte(`{"type":"object","properties":{"confirmed":{"type":"boolean"}}}`))
				if err != nil {
					t.Fatal(err)
				}
				next, err := tool.RefreshMetadata(catalogdomain.ToolSpec{
					SourceKey: tool.SourceKey(), Name: tool.Name(), Title: "Fresh Search Tool",
					Description: "Catalog metadata updated while MCP remains connected",
					InputSchema: input, OutputSchema: &output, Tags: append(tool.Tags(), "refreshed"),
				}, time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				changed = append(changed, next)
			}
			if err := writer.ReplaceProviderTools(ctx, pid, changed); err != nil {
				t.Fatal(err)
			}
			profileAfter, err := f.runtime.Profiles.Get.Execute(ctx, f.profile.Slug())
			if err != nil || profileAfter.Revision() != beforeProfile.Revision() {
				t.Fatalf("Profile unexpectedly changed: %v / %v", profileAfter.Revision(), err)
			}
			if mode != profiledomain.ExposureDirect {
				actual := readDescribedTool(t, f, f.ids["echo_alpha"])
				if actual["title"] != "Fresh Search Tool" || actual["description"] != "Catalog metadata updated while MCP remains connected" {
					t.Fatalf("stale MCP describe: %#v", actual)
				}
				input, ok := actual["inputSchema"].(map[string]any)
				if !ok {
					t.Fatalf("missing updated input: %#v", actual)
				}
				fields, _ := input["properties"].(map[string]any)
				if _, ok := fields["fresh"]; !ok {
					t.Fatalf("stale input Schema: %#v", actual)
				}
				output, ok := actual["outputSchema"].(map[string]any)
				if !ok {
					t.Fatalf("missing updated output Schema: %#v", actual)
				}
				outputFields, _ := output["properties"].(map[string]any)
				if _, ok := outputFields["confirmed"]; !ok {
					t.Fatalf("stale output Schema: %#v", actual)
				}
				tags, ok := actual["tags"].([]any)
				if !ok {
					t.Fatalf("missing refreshed Tool tags: %#v", actual)
				}
				var tagFound bool
				for _, tag := range tags {
					if tag == "refreshed" {
						tagFound = true
					}
				}
				if !tagFound {
					t.Fatalf("stale tags: %#v", actual)
				}
			}
			if mode != profiledomain.ExposureDynamic {
				listed, err := f.session.ListTools(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				var found bool
				for _, tool := range listed.Tools {
					if tool.Title == "[Upstream] Fresh Search Tool" || tool.Title == "Fresh Search Tool" {
						found = true
					}
				}
				if !found {
					t.Fatalf("direct/hybrid tools/list retained old Tool title: %+v", listed.Tools)
				}
			}
			f.assertStableListener(t)
		})
	}
}
func readDescribedTool(t *testing.T, f *liveMCPFixture, id catalogdomain.ToolID) map[string]any {
	t.Helper()
	args, _ := json.Marshal(map[string]string{"toolId": id.String()})
	out, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "mender_describe_tool", Arguments: json.RawMessage(args)})
	if err != nil || out.IsError {
		t.Fatalf("describe failure: %+v / %v", out, err)
	}
	result, ok := out.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("malformed describe: %#v", out.StructuredContent)
	}
	return result
}
func mustHotRefreshProvider(t *testing.T, f *liveMCPFixture) catalogdomain.ProviderID {
	t.Helper()
	tool, err := f.runtime.Catalog.DescribeTool.Execute(context.Background(), f.ids["echo_alpha"])
	if err != nil {
		t.Fatal(err)
	}
	return tool.ProviderID()
}
