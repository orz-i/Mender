package httpapi

import (
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

func TestToolDetailUsesExactSourceRefWithoutSyntheticProvider(t *testing.T) {
	for _, tt := range []struct {
		kind        catalog.SourceKind
		id          string
		hasProvider bool
	}{
		{catalog.SourceProvider, "prov_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{catalog.SourceAgent, "agent_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{catalog.SourceNative, "workspace", false},
	} {
		t.Run(string(tt.kind), func(t *testing.T) {
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
			item, err := toolProjection(tool)
			if err != nil {
				t.Fatal(err)
			}
			if item["sourceId"] != source.String() || item["sourceKind"] != string(tt.kind) {
				t.Fatalf("lost canonical source: %+v", item)
			}
			_, exists := item["providerId"]
			if exists != tt.hasProvider {
				t.Fatalf("fabricated Provider attribution: %+v", item)
			}
		})
	}
}
