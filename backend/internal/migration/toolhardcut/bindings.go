package toolhardcut

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
)

type profileBinding struct {
	profileID, oldID, newID, constraints string
	targets                              []string
	direct                               bool
}

// assessBindings checks fine-grained Profile ToolBinding and Policy conditions
// before any ToolID is collapsed. A Sandbox grant cannot replace an old
// per-Tool target constraint. No automatic grant or Rule changes occur.
func assessBindings(ctx context.Context, tx *sql.Tx, r *Report, state inventory) error {
	direct := map[string]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT profile_id,tool_id FROM mcp_delivery_direct_tools ORDER BY profile_id,tool_id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p, t string
		if err := rows.Scan(&p, &t); err != nil {
			_ = rows.Close()
			return err
		}
		direct[p+"\x00"+t] = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	modes := map[string]string{}
	modeRows, err := tx.QueryContext(ctx, `SELECT profile_id,mode FROM mcp_delivery_configs ORDER BY profile_id`)
	if err != nil {
		return err
	}
	for modeRows.Next() {
		var profileID, mode string
		if err := modeRows.Scan(&profileID, &mode); err != nil {
			_ = modeRows.Close()
			return err
		}
		modes[profileID] = mode
	}
	err = modeRows.Err()
	_ = modeRows.Close()
	if err != nil {
		return err
	}
	groups := map[string][]profileBinding{}
	rows, err = tx.QueryContext(ctx, `SELECT profile_id,tool_id,alias,title_override,description_override,
  policy_constraint,concurrency_constraint,credential_refs_json
  FROM profile_tools ORDER BY profile_id,tool_id`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p, id, alias, title, desc, policy, serial, credentials string
		if err := rows.Scan(&p, &id, &alias, &title, &desc, &policy, &serial, &credentials); err != nil {
			_ = rows.Close()
			return err
		}
		if _, err := catalog.ParseToolID(id); err != nil {
			r.block(BlockSchema, []string{id}, "invalid Profile ToolID reference")
		}
		mapping, ok := state.tools[id]
		if !ok {
			r.block(BlockSchema, []string{p, id}, "Profile references an unknown legacy Tool")
			continue
		}
		if _, err := catalog.ParseToolID(mapping.NewToolID); err != nil {
			r.block(BlockSchema, []string{p, id}, "planned ToolID invalid")
		}
		mode, knownMode := modes[p]
		if !knownMode || (mode != "direct" && mode != "dynamic" && mode != "hybrid") {
			r.block(BlockExport, []string{p}, "Profile has no valid MCP delivery mode")
			continue
		}
		directSelected := mode == "direct" || (mode == "hybrid" && direct[p+"\x00"+id])
		credHash := digest(credentials)
		metaHash := digest(alias, title, desc, policy, serial, credHash, strconv.FormatBool(directSelected))
		k := p + "\x00" + mapping.NewToolID
		groups[k] = append(groups[k], profileBinding{
			profileID: p, oldID: id, newID: mapping.NewToolID, constraints: metaHash,
			targets: append([]string(nil), mapping.AllowedTargetRefs...), direct: directSelected,
		})
		if mapping.OldToolID != mapping.NewToolID || mapping.OldSource != mapping.NewSource {
			change := "source_namespace_change_requires_client_review"
			if mapping.OldToolID != mapping.NewToolID {
				change = "breaking_tool_identity_requires_client_review"
			}
			r.MCPExportDiffs = append(r.MCPExportDiffs, MCPExportDiff{
				ProfileID: p, OldToolID: id, NewToolID: mapping.NewToolID, Direct: directSelected,
				Action: change,
			})
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bindings := groups[key]
		first := bindings[0]
		diff := ProfileBindingDiff{
			ProfileID: first.profileID, NewToolID: first.newID, OldToolIDs: []string{},
			ConstraintDigest: first.constraints, Action: "keep", AllowedTargetRefs: []string{},
		}
		for _, b := range bindings {
			diff.OldToolIDs = append(diff.OldToolIDs, b.oldID)
			diff.AllowedTargetRefs = append(diff.AllowedTargetRefs, b.targets...)
			if b.constraints != first.constraints {
				r.block(BlockBinding, []string{first.profileID, first.newID}, "multiple legacy ToolBindings have incompatible alias/policy/concurrency/credential/Direct restrictions")
				diff.Action = "blocked"
			}
		}
		diff.AllowedTargetRefs = sortedUnique(diff.AllowedTargetRefs)
		if len(bindings) > 1 && diff.Action != "blocked" {
			diff.Action = "merge_explicit_scope"
		}
		if len(bindings) == 1 && first.oldID != first.newID {
			diff.Action = "remap_explicit_scope"
		}
		if first.oldID != first.newID && len(diff.AllowedTargetRefs) == 0 {
			r.block(BlockTarget, []string{first.profileID, first.newID}, "old native Tool cannot be migrated without exact approved Sandbox targets")
			diff.Action = "blocked"
		}
		if diff.Action != "keep" {
			r.ProfileBindingDiffs = append(r.ProfileBindingDiffs, diff)
		}
	}
	return assessPolicyAndPlacementReferences(ctx, tx, r, state)
}

func assessPolicyAndPlacementReferences(ctx context.Context, tx *sql.Tx, r *Report, state inventory) error {
	rows, err := tx.QueryContext(ctx, "SELECT rule_id,match_json FROM policy_rules ORDER BY rule_id")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			r.block(BlockPolicy, []string{id}, "Policy match JSON unparseable")
			r.PolicyRuleDiffs = append(r.PolicyRuleDiffs, PolicyRuleDiff{RuleID: id, Action: "blocked"})
			continue
		}
		changed := false
		for _, field := range []string{"providerIds", "toolIds"} {
			val, ok := fields[field]
			if !ok {
				continue
			}
			var ids []string
			if err := json.Unmarshal(val, &ids); err != nil {
				r.block(BlockPolicy, []string{id}, "unreviewed Policy selector type")
				changed = true
				continue
			}
			for _, ref := range ids {
				if field == "providerIds" {
					if _, ok := state.facades[ref]; ok {
						r.block(BlockPolicy, []string{id, ref}, "Provider-facade Policy rule cannot retain its exact Target semantics without rule migration")
						changed = true
					}
				} else if mapping, ok := state.tools[ref]; ok && mapping.OldToolID != mapping.NewToolID {
					r.block(BlockPolicy, []string{id, ref}, "ToolID-specific Policy rule needs explicit per-target semantics")
					changed = true
				}
			}
		}
		action := "keep"
		if changed {
			action = "blocked"
		}
		r.PolicyRuleDiffs = append(r.PolicyRuleDiffs, PolicyRuleDiff{RuleID: id, Action: action})
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}

	rows, err = tx.QueryContext(ctx, `SELECT agent_id,workspace_key,sandbox_id,tool_ids_json
 FROM agent_tool_placements ORDER BY agent_id,workspace_key`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var agent, key, sandboxID, raw string
		if err := rows.Scan(&agent, &key, &sandboxID, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		var ids []string
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			r.block(BlockSchema, []string{agent, key}, "Agent ToolPlacement references unparseable")
			continue
		}
		for _, id := range ids {
			mapping, exists := state.tools[id]
			if !exists {
				r.block(BlockSchema, []string{agent, id}, "Agent ToolPlacement references missing Tool")
				continue
			}
			if mapping.OldToolID != mapping.NewToolID {
				if !contains(mapping.AllowedTargetRefs, sandboxID) {
					r.block(BlockTarget, []string{agent, id, sandboxID}, "Agent placement target was not authorized by the original Tool facade")
				} else {
					r.block(BlockTarget, []string{agent, id}, "Agent governed Tool binding needs explicit migration and new WorkspaceRef")
				}
			}
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}

	// A shared Workspace identity must not silently merge Agent roots and
	// grants. Preserve old alias/grant authority until owner-approved path
	// attestation and canonical WorkspaceRef migration are supplied by E/F.
	rows, err = tx.QueryContext(ctx, `SELECT agent_id,workspace_key FROM agent_workspaces ORDER BY agent_id,workspace_key`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var agent, key string
		if err := rows.Scan(&agent, &key); err != nil {
			_ = rows.Close()
			return err
		}
		ref := plannedWorkspaceID(agent, key, "")
		r.WorkspacePlacements = append(r.WorkspacePlacements, WorkspacePlacement{
			Source: "agent:" + agent, WorkspaceKey: key, PlannedWorkspaceID: ref,
			Verification: "unverified",
		})
		r.block(BlockWorkspace, []string{agent, key}, "Agent workspace root and grants require owner-approved identity and cwd attestation")
	}
	err = rows.Err()
	_ = rows.Close()
	return err
}

