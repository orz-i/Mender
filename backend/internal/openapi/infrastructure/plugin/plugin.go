package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

// Plugin is Mender's built-in OpenAPI Provider plugin.
type Plugin struct {
	sources       sourceLoader
	newHTTPClient func() *http.Client
}

// New constructs the built-in OpenAPI Provider plugin.
func New() *Plugin {
	return &Plugin{
		sources:       newSourceLoader(),
		newHTTPClient: newExecutionHTTPClient,
	}
}

// Descriptor implements providerplugin.Plugin.
func (*Plugin) Descriptor() providerplugin.Descriptor {
	return Descriptor()
}

// ValidateConfig implements providerplugin.Plugin without source loading.
func (*Plugin) ValidateConfig(
	ctx context.Context,
	raw json.RawMessage,
) error {
	return ValidateConfig(ctx, raw)
}

// InspectConfig implements providerplugin.Plugin without source loading.
func (*Plugin) InspectConfig(
	ctx context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	return InspectConfig(ctx, raw)
}

// Open loads one OpenAPI description and builds an immutable operation index.
func (p *Plugin) Open(
	ctx context.Context,
	request providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	if request.ConfigVersion != ConfigVersion {
		return nil, fmt.Errorf(
			"open OpenAPI plugin: unsupported config version %d",
			request.ConfigVersion,
		)
	}
	config, err := DecodeConfig(request.Config)
	if err != nil {
		return nil, fmt.Errorf("open OpenAPI plugin: %w", err)
	}

	sources := p.sources
	if sources.httpClient == nil {
		sources = newSourceLoader()
	}
	parsed, err := loadOpenAPIDocument(ctx, sources, config.Source)
	if err != nil {
		return nil, fmt.Errorf("open OpenAPI plugin: %w", err)
	}
	bindings, err := discoverOperationBindings(parsed, config)
	if err != nil {
		return nil, fmt.Errorf("open OpenAPI plugin: %w", err)
	}

	bySourceKey := make(map[string]operationBinding, len(bindings))
	for _, binding := range bindings {
		bySourceKey[binding.sourceKey] = binding
	}

	newClient := p.newHTTPClient
	if newClient == nil {
		newClient = newExecutionHTTPClient
	}
	client := newClient()
	if client == nil {
		return nil, fmt.Errorf("open OpenAPI plugin: HTTP client factory returned nil")
	}

	return &Instance{
		bindings:    append([]operationBinding(nil), bindings...),
		bySourceKey: bySourceKey,
		client:      client,
	}, nil
}

// Instance is one immutable parsed OpenAPI Provider snapshot.
type Instance struct {
	bindings    []operationBinding
	bySourceKey map[string]operationBinding
	client      *http.Client
}

// DiscoverTools implements Catalog ToolSource.
func (i *Instance) DiscoverTools(
	_ context.Context,
	_ catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	if i == nil {
		return nil, fmt.Errorf("discover OpenAPI Tools: instance is nil")
	}
	tools := make([]catalogapplication.DiscoveredTool, 0, len(i.bindings))
	for _, binding := range i.bindings {
		tools = append(tools, catalogapplication.DiscoveredTool{
			SourceKey:    binding.sourceKey,
			Name:         binding.name,
			Title:        binding.title,
			Description:  binding.description,
			InputSchema:  append([]byte(nil), binding.inputSchema...),
			OutputSchema: append([]byte(nil), binding.outputSchema...),
			Tags:         append([]string(nil), binding.tags...),
		})
	}
	slices.SortFunc(tools, func(a, b catalogapplication.DiscoveredTool) int {
		return strings.Compare(a.SourceKey, b.SourceKey)
	})
	return tools, nil
}

// Execute implements Invocation ToolExecutor.
func (i *Instance) Execute(
	ctx context.Context,
	request invocationapplication.ExecutionRequest,
	credentials invocationapplication.CredentialResolver,
	_ invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	if i == nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI Tool: instance is nil",
		)
	}
	sourceKey := request.Tool.SourceKey()
	binding, exists := i.bySourceKey[sourceKey]
	if !exists {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI Tool %q: operation binding not found",
			sourceKey,
		)
	}

	httpRequest, err := buildHTTPRequest(
		ctx,
		binding,
		request.Arguments.Bytes(),
	)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI Tool %q: %w",
			sourceKey,
			err,
		)
	}
	if err := applyOperationSecurity(
		ctx,
		httpRequest,
		binding.security,
		credentials,
	); err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute OpenAPI Tool %q: %w",
			sourceKey,
			err,
		)
	}
	return executeHTTPRequest(ctx, i.client, httpRequest)
}

// Close releases idle HTTP connections owned by the instance.
func (i *Instance) Close() error {
	if i == nil || i.client == nil {
		return nil
	}
	i.client.CloseIdleConnections()
	return nil
}

var _ providerplugin.Plugin = (*Plugin)(nil)
var _ providerplugin.Instance = (*Instance)(nil)
