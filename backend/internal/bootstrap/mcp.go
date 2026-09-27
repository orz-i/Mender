package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/headertemplate"
	mcpprovider "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/provider"
	mcpserver "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/server"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// RegisterMCPProvider persists canonical Provider metadata and adapter-owned
// connection configuration. Config failure is compensated by removing the newly
// created Provider.
func (r *Runtime) RegisterMCPProvider(
	ctx context.Context,
	request catalogapplication.RegisterProviderRequest,
	config mcpconfigstore.Config,
) (catalogdomain.Provider, error) {
	provider, err := r.Catalog.RegisterProvider.Execute(ctx, request)
	if err != nil {
		return catalogdomain.Provider{}, err
	}

	config.ProviderID = provider.ID()
	if err := r.mcpConfigs.Save(ctx, config); err != nil {
		removeErr := r.Catalog.RemoveProvider.Execute(ctx, provider.ID())
		if removeErr != nil {
			return catalogdomain.Provider{}, errors.Join(
				fmt.Errorf("register MCP provider: save config: %w", err),
				fmt.Errorf("register MCP provider: compensate provider removal: %w", removeErr),
			)
		}
		return catalogdomain.Provider{}, fmt.Errorf("register MCP provider: save config: %w", err)
	}

	return provider, nil
}

// MCPProviderConfig returns persisted adapter configuration for one Provider.
func (r *Runtime) MCPProviderConfig(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (mcpconfigstore.Config, error) {
	return r.mcpConfigs.Get(ctx, providerID)
}

// ListMCPProviderConfigs returns all persisted MCP Provider configurations.
func (r *Runtime) ListMCPProviderConfigs(
	ctx context.Context,
) ([]mcpconfigstore.Config, error) {
	return r.mcpConfigs.List(ctx)
}

// ConnectConfiguredMCPProvider connects a persisted Provider configuration.
//
// synchronize=false registers an executor without refreshing Catalog metadata.
// This is used by tool call and serve startup paths.
func (r *Runtime) ConnectConfiguredMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	synchronize bool,
) (catalogapplication.SyncProviderToolsResult, error) {
	config, err := r.mcpConfigs.Get(ctx, providerID)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("connect configured MCP provider: %w", err)
	}

	connection, err := r.connectStoredMCP(ctx, config)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}

	result, err := r.activateMCPProvider(ctx, providerID, connection, synchronize)
	if err != nil {
		_ = connection.Close()
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return result, nil
}

// SyncConfiguredMCPProvider connects and refreshes one Provider's Catalog snapshot.
func (r *Runtime) SyncConfiguredMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (catalogapplication.SyncProviderToolsResult, error) {
	return r.ConnectConfiguredMCPProvider(ctx, providerID, true)
}

// ActivateConfiguredMCPProvider connects one Provider for execution without resynchronizing.
func (r *Runtime) ActivateConfiguredMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	_, err := r.ConnectConfiguredMCPProvider(ctx, providerID, false)
	return err
}

func (r *Runtime) connectStoredMCP(
	ctx context.Context,
	config mcpconfigstore.Config,
) (*mcpprovider.Connection, error) {
	switch config.Transport {
	case mcpconfigstore.TransportHTTP:
		headers, err := headertemplate.Resolve(config.Headers, os.LookupEnv)
		if err != nil {
			return nil, fmt.Errorf("connect configured MCP HTTP provider: resolve headers: %w", err)
		}
		for _, binding := range config.CredentialHeaders {
			secret, err := r.Credentials.Resolver.Resolve(ctx, binding.Reference)
			if err != nil {
				return nil, fmt.Errorf(
					"connect configured MCP HTTP provider: resolve credential %s: %w",
					binding.Reference,
					err,
				)
			}
			value := string(secret)
			for i := range secret {
				secret[i] = 0
			}
			if strings.ContainsAny(value, "\r\n\x00") {
				return nil, fmt.Errorf(
					"connect configured MCP HTTP provider: credential header %q contains an invalid value",
					binding.Name,
				)
			}
			if strings.TrimSpace(binding.Scheme) != "" {
				value = strings.TrimSpace(binding.Scheme) + " " + value
			}
			headers[strings.TrimSpace(binding.Name)] = value
		}
		connection, err := mcpprovider.ConnectHTTP(ctx, mcpprovider.HTTPConfig{
			Endpoint:            config.Endpoint,
			Headers:             headers,
			RequiredCredentials: config.RequiredCredentialRefs(),
		})
		if err != nil {
			return nil, fmt.Errorf("connect configured MCP HTTP provider: %w", err)
		}
		return connection, nil

	case mcpconfigstore.TransportStdio:
		environment := make(map[string]string, len(config.CredentialEnvironment))
		for _, binding := range config.CredentialEnvironment {
			secret, err := r.Credentials.Resolver.Resolve(ctx, binding.Reference)
			if err != nil {
				return nil, fmt.Errorf(
					"connect configured MCP stdio provider: resolve credential %s: %w",
					binding.Reference,
					err,
				)
			}
			value := string(secret)
			for i := range secret {
				secret[i] = 0
			}
			if strings.ContainsRune(value, 0) {
				return nil, fmt.Errorf(
					"connect configured MCP stdio provider: credential environment %q contains an invalid value",
					binding.Name,
				)
			}
			environment[strings.TrimSpace(binding.Name)] = value
		}
		connection, err := mcpprovider.ConnectCommand(ctx, mcpprovider.CommandConfig{
			Executable:          config.Executable,
			Args:                append([]string(nil), config.Args...),
			Directory:           config.Directory,
			Environment:         environment,
			RequiredCredentials: config.RequiredCredentialRefs(),
		})
		if err != nil {
			return nil, fmt.Errorf("connect configured MCP stdio provider: %w", err)
		}
		return connection, nil

	default:
		return nil, fmt.Errorf("connect configured MCP provider: unsupported transport %q", config.Transport)
	}
}

