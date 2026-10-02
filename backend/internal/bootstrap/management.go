package bootstrap

import (
	"context"
	"errors"
	"fmt"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
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
	if err := r.activateConfiguredProvider(ctx, tool.ProviderID(), executionContext); err != nil {
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

	currentGeneric, err := r.providerConfigs.GetProviderConfig(ctx, providerID)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update MCP provider: load provider config: %w",
			err,
		)
	}
	nextGeneric, err := legacyMCPProviderConfiguration(config)
	if err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update MCP provider: map provider config: %w",
			err,
		)
	}

	if err := r.providerConfigs.Save(ctx, nextGeneric); err != nil {
		return catalogdomain.Provider{}, fmt.Errorf(
			"update MCP provider: save provider config: %w",
			err,
		)
	}
	if err := r.mcpConfigs.Save(ctx, config); err != nil {
		restoreErr := r.providerConfigs.Save(ctx, currentGeneric)
		if restoreErr != nil {
			return catalogdomain.Provider{}, errors.Join(
				fmt.Errorf("update MCP provider: save compatibility config: %w", err),
				fmt.Errorf(
					"update MCP provider: restore provider config: %w",
					restoreErr,
				),
			)
		}
		return catalogdomain.Provider{}, fmt.Errorf(
			"update MCP provider: save compatibility config: %w",
			err,
		)
	}

	request.ProviderID = providerID
	updatedProvider, err := r.Catalog.UpdateProvider.Execute(ctx, request)
	if err != nil {
		restoreCompatibilityErr := r.mcpConfigs.Save(ctx, currentConfig)
		restoreGenericErr := r.providerConfigs.Save(ctx, currentGeneric)
		return catalogdomain.Provider{}, errors.Join(
			fmt.Errorf("update MCP provider: metadata: %w", err),
			wrapOptionalError(
				"update MCP provider: restore compatibility config",
				restoreCompatibilityErr,
			),
			wrapOptionalError(
				"update MCP provider: restore provider config",
				restoreGenericErr,
			),
		)
	}

	return updatedProvider, nil
}

func (r *Runtime) DeleteMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	if err := r.ensureProviderUnused(ctx, providerID); err != nil {
		return err
	}

	compatConfig, compatErr := r.mcpConfigs.Get(ctx, providerID)
	hadCompatConfig := compatErr == nil
	if compatErr != nil && !errors.Is(compatErr, mcpconfigstore.ErrNotFound) {
		return fmt.Errorf("delete MCP provider: load compatibility config: %w", compatErr)
	}

	genericConfig, genericErr := r.providerConfigs.GetProviderConfig(ctx, providerID)
	hadGenericConfig := genericErr == nil
	if genericErr != nil && !errors.Is(genericErr, providerconfigstore.ErrNotFound) {
		return fmt.Errorf("delete MCP provider: load provider config: %w", genericErr)
	}

	if err := r.providerInstances.CloseProvider(providerID); err != nil {
		return fmt.Errorf("delete MCP provider: close active instance: %w", err)
	}

	if hadGenericConfig {
		if err := r.providerConfigs.Delete(ctx, providerID); err != nil {
			return fmt.Errorf("delete MCP provider: delete provider config: %w", err)
		}
	}
	if hadCompatConfig {
		if err := r.mcpConfigs.Delete(ctx, providerID); err != nil {
			restoreErr := error(nil)
			if hadGenericConfig {
				restoreErr = r.providerConfigs.Save(ctx, genericConfig)
			}
			return errors.Join(
				fmt.Errorf("delete MCP provider: delete compatibility config: %w", err),
				wrapOptionalError(
					"delete MCP provider: restore provider config",
					restoreErr,
				),
			)
		}
	}

	if err := r.Catalog.RemoveProvider.Execute(ctx, providerID); err != nil {
		var restoreGenericErr error
		if hadGenericConfig {
			restoreGenericErr = r.providerConfigs.Save(ctx, genericConfig)
		}
		var restoreCompatErr error
		if hadCompatConfig {
			restoreCompatErr = r.mcpConfigs.Save(ctx, compatConfig)
		}
		return errors.Join(
			fmt.Errorf("delete MCP provider: remove provider: %w", err),
			wrapOptionalError(
				"delete MCP provider: restore provider config",
				restoreGenericErr,
			),
			wrapOptionalError(
				"delete MCP provider: restore compatibility config",
				restoreCompatErr,
			),
		)
	}
	return nil
}

func wrapOptionalError(message string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}
