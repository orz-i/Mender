package domain

import (
	"testing"
	"time"
)

func TestToolFromSourcePreservesAgentNativeAndProviderIdentity(t *testing.T) {
	input, err := NewSchema([]byte(`{"type":"object","additionalProperties":false}`))
	if err != nil {
		t.Fatal(err)
	}
	sourceIDs := []struct {
		kind SourceKind
		id   string
		key  string
	}{
		{SourceProvider, "prov_0123456789abcdef0123456789abcdef", "issues.list"},
		{SourceAgent, "agent_0123456789abcdef0123456789abcdef", "agent.delegate"},
		{SourceNative, "workspace", "workspace.read"},
		{SourceNative, "jobs", "jobs.result"},
	}
	for _, tc := range sourceIDs {
		t.Run(string(tc.kind)+"_"+tc.key, func(t *testing.T) {
			src, err := NewSourceRef(tc.kind, tc.id)
			if err != nil {
				t.Fatal(err)
			}
			id, _ := ParseToolID("tool_0123456789abcdef0123456789abcdef")
			now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
			spec := ToolSpec{SourceKey: tc.key, Name: "invoke", Title: "Invoke", InputSchema: input}
			tool, err := NewToolFromSource(id, src, spec, now)
			if err != nil {
				t.Fatal(err)
			}
			if tool.SourceRef() != src || tool.ID() != id || tool.SourceKey() != tc.key {
				t.Fatalf("typed provenance lost: %+v", tool)
			}
			if tc.kind != SourceProvider && tool.ProviderID() != "" {
				t.Fatalf("native/Agent Tool forged ProviderID: %q", tool.ProviderID())
			}
			if tc.kind == SourceProvider && tool.ProviderID().String() != tc.id {
				t.Fatal("real Provider ID lost")
			}
			spec.Title = "New title"
			refreshed, err := tool.RefreshMetadata(spec, now.Add(time.Second))
			if err != nil || refreshed.SourceRef() != src || refreshed.ID() != id ||
				!refreshed.CreatedAt().Equal(now) || refreshed.Title() != "New title" {
				t.Fatalf("metadata refreshed identity: %+v %v", refreshed, err)
			}
			restored, err := RestoreToolFromSource(id, src, spec, tool.CreatedAt(), refreshed.UpdatedAt())
			if err != nil || restored.SourceRef() != src || restored.ProviderID() != tool.ProviderID() {
				t.Fatalf("restored typed identity: %+v %v", restored, err)
			}
			if _, err := refreshed.RefreshMetadata(ToolSpec{SourceKey: "changed", Name: "invoke", InputSchema: input}, now.Add(2*time.Second)); err == nil {
				t.Fatal("accepted source key mutation")
			}
		})
	}
}

func TestToolFromSourceRejectsInvalidProvenanceAndTimestamps(t *testing.T) {
	input, _ := NewSchema([]byte(`{}`))
	id, _ := ParseToolID("tool_0123456789abcdef0123456789abcdef")
	agent, _ := AgentSource("agent_0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	spec := ToolSpec{SourceKey: "agent.delegate", Name: "delegate", InputSchema: input}
	if _, err := NewToolFromSource(id, SourceRef{}, spec, now); err == nil {
		t.Fatal("unverified provenance accepted")
	}
	if _, err := NewToolFromSource(id, agent, spec, time.Time{}); err == nil {
		t.Fatal("zero time accepted")
	}
	if _, err := RestoreToolFromSource(id, agent, spec, now.Add(time.Hour), now); err == nil {
		t.Fatal("reverse timestamp accepted")
	}
	if _, err := NewToolFromSource("", agent, spec, now); err == nil {
		t.Fatal("empty ToolID accepted")
	}
	if _, err := NewToolFromSource(id, agent, ToolSpec{SourceKey: "agent.delegate", Name: "delegate"}, now); err == nil {
		t.Fatal("missing schema accepted")
	}
	if _, err := NewTool(id, "", spec, now); err == nil {
		t.Fatal("legacy Provider constructor accepted empty Provider")
	}
}