// ConnectMCPHTTP connects, synchronizes, and registers a remote MCP Provider.
// On success the Runtime owns the connection and closes it with Runtime.Close.
func (r *Runtime) ConnectMCPHTTP(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	config mcpprovider.HTTPConfig,
) (catalogapplication.SyncProviderToolsResult, error) {
	connection, err := mcpprovider.ConnectHTTP(ctx, config)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}

	result, err := r.activateMCPProvider(ctx, providerID, connection, true)
	if err != nil {
		_ = connection.Close()
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return result, nil
}

// ConnectMCPCommand starts, synchronizes, and registers a local stdio MCP Provider.
// On success the Runtime owns the process connection and closes it with Runtime.Close.
func (r *Runtime) ConnectMCPCommand(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	config mcpprovider.CommandConfig,
) (catalogapplication.SyncProviderToolsResult, error) {
	connection, err := mcpprovider.ConnectCommand(ctx, config)
	if err != nil {
		return catalogapplication.SyncProviderToolsResult{}, err
	}

	result, err := r.activateMCPProvider(ctx, providerID, connection, true)
	if err != nil {
		_ = connection.Close()
		return catalogapplication.SyncProviderToolsResult{}, err
	}
	return result, nil
}

// AttachMCPProvider synchronizes and registers an already-connected MCP Provider.
//
// This is primarily useful for embedded/in-memory transports. Ownership transfers
// to Runtime only after successful synchronization and executor registration.
func (r *Runtime) AttachMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	connection *mcpprovider.Connection,
) (catalogapplication.SyncProviderToolsResult, error) {
	if connection == nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("attach MCP provider: connection must not be nil")
	}
	return r.activateMCPProvider(ctx, providerID, connection, true)
}

func (r *Runtime) activateMCPProvider(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
	connection *mcpprovider.Connection,
	synchronize bool,
) (catalogapplication.SyncProviderToolsResult, error) {
	if _, err := r.Catalog.GetProvider.Execute(ctx, providerID); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("activate MCP provider: provider: %w", err)
	}

	var result catalogapplication.SyncProviderToolsResult
	if synchronize {
		syncTools := r.Catalog.SyncProviderTools(connection)
		var err error
		result, err = syncTools.Execute(ctx, providerID)
		if err != nil {
			return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("activate MCP provider: synchronize: %w", err)
		}
	}

	if err := r.RegisterExecutor(providerID, connection); err != nil {
		return catalogapplication.SyncProviderToolsResult{}, fmt.Errorf("activate MCP provider: register executor: %w", err)
	}

	r.addCloser(connection)
	return result, nil
}

// NewMCPServer builds a Profile-scoped downstream MCP server backed by the
// existing Catalog queries and shared Invocation pipeline.
func (r *Runtime) NewMCPServer(
	ctx context.Context,
	profile profiledomain.Profile,
) (*mcpserver.Adapter, error) {
	return mcpserver.New(ctx, profile, mcpserver.Dependencies{
		DescribeTool: r.Catalog.DescribeTool.Execute,
		GetProvider:  r.Catalog.GetProvider.Execute,
		SearchTools:  r.Catalog.SearchTools.Execute,
		InvokeTool:   r.InvokeTool.Execute,
	})
}
