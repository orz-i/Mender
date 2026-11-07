package curated

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestPlannedNativeCatalogExactHostApprovedSchemas(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	tools, err := PlannedNativeCatalogTools(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 9 {
		t.Fatalf("expected 9 native tools, got %d", len(tools))
	}
	expected, _ := PlannedNativeTools()
	for i, tool := range tools {
		entry := expected[i]
		if tool.ID() != entry.ToolID || tool.SourceRef() != entry.Source ||
			tool.SourceRef().Kind() != catalog.SourceNative || tool.SourceKey() != entry.SourceKey ||
			tool.ProviderID() != "" || tool.CreatedAt() != now || tool.UpdatedAt() != now {
			t.Fatalf("pinned provenance or timestamp changed for %s", entry.SourceKey)
		}
		if tool.Name() == "" || tool.Title() == "" || tool.OutputSchema() == nil {
			t.Fatalf("missing native tool metadata: %s", tool.SourceKey())
		}
		var input struct {
			Type                 string                     `json:"type"`
			AdditionalProperties bool                       `json:"additionalProperties"`
			Required             []string                   `json:"required"`
			Properties           map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema().Bytes(), &input); err != nil {
			t.Fatal(err)
		}
		if input.Type != "object" || input.AdditionalProperties || len(input.Required) == 0 {
			t.Fatalf("unbounded native input for %s: %+v", tool.SourceKey(), input)
		}
		if _, legacyAlias := input.Properties["workspace"]; legacyAlias {
			t.Fatalf("native Tool retained Provider-specific workspace alias: %s", tool.SourceKey())
		}
		if _, legacyProviderID := input.Properties["providerId"]; legacyProviderID {
			t.Fatalf("native Tool exposes internal Provider ID: %s", tool.SourceKey())
		}
		if strings.HasPrefix(tool.SourceKey(), "workspace.") {
			if _, ok := input.Properties["workspaceId"]; !ok {
				t.Fatalf("native Workspace has no requested workspace identity: %s", tool.SourceKey())
			}
			if _, ok := input.Properties["sandboxId"]; ok {
				t.Fatalf("Workspace Tool accepts raw Sandbox selection: %s", tool.SourceKey())
			}
		}
		if strings.HasPrefix(tool.SourceKey(), "jobs.") {
			if _, ok := input.Properties["sandboxId"]; !ok {
				t.Fatalf("native SSH Jobs has no explicit target identity: %s", tool.SourceKey())
			}
			if !reflect.DeepEqual(input.Required[:1], []string{"sandboxId"}) {
				t.Fatalf("native Job tools do not require target first: %s: %v", tool.SourceKey(), input.Required)
			}
			if tool.SourceKey() != "jobs.submit" {
				if _, ok := input.Properties["taskId"]; !ok {
					t.Fatalf("Job control has no Task ID: %s", tool.SourceKey())
				}
			}
		}
		var output any
		if err := json.Unmarshal(tool.OutputSchema().Bytes(), &output); err != nil {
			t.Fatalf("invalid output schema: %s: %v", tool.SourceKey(), err)
		}
	}
	second, err := PlannedNativeCatalogTools(now)
	if err != nil {
		t.Fatal(err)
	}
	for i := range tools {
		if second[i].ID() != tools[i].ID() ||
			!reflect.DeepEqual(second[i].InputSchema().Bytes(), tools[i].InputSchema().Bytes()) {
			t.Fatal("native catalog generation is not deterministic")
		}
	}
}

func TestNativeCatalogHasNoHiddenAgentOrBrowserExecution(t *testing.T) {
	tools, err := PlannedNativeCatalogTools(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if strings.HasPrefix(tool.SourceKey(), "agent.") ||
			strings.HasPrefix(tool.SourceKey(), "browser.") {
			t.Fatalf("unverified Agent/Browser operation entered native tool registry: %s", tool.SourceKey())
		}
	}
	if _, err := PlannedNativeCatalogTools(time.Time{}); err == nil {
		t.Fatal("native tools accepted zero timestamp")
	}
}
