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
	providerID   ProviderID
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

// NewTool constructs a Tool from source metadata.
func NewTool(
	id ToolID,
	providerID ProviderID,
	spec ToolSpec,
	now time.Time,
) (Tool, error) {
	if _, err := ParseToolID(id.String()); err != nil {
		return Tool{}, err
	}
	if _, err := ParseProviderID(providerID.String()); err != nil {
		return Tool{}, err
	}
	if err := validateToolSpec(spec); err != nil {
		return Tool{}, err
	}
	if now.IsZero() {
		return Tool{}, validationError("now", "must not be zero")
	}

	now = now.UTC()
	return toolFromParts(id, providerID, spec, now, now), nil
}

// RestoreTool rehydrates a persisted Tool while enforcing invariants.
func RestoreTool(
	id ToolID,
	providerID ProviderID,
	spec ToolSpec,
	createdAt time.Time,
	updatedAt time.Time,
) (Tool, error) {
	if _, err := ParseToolID(id.String()); err != nil {
		return Tool{}, err
	}
	if _, err := ParseProviderID(providerID.String()); err != nil {
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

	return toolFromParts(id, providerID, spec, createdAt.UTC(), updatedAt.UTC()), nil
}

// RefreshMetadata returns an updated Tool while preserving Mender identity,
// Provider identity, source identity, and creation time.
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

	return toolFromParts(t.id, t.providerID, spec, t.createdAt, now.UTC()), nil
}

func toolFromParts(
	id ToolID,
	providerID ProviderID,
	spec ToolSpec,
	createdAt time.Time,
	updatedAt time.Time,
) Tool {
	return Tool{
		id:           id,
		providerID:   providerID,
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

func (t Tool) ProviderID() ProviderID {
	return t.providerID
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
