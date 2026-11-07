package binding

import (
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestWailsToolDetailsNeverFabricateProviderForAgentOrNative(t *testing.T) {
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
		result, err := toolProjection(tool)
		if err != nil {
			t.Fatal(err)
		}
		if result.SourceID != source.String() || result.SourceKind != string(tt.kind) ||
			(result.ProviderID != "") != tt.provider {
			t.Fatalf("Wails Tool identity crossed resources: %+v", result)
		}
	}
}
