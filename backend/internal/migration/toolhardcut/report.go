package toolhardcut

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// Report is a data-minimized, deterministic, READ-ONLY assessment. The report
// never includes raw Provider configs, Credential JSON, roots, transcripts or logs.
type Report struct {
	ReportVersion          int                     `json:"reportVersion"`
	Mode                   string                  `json:"mode"`
	Status                 string                  `json:"status"`
	Snapshot               Snapshot                `json:"snapshot"`
	ToolMappings           []ToolMapping           `json:"toolMappings"`
	WorkspacePlacements    []WorkspacePlacement    `json:"workspacePlacements"`
	ProfileBindingDiffs    []ProfileBindingDiff    `json:"profileBindingDiffs"`
	PolicyRuleDiffs        []PolicyRuleDiff        `json:"policyRuleDiffs"`
	MCPExportDiffs         []MCPExportDiff         `json:"mcpExportDiffs"`
	AgentTaskEvidence      []TaskEvidence          `json:"agentTaskEvidence"`
	SSHRecoveryObligations []SSHRecoveryObligation `json:"sshRecoveryObligations"`
	Conflicts              []Conflict              `json:"conflicts"`
	Counts                 Counts                  `json:"counts"`
	ManifestDigest         string                  `json:"manifestDigest"`
}
type Snapshot struct {
	SchemaVersions        map[string]int `json:"schemaVersions"`
	DatabaseFingerprint   string         `json:"databaseFingerprint"`
	RuntimeWriterQuiesced bool           `json:"runtimeWriterQuiesced"`
}
type ToolMapping struct {
	OldToolID         string   `json:"oldToolId"`
	OldSource         string   `json:"oldSource"`
	NewToolID         string   `json:"newToolId"`
	NewSource         string   `json:"newSource"`
	Operation         string   `json:"operation"`
	AllowedTargetRefs []string `json:"allowedTargetRefs"`
	Action            string   `json:"action"`
	Status            string   `json:"status"`
}
type WorkspacePlacement struct {
	Source             string `json:"source"`
	WorkspaceKey       string `json:"workspaceKey"`
	SandboxID          string `json:"sandboxId"`
	PlannedWorkspaceID string `json:"plannedWorkspaceId"`
	Verification       string `json:"verification"`
}
type ProfileBindingDiff struct {
	ProfileID         string   `json:"profileId"`
	NewToolID         string   `json:"newToolId"`
	OldToolIDs        []string `json:"oldToolIds"`
	AllowedTargetRefs []string `json:"allowedTargetRefs"`
	ConstraintDigest  string   `json:"constraintDigest"`
	Action            string   `json:"action"`
}
type PolicyRuleDiff struct {
	RuleID string `json:"ruleId"`
	Action string `json:"action"`
}
type MCPExportDiff struct {
	ProfileID string `json:"profileId"`
	OldToolID string `json:"oldToolId"`
	NewToolID string `json:"newToolId"`
	Direct    bool   `json:"direct"`
	Action    string `json:"action"`
}
type TaskEvidence struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}
type SSHRecoveryObligation struct {
	SandboxID    string `json:"sandboxId"`
	Verification string `json:"verification"`
}
type Conflict struct {
	Code           string   `json:"code"`
	ResourceRefs   []string `json:"resourceRefs"`
	Reason         string   `json:"reason"`
	RequiredAction string   `json:"requiredAction"`
}
type Counts struct {
	ToolsBefore        int `json:"toolsBefore"`
	ToolsAfter         int `json:"toolsAfter"`
	UnresolvedBlockers int `json:"unresolvedBlockers"`
}

const (
	BlockSchema    = "SCHEMA_OR_REFERENTIAL_INTEGRITY"
	BlockOwner     = "SOURCE_OWNER_CONFLICT"
	BlockToolID    = "TOOL_ID_COLLISION"
	BlockBinding   = "BINDING_CONSTRAINT_COLLISION"
	BlockTarget    = "BINDING_TARGET_EXPANSION"
	BlockWorkspace = "WORKSPACE_IDENTITY_AMBIGUOUS"
	BlockPlacement = "RUNTIME_PLACEMENT_UNVERIFIED"
	BlockPolicy    = "POLICY_SEMANTIC_DRIFT"
	BlockTask      = "TASK_STATE_UNCERTAIN"
	BlockSSH       = "SSH_JOB_RECOVERY_UNPROVEN"
	BlockExport    = "MCP_EXPORT_COLLISION"
	BlockBackup    = "BACKUP_OR_WRITER_UNSAFE"
)