func assessTasksAndLeases(ctx context.Context, tx *sql.Tx, r *Report) error {
	rows, err := tx.QueryContext(ctx, `SELECT status,COUNT(*) FROM agent_tasks GROUP BY status ORDER BY status`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			_ = rows.Close()
			return err
		}
		r.AgentTaskEvidence = append(r.AgentTaskEvidence, TaskEvidence{Status: status, Count: count})
		switch status {
		case "queued", "running", "unknown":
			r.block(BlockTask, []string{"agent_tasks", status}, fmt.Sprintf("%d nonterminal or uncertain Agent Tasks require preserved native recovery evidence; never replay", count))
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	var leases int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sandbox_execution_leases").Scan(&leases); err != nil {
		return err
	}
	if leases > 0 {
		r.block(BlockTask, []string{"sandbox_execution_leases"}, "active or uncertain Sandbox durable lease prevents cutover")
	}
	rows, err = tx.QueryContext(ctx, "SELECT sandbox_id FROM sandbox_ssh_job_obligations ORDER BY sandbox_id")
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		r.SSHRecoveryObligations = append(r.SSHRecoveryObligations, SSHRecoveryObligation{SandboxID: id, Verification: "unverified"})
		r.block(BlockSSH, []string{id}, "SSH Job remote presence/recovery proof unavailable from read-only SQLite; must not replay or destroy evidence")
	}
	err = rows.Err()
	_ = rows.Close()
	return err
}
func contains(set []string, needle string) bool {
	for _, v := range set {
		if strings.EqualFold(v, needle) {
			return true
		}
	}
	return false
}
