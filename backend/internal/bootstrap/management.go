package bootstrap

import (
	"context"
	"errors"
	"fmt"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

var ErrProviderInUse = errors.New("provider is referenced by a Profile or Policy Rule")

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

func (r *Runtime) SearchCatalog(
	ctx context.Context,
	query string,
	limit int,
	providerID *catalogdomain.ProviderID,
	profileSlug *profiledomain.Slug,
) ([]catalogapplication.ToolSearchResult, error) {
	request := catalogapplication.SearchToolsRequest{
		Query:      query,
		Limit:      limit,
		ProviderID: providerID,
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
	executionContext invocationdomain.InvocationContext,
) (invocationapplication.InvokeToolResponse, error) {
	request := invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: arguments,
		Context:   executionContext,
	}

	if executionContext.Scope().Kind() == invocationdomain.ScopeProfile {
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
	if err := r.ActivateConfiguredMCPProvider(ctx, tool.ProviderID()); err != nil {
		return invocationapplication.InvokeToolResponse{}, fmt.Errorf("activate tool provider: %w", err)
	}
	return r.InvokeTool.Execute(ctx, request)
}

func (r *Runtime) UpdateMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	request catalogapplication.UpdateProviderRequest,
	config mcpconfigstore.Config,
) (catalogdomain.Provider, error) {
	currentProvider, err := r.Catalog.GetProvider.Execute(ctx, providerID)
	if err != nil {
		return catalogdomain.Provider{}, err
	}

	config.ProviderID = providerID
	if err := config.Validate(); err != nil {
		return catalogdomain.Provider{}, err
	}
	if _, err := currentProvider.UpdateMetadata(
		request.DisplayName,
		request.Description,
		r.Catalog.clock.Now(),
	); err != nil {
		return catalogdomain.Provider{}, err
	}

	currentConfig, err := r.mcpConfigs.Get(ctx, providerID)
	if err != nil {
		return catalogdomain.Provider{}, err
	}
	if currentConfig.Transport != config.Transport {
		return catalogdomain.Provider{}, catalogdomain.ValidationError{
			Field:  "transport",
			Reason: "must not change after Provider registration",
		}
	}

	if err := r.mcpConfigs.Save(ctx, config); err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update MCP provider: save config: %w",
			err,
		)
	}

	request.ProviderID = providerID
	updatedProvider, err := r.Catalog.UpdateProvider.Execute(ctx, request)
	if err != nil {
		restoreErr := r.mcpConfigs.Save(ctx, currentConfig)
		if restoreErr != nil {
			return catalogdomain.Provider{}, errors.Join(
				fmt.Errorf("update MCP provider: metadata: %w", err),
				fmt.Errorf("update MCP provider: restore config: %w", restoreErr),
			)
		}
		return catalogdomain.Provider{}, err
	}

	return updatedProvider, nil
}

func (r *Runtime) DeleteMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	profiles, err := r.Profiles.List.Execute(ctx)
	if err != nil {
		return fmt.Errorf("delete MCP provider: list profiles: %w", err)
	}

	for _, profile := range profiles {
		for _, selection := range profile.Selections() {
			tool, describeErr := r.Catalog.DescribeTool.Execute(ctx, selection.ToolID)
			if describeErr != nil {
				if errors.Is(describeErr, catalogapplication.ErrToolNotFound) {
					continue
				}
				return fmt.Errorf("delete MCP provider: inspect profile %s: %w", profile.Slug(), describeErr)
			}
			if tool.ProviderID() == providerID {
				return fmt.Errorf("%w: profile %s selects tool %s", ErrProviderInUse, profile.Slug(), tool.ID())
			}
		}
	}

	policy, err := r.Policy.Get.Execute(ctx)
	if err != nil {
		return fmt.Errorf("delete MCP provider: inspect Policy: %w", err)
	}
	for _, rule := range policy.Rules() {
		for _, referencedProviderID := range rule.Match().ProviderIDs() {
			if referencedProviderID == providerID.String() {
				return fmt.Errorf(
					"%w: Policy Rule %s references Provider %s",
					ErrProviderInUse,
					rule.ID(),
					providerID,
				)
			}
		}
	}

	config, configErr := r.mcpConfigs.Get(ctx, providerID)
	hadConfig := configErr == nil
	if configErr != nil && !errors.Is(configErr, mcpconfigstore.ErrNotFound) {
		return fmt.Errorf("delete MCP provider: load config: %w", configErr)
	}

	if err := r.providerInstances.CloseProvider(providerID); err != nil {
		return fmt.Errorf("delete MCP provider: close active instance: %w", err)
	}

	if hadConfig {
		if err := r.mcpConfigs.Delete(ctx, providerID); err != nil {
			return fmt.Errorf("delete MCP provider: delete config: %w", err)
		}
	}

	if err := r.Catalog.RemoveProvider.Execute(ctx, providerID); err != nil {
		if hadConfig {
			restoreErr := r.mcpConfigs.Save(ctx, config)
			if restoreErr != nil {
				return errors.Join(
					fmt.Errorf("delete MCP provider: remove provider: %w", err),
					fmt.Errorf("delete MCP provider: restore config: %w", restoreErr),
				)
			}
		}
		return fmt.Errorf("delete MCP provider: remove provider: %w", err)
	}
	return nil
}
