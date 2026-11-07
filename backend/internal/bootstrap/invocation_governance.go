package bootstrap

import (
	"context"
	"errors"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	providerconfigstore "github.com/orz-i/Mender/backend/internal/providerplugin/configstore"
)

// invocationGovernanceFactExtractor routes reviewed built-in Provider fact
// extraction from persisted configuration without activating Provider instances.
type invocationGovernanceFactExtractor struct {
	configs *providerconfigstore.Store
	plugins *providerplugin.Registry
}

func newInvocationGovernanceFactExtractor(
	configs *providerconfigstore.Store,
	plugins *providerplugin.Registry,
) invocationapplication.GovernanceFactExtractor {
	return invocationGovernanceFactExtractor{configs: configs, plugins: plugins}
}

func (e invocationGovernanceFactExtractor) Extract(
	ctx context.Context,
	tool catalogdomain.Tool,
	arguments invocationdomain.JSONValue,
	_ governancedomain.Context,
) (invocationapplication.GovernanceFactExtraction, error) {
	// The current production composer is still Provider-v2. It must reject
	// future native/Agent sources until F4 installs their host-owned fact
	// extraction and exact pre-Policy placement in the single pipeline.
	if tool.SourceRef().Kind() != catalogdomain.SourceProvider {
		return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf("extract governance facts: native/Agent source executor not composed")
	}
	if e.configs == nil || e.plugins == nil {
		return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf(
			"extract governance facts: Provider governance routing is unavailable",
		)
	}

	config, err := e.configs.GetProviderConfig(ctx, tool.ProviderID())
	if errors.Is(err, providerconfigstore.ErrNotFound) {
		return invocationapplication.GovernanceFactExtraction{Facts: invocationdomain.UnknownGovernanceFacts()}, nil
	}
	if err != nil {
		return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf(
			"extract governance facts: load Provider config: %w",
			err,
		)
	}
	if err := config.Validate(); err != nil {
		return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf(
			"extract governance facts: invalid Provider config: %w",
			err,
		)
	}

	// Installed external plugins remain unknown by default. Policy facts are
	// security-sensitive host input and are not delegated across the plugin RPC.
	if config.InstallationID != nil {
		return invocationapplication.GovernanceFactExtraction{Facts: invocationdomain.UnknownGovernanceFacts()}, nil
	}

	plugin, err := e.plugins.Lookup(config.PluginID)
	if errors.Is(err, providerplugin.ErrNotFound) {
		return invocationapplication.GovernanceFactExtraction{Facts: invocationdomain.UnknownGovernanceFacts()}, nil
	}
	if err != nil {
		return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf(
			"extract governance facts: resolve built-in plugin: %w",
			err,
		)
	}
	extractor, ok := plugin.(providerplugin.GovernanceFactProvider)
	if !ok {
		return invocationapplication.GovernanceFactExtraction{Facts: invocationdomain.UnknownGovernanceFacts()}, nil
	}
	facts, err := extractor.ExtractGovernanceFacts(ctx, config.Config, tool, arguments)
	if err != nil {
		return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf(
			"extract governance facts for plugin %s: %w",
			config.PluginID,
			err,
		)
	}
	if err := facts.Validate(); err != nil {
		return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf(
			"extract governance facts for plugin %s: invalid facts: %w",
			config.PluginID,
			err,
		)
	}
	return invocationapplication.GovernanceFactExtraction{Facts: facts}, nil
}

var _ invocationapplication.GovernanceFactExtractor = invocationGovernanceFactExtractor{}
