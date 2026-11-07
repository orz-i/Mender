package toolhardcut

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

type facade struct {
	provider string
	kind     string
	owner    string
	targets  []string
}
type inventory struct {
	tools      map[string]ToolMapping
	facades    map[string]facade
	newIDs     map[string]bool
	sourceKeys map[string]map[string]bool
}
type providerConfig struct {
	plugin       string
	version      int
	body         string
	installation sql.NullString
}

func inventorySources(ctx context.Context, tx *sql.Tx, r *Report) (inventory, error) {
	state := inventory{
		tools: map[string]ToolMapping{}, facades: map[string]facade{},
		newIDs: map[string]bool{}, sourceKeys: map[string]map[string]bool{},
	}
	configs := map[string]providerConfig{}
	rows, err := tx.QueryContext(ctx, `SELECT provider_id,plugin_id,config_version,config_json,plugin_installation_id FROM provider_configs ORDER BY provider_id`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var id string
		var c providerConfig
		if err := rows.Scan(&id, &c.plugin, &c.version, &c.body, &c.installation); err != nil {
			_ = rows.Close()
			return state, err
		}
		configs[id] = c
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return state, err
	}

	// Strict ownership pins. A manually inserted Provider with the same
	// built-in plugin ID is never silently adopted as a resource facade.
	for _, def := range []struct{ kind, query string }{
		{"agent", `SELECT provider_id,agent_id FROM agent_tool_facades ORDER BY provider_id`},
		{"workspace", `SELECT provider_id,sandbox_id FROM sandbox_tool_suite_bindings ORDER BY provider_id`},
		{"jobs", `SELECT provider_id,sandbox_id FROM sandbox_job_suite_bindings ORDER BY provider_id`},
	} {
		rows, err := tx.QueryContext(ctx, def.query)
		if err != nil {
			return state, err
		}
		for rows.Next() {
			var id, owner string
			if err := rows.Scan(&id, &owner); err != nil {
				_ = rows.Close()
				return state, err
			}
			if existing, ok := state.facades[id]; ok {
				r.block(BlockOwner, []string{id, existing.owner, owner}, "duplicate facade Provider ownership")
				continue
			}
			source := facade{provider: id, kind: def.kind, owner: owner}
			if def.kind == "agent" {
				if _, err := catalog.AgentSource(owner); err != nil {
					r.block(BlockOwner, []string{id}, "invalid pinned Agent ID")
				}
			} else {
				if _, err := sandbox.ParseID(owner); err != nil {
					r.block(BlockOwner, []string{id}, "invalid pinned Sandbox ID")
				}
				source.targets = []string{owner}
			}
			state.facades[id] = source
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return state, err
		}
	}
	// Extra aliases are only valid for exact Workspace Suite Provider pins.
	rows, err = tx.QueryContext(ctx, `SELECT provider_id,workspace_key,sandbox_id FROM workspace_tool_targets ORDER BY provider_id,workspace_key`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var id, key, target string
		if err := rows.Scan(&id, &key, &target); err != nil {
			_ = rows.Close()
			return state, err
		}
		f, ok := state.facades[id]
		if !ok || f.kind != "workspace" {
			r.block(BlockOwner, []string{id}, "unowned legacy Workspace target alias")
			continue
		}
		if _, err := sandbox.ParseID(target); err != nil {
			r.block(BlockWorkspace, []string{id}, "invalid Workspace Sandbox target")
			continue
		}
		f.targets = append(f.targets, target)
		state.facades[id] = f
		r.WorkspacePlacements = append(r.WorkspacePlacements, WorkspacePlacement{
			Source: id, WorkspaceKey: key, SandboxID: target,
			PlannedWorkspaceID: plannedWorkspaceID(id, key, target), Verification: "unverified",
		})
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return state, err
	}
	for provider, f := range state.facades {
		c, ok := configs[provider]
		expectedPlugin := "io.mender.provider." + map[string]string{"agent": "agent-delegation", "workspace": "workspace", "jobs": "sandbox-jobs"}[f.kind]
		version := map[string]int{"agent": 1, "workspace": 2, "jobs": 1}[f.kind]
		if !ok || c.plugin != expectedPlugin || c.version != version || c.installation.Valid {
			r.block(BlockOwner, []string{provider}, "facade Provider configuration pin is missing or not host-owned")
		} else {
			var payload struct {
				AgentID   string `json:"agentId"`
				SandboxID string `json:"sandboxId"`
			}
			if json.Unmarshal([]byte(c.body), &payload) != nil ||
				(f.kind == "agent" && payload.AgentID != f.owner) ||
				(f.kind != "agent" && payload.SandboxID != f.owner) {
				r.block(BlockOwner, []string{provider}, "facade configuration is retargeted or unparseable")
			}
		}
		table, column := "sandboxes", "id"
		if f.kind == "agent" {
			table, column = "agents", "agent_id"
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM "+table+" WHERE "+column+"=?)", f.owner).Scan(&exists); err != nil {
			return state, err
		}
		if !exists {
			r.block(BlockOwner, []string{provider}, "pinned resource owner not present")
		}
		if f.kind == "jobs" {
			r.block(BlockSSH, []string{provider, f.owner}, "SSH Job facade requires verified backend ownership and task recovery admission before consolidation")
		}
		if f.kind == "workspace" {
			r.WorkspacePlacements = append(r.WorkspacePlacements, WorkspacePlacement{
				Source: provider, WorkspaceKey: "default", SandboxID: f.owner,
				PlannedWorkspaceID: plannedWorkspaceID(provider, "default", f.owner),
				Verification:       "unverified",
			})
		}
	}
	// A local target with an actual canonical directory can be attested
	// without starting a driver. Docker/SSH/external roots are NOT assumed
	// equivalent to host paths, and remain hard blockers until F verifies
	// target-side placement and recovery.
	for i := range r.WorkspacePlacements {
		p := &r.WorkspacePlacements[i]
		verified, err := attestLocalSandboxRoot(ctx, tx, p.SandboxID)
		if err != nil {
			return state, err
		}
		if verified {
			p.Verification = "verified_local_root"
		} else {
			r.block(BlockPlacement, []string{p.Source, p.SandboxID}, "Workspace backend root and ownership cannot be attested from local, canonical Sandbox metadata")
		}
	}
	for id, c := range configs {
		if !strings.HasPrefix(c.plugin, "io.mender.provider.") {
			continue
		}
		if c.plugin != "io.mender.provider.agent-delegation" &&
			c.plugin != "io.mender.provider.workspace" &&
			c.plugin != "io.mender.provider.sandbox-jobs" {
			continue
		}
		if _, ok := state.facades[id]; !ok {
			r.block(BlockOwner, []string{id}, "resource-style Provider configuration has no exact owner pin")
		}
	}
	nativeIDs := map[string]string{}
	reserved, err := curated.PlannedNativeTools()
	if err != nil {
		return state, err
	}
	for _, tool := range reserved {
		nativeIDs[tool.SourceKey] = tool.ToolID.String()
	}
	legacyIDs := map[string]bool{}
	rows, err = tx.QueryContext(ctx, `SELECT id,provider_id,source_key FROM tools ORDER BY id`)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var oldID, pid, key string
		if err := rows.Scan(&oldID, &pid, &key); err != nil {
			_ = rows.Close()
			return state, err
		}
		r.Counts.ToolsBefore++
		if _, err := catalog.ParseToolID(oldID); err != nil {
			r.block(BlockSchema, []string{oldID}, "malformed existing ToolID")
		}
		if _, err := catalog.ParseProviderID(pid); err != nil {
			r.block(BlockSchema, []string{oldID}, "malformed existing ProviderID")
		}
		legacyIDs[oldID] = true
		mapping := ToolMapping{OldToolID: oldID, OldSource: "provider:" + pid, NewToolID: oldID,
			NewSource: "provider:" + pid, Operation: key, Action: "keep", Status: "verified", AllowedTargetRefs: []string{}}
		if f, ok := state.facades[pid]; ok {
			if state.sourceKeys[pid] == nil {
				state.sourceKeys[pid] = map[string]bool{}
			}
			state.sourceKeys[pid][key] = true
			switch f.kind {
			case "agent":
				mapping.NewSource = "agent:" + f.owner
			case "workspace", "jobs":
				mapping.NewSource = "native:" + f.kind
				if f.kind == "workspace" {
					mapping.NewSource = "native:workspace"
				}
				nativeID, ok := nativeIDs[key]
				if !ok {
					r.block(BlockOwner, []string{oldID, pid}, "unreviewed operation under internal facade")
				} else {
					mapping.NewToolID = nativeID
				}
				mapping.AllowedTargetRefs = append(mapping.AllowedTargetRefs, f.targets...)
				mapping.Action = "merge"
				mapping.Status = "requires_review"
			}
		} else if c, ok := configs[pid]; ok && (c.plugin == "io.mender.provider.agent-delegation" ||
			c.plugin == "io.mender.provider.workspace" || c.plugin == "io.mender.provider.sandbox-jobs") {
			r.block(BlockOwner, []string{pid, oldID}, "orphan facade Tool without owner pin")
		}
		state.tools[oldID] = mapping
		r.ToolMappings = append(r.ToolMappings, mapping)
		state.newIDs[mapping.NewToolID] = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return state, err
	}
	for _, tool := range reserved {
		id := tool.ToolID.String()
		if legacyIDs[id] {
			r.block(BlockToolID, []string{id}, "reserved native ToolID collides with legacy Tool")
		}
		state.newIDs[id] = true
	}
	for pid, f := range state.facades {
		expected := []string{}
		switch f.kind {
		case "agent":
			expected = []string{"agent.delegate", "agent.status", "agent.result", "agent.cancel", "agent.resume"}
		case "workspace":
			expected = []string{"workspace.read", "workspace.search", "workspace.edit", "workspace.shell"}
		case "jobs":
			expected = []string{"jobs.submit", "jobs.status", "jobs.logs", "jobs.result", "jobs.cancel"}
		}
		keys := state.sourceKeys[pid]
		if len(keys) != len(expected) {
			r.block(BlockOwner, []string{pid}, "facade Tool snapshot is incomplete or contains unreviewed operations")
		}
		for _, key := range expected {
			if !keys[key] {
				r.block(BlockOwner, []string{pid}, "facade Tool snapshot is missing "+key)
			}
		}
	}
	r.Counts.ToolsAfter = len(state.newIDs)
	return state, nil
}
func plannedWorkspaceID(provider, key, sandboxID string) string {
	hash := sha256.Sum256([]byte("mender.toolhardcut.workspace.v1\x00" + provider + "\x00" + key + "\x00" + sandboxID))
	return "wrk_" + hex.EncodeToString(hash[:16])
}
func sortedUnique(input []string) []string {
	result := append([]string(nil), input...)
	sort.Strings(result)
	unique := result[:0]
	for _, v := range result {
		if len(unique) == 0 || unique[len(unique)-1] != v {
			unique = append(unique, v)
		}
	}
	return unique
}
