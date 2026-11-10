package bootstrap

import (
	"context"
	"errors"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerapplication "github.com/orz-i/Mender/backend/internal/providerplugin/application"
)

var ErrProviderInUse = providerapplication.ErrProviderInUse

type RuntimeStatus struct {
	Ready                bool
	PolicyMode           string
	ApprovalMode         string
	CredentialResolver   string
	CredentialPersistent bool
}

func (r *Runtime) Status() RuntimeStatus {
	return RuntimeStatus{
		Ready:                true,
		PolicyMode:           "configured",
		ApprovalMode:         "fail_closed_when_required",
		CredentialResolver:   "inventory",
		CredentialPersistent: true,
	}
}

// PageCatalog is a read-only, revision-bound Tool discovery projection.
// Profile-scoped search remains filtered by the exact current ToolIDs.
func (r *Runtime) PageCatalog(ctx context.Context, request catalogapplication.PageToolsRequest,
	profileSlug *profiledomain.Slug) (catalogapplication.ToolPage, error) {
	if profileSlug != nil {
		if request.Mode != "search" {
			return catalogapplication.ToolPage{}, catalogapplication.ErrInvalidToolCursor
		}
		profile, err := r.Profiles.Get.Execute(ctx, *profileSlug)
		if err != nil {
			return catalogapplication.ToolPage{}, err
		}
		request.Profile = profileSlug.String()
		request.AllowedToolIDs = profile.ToolIDs()
		if request.AllowedToolIDs == nil {
			request.AllowedToolIDs = []catalogdomain.ToolID{} // empty Profile never broadens search
		}
	}
	return r.Catalog.PageTools.Execute(ctx, request)
}

func (r *Runtime) SearchCatalog(
	ctx context.Context,
	query string,
	limit int,
	sourceRef *catalogdomain.SourceRef,
	profileSlug *profiledomain.Slug,
) ([]catalogapplication.ToolSearchResult, error) {
	request := catalogapplication.SearchToolsRequest{
		Query:     query,
		Limit:     limit,
		SourceRef: sourceRef,
	}
	if profileSlug != nil {
		profile, err := r.Profiles.Get.Execute(ctx, *profileSlug)
		if err != nil {
			return nil, err
		}
		request.AllowedToolIDs = profile.ToolIDs()
	}
	return r.Catalog.SearchTools.Execute(ctx, request)
}

func (r *Runtime) CallConfiguredTool(
	ctx context.Context,
	toolID catalogdomain.ToolID,
	arguments []byte,
	executionContext governancedomain.Context,
) (invocationapplication.InvokeToolResponse, error) {
	request := invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: arguments,
		Context:   executionContext,
	}

	if executionContext.Scope().Kind() == governancedomain.ScopeProfile {
		profileID, err := profiledomain.ParseProfileID(executionContext.Scope().Reference())
		if err != nil {
			return invocationapplication.InvokeToolResponse{}, fmt.Errorf(
				"call configured tool: profile scope: %w",
				err,
			)
		}
		profile, err := r.Profiles.GetByID.Execute(ctx, profileID)
		if err != nil {
			return invocationapplication.InvokeToolResponse{}, err
		}
		if !profile.Contains(toolID) {
			return r.InvokeTool.Execute(ctx, request)
		}
	}

	tool, err := r.Catalog.DescribeTool.Execute(ctx, toolID)
	if err != nil {
		if errors.Is(err, catalogapplication.ErrToolNotFound) {
			return r.InvokeTool.Execute(ctx, request)
		}
		return invocationapplication.InvokeToolResponse{}, err
	}
	// Native Workspace/Jobs and Agent as Tool have a registered SourceRef
	// executor and no Provider to activate. They always enter the same
	// Invocation gate; this is not a shortcut around ToolBinding or Policy.
	if tool.SourceRef().Kind() != catalogdomain.SourceProvider {
		return r.InvokeTool.Execute(ctx, request)
	}
	if tool.ProviderID() == "" {
		return invocationapplication.InvokeToolResponse{}, fmt.Errorf("activate tool provider: Tool %s has no verified Provider owner", toolID)
	}
	if err := r.activateConfiguredProvider(ctx, tool.ProviderID(), executionContext); err != nil {
		return invocationapplication.InvokeToolResponse{}, fmt.Errorf("activate tool provider: %w", err)
	}
	return r.InvokeTool.Execute(ctx, request)
}

func wrapOptionalError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
