package application

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

// ProviderToolRepository owns the current Tool snapshot for one Provider.
type ProviderToolRepository interface {
	ListProviderTools(context.Context, domain.ProviderID) ([]domain.Tool, error)
	ReplaceProviderTools(context.Context, domain.ProviderID, []domain.Tool) error
}

// ToolIDGenerator allocates opaque Tool identities.
type ToolIDGenerator interface {
	NewToolID() (domain.ToolID, error)
}

// ToolSource discovers one Provider's current source metadata.
type ToolSource interface {
	DiscoverTools(context.Context, domain.Provider) ([]DiscoveredTool, error)
}

// DiscoveredTool is protocol-independent metadata returned by a ToolSource.
type DiscoveredTool struct {
	SourceKey    string
	Name         string
	Title        string
	Description  string
	InputSchema  []byte
	OutputSchema []byte
	Tags         []string
}

// SyncProviderToolsResult summarizes one snapshot replacement.
type SyncProviderToolsResult struct {
	Created int
	Updated int
	Removed int
	Total   int
}

// SyncProviderTools synchronizes one Provider's source snapshot into Catalog.
type SyncProviderTools struct {
	providers ProviderGetter
	tools     ProviderToolRepository
	source    ToolSource
	ids       ToolIDGenerator
	clock     Clock
}

func NewSyncProviderTools(
	providers ProviderGetter,
	tools ProviderToolRepository,
	source ToolSource,
	ids ToolIDGenerator,
	clock Clock,
) SyncProviderTools {
	return SyncProviderTools{
		providers: providers,
		tools:     tools,
		source:    source,
		ids:       ids,
		clock:     clock,
	}
}

func (uc SyncProviderTools) Execute(
	ctx context.Context,
	providerID domain.ProviderID,
) (SyncProviderToolsResult, error) {
	provider, err := uc.providers.GetProvider(ctx, providerID)
	if err != nil {
		return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: %w", err)
	}

	discovered, err := uc.source.DiscoverTools(ctx, provider)
	if err != nil {
		return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: discover: %w", err)
	}

	current, err := uc.tools.ListProviderTools(ctx, providerID)
	if err != nil {
		return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: load current snapshot: %w", err)
	}

	slices.SortFunc(discovered, func(a, b DiscoveredTool) int {
		return strings.Compare(strings.TrimSpace(a.SourceKey), strings.TrimSpace(b.SourceKey))
	})

	existingByKey := make(map[string]domain.Tool, len(current))
	for _, tool := range current {
		existingByKey[tool.SourceKey()] = tool
	}

	seen := make(map[string]struct{}, len(discovered))
	for _, item := range discovered {
		sourceKey := strings.TrimSpace(item.SourceKey)
		if _, duplicate := seen[sourceKey]; duplicate {
			return SyncProviderToolsResult{}, fmt.Errorf("%w: %q", ErrDuplicateSourceKey, sourceKey)
		}
		seen[sourceKey] = struct{}{}
	}

	now := uc.clock.Now()
	next := make([]domain.Tool, 0, len(discovered))
	result := SyncProviderToolsResult{Total: len(discovered)}

	for _, item := range discovered {
		sourceKey := strings.TrimSpace(item.SourceKey)

		spec, specErr := discoveredToolSpec(item)
		if specErr != nil {
			return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: %q: %w", sourceKey, specErr)
		}

		if existing, exists := existingByKey[sourceKey]; exists {
			refreshed, refreshErr := existing.RefreshMetadata(spec, now)
			if refreshErr != nil {
				return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: refresh %q: %w", sourceKey, refreshErr)
			}
			next = append(next, refreshed)
			result.Updated++
			continue
		}

		id, idErr := uc.ids.NewToolID()
		if idErr != nil {
			return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: allocate tool id: %w", idErr)
		}

		tool, toolErr := domain.NewTool(id, providerID, spec, now)
		if toolErr != nil {
			return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: create %q: %w", sourceKey, toolErr)
		}
		next = append(next, tool)
		result.Created++
	}

	for sourceKey := range existingByKey {
		if _, retained := seen[sourceKey]; !retained {
			result.Removed++
		}
	}

	if err := uc.tools.ReplaceProviderTools(ctx, providerID, next); err != nil {
		return SyncProviderToolsResult{}, fmt.Errorf("sync provider tools: replace snapshot: %w", err)
	}

	return result, nil
}

func discoveredToolSpec(item DiscoveredTool) (domain.ToolSpec, error) {
	input, err := domain.NewSchema(item.InputSchema)
	if err != nil {
		return domain.ToolSpec{}, fmt.Errorf("input schema: %w", err)
	}

	var output *domain.Schema
	if len(item.OutputSchema) > 0 {
		schema, outputErr := domain.NewSchema(item.OutputSchema)
		if outputErr != nil {
			return domain.ToolSpec{}, fmt.Errorf("output schema: %w", outputErr)
		}
		output = &schema
	}

	return domain.ToolSpec{
		SourceKey:    item.SourceKey,
		Name:         item.Name,
		Title:        item.Title,
		Description:  item.Description,
		InputSchema:  input,
		OutputSchema: output,
		Tags:         slices.Clone(item.Tags),
	}, nil
}
