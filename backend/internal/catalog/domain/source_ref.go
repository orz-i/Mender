package domain

import (
	"fmt"
	"strings"
)

// SourceKind identifies the owner of a canonical Tool, not its execution target.
// A source is a verified identity value, never a new resource or grant.
type SourceKind string

const (
	SourceProvider SourceKind = "provider"
	SourceAgent    SourceKind = "agent"
	SourceNative   SourceKind = "native"
)

// SourceRef is the canonical provenance of one Tool. The legacy Catalog still
// persists provider_id until the atomic hard cut; this type does not make
// Agent/native Tools available for registration in the old execution pipeline.
type SourceRef struct {
	kind SourceKind
	id   string
}

func NewSourceRef(kind SourceKind, id string) (SourceRef, error) {
	switch kind {
	case SourceProvider:
		if _, err := ParseProviderID(id); err != nil {
			return SourceRef{}, fmt.Errorf("tool source provider: %w", err)
		}
	case SourceAgent:
		if err := validateOpaqueID(id, "agent_", "agent_id"); err != nil {
			return SourceRef{}, fmt.Errorf("tool source agent: %w", err)
		}
	case SourceNative:
		if id != "workspace" && id != "jobs" {
			return SourceRef{}, fmt.Errorf("tool source native: unsupported source %q", id)
		}
	default:
		return SourceRef{}, fmt.Errorf("tool source: unknown kind %q", kind)
	}
	return SourceRef{kind: kind, id: id}, nil
}

// ParseSourceRef accepts a strict canonical, unambiguous source identifier.
// IDs are not permissions; the runtime must independently verify ownership.
func ParseSourceRef(encoded string) (SourceRef, error) {
	kind, id, ok := strings.Cut(encoded, ":")
	if !ok {
		return SourceRef{}, fmt.Errorf("tool source: expected kind:id")
	}
	return NewSourceRef(SourceKind(kind), id)
}

func ProviderSource(id ProviderID) (SourceRef, error) {
	return NewSourceRef(SourceProvider, id.String())
}

func AgentSource(id string) (SourceRef, error) {
	return NewSourceRef(SourceAgent, id)
}

func NativeSource(id string) (SourceRef, error) {
	return NewSourceRef(SourceNative, id)
}

func (s SourceRef) Validate() error {
	_, err := NewSourceRef(s.kind, s.id)
	return err
}

func (s SourceRef) Kind() SourceKind { return s.kind }
func (s SourceRef) ID() string       { return s.id }
func (s SourceRef) String() string {
	if s.Validate() != nil {
		return ""
	}
	return string(s.kind) + ":" + s.id
}
