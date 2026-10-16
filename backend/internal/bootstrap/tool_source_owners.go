package bootstrap

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// ToolSourceOwner is inert, derived display attribution for an exact Provider.
// It is not a second resource identity, a grant, or a health/availability probe.
type ToolSourceOwner struct {
	Kind       string // provider, agent, sandbox, unclaimed
	ResourceID string
	Name       string
}

// ListToolSourceOwners joins existing exact resource -> facade relations;
// it never guesses an owner from PluginID, namespace or config arguments.
func (r *Runtime) ListToolSourceOwners(ctx context.Context) (map[catalogdomain.ProviderID]ToolSourceOwner, error) {
	if r == nil {
		return nil, fmt.Errorf("tool source owners: runtime is nil")
	}
	db, err := openProviderReferenceDB(ctx, r.providerDatabasePath)
	if err != nil {
		return nil, fmt.Errorf("tool source owners: open: %w", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx, `
 SELECT f.provider_id, 'agent', a.agent_id, a.name
 FROM agent_tool_facades f JOIN agents a ON a.agent_id=f.agent_id
 UNION ALL
 SELECT f.provider_id, 'sandbox', s.id, s.display_name
 FROM sandbox_tool_suite_bindings f JOIN sandboxes s ON s.id=f.sandbox_id
 UNION ALL
 SELECT f.provider_id, 'sandbox', s.id, s.display_name
 FROM sandbox_job_suite_bindings f JOIN sandboxes s ON s.id=f.sandbox_id
 `)
	if err != nil {
		return nil, fmt.Errorf("tool source owners: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	owners := map[catalogdomain.ProviderID]ToolSourceOwner{}
	for rows.Next() {
		var rawID, kind, resourceID, name string
		if err = rows.Scan(&rawID, &kind, &resourceID, &name); err != nil {
			return nil, fmt.Errorf("tool source owners: scan: %w", err)
		}
		id, parseErr := catalogdomain.ParseProviderID(rawID)
		if parseErr != nil {
			return nil, fmt.Errorf("tool source owners: provider id: %w", parseErr)
		}
		if previous, exists := owners[id]; exists {
			return nil, fmt.Errorf("tool source owners: conflicting ownership for Provider %s (%s and %s)", id, previous.ResourceID, resourceID)
		}
		owners[id] = ToolSourceOwner{Kind: kind, ResourceID: resourceID, Name: name}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("tool source owners: rows: %w", err)
	}
	return owners, nil
}

// ToolSourceForProvider gives a standalone Provider its own display identity;
// an unbound managed facade remains explicitly unclaimed, not assigned by
// matching the internal adapter name or inspecting unverified config.
func ToolSourceForProvider(
	provider catalogdomain.Provider,
	config *providerplugin.Configuration,
	owners map[catalogdomain.ProviderID]ToolSourceOwner,
) ToolSourceOwner {
	if config == nil {
		return ToolSourceOwner{Kind: "unclaimed"}
	}
	owner, claimed := owners[provider.ID()]
	if isManagedProviderFacade(config.PluginID) {
		if claimed {
			return owner
		}
		return ToolSourceOwner{Kind: "unclaimed"}
	}
	if claimed {
		// A claimed resource cannot be represented as an independent source
		// when the plugin identity disagrees with its owner binding.
		return ToolSourceOwner{Kind: "unclaimed"}
	}
	return ToolSourceOwner{Kind: "provider", ResourceID: provider.ID().String(), Name: provider.DisplayName()}
}
