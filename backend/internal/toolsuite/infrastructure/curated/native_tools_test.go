package curated

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	suite "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
)

func TestNativeRegistryFixedIDsAndSuiteCoverage(t *testing.T) {
	tools, err := PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 9 {
		t.Fatalf("native tools count = %d, want 9", len(tools))
	}
	byKey := make(map[string]NativeTool)
	for _, tool := range tools {
		if tool.Source.Kind() != catalog.SourceNative {
			t.Fatalf("non-native provenance for %s", tool.SourceKey)
		}
		source, _, ok := strings.Cut(tool.SourceKey, ".")
		if !ok || tool.Source.ID() != source {
			t.Fatalf("source mismatch: %+v", tool)
		}
		digest := sha256.Sum256([]byte("io.mender.native.tool.v1/" + tool.SourceKey))
		want := "tool_" + hex.EncodeToString(digest[:16])
		if tool.ToolID.String() != want {
			t.Fatalf("reserved ID for %s changed: %s vs %s", tool.SourceKey, tool.ToolID, want)
		}
		if _, ok := byKey[tool.SourceKey]; ok {
			t.Fatalf("duplicate native source key %s", tool.SourceKey)
		}
		byKey[tool.SourceKey] = tool
	}
	for _, suiteID := range []string{"sandbox.workspace", "sandbox.jobs"} {
		definition, ok := Find(suite.ID(suiteID))
		if !ok {
			t.Fatalf("missing curated suite %s", suiteID)
		}
		for _, key := range definition.ToolSourceKeys {
			tool, found := byKey[key]
			if !found || !slices.Contains(definition.RequiredCapabilities, tool.Capability) {
				t.Fatalf("reserved tool %s does not match required suite capability", key)
			}
		}
	}
}

func TestNativeRegistryDefensiveCopiesAndNoAgentFacade(t *testing.T) {
	first, err := PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	first[0].SourceKey = "malicious"
	second, err := PlannedNativeTools()
	if err != nil || second[0].SourceKey != "workspace.read" {
		t.Fatalf("native tool registry was mutated: %+v, %v", second, err)
	}
	for _, tool := range second {
		if strings.HasPrefix(tool.SourceKey, "agent.") {
			t.Fatal("agent tools are per-AgentID, not global native Tools")
		}
	}
}
