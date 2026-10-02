package plugin

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestWorkspaceGovernanceFactsAreStructuredWithoutTouchingWorkspace(t *testing.T) {
	t.Parallel()

	plugin := New(HostShellLauncher{})
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")
	config, err := json.Marshal(Config{Root: nonexistent})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		sourceKey string
		input     map[string]any
		effect    invocationdomain.EffectFact
		resource  string
		knowledge invocationdomain.FactKnowledge
	}{
		{
			name:      "read",
			sourceKey: sourceKeyRead,
			input:     map[string]any{"path": "src/main.go"},
			effect:    "filesystem.read",
			resource:  "src/main.go",
			knowledge: invocationdomain.FactKnowledgeKnown,
		},
		{
			name:      "search root",
			sourceKey: sourceKeySearch,
			input:     map[string]any{"mode": "files", "pattern": "*.go"},
			effect:    "filesystem.search",
			resource:  ".",
			knowledge: invocationdomain.FactKnowledgeKnown,
		},
		{
			name:      "edit",
			sourceKey: sourceKeyEdit,
			input:     map[string]any{"path": "draft.md", "operation": "delete"},
			effect:    "filesystem.write",
			resource:  "draft.md",
			knowledge: invocationdomain.FactKnowledgeKnown,
		},
		{
			name:      "shell",
			sourceKey: sourceKeyShell,
			input:     map[string]any{"command": "echo hi", "cwd": "scripts"},
			effect:    "process.execute",
			resource:  "scripts",
			knowledge: invocationdomain.FactKnowledgeKnown,
		},
		{
			name:      "unknown source",
			sourceKey: "workspace.future",
			input:     map[string]any{},
			knowledge: invocationdomain.FactKnowledgeUnknown,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(test.input)
			if err != nil {
				t.Fatal(err)
			}
			args, err := invocationdomain.NewJSONValue(raw)
			if err != nil {
				t.Fatal(err)
			}
			facts, err := plugin.ExtractGovernanceFacts(
				context.Background(),
				config,
				workspaceTestTool(t, test.sourceKey),
				args,
			)
			if err != nil {
				t.Fatalf("ExtractGovernanceFacts() error = %v", err)
			}
			if facts.Knowledge() != test.knowledge {
				t.Fatalf("Knowledge() = %s, want %s", facts.Knowledge(), test.knowledge)
			}
			if test.knowledge == invocationdomain.FactKnowledgeUnknown {
				return
			}
			if effects := facts.Effects(); len(effects) != 1 || effects[0] != test.effect {
				t.Fatalf("Effects() = %v, want %s", effects, test.effect)
			}
			resources := facts.Resources()
			if len(resources) != 1 ||
				resources[0].Kind() != "workspace_path" ||
				resources[0].Value() != test.resource {
				t.Fatalf("Resources() = %#v", resources)
			}
		})
	}
}
