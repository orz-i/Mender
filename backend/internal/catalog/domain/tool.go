package domain

import (
	"slices"
	"strings"
	"time"
)

const (
	maxSourceKeyLength       = 1024
	maxToolNameLength        = 255
	maxToolTitleLength       = 255
	maxToolDescriptionLength = 16384
	maxTagLength             = 64
	maxTags                  = 64
)

// Tool is a canonical executable capability known to Mender.
type Tool struct {
	id           ToolID
	source       SourceRef
	sourceKey    string
	name         string
	title        string
	description  string
	inputSchema  Schema
	outputSchema *Schema
	tags         []string
	createdAt    time.Time
	updatedAt    time.Time
}

// ToolSpec contains mutable source metadata for creating or refreshing a Tool.
type ToolSpec struct {
	SourceKey    string
	Name         string
	Title        string
	Description  string
	InputSchema  Schema
	OutputSchema *Schema
	Tags         []string
}

// NewTool is the current Provider-backed creation contract. The production
// Catalog still stores provider_id until the single cross-context hard cut.
func NewTool(id ToolID, providerID ProviderID, spec ToolSpec, now time.Time) (Tool, error) {
	source, err := ProviderSource(providerID)
	if err != nil {
		return Tool{}, err
	}
	return NewToolFromSource(id, source, spec, now)
}

// NewToolFromSource creates a Tool with exactly one verified provenance.
// Source identity does not grant execution or permit publication in a Profile.
func NewToolFromSource(id ToolID, source SourceRef, spec ToolSpec, now time.Time) (Tool, error) {
	if _, err := ParseToolID(id.String()); err != nil {
		return Tool{}, err
	}
	if err := source.Validate(); err != nil {
		return Tool{}, err
	}
	if err := validateToolSpec(spec); err != nil {
		return Tool{}, err
	}
	if now.IsZero() {
		return Tool{}, validationError("now", "must not be zero")
	}
	return toolFromParts(id, source, spec, now.UTC(), now.UTC()), nil
}

// RestoreTool rehydrates the existing Provider-only SQLite projection.
func RestoreTool(id ToolID, providerID ProviderID, spec ToolSpec, createdAt, updatedAt time.Time) (Tool, error) {
	source, err := ProviderSource(providerID)
	if err != nil {
		return Tool{}, err
	}
	return RestoreToolFromSource(id, source, spec, createdAt, updatedAt)
}

// RestoreToolFromSource validates a persisted Tool independent of Provider.
func RestoreToolFromSource(id ToolID, source SourceRef, spec ToolSpec, createdAt, updatedAt time.Time) (Tool, error) {
	if _, err := ParseToolID(id.String()); err != nil {
		return Tool{}, err
	}
	if err := source.Validate(); err != nil {
		return Tool{}, err
	}
	if err := validateToolSpec(spec); err != nil {
		return Tool{}, err
	}
	if createdAt.IsZero() || updatedAt.IsZero() {
		return Tool{}, validationError("timestamps", "must not be zero")
	}
	if updatedAt.Before(createdAt) {
		return Tool{}, validationError("updated_at", "must not be before created_at")
	}
	return toolFromParts(id, source, spec, createdAt.UTC(), updatedAt.UTC()), nil
}

// RefreshMetadata returns an updated Tool while preserving ToolID, exact
// SourceRef, SourceKey and creation time.
func (t Tool) RefreshMetadata(spec ToolSpec, now time.Time) (Tool, error) {
	if spec.SourceKey != t.sourceKey {
		return Tool{}, validationError("source_key", "cannot change when refreshing an existing tool")
	}
	if err := validateToolSpec(spec); err != nil {
		return Tool{}, err
	}
	if now.IsZero() {
		return Tool{}, validationError("now", "must not be zero")
	}
	if now.Before(t.createdAt) {
		return Tool{}, validationError("now", "must not be before created_at")
	}

	return toolFromParts(t.id, t.source, spec, t.createdAt, now.UTC()), nil
}

func toolFromParts(
	id ToolID,
	source SourceRef,
	spec ToolSpec,
	createdAt time.Time,
	updatedAt time.Time,
) Tool {
	return Tool{
		id:           id,
		source:       source,
		sourceKey:    strings.TrimSpace(spec.SourceKey),
		name:         strings.TrimSpace(spec.Name),
		title:        strings.TrimSpace(spec.Title),
		description:  strings.TrimSpace(spec.Description),
		inputSchema:  mustCloneSchema(spec.InputSchema),
		outputSchema: cloneOptionalSchema(spec.OutputSchema),
		tags:         normalizeTags(spec.Tags),
		createdAt:    createdAt,
		updatedAt:    updatedAt,
	}
}

func validateToolSpec(spec ToolSpec) error {
	sourceKey := strings.TrimSpace(spec.SourceKey)
	if sourceKey == "" {
		return validationError("source_key", "must not be empty")
	}
	if len(sourceKey) > maxSourceKeyLength {
		return validationError("source_key", "is too long")
	}

	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return validationError("name", "must not be empty")
	}
	if len(name) > maxToolNameLength {
		return validationError("name", "is too long")
	}
	if len(strings.TrimSpace(spec.Title)) > maxToolTitleLength {
		return validationError("title", "is too long")
	}
	if len(strings.TrimSpace(spec.Description)) > maxToolDescriptionLength {
		return validationError("description", "is too long")
	}
	if spec.InputSchema.IsZero() {
		return validationError("input_schema", "must be provided")
	}
	if len(spec.Tags) > maxTags {
		return validationError("tags", "contains too many entries")
	}
	for _, tag := range spec.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return validationError("tags", "must not contain empty values")
		}
		if len(tag) > maxTagLength {
			return validationError("tags", "contains a value that is too long")
		}
	}

	return nil
}

func mustCloneSchema(schema Schema) Schema {
	cloned, err := NewSchema(schema.Bytes())
	if err != nil {
		panic("catalog domain: cloning validated schema: " + err.Error())
	}
	return cloned
}

func normalizeTags(tags []string) []string {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, tag)
	}

	slices.SortFunc(normalized, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return normalized
}

func (t Tool) ID() ToolID {
	return t.id
}

// SourceRef returns the exact immutable Tool provenance; it is never a grant.
func (t Tool) SourceRef() SourceRef { return t.source }

// ProviderID is defined only for an actual external Provider source.
// Agent-owned and native Tools have no ProviderID and return the empty value;
// callers must use SourceRef for identity and governance.
func (t Tool) ProviderID() ProviderID {
	if t.source.Kind() != SourceProvider {
		return ""
	}
	return ProviderID(t.source.ID())
}

func (t Tool) SourceKey() string {
	return t.sourceKey
}

func (t Tool) Name() string {
	return t.name
}

func (t Tool) Title() string {
	return t.title
}

func (t Tool) Description() string {
	return t.description
}

func (t Tool) InputSchema() Schema {
	return mustCloneSchema(t.inputSchema)
}

func (t Tool) OutputSchema() *Schema {
	return cloneOptionalSchema(t.outputSchema)
}

func (t Tool) Tags() []string {
	return slices.Clone(t.tags)
}

func (t Tool) CreatedAt() time.Time {
	return t.createdAt
}

func (t Tool) UpdatedAt() time.Time {
	return t.updatedAt
}
