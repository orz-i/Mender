package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	"github.com/orz-i/Mender/backend/internal/providerplugin/manifest"
)

const (
	maxSourceReferenceBytes = 4096
	maxManifestBytes        = 64 * 1024
)

var digestPattern = regexp.MustCompile("^sha256:[0-9a-f]{64}$")

// SourceKind identifies where an installed plugin package came from.
type SourceKind string

const (
	SourceLocal SourceKind = "local"
)

// Status is the administrative availability of one plugin installation.
type Status string

const (
	StatusEnabled  Status = "enabled"
	StatusDisabled Status = "disabled"
	StatusInvalid  Status = "invalid"
)

// Installation is one immutable external Provider plugin package known to Mender.
type Installation struct {
	id              InstallationID
	pluginID        providerplugin.ID
	packageVersion  string
	sourceKind      SourceKind
	sourceReference string
	digest          string
	manifestJSON    []byte
	status          Status
	installedAt     time.Time
}

// NewInstallation constructs a validated installed plugin package record.
func NewInstallation(
	id InstallationID,
	pluginID providerplugin.ID,
	packageVersion string,
	sourceKind SourceKind,
	sourceReference string,
	digest string,
	manifestJSON []byte,
	status Status,
	installedAt time.Time,
) (Installation, error) {
	if _, err := ParseInstallationID(id.String()); err != nil {
		return Installation{}, err
	}
	if _, err := providerplugin.ParseID(pluginID.String()); err != nil {
		return Installation{}, err
	}
	if sourceKind != SourceLocal {
		return Installation{}, fmt.Errorf("plugin installation source kind: unsupported value %q", sourceKind)
	}
	sourceReference = strings.TrimSpace(sourceReference)
	if sourceReference == "" {
		return Installation{}, fmt.Errorf("plugin installation source reference: must not be empty")
	}
	if len(sourceReference) > maxSourceReferenceBytes {
		return Installation{}, fmt.Errorf(
			"plugin installation source reference: must be at most %d bytes",
			maxSourceReferenceBytes,
		)
	}
	if !digestPattern.MatchString(digest) {
		return Installation{}, fmt.Errorf("plugin installation digest: must be lowercase sha256")
	}
	if len(manifestJSON) == 0 || len(manifestJSON) > maxManifestBytes {
		return Installation{}, fmt.Errorf(
			"plugin installation manifest: must contain between 1 and %d bytes",
			maxManifestBytes,
		)
	}
	parsedManifest, err := manifest.Parse(manifestJSON)
	if err != nil {
		return Installation{}, fmt.Errorf("plugin installation manifest: %w", err)
	}
	if parsedManifest.PluginID != pluginID.String() {
		return Installation{}, fmt.Errorf(
			"plugin installation manifest: plugin id %s does not match %s",
			parsedManifest.PluginID,
			pluginID,
		)
	}
	if parsedManifest.Version != packageVersion {
		return Installation{}, fmt.Errorf(
			"plugin installation manifest: package version %s does not match %s",
			parsedManifest.Version,
			packageVersion,
		)
	}
	if err := ValidateStatus(status); err != nil {
		return Installation{}, err
	}
	if installedAt.IsZero() {
		return Installation{}, fmt.Errorf("plugin installation installed at: must not be zero")
	}

	return Installation{
		id:              id,
		pluginID:        pluginID,
		packageVersion:  packageVersion,
		sourceKind:      sourceKind,
		sourceReference: sourceReference,
		digest:          digest,
		manifestJSON:    append([]byte(nil), manifestJSON...),
		status:          status,
		installedAt:     installedAt.UTC(),
	}, nil
}

// Validate checks the complete persisted installation invariant set.
func (i Installation) Validate() error {
	_, err := NewInstallation(
		i.id,
		i.pluginID,
		i.packageVersion,
		i.sourceKind,
		i.sourceReference,
		i.digest,
		i.manifestJSON,
		i.status,
		i.installedAt,
	)
	return err
}

// ValidateStatus checks supported administrative installation states.
func ValidateStatus(status Status) error {
	switch status {
	case StatusEnabled, StatusDisabled, StatusInvalid:
		return nil
	default:
		return fmt.Errorf("plugin installation status: unsupported value %q", status)
	}
}

// WithStatus returns the same immutable installation package with new administrative status.
func (i Installation) WithStatus(status Status) (Installation, error) {
	if err := ValidateStatus(status); err != nil {
		return Installation{}, err
	}
	i.status = status
	return i, nil
}

func (i Installation) ID() InstallationID {
	return i.id
}

func (i Installation) PluginID() providerplugin.ID {
	return i.pluginID
}

func (i Installation) PackageVersion() string {
	return i.packageVersion
}

func (i Installation) SourceKind() SourceKind {
	return i.sourceKind
}

func (i Installation) SourceReference() string {
	return i.sourceReference
}

func (i Installation) Digest() string {
	return i.digest
}

func (i Installation) ManifestJSON() []byte {
	return append([]byte(nil), i.manifestJSON...)
}

func (i Installation) Status() Status {
	return i.status
}

func (i Installation) InstalledAt() time.Time {
	return i.installedAt
}
