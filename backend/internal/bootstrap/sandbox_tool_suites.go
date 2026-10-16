package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	suitedomain "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
	suitecatalogue "github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	workspaceplugin "github.com/orz-i/Mender/backend/internal/workspace/infrastructure/plugin"
)

type SandboxTool struct {
	ToolID    string `json:"toolId"`
	SourceKey string `json:"sourceKey"`
	Name      string `json:"name"`
	Title     string `json:"title"`
}

type SandboxToolSuite struct {
	SuiteID      suitedomain.ID   `json:"suiteId"`
	SandboxID    sandboxdomain.ID `json:"sandboxId"`
	Status       string           `json:"status"`
	ProviderID   string           `json:"providerId,omitempty"`
	ProviderName string           `json:"providerName,omitempty"`
	Available    bool             `json:"available"`
	Issue        string           `json:"issue,omitempty"`
	Tools        []SandboxTool    `json:"tools"`
}

func (r *Runtime) ListToolSuiteDefinitions(ctx context.Context) ([]suitedomain.Definition, error) {
	if r == nil {
		return nil, fmt.Errorf("tool suite catalogue unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list := suitecatalogue.List()
	for _, item := range list {
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("reviewed suite %s: %w", item.SuiteID, err)
		}
	}
	return list, nil
}

// sandboxToolOwner rejects Profile service Principals and arbitrary MCP calls.
// Owner administration never grants Profile membership or execution authority.
func (r *Runtime) sandboxToolOwner(caller gov.Context) error {
	if r == nil || caller.Validate() != nil || caller.Principal() != r.ownerPrincipal ||
		caller.Scope().Kind() != gov.ScopeCatalog {
		return sandboxdomain.ErrNotAuthorized
	}
	switch caller.Origin() {
	case gov.OriginCLI, gov.OriginDesktop, gov.OriginManagementHTTP, gov.OriginInternal:
		return nil
	default:
		return sandboxdomain.ErrNotAuthorized
	}
}

func (r *Runtime) sandboxWorkspaceCapabilitySnapshot(ctx context.Context, id sandboxdomain.ID) (bool, string, error) {
	item, err := r.Sandboxes.Get(ctx, id)
	if err != nil {
		return false, "", err
	}
	run, err := r.Sandboxes.GetRun(ctx, id)
	if err != nil {
		return false, "", err
	}
	// Backend availability (including Docker Engine liveness) is a cached
	// host observation; a static extension descriptor alone is not readiness.
	if !r.Sandboxes.BackendAvailable(item) {
		return false, "Sandbox backend is unavailable", nil
	}
	spec := item.Spec()
	// Only the exact registered backend may provide verified Workspace v1
	// contracts. Descriptor strings alone never make a Suite executable.
	available, err := r.Sandboxes.backendPlugins.VerifiedWorkspaceContracts(spec.Backend, spec.Binding)
	if err != nil {
		return false, "Workspace operations unavailable on this exact Sandbox backend", nil
	}
	definition, _ := suitecatalogue.Find(suitedomain.SandboxWorkspace)
	if err = suitedomain.RequireContracts(definition, suitedomain.SandboxResource, available); err != nil {
		return false, "Workspace operations unavailable on this exact Sandbox backend", nil
	}
	if run.State != sandboxdomain.StateReady {
		return false, "Sandbox is not ready; tools cannot be prepared or executed", nil
	}
	return true, "", nil
}

// SandboxWorkspaceTools makes only inert, owner-scoped observations. It does
// not adopt legacy Providers, start a backend or fix a broken suite on GET.
func (r *Runtime) SandboxWorkspaceTools(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SandboxToolSuite, error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return SandboxToolSuite{}, err
	}
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return SandboxToolSuite{}, err
	}
	available, issue, err := r.sandboxWorkspaceCapabilitySnapshot(ctx, id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	result := SandboxToolSuite{SuiteID: suitedomain.SandboxWorkspace, SandboxID: id, Status: "missing",
		Available: available, Issue: issue, Tools: []SandboxTool{}}
	configs, err := r.providerConfigs.List(ctx)
	if err != nil {
		return result, err
	}
	matches := []catalogdomain.ProviderID{}
	for _, config := range configs {
		if config.PluginID != workspaceplugin.ID || config.InstallationID != nil || config.ConfigVersion != workspaceplugin.ConfigVersion {
			continue
		}
		decoded, e := workspaceplugin.DecodeConfig(config.Config)
		if e == nil && decoded.SandboxID == id.String() {
			matches = append(matches, config.ProviderID)
		}
	}
	pinned, err := r.Sandboxes.store.GetWorkspaceSuite(ctx, id)
	if err != nil {
		return result, err
	}
	if len(matches) > 1 || (pinned != "" && (len(matches) != 1 || pinned != matches[0].String())) {
		result.Status = "conflict"
		return result, nil
	}
	if len(matches) == 0 {
		if !available {
			result.Status = "incompatible"
		}
		return result, nil
	}
	providerID := matches[0]
	provider, err := r.Catalog.GetProvider.Execute(ctx, providerID)
	if err != nil {
		result.Status = "conflict"
		return result, nil
	}
	result.ProviderID = providerID.String()
	result.ProviderName = provider.DisplayName()
	tools, err := r.Catalog.store.ListProviderTools(ctx, providerID)
	if err != nil {
		return result, err
	}
	present := map[string]bool{}
	for _, tool := range tools {
		result.Tools = append(result.Tools, SandboxTool{
			ToolID: tool.ID().String(), SourceKey: tool.SourceKey(),
			Name: tool.Name(), Title: tool.Title(),
		})
		present[tool.SourceKey()] = true
	}
	if pinned == "" {
		result.Status = "legacy"
	} else {
		result.Status = "ready"
		for _, key := range []string{"workspace.read", "workspace.search", "workspace.edit", "workspace.shell"} {
			if !present[key] {
				result.Status = "incomplete"
				break
			}
		}
	}
	if !available {
		result.Status = "incompatible"
	}
	return result, nil
}

