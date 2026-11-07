package cli

import (
	"testing"
	"time"

	catalogapp "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestCLIJsonToolSourceIdentityIsNotASandboxProvider(t *testing.T) {
	for _, tt := range []struct {
		kind     catalog.SourceKind
		id       string
		provider bool
	}{
		{catalog.SourceProvider, "prov_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{catalog.SourceAgent, "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{catalog.SourceNative, "workspace", false},
	} {
		source, err := catalog.NewSourceRef(tt.kind, tt.id)
		if err != nil {
			t.Fatal(err)
		}
		id, err := catalog.ParseToolID("tool_11111111111111111111111111111111")
		if err != nil {
			t.Fatal(err)
		}
		schema, err := catalog.NewSchema([]byte("{\"type\":\"object\"}"))
		if err != nil {
			t.Fatal(err)
		}
		tool, err := catalog.NewToolFromSource(id, source, catalog.ToolSpec{
			SourceKey: "test.operation", Name: "test", InputSchema: schema,
		}, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		for _, result := range []map[string]any{
			searchResultProjection(catalogapp.ToolSearchResult{ToolID: id, SourceRef: source, Name: "test"}),
			func() map[string]any {
				detail, e := toolProjection(tool)
				if e != nil {
					t.Fatal(e)
				}
				return detail
			}(),
		} {
			_, hasProvider := result["providerId"]
			if result["sourceId"] != source.String() ||
				result["sourceKind"] != string(tt.kind) || hasProvider != tt.provider {
				t.Fatalf("CLI invented a resource Provider: %+v", result)
			}
		}
	}
}
