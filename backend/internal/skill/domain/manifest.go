package domain

// Metadata is the portable Agent Skills metadata extracted from SKILL.md.
//
// The original SKILL.md remains the canonical content resource. Metadata is a
// bounded projection used for discovery, validation, and management.
type Metadata struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Values        map[string]string
	AllowedTools  string
}

// Clone returns a deep copy safe for crossing application boundaries.
func (m Metadata) Clone() Metadata {
	values := make(map[string]string, len(m.Values))
	for key, value := range m.Values {
		values[key] = value
	}
	m.Values = values
	return m
}

// ResourceKind classifies one file using Agent Skills directory conventions.
type ResourceKind string

const (
	ResourceManifest  ResourceKind = "manifest"
	ResourceScript    ResourceKind = "script"
	ResourceReference ResourceKind = "reference"
	ResourceAsset     ResourceKind = "asset"
	ResourceOther     ResourceKind = "other"
)

// Resource is one regular file in an exact Skill content snapshot.
type Resource struct {
	Path       string
	Kind       ResourceKind
	Size       int64
	SHA256     string
	Executable bool
}

// Snapshot is a validated, side-effect-free projection of one exact Skill
// directory.
//
// Digest is deterministic over normalized relative paths and every resource's
// content hash, size, classification, and executable hint.
type Snapshot struct {
	Metadata   Metadata
	Resources  []Resource
	Digest     string
	TotalBytes int64
}

// Clone returns a deep copy safe for storage/application boundaries.
func (s Snapshot) Clone() Snapshot {
	s.Metadata = s.Metadata.Clone()
	s.Resources = append([]Resource(nil), s.Resources...)
	return s
}
