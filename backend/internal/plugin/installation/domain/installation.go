package domain

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	"github.com/orz-i/Mender/backend/internal/plugin/manifest"
)

const maxSourceReferenceBytes = 4096

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// SourceKind identifies where an immutable package was acquired.
type SourceKind string

const SourceLocal SourceKind = "local"

// Status is the administrative availability of a package installation.
type Status string

const (
	StatusEnabled  Status = "enabled"
	StatusDisabled Status = "disabled"
	StatusInvalid  Status = "invalid"
)

// Installation is an immutable native package, not a Provider/Sandbox/Tunnel resource.
type Installation struct {
	id                   InstallationID
	pluginID             platformplugin.ID
	packageVersion       string
	sourceKind           SourceKind
	sourceReference      string
	digest               string
	manifestJSON         []byte // normalized generic manifest v2
	artifactManifestName string // original file name in the byte-for-byte pinned package
	verifiedDescriptor   *platformplugin.Descriptor
	status               Status
	installedAt          time.Time
}

func NewInstallation(id InstallationID, pluginID platformplugin.ID, packageVersion string,
	sourceKind SourceKind, sourceReference, digest string, manifestJSON []byte,
	status Status, installedAt time.Time) (Installation, error) {
	if _, err := ParseInstallationID(id.String()); err != nil {
		return Installation{}, err
	}
	if _, err := platformplugin.ParseID(pluginID.String()); err != nil {
		return Installation{}, err
	}
	if sourceKind != SourceLocal {
		return Installation{}, fmt.Errorf("plugin installation: invalid source kind %q", sourceKind)
	}
	sourceReference = strings.TrimSpace(sourceReference)
	if sourceReference == "" || len(sourceReference) > maxSourceReferenceBytes {
		return Installation{}, fmt.Errorf("plugin installation: invalid source reference")
	}
	if !digestPattern.MatchString(digest) {
		return Installation{}, fmt.Errorf("plugin installation: invalid sha256 digest")
	}
	m, err := manifest.Parse(manifestJSON)
	if err != nil {
		return Installation{}, fmt.Errorf("plugin installation: manifest: %w", err)
	}
	if m.PluginID != pluginID.String() || m.Version != packageVersion {
		return Installation{}, fmt.Errorf("plugin installation: manifest identity/version mismatch")
	}
	if err := ValidateStatus(status); err != nil {
		return Installation{}, err
	}
	if installedAt.IsZero() {
		return Installation{}, fmt.Errorf("plugin installation: installedAt must not be zero")
	}
	return Installation{
		id: id, pluginID: pluginID, packageVersion: packageVersion,
		sourceKind: sourceKind, sourceReference: sourceReference, digest: digest,
		manifestJSON: bytes.Clone(manifestJSON), artifactManifestName: manifest.FileName,
		status: status, installedAt: installedAt.UTC(),
	}, nil
}

// WithArtifactManifestName restores the location of a migrated package's
// original manifest. This does NOT parse its legacy format during operations.
func (i Installation) WithArtifactManifestName(name string) (Installation, error) {
	if name != manifest.FileName && name != "mender-provider.json" {
		return Installation{}, fmt.Errorf("plugin installation: unsupported artifact manifest %q", name)
	}
	if name == "mender-provider.json" && i.Extension().Kind != platformplugin.ProviderTools {
		return Installation{}, fmt.Errorf("plugin installation: legacy artifact must be a Provider extension")
	}
	i.artifactManifestName = name
	return i, nil
}

func (i Installation) Validate() error {
	_, err := NewInstallation(i.id, i.pluginID, i.packageVersion, i.sourceKind,
		i.sourceReference, i.digest, i.manifestJSON, i.status, i.installedAt)
	if err != nil {
		return err
	}
	if i.artifactManifestName != manifest.FileName && i.artifactManifestName != "mender-provider.json" {
		return fmt.Errorf("plugin installation: invalid artifact manifest file")
	}
	if i.artifactManifestName == "mender-provider.json" && i.Extension().Kind != platformplugin.ProviderTools {
		return fmt.Errorf("plugin installation: invalid legacy extension kind")
	}
	if i.verifiedDescriptor != nil {
		if err := i.validateVerifiedDescriptor(*i.verifiedDescriptor); err != nil {
			return err
		}
	}
	return nil
}

func (i Installation) Manifest() (manifest.Manifest, error) { return manifest.Parse(i.manifestJSON) }

func (i Installation) Extension() platformplugin.Extension {
	m, err := i.Manifest()
	if err != nil {
		return platformplugin.Extension{}
	}
	return m.Extensions[0]
}

func (i Installation) WithVerifiedDescriptor(d platformplugin.Descriptor) (Installation, error) {
	d = d.Clone()
	if err := i.validateVerifiedDescriptor(d); err != nil {
		return Installation{}, err
	}
	i.verifiedDescriptor = &d
	return i, nil
}

func (i Installation) validateVerifiedDescriptor(d platformplugin.Descriptor) error {
	if err := d.Validate(); err != nil {
		return fmt.Errorf("verified plugin descriptor: %w", err)
	}
	if d.PluginID != i.pluginID || d.PackageVersion != i.packageVersion || d.Extension != i.Extension() {
		return fmt.Errorf("verified plugin descriptor: package or extension identity mismatch")
	}
	return nil
}

func ValidateStatus(status Status) error {
	switch status {
	case StatusEnabled, StatusDisabled, StatusInvalid:
		return nil
	default:
		return fmt.Errorf("plugin installation: invalid status %q", status)
	}
}

func (i Installation) WithStatus(status Status) (Installation, error) {
	if err := ValidateStatus(status); err != nil {
		return Installation{}, err
	}
	i.status = status
	return i, nil
}

func (i Installation) ID() InstallationID           { return i.id }
func (i Installation) PluginID() platformplugin.ID  { return i.pluginID }
func (i Installation) PackageVersion() string       { return i.packageVersion }
func (i Installation) SourceKind() SourceKind       { return i.sourceKind }
func (i Installation) SourceReference() string      { return i.sourceReference }
func (i Installation) Digest() string               { return i.digest }
func (i Installation) ManifestJSON() []byte         { return bytes.Clone(i.manifestJSON) }
func (i Installation) ArtifactManifestName() string { return i.artifactManifestName }
func (i Installation) VerifiedDescriptor() (platformplugin.Descriptor, bool) {
	if i.verifiedDescriptor == nil {
		return platformplugin.Descriptor{}, false
	}
	return i.verifiedDescriptor.Clone(), true
}
func (i Installation) Status() Status         { return i.status }
func (i Installation) InstalledAt() time.Time { return i.installedAt }
