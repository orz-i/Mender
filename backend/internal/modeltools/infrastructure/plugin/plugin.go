package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

type engineFactory func(Config) (EngineAdapter, error)

// Plugin is Mender's built-in Model Tool Bridge Provider plugin.
type Plugin struct {
	newEngine engineFactory
}

// New constructs the built-in Model Tool Bridge Provider plugin.
func New() *Plugin {
	return &Plugin{newEngine: defaultEngineFactory}
}

// Descriptor implements providerplugin.Plugin.
func (*Plugin) Descriptor() providerplugin.Descriptor {
	return Descriptor()
}

// ValidateConfig implements providerplugin.Plugin.
func (*Plugin) ValidateConfig(
	ctx context.Context,
	raw json.RawMessage,
) error {
	return ValidateConfig(ctx, raw)
}

// InspectConfig implements providerplugin.Plugin.
func (*Plugin) InspectConfig(
	ctx context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	return InspectConfig(ctx, raw)
}

// Open creates a static capability snapshot without resolving Credentials or
// making provider API requests.
func (p *Plugin) Open(
	_ context.Context,
	request providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	if request.ConfigVersion != ConfigVersion {
		return nil, fmt.Errorf(
			"open model tools plugin: unsupported config version %d",
			request.ConfigVersion,
		)
	}
	config, err := DecodeConfig(request.Config)
	if err != nil {
		return nil, fmt.Errorf("open model tools plugin: %w", err)
	}

	factory := p.newEngine
	if factory == nil {
		factory = defaultEngineFactory
	}
	engine, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("open model tools plugin: %w", err)
	}
	if engine == nil {
		return nil, fmt.Errorf("open model tools plugin: engine factory returned nil")
	}

	return &Instance{
		config: config,
		engine: engine,
		tools:  discoveredCapabilityTools(config),
	}, nil
}

func defaultEngineFactory(config Config) (EngineAdapter, error) {
	switch config.Engine {
	case EngineOpenAI:
		return newOpenAIEngine(config.Model), nil
	case EngineAnthropic:
		return newAnthropicEngine(config.Model), nil
	case EngineGemini:
		return newGeminiEngine(config.Model), nil
	default:
		return nil, fmt.Errorf("unsupported model tools engine %q", config.Engine)
	}
}

// Instance is one configured engine plus static normalized capabilities.
type Instance struct {
	config Config
	engine EngineAdapter
	tools  []catalogapplication.DiscoveredTool
}

// DiscoverTools implements catalog ToolSource.
func (i *Instance) DiscoverTools(
	_ context.Context,
	_ catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	if i == nil {
		return nil, fmt.Errorf("discover model tools: instance is nil")
	}
	result := make([]catalogapplication.DiscoveredTool, 0, len(i.tools))
	for _, tool := range i.tools {
		result = append(result, catalogapplication.DiscoveredTool{
			SourceKey:    tool.SourceKey,
			Name:         tool.Name,
			Title:        tool.Title,
			Description:  tool.Description,
			InputSchema:  append([]byte(nil), tool.InputSchema...),
			OutputSchema: append([]byte(nil), tool.OutputSchema...),
			Tags:         append([]string(nil), tool.Tags...),
		})
	}
	return result, nil
}

// Execute implements Invocation ToolExecutor.
func (i *Instance) Execute(
	ctx context.Context,
	request invocationapplication.ExecutionRequest,
	credentials invocationapplication.CredentialResolver,
	_ invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	if i == nil || i.engine == nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute model tool: instance is unavailable",
		)
	}
	if request.Tool.SourceKey() != string(CapabilityWebSearch) ||
		!i.config.HasCapability(CapabilityWebSearch) {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute model tool %q: capability is unavailable",
			request.Tool.SourceKey(),
		)
	}

	searchRequest, err := decodeWebSearchRequest(request.Arguments.Bytes())
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	if credentials == nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute model tool: credential resolver is unavailable",
		)
	}
	secret, err := credentials.Resolve(ctx, i.config.CredentialRef)
	if err != nil {
		return invocationdomain.Outcome{}, err
	}
	defer zeroBytes(secret)

	result, err := i.engine.WebSearch(ctx, searchRequest, secret)
	if err != nil {
		var toolErr ToolError
		if errors.As(err, &toolErr) {
			return invocationdomain.Failed(
				invocationdomain.FailureSourceTool,
				invocationdomain.FailureToolError,
				toolErr.Error(),
			)
		}
		return invocationdomain.Outcome{}, err
	}

	raw, err := result.JSON()
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute model tool: normalize result: %w",
			err,
		)
	}
	value, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute model tool: canonical JSON result: %w",
			err,
		)
	}
	return invocationdomain.Complete(value)
}

// Close releases engine transport resources.
func (i *Instance) Close() error {
	if i == nil || i.engine == nil {
		return nil
	}
	return i.engine.Close()
}

func decodeWebSearchRequest(raw []byte) (WebSearchRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input struct {
		Query string `json:"query"`
	}
	if err := decoder.Decode(&input); err != nil {
		return WebSearchRequest{}, fmt.Errorf(
			"execute model tool: decode web_search arguments: %w",
			err,
		)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return WebSearchRequest{}, fmt.Errorf(
				"execute model tool: multiple argument JSON values",
			)
		}
		return WebSearchRequest{}, fmt.Errorf(
			"execute model tool: trailing argument data: %w",
			err,
		)
	}
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return WebSearchRequest{}, fmt.Errorf(
			"execute model tool: web_search query must not be empty",
		)
	}
	if len(query) > maxQueryBytes {
		return WebSearchRequest{}, fmt.Errorf(
			"execute model tool: web_search query exceeds %d bytes",
			maxQueryBytes,
		)
	}
	return WebSearchRequest{Query: query}, nil
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

var _ providerplugin.Plugin = (*Plugin)(nil)
var _ providerplugin.Instance = (*Instance)(nil)