func sandboxFacadeNamespace(id sandboxdomain.ID) string {
	// A stable, collision-resistant 128-bit Sandbox ID; namespace has no
	// user-derived/external parameters and fits the Catalog slug constraint.
	return "sandbox-" + strings.TrimPrefix(id.String(), "sbx_")
}

// EnsureSandboxWorkspaceTools only runs on explicit, owner-governed mutation.
// It can adopt one legacy Provider preserving ToolID, but never auto-merges.
func (r *Runtime) EnsureSandboxWorkspaceTools(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SandboxToolSuite, error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return SandboxToolSuite{}, err
	}
	item, err := r.Sandboxes.Get(ctx, id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	if item.OwnerKind() != string(caller.Principal().Kind()) || item.OwnerRef() != caller.Principal().Reference() {
		return SandboxToolSuite{}, sandboxdomain.ErrNotAuthorized
	}
	state, err := r.SandboxWorkspaceTools(ctx, id, caller)
	if err != nil {
		return state, err
	}
	if !state.Available || state.Status == "incompatible" {
		return state, fmt.Errorf("%w: Sandbox Workspace backend cannot meet all required capabilities", suitedomain.ErrIncompatible)
	}
	if state.Status == "conflict" {
		return state, fmt.Errorf("%w: multiple or mismatched Workspace Providers", sandboxdomain.ErrConflict)
	}
	if state.Status == "ready" {
		return state, nil
	}
	if state.ProviderID == "" {
		config, e := json.Marshal(workspaceplugin.Config{SandboxID: id.String()})
		if e != nil {
			return state, e
		}
		provider, e := r.registerConfiguredProvider(ctx, catalogapplication.RegisterProviderRequest{
			Namespace: sandboxFacadeNamespace(id), DisplayName: item.Spec().Name + " Workspace",
			Description: "Managed Workspace Tools for " + item.Spec().Name,
		}, workspaceplugin.ID, config, caller, true)
		if e != nil {
			current, readErr := r.SandboxWorkspaceTools(ctx, id, caller)
			if readErr != nil || current.ProviderID == "" || current.Status == "conflict" {
				return state, fmt.Errorf("create Sandbox Workspace facade: %w", e)
			}
			state = current
		} else {
			state.ProviderID = provider.ID().String()
		}
	}
	providerID, err := catalogdomain.ParseProviderID(state.ProviderID)
	if err != nil {
		return state, err
	}
	if _, err = r.SyncConfiguredProvider(ctx, providerID, caller); err != nil {
		return state, fmt.Errorf("synchronize Sandbox Workspace facade: %w", err)
	}
	if err = r.Sandboxes.store.ClaimWorkspaceSuite(ctx, id, providerID.String()); err != nil {
		return state, fmt.Errorf("claim Sandbox Workspace suite: %w", err)
	}
	return r.SandboxWorkspaceTools(ctx, id, caller)
}

// RemoveSandboxWorkspaceTools is explicit owner-only teardown. Generic
// Provider delete cannot bypass resource ownership; usage policy is checked
// BEFORE releasing the persisted binding.
func (r *Runtime) RemoveSandboxWorkspaceTools(ctx context.Context, id sandboxdomain.ID, caller gov.Context) error {
	if err := r.sandboxToolOwner(caller); err != nil {
		return err
	}
	resource, err := r.Sandboxes.Get(ctx, id)
	if err != nil {
		return err
	}
	if resource.OwnerKind() != string(caller.Principal().Kind()) || resource.OwnerRef() != caller.Principal().Reference() {
		return sandboxdomain.ErrNotAuthorized
	}
	state, err := r.SandboxWorkspaceTools(ctx, id, caller)
	if err != nil {
		return err
	}
	if state.Status == "conflict" || state.Status == "legacy" {
		return sandboxdomain.ErrConflict
	}
	if state.ProviderID == "" {
		return nil
	}
	pid, err := catalogdomain.ParseProviderID(state.ProviderID)
	if err != nil {
		return err
	}
	if err = r.ensureProviderUnused(ctx, pid); err != nil {
		return err
	}
	if err = r.Sandboxes.store.ReleaseWorkspaceSuite(ctx, id, state.ProviderID); err != nil {
		return err
	}
	if err = r.DeleteConfiguredProvider(ctx, pid); err != nil {
		restoreErr := r.Sandboxes.store.ClaimWorkspaceSuite(context.WithoutCancel(ctx), id, state.ProviderID)
		return errors.Join(err, restoreErr)
	}
	return nil
}
