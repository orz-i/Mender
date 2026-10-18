package server

import (
	"testing"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// A concrete Tool-name search must not include status/result merely because
// their generic descriptions contain "delegated". Description-only queries
// still work when no name, title, source or tag provides a stronger match.
func TestToolDiscoverySuppressesWeakDescriptionsAfterNamedMatches(t *testing.T) {
	t.Parallel()
	fixture := newServerFixture(t)
	provider := fixture.provider.ID()
	fixture.tools = []catalogdomain.Tool{
		testCanonicalTool(t, "tool_00000000000000000000000000000041",
			provider, "agent.delegate", "delegate", "Delegate to Agent",
			"Submit one bounded agent task"),
		testCanonicalTool(t, "tool_00000000000000000000000000000042",
			provider, "agent.status", "status", "Check Agent task",
			"Query one delegated task and inspect bounded execution"),
		testCanonicalTool(t, "tool_00000000000000000000000000000043",
			provider, "agent.result", "result", "Read Agent task result",
			"Read the bounded result of one delegated task"),
	}
	bindings := make([]profiledomain.ToolBindingSpec, 0, len(fixture.tools))
	for _, tool := range fixture.tools {
		bindings = append(bindings, profiledomain.ToolBindingSpec{ToolID: tool.ID()})
	}
	client := connectClient(t, fixture.adapter(t, fixture.profile(t, profiledomain.ExposureDynamic, bindings)))

	cases := []struct {
		name string
		args map[string]any
		want int
	}{
		{"cross-source named", map[string]any{"view": "tools", "query": "delegate"}, 1},
		{"source-scoped named", map[string]any{"view": "tools", "query": "delegate", "sourceId": provider.String()}, 1},
		{"cross-source prose fallback", map[string]any{"view": "tools", "query": "bounded"}, 3},
		{"source-scoped prose fallback", map[string]any{"view": "tools", "query": "bounded", "sourceId": provider.String()}, 3},
		{"source title browse", map[string]any{"view": "tools", "query": "GitHub", "sourceId": provider.String()}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callSourceSearch(t, client, tc.args)
			var tools []any
			if tc.args["sourceId"] != nil {
				tools = result["tools"].([]any)
			} else {
				groups := result["groups"].([]any)
				if len(groups) != 1 {
					t.Fatalf("unexpected groups: %#v", result)
				}
				tools = groups[0].(map[string]any)["tools"].([]any)
			}
			if len(tools) != tc.want {
				t.Fatalf("query=%#v returned %d tools, want %d: %#v", tc.args, len(tools), tc.want, result)
			}
			if tc.want == 1 && tools[0].(map[string]any)["toolId"] != fixture.tools[0].ID().String() {
				t.Fatalf("named delegate mismatch: %#v", result)
			}
		})
	}
}
