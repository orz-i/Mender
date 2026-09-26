package provider

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDiscoveredToolMapsSchemasAndDisplayMetadata(t *testing.T) {
	t.Parallel()

	readOnly := true
	tool := &mcp.Tool{
		Name:        "issues.create",
		Description: "Create an issue",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string"},
			},
		},
		OutputSchema: map[string]any{"type": "string"},
		Annotations: &mcp.ToolAnnotations{
			Title:        "Create issue",
			ReadOnlyHint: readOnly,
		},
	}

	discovered, err := discoveredTool(tool)
	if err != nil {
		t.Fatalf("discoveredTool() error = %v", err)
	}

	if discovered.SourceKey != tool.Name || discovered.Name != tool.Name {
		t.Fatalf("identity mapping = source %q name %q", discovered.SourceKey, discovered.Name)
	}
	if discovered.Title != "Create issue" {
		t.Fatalf("Title = %q", discovered.Title)
	}

	var input map[string]any
	if err := json.Unmarshal(discovered.InputSchema, &input); err != nil {
		t.Fatalf("input schema JSON error = %v", err)
	}
	if input["type"] != "object" {
		t.Fatalf("input schema = %#v", input)
	}

	// Security-relevant MCP annotations are deliberately not projected into
	// tags or policy metadata by the Catalog mapping.
	if len(discovered.Tags) != 1 || discovered.Tags[0] != "mcp" {
		t.Fatalf("Tags = %#v", discovered.Tags)
	}
}

func TestDiscoveredToolPrefersExplicitTitle(t *testing.T) {
	t.Parallel()

	discovered, err := discoveredTool(&mcp.Tool{
		Name:        "tool",
		Title:       "Explicit",
		InputSchema: map[string]any{"type": "object"},
		Annotations: &mcp.ToolAnnotations{Title: "Annotation"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if discovered.Title != "Explicit" {
		t.Fatalf("Title = %q", discovered.Title)
	}
}
