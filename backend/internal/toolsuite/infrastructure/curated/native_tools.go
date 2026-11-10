package curated

import (
	"fmt"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	suite "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
)

// NativeTool is a reserved, immutable identity for a host-reviewed Tool.
// The v3 Catalog publishes these canonical definitions independently of
// Sandbox instances or Agent-owned resources; registration itself never
// creates a Profile grant or execution placement.
type NativeTool struct {
	ToolID     catalog.ToolID
	Source     catalog.SourceRef
	SourceKey  string
	Capability suite.Capability
}

// Fixed values are SHA-256("io.mender.native.tool.v1/" + sourceKey) truncated to
// 128 bits and encoded with the canonical ToolID prefix. Persist these exact
// constants rather than re-hashing at each process start or during migration.
var reservedNativeTools = []struct {
	id         string
	source     string
	key        string
	capability suite.Capability
}{
	{"tool_882fb9c25db58744f34da684f7f7ab5f", "workspace", "workspace.read", suite.FileRead},
	{"tool_33a65e204e96265604efddd7536eabd9", "workspace", "workspace.search", suite.FileSearch},
	{"tool_4ba8e255e6486aa5a425f8e87b348ded", "workspace", "workspace.edit", suite.FileEdit},
	{"tool_1f28bd14ee01b125dcda89402b892ca9", "workspace", "workspace.shell", suite.ProcessShell},
	{"tool_3c766860cad00c990eea717f7189fb57", "jobs", "jobs.submit", suite.JobSubmit},
	{"tool_5a5206f3704bb3b15f73493f91556d64", "jobs", "jobs.status", suite.JobObserve},
	{"tool_aaf99df8c4418467e4d28dc5b5986600", "jobs", "jobs.logs", suite.JobObserve},
	{"tool_19b4bffd11581c24979594e5bd319497", "jobs", "jobs.result", suite.JobObserve},
	{"tool_2829fcf2f091f0566c5579b4d22bd70f", "jobs", "jobs.cancel", suite.JobCancel},
}

// PlannedNativeTools is the pure host-owned canonical identity registry.
// It does not query or activate Providers, open Sandboxes or infer execution
// availability from a Suite definition. The v3 Runtime separately checks each
// Tool's Profile scope, target and backend capability at invocation.
func PlannedNativeTools() ([]NativeTool, error) {
	result := make([]NativeTool, 0, len(reservedNativeTools))
	seenID := make(map[catalog.ToolID]bool, len(reservedNativeTools))
	seenKey := make(map[string]bool, len(reservedNativeTools))
	for _, def := range reservedNativeTools {
		id, err := catalog.ParseToolID(def.id)
		if err != nil {
			return nil, fmt.Errorf("native tool registry: %w", err)
		}
		source, err := catalog.NativeSource(def.source)
		if err != nil {
			return nil, fmt.Errorf("native tool registry: %w", err)
		}
		if def.key == "" || seenID[id] || seenKey[def.key] {
			return nil, fmt.Errorf("native tool registry: duplicate or empty key %q", def.key)
		}
		seenID[id], seenKey[def.key] = true, true
		result = append(result, NativeTool{ToolID: id, Source: source,
			SourceKey: def.key, Capability: def.capability})
	}
	return result, nil
}
