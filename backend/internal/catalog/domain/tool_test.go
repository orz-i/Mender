package domain

import (
	"slices"
	"testing"
	"time"
)

func TestToolRefreshPreservesIdentity(t *testing.T) {
	t.Parallel()

	input, err := NewSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}

	id, _ := ParseToolID("tool_0123456789abcdef0123456789abcdef")
	providerID, _ := ParseProviderID("prov_0123456789abcdef0123456789abcdef")
	source, _ := ProviderSource(providerID)
	created := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	tool, err := NewToolFromSource(id, source, ToolSpec{
		SourceKey:   "issues.create",
		Name:        "create_issue",
		Title:       "Create issue",
		Description: "Create an issue",
		InputSchema: input,
		Tags:        []string{"GitHub", "write", "github"},
	}, created)
	if err != nil {
		t.Fatalf("NewToolFromSource() error = %v", err)
	}

	updated := created.Add(time.Hour)
	refreshed, err := tool.RefreshMetadata(ToolSpec{
		SourceKey:   "issues.create",
		Name:        "issue_create",
		Title:       "Create an issue",
		Description: "Create a repository issue",
		InputSchema: input,
		Tags:        []string{"write", "issues"},
	}, updated)
	if err != nil {
		t.Fatalf("RefreshMetadata() error = %v", err)
	}

	if refreshed.ID() != tool.ID() {
		t.Fatal("RefreshMetadata() changed ToolID")
	}
	if refreshed.ProviderID() != tool.ProviderID() {
		t.Fatal("RefreshMetadata() changed ProviderID")
	}
	if refreshed.SourceKey() != tool.SourceKey() {
		t.Fatal("RefreshMetadata() changed SourceKey")
	}
	if !refreshed.CreatedAt().Equal(tool.CreatedAt()) {
		t.Fatal("RefreshMetadata() changed CreatedAt")
	}
	if !refreshed.UpdatedAt().Equal(updated) {
		t.Fatalf("UpdatedAt() = %v, want %v", refreshed.UpdatedAt(), updated)
	}
	if refreshed.Name() != "issue_create" {
		t.Fatalf("Name() = %q", refreshed.Name())
	}
	if !slices.Equal(tool.Tags(), []string{"GitHub", "write"}) {
		t.Fatalf("normalized tags = %#v", tool.Tags())
	}
}

func TestToolProvenanceIsTyped(t *testing.T) {
	now := time.Now().UTC()
	toolID, _ := ParseToolID("tool_0123456789abcdef0123456789abcdef")
	input, _ := NewSchema([]byte(`{"type":"object"}`))

	providerID, _ := ParseProviderID("prov_0123456789abcdef0123456789abcdef")
	provSource, _ := ProviderSource(providerID)
	agentSource, _ := AgentSource("agent_0123456789abcdef0123456789abcdef")
	nativeSource, _ := NativeSource("workspace")

	for _, tc := range []struct {
		name     string
		source   SourceRef
		expected string
		kind     SourceKind
	}{
		{name: "provider source", source: provSource, expected: "provider:" + providerID.String(), kind: SourceProvider},
		{name: "agent source", source: agentSource, expected: "agent:agent_0123456789abcdef0123456789abcdef", kind: SourceAgent},
		{name: "native source", source: nativeSource, expected: "native:workspace", kind: SourceNative},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool, err := NewToolFromSource(toolID, tc.source, ToolSpec{SourceKey: "test.tool", Name: "test", InputSchema: input}, now)
			if err != nil {
				t.Fatalf("NewToolFromSource: %v", err)
			}
			ref := tool.SourceRef()
			if ref.Kind() != tc.kind || ref.String() != tc.expected {
				t.Fatalf("unexpected source: %+v, want kind=%v string=%s", ref, tc.kind, tc.expected)
			}
		})
	}
}

func TestToolRefreshRejectsSourceKeyChange(t *testing.T) {
	t.Parallel()

	input, _ := NewSchema([]byte(`{"type":"object"}`))
	id, _ := ParseToolID("tool_0123456789abcdef0123456789abcdef")
	nativeSource, _ := NativeSource("workspace")
	now := time.Now().UTC()

	tool, err := NewToolFromSource(id, nativeSource, ToolSpec{
		SourceKey:   "old",
		Name:        "tool",
		InputSchema: input,
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.RefreshMetadata(ToolSpec{
		SourceKey:   "new",
		Name:        "tool",
		InputSchema: input,
	}, now.Add(time.Second))
	if err == nil {
		t.Fatal("RefreshMetadata() expected source key error")
	}
}
