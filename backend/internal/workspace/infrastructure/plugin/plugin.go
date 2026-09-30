package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"unicode/utf8"

	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

type Plugin struct{}

func New() *Plugin {
	return &Plugin{}
}

func (*Plugin) Descriptor() providerplugin.Descriptor {
	return Descriptor()
}

func (*Plugin) ValidateConfig(ctx context.Context, raw json.RawMessage) error {
	return ValidateConfig(ctx, raw)
}

func (*Plugin) InspectConfig(
	ctx context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	return InspectConfig(ctx, raw)
}

func (*Plugin) Open(
	_ context.Context,
	request providerplugin.OpenRequest,
) (providerplugin.Instance, error) {
	if request.ConfigVersion != ConfigVersion {
		return nil, fmt.Errorf(
			"open Workspace plugin: unsupported config version %d",
			request.ConfigVersion,
		)
	}
	config, err := DecodeConfig(request.Config)
	if err != nil {
		return nil, fmt.Errorf("open Workspace plugin: %w", err)
	}
	root, err := config.CanonicalRoot()
	if err != nil {
		return nil, fmt.Errorf("open Workspace plugin: %w", err)
	}
	resolver, err := newWorkspacePathResolver(root)
	if err != nil {
		return nil, fmt.Errorf("open Workspace plugin: %w", err)
	}
	return &Instance{
		root:     root,
		resolver: resolver,
		tools:    discoveredFileTools(),
	}, nil
}

type Instance struct {
	root     string
	resolver workspacePathResolver
	tools    []catalogapplication.DiscoveredTool
}

func (i *Instance) DiscoverTools(
	_ context.Context,
	_ catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	if i == nil {
		return nil, fmt.Errorf("discover Workspace Tools: instance is nil")
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
			Tags:         slices.Clone(tool.Tags),
		})
	}
	return result, nil
}

func (i *Instance) Execute(
	ctx context.Context,
	request invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver,
	_ invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	if i == nil {
		return invocationdomain.Outcome{}, fmt.Errorf("execute Workspace Tool: instance is nil")
	}
	switch request.Tool.SourceKey() {
	case sourceKeyRead:
		return i.executeRead(ctx, request.Arguments)
	case sourceKeySearch:
		return i.executeSearch(ctx, request.Arguments)
	case sourceKeyEdit:
		return i.executeEdit(ctx, request.Arguments)
	default:
		return invocationdomain.Outcome{}, fmt.Errorf(
			"execute Workspace Tool %q: capability is unavailable",
			request.Tool.SourceKey(),
		)
	}
}

func (*Instance) Close() error {
	return nil
}

const (
	sourceKeyRead   = "workspace.read"
	sourceKeySearch = "workspace.search"
	sourceKeyEdit   = "workspace.edit"
)

func discoveredFileTools() []catalogapplication.DiscoveredTool {
	return []catalogapplication.DiscoveredTool{
		{
			SourceKey:    sourceKeyRead,
			Name:         "read",
			Title:        "Read workspace file",
			Description:  "Read a bounded range of lines from one text file inside the configured workspace.",
			InputSchema:  readInputSchema,
			OutputSchema: readOutputSchema,
			Tags:         []string{"workspace", "filesystem", "read"},
		},
		{
			SourceKey:    sourceKeySearch,
			Name:         "search",
			Title:        "Search workspace",
			Description:  "Find workspace files or search bounded text content without invoking a shell.",
			InputSchema:  searchInputSchema,
			OutputSchema: searchOutputSchema,
			Tags:         []string{"workspace", "filesystem", "search"},
		},
		{
			SourceKey:    sourceKeyEdit,
			Name:         "edit",
			Title:        "Edit workspace file",
			Description:  "Create, deterministically replace text in, or delete one regular file inside the workspace.",
			InputSchema:  editInputSchema,
			OutputSchema: editOutputSchema,
			Tags:         []string{"workspace", "filesystem", "write"},
		},
	}
}

func completeJSON(value any) (invocationdomain.Outcome, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("encode Workspace Tool result: %w", err)
	}
	content, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("normalize Workspace Tool result: %w", err)
	}
	return invocationdomain.Complete(content)
}

func toolFailure(err error) (invocationdomain.Outcome, error) {
	if err == nil {
		err = fmt.Errorf("workspace tool failed")
	}
	message := err.Error()
	const maxToolFailureRunes = 1800
	if utf8.RuneCountInString(message) > maxToolFailureRunes {
		runes := []rune(message)
		message = string(runes[:maxToolFailureRunes]) + "..."
	}
	return invocationdomain.Failed(
		invocationdomain.FailureSourceTool,
		invocationdomain.FailureToolError,
		message,
	)
}

func decodeStrict(raw []byte, target any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s: multiple JSON values", label)
		}
		return fmt.Errorf("%s: trailing data: %w", label, err)
	}
	return nil
}

var _ providerplugin.Plugin = (*Plugin)(nil)
var _ providerplugin.Instance = (*Instance)(nil)
