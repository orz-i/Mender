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

// invocationGovernanceFactExtractor routes Provider facts through reviewed
// configuration and native/Agent facts through exact, host-registered SourceRef
// executors. Neither path activates a Provider during preflight.
type invocationGovernanceFactExtractor struct {
	configs     *providerconfigstore.Store
	plugins     *providerplugin.Registry
	sourceFacts invocationapplication.GovernanceFactExtractor
}

func newInvocationGovernanceFactExtractor(
	configs *providerconfigstore.Store,
	plugins *providerplugin.Registry,
	sourceFacts invocationapplication.GovernanceFactExtractor,
) invocationapplication.GovernanceFactExtractor {
	return invocationGovernanceFactExtractor{configs: configs, plugins: plugins, sourceFacts: sourceFacts}
}

func (e invocationGovernanceFactExtractor) Extract(
	ctx context.Context,
	tool catalogdomain.Tool,
	arguments invocationdomain.JSONValue,
	caller governancedomain.Context,
) (invocationapplication.GovernanceFactExtraction, error) {
	if tool.SourceRef().Kind() != catalogdomain.SourceProvider {
		// No Provider plugin can declare the trusted Native/Agent facts port.
		// The same registered SourceRef executor must supply both facts and
		// execution. An unregistered source fails before Policy/approval.
		if e.sourceFacts == nil {
			return invocationapplication.GovernanceFactExtraction{}, fmt.Errorf("extract governance facts: source executor not composed")
		}
		return e.sourceFacts.Extract(ctx, tool, arguments, caller)
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
