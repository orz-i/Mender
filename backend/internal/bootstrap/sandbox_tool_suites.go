package bootstrap

import (
	"context"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	suitedomain "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
	suitecatalogue "github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

// Sandbox Tool Suites are read-only capability projections. A Sandbox never
// owns/creates a Provider or generic Workspace ToolID in the v3 Catalog.
type SandboxTool struct {
	ToolID    string `json:"toolId"`
	SourceKey string `json:"sourceKey"`
	Name      string `json:"name"`
	Title     string `json:"title"`
}

type SandboxToolSuite struct {
	SuiteID   suitedomain.ID   `json:"suiteId"`
	SandboxID sandboxdomain.ID `json:"sandboxId"`
	Status    string           `json:"status"`
	Available bool             `json:"available"`
	Issue     string           `json:"issue,omitempty"`
	Tools     []SandboxTool    `json:"tools"`
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
// Resource status observation cannot grant execution rights.
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
	if !r.Sandboxes.BackendAvailable(item) {
		return false, "Sandbox backend is unavailable", nil
	}
	spec := item.Spec()
	contracts, err := r.Sandboxes.backendPlugins.VerifiedWorkspaceContracts(spec.Backend, spec.Binding)
	if err != nil {
		return false, "Workspace operations unavailable on this exact Sandbox backend", nil
	}
	definition, _ := suitecatalogue.Find(suitedomain.SandboxWorkspace)
	if err = suitedomain.RequireContracts(definition, suitedomain.SandboxResource, contracts); err != nil {
		return false, "Workspace operations unavailable on this exact Sandbox backend", nil
	}
	if run.State != sandboxdomain.StateReady {
		return false, "Sandbox is not ready", nil
	}
	return true, "", nil
}

// listNativeSandboxTools returns a single canonical native Catalog source,
// not per-Sandbox Provider clones. The result does not grant ToolBindings.
func (r *Runtime) listNativeSandboxTools(ctx context.Context, id sandboxdomain.ID,
	caller gov.Context, suiteID suitedomain.ID, sourceKey string, available bool, issue string) (SandboxToolSuite, error) {
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
	result := SandboxToolSuite{SuiteID: suiteID, SandboxID: id, Status: "ready",
		Available: available, Issue: issue, Tools: []SandboxTool{}}
	if !available {
		result.Status = "incompatible"
	}
	store, ok := r.Catalog.store.(interface {
		ListSourceTools(context.Context, catalogdomain.SourceRef) ([]catalogdomain.Tool, error)
	})
	if !ok {
		return SandboxToolSuite{}, sandboxdomain.ErrBackendUnavailable
	}
	source, err := catalogdomain.NativeSource(sourceKey)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	tools, err := store.ListSourceTools(ctx, source)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	for _, tool := range tools {
		result.Tools = append(result.Tools, SandboxTool{
			ToolID: tool.ID().String(), SourceKey: tool.SourceKey(),
			Name: tool.Name(), Title: tool.Title(),
		})
	}
	if len(tools) == 0 {
		result.Status = "missing"
	}
	return result, nil
}

func (r *Runtime) SandboxWorkspaceTools(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SandboxToolSuite, error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return SandboxToolSuite{}, err
	}
	available, issue, err := r.sandboxWorkspaceCapabilitySnapshot(ctx, id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	return r.listNativeSandboxTools(ctx, id, caller, suitedomain.SandboxWorkspace, "workspace", available, issue)
}