func newReport() Report {
	return Report{
		ReportVersion: 1, Mode: "assessment", Status: "blocked",
		Snapshot:     Snapshot{SchemaVersions: map[string]int{}, RuntimeWriterQuiesced: false},
		ToolMappings: []ToolMapping{}, WorkspacePlacements: []WorkspacePlacement{},
		ProfileBindingDiffs: []ProfileBindingDiff{}, PolicyRuleDiffs: []PolicyRuleDiff{},
		MCPExportDiffs: []MCPExportDiff{}, AgentTaskEvidence: []TaskEvidence{},
		SSHRecoveryObligations: []SSHRecoveryObligation{}, Conflicts: []Conflict{},
	}
}
func (r *Report) block(code string, ids []string, reason string) {
	refs := append([]string(nil), ids...)
	sort.Strings(refs)
	r.Conflicts = append(r.Conflicts, Conflict{Code: code, ResourceRefs: refs,
		Reason: reason, RequiredAction: "explicit_owner_review"})
}
func (r *Report) finalize() error {
	sort.Slice(r.ToolMappings, func(i, j int) bool { return r.ToolMappings[i].OldToolID < r.ToolMappings[j].OldToolID })
	sort.Slice(r.WorkspacePlacements, func(i, j int) bool {
		a, b := r.WorkspacePlacements[i], r.WorkspacePlacements[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.WorkspaceKey != b.WorkspaceKey {
			return a.WorkspaceKey < b.WorkspaceKey
		}
		return a.SandboxID < b.SandboxID
	})
	sort.Slice(r.ProfileBindingDiffs, func(i, j int) bool {
		a, b := r.ProfileBindingDiffs[i], r.ProfileBindingDiffs[j]
		if a.ProfileID != b.ProfileID {
			return a.ProfileID < b.ProfileID
		}
		return a.NewToolID < b.NewToolID
	})
	sort.Slice(r.PolicyRuleDiffs, func(i, j int) bool { return r.PolicyRuleDiffs[i].RuleID < r.PolicyRuleDiffs[j].RuleID })
	sort.Slice(r.MCPExportDiffs, func(i, j int) bool {
		a, b := r.MCPExportDiffs[i], r.MCPExportDiffs[j]
		if a.ProfileID != b.ProfileID {
			return a.ProfileID < b.ProfileID
		}
		return a.OldToolID < b.OldToolID
	})
	sort.Slice(r.AgentTaskEvidence, func(i, j int) bool { return r.AgentTaskEvidence[i].Status < r.AgentTaskEvidence[j].Status })
	sort.Slice(r.SSHRecoveryObligations, func(i, j int) bool {
		return r.SSHRecoveryObligations[i].SandboxID < r.SSHRecoveryObligations[j].SandboxID
	})
	sort.Slice(r.Conflicts, func(i, j int) bool {
		a, b := r.Conflicts[i], r.Conflicts[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if joinedA, joinedB := strings.Join(a.ResourceRefs, "\x00"), strings.Join(b.ResourceRefs, "\x00"); joinedA != joinedB {
			return joinedA < joinedB
		}
		return a.Reason < b.Reason
	})
	for i := range r.ToolMappings {
		sort.Strings(r.ToolMappings[i].AllowedTargetRefs)
	}
	for i := range r.ProfileBindingDiffs {
		sort.Strings(r.ProfileBindingDiffs[i].OldToolIDs)
		sort.Strings(r.ProfileBindingDiffs[i].AllowedTargetRefs)
	}
	r.Counts.UnresolvedBlockers = len(r.Conflicts)
	if len(r.Conflicts) != 0 {
		r.Status = "blocked"
	} else if len(r.MCPExportDiffs) > 0 {
		r.Status = "requires_review"
	} else {
		r.Status = "ready"
	}
	r.ManifestDigest = ""
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	h := sha256.Sum256(raw)
	r.ManifestDigest = "sha256:" + hex.EncodeToString(h[:])
	return nil
}
func digest(v ...string) string {
	h := sha256.New()
	for _, s := range v {
		var prefix [8]byte
		binary.BigEndian.PutUint64(prefix[:], uint64(len(s)))
		_, _ = h.Write(prefix[:])
		_, _ = h.Write([]byte(s))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
