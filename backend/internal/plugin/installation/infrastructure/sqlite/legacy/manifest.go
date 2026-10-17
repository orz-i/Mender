package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	"golang.org/x/mod/semver"
)

const (
	// FileName is the manifest filename at the root of an external plugin package.
	FileName            = "mender-provider.json"
	CurrentVersion      = 1
	maxManifestBytes    = 64 * 1024
	maxPackageVersion   = 128
	maxDisplayName      = 160
	maxDescription      = 4096
	maxProtocolVersions = 16
	maxEntrypoints      = 32
	maxEntrypointPath   = 1024
)

var platformPattern = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9_]+$`)

// Manifest is the bounded package metadata for one external Provider plugin.
type Manifest struct {
	ManifestVersion  int               `json:"manifestVersion"`
	PluginID         string            `json:"pluginId"`
	Version          string            `json:"version"`
	ProtocolVersions []uint32          `json:"protocolVersions"`
	DisplayName      string            `json:"displayName"`
	Description      string            `json:"description"`
	Entrypoints      map[string]string `json:"entrypoints"`
}

// Parse strict-decodes and validates one manifest document.
func Parse(raw []byte) (Manifest, error) {
	if len(raw) == 0 {
		return Manifest{}, fmt.Errorf("provider plugin manifest: must not be empty")
	}
	if len(raw) > maxManifestBytes {
		return Manifest{}, fmt.Errorf(
			"provider plugin manifest: must be at most %d bytes",
			maxManifestBytes,
		)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var result Manifest
	if err := decoder.Decode(&result); err != nil {
		return Manifest{}, fmt.Errorf("provider plugin manifest: decode: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Manifest{}, err
	}
	if err := result.Validate(); err != nil {
		return Manifest{}, err
	}
	return result, nil
}

// Validate checks manifest identity, compatibility, and entrypoint invariants.
func (m Manifest) Validate() error {
	if m.ManifestVersion != CurrentVersion {
		return fmt.Errorf(
			"provider plugin manifest: unsupported manifest version %d",
			m.ManifestVersion,
		)
	}
	if _, err := platformplugin.ParseID(m.PluginID); err != nil {
		return fmt.Errorf("provider plugin manifest: %w", err)
	}
	if err := validatePackageVersion(m.Version); err != nil {
		return err
	}
	if err := validateBoundedText("display name", m.DisplayName, maxDisplayName, true); err != nil {
		return err
	}
	if err := validateBoundedText("description", m.Description, maxDescription, false); err != nil {
		return err
	}
	if err := validateProtocolVersions(m.ProtocolVersions); err != nil {
		return err
	}
	if err := validateEntrypoints(m.Entrypoints); err != nil {
		return err
	}
	return nil
}

// Entrypoint returns the normalized manifest path for one OS/architecture pair.
func (m Manifest) Entrypoint(goos, goarch string) (string, error) {
	key := goos + "/" + goarch
	value, ok := m.Entrypoints[key]
	if !ok {
		return "", fmt.Errorf(
			"provider plugin manifest: no entrypoint for platform %s",
			key,
		)
	}
	if err := validateEntrypointPath(value); err != nil {
		return "", err
	}
	return value, nil
}

func validatePackageVersion(value string) error {
	if value == "" {
		return fmt.Errorf("provider plugin manifest: package version must not be empty")
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("provider plugin manifest: package version must not contain surrounding whitespace")
	}
	if len(value) > maxPackageVersion {
		return fmt.Errorf(
			"provider plugin manifest: package version must be at most %d characters",
			maxPackageVersion,
		)
	}
	if !semver.IsValid("v" + value) {
		return fmt.Errorf(
			"provider plugin manifest: package version %q is not valid SemVer",
			value,
		)
	}
	return nil
}

func validateBoundedText(
	field string,
	value string,
	limit int,
	required bool,
) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("provider plugin manifest: %s must be valid UTF-8", field)
	}
	trimmed := strings.TrimSpace(value)
	if required && trimmed == "" {
		return fmt.Errorf("provider plugin manifest: %s must not be empty", field)
	}
	if value != trimmed {
		return fmt.Errorf(
			"provider plugin manifest: %s must not contain surrounding whitespace",
			field,
		)
	}
	if len(value) > limit {
		return fmt.Errorf(
			"provider plugin manifest: %s must be at most %d bytes",
			field,
			limit,
		)
	}
	return nil
}

func validateProtocolVersions(versions []uint32) error {
	if len(versions) == 0 {
		return fmt.Errorf("provider plugin manifest: protocol versions must not be empty")
	}
	if len(versions) > maxProtocolVersions {
		return fmt.Errorf(
			"provider plugin manifest: protocol versions must contain at most %d entries",
			maxProtocolVersions,
		)
	}
	seen := make(map[uint32]struct{}, len(versions))
	for _, version := range versions {
		if version == 0 {
			return fmt.Errorf("provider plugin manifest: protocol version must be positive")
		}
		if _, exists := seen[version]; exists {
			return fmt.Errorf(
				"provider plugin manifest: duplicate protocol version %d",
				version,
			)
		}
		seen[version] = struct{}{}
	}
	return nil
}

func validateEntrypoints(entrypoints map[string]string) error {
	if len(entrypoints) == 0 {
		return fmt.Errorf("provider plugin manifest: entrypoints must not be empty")
	}
	if len(entrypoints) > maxEntrypoints {
		return fmt.Errorf(
			"provider plugin manifest: entrypoints must contain at most %d entries",
			maxEntrypoints,
		)
	}
	for platform, entrypoint := range entrypoints {
		if !platformPattern.MatchString(platform) {
			return fmt.Errorf(
				"provider plugin manifest: invalid platform %q",
				platform,
			)
		}
		if err := validateEntrypointPath(entrypoint); err != nil {
			return fmt.Errorf(
				"provider plugin manifest: platform %s: %w",
				platform,
				err,
			)
		}
	}
	return nil
}

func validateEntrypointPath(value string) error {
	if value == "" {
		return fmt.Errorf("entrypoint must not be empty")
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("entrypoint must be valid UTF-8")
	}
	if len(value) > maxEntrypointPath {
		return fmt.Errorf(
			"entrypoint must be at most %d bytes",
			maxEntrypointPath,
		)
	}
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("entrypoint must not contain NUL")
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("entrypoint must use forward slashes")
	}
	if path.IsAbs(value) {
		return fmt.Errorf("entrypoint must be relative")
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("entrypoint must remain inside the plugin package")
	}
	if clean != value {
		return fmt.Errorf("entrypoint must be normalized")
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("provider plugin manifest: decode trailing data: %w", err)
	}
	return fmt.Errorf("provider plugin manifest: must contain one JSON document")
}
