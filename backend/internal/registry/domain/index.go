package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	"golang.org/x/mod/semver"
)

var (
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	sha256Pattern = regexp.MustCompile(`^(sha256:)?[0-9a-f]{64}$`)
)

const (
	MaxPackageSizeBytes = 256 * 1024 * 1024 // 256MB cutoff
)

// ArtifactTarget represents a downloadable binary package artifact for a specific OS/Architecture.
type ArtifactTarget struct {
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	TreeDigest string `json:"treeDigest"`
	Size       int64  `json:"size"`
}

// SHA256Hex returns the 64-character lowercase hex string without sha256: prefix.
func (t ArtifactTarget) SHA256Hex() string {
	clean := strings.ToLower(strings.TrimSpace(t.SHA256))
	return strings.TrimPrefix(clean, "sha256:")
}

// Validate validates target fields.
func (t ArtifactTarget) Validate() error {
	trimmedURL := strings.TrimSpace(t.URL)
	if trimmedURL == "" {
		return fmt.Errorf("target: url must not be empty")
	}
	parsedURL, err := url.ParseRequestURI(trimmedURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf("target: invalid http/https url %q", t.URL)
	}
	if !sha256Pattern.MatchString(strings.ToLower(strings.TrimSpace(t.SHA256))) {
		return fmt.Errorf("target: invalid sha256 checksum %q", t.SHA256)
	}
	if !digestPattern.MatchString(strings.ToLower(strings.TrimSpace(t.TreeDigest))) {
		return fmt.Errorf("target: invalid tree digest %q", t.TreeDigest)
	}
	if t.Size <= 0 || t.Size > MaxPackageSizeBytes {
		return fmt.Errorf("target: size %d out of bounds (1..%d)", t.Size, MaxPackageSizeBytes)
	}
	return nil
}

// RegistryPluginEntry describes one available plugin offering in the registry catalog.
type RegistryPluginEntry struct {
	PluginID    string                    `json:"pluginId"`
	Version     string                    `json:"version"`
	DisplayName string                    `json:"displayName"`
	Description string                    `json:"description,omitempty"`
	Category    string                    `json:"category,omitempty"`
	Targets     map[string]ArtifactTarget `json:"targets"`
}

// Target returns the ArtifactTarget matching the requested platform (e.g. "linux/amd64").
func (e RegistryPluginEntry) Target(goos, goarch string) (ArtifactTarget, error) {
	key := goos + "/" + goarch
	target, ok := e.Targets[key]
	if !ok {
		return ArtifactTarget{}, fmt.Errorf("%w: platform %s not supported by %s@%s", ErrTargetNotFound, key, e.PluginID, e.Version)
	}
	return target, nil
}

// Validate validates the plugin entry fields.
func (e RegistryPluginEntry) Validate() error {
	if _, err := platformplugin.ParseID(e.PluginID); err != nil {
		return fmt.Errorf("plugin entry %q: %w", e.PluginID, err)
	}
	cleanVer := strings.TrimSpace(e.Version)
	if cleanVer == "" || !semver.IsValid("v"+cleanVer) {
		return fmt.Errorf("plugin entry %q: invalid semver %q", e.PluginID, e.Version)
	}
	if strings.TrimSpace(e.DisplayName) == "" {
		return fmt.Errorf("plugin entry %q: missing displayName", e.PluginID)
	}
	if len(e.Targets) == 0 {
		return fmt.Errorf("plugin entry %q: must specify at least one target", e.PluginID)
	}
	for platform, target := range e.Targets {
		if !strings.Contains(platform, "/") {
			return fmt.Errorf("plugin entry %q: target platform %q missing slash separator", e.PluginID, platform)
		}
		if err := target.Validate(); err != nil {
			return fmt.Errorf("plugin entry %q target %q: %w", e.PluginID, platform, err)
		}
	}
	return nil
}

// CatalogIndex represents the signed index metadata containing all registered plugin offerings.
type CatalogIndex struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Sequence      int64                 `json:"sequence"`
	IssuedAt      time.Time             `json:"issuedAt"`
	ExpiresAt     time.Time             `json:"expiresAt"`
	Plugins       []RegistryPluginEntry `json:"plugins"`
}

// ValidateFreshness performs anti-freeze validation: issuedAt <= now <= expiresAt.
func (idx CatalogIndex) ValidateFreshness(now time.Time) error {
	if idx.IssuedAt.IsZero() || idx.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: issuedAt and expiresAt must not be zero", ErrAntiFreeze)
	}
	if idx.ExpiresAt.Before(idx.IssuedAt) {
		return fmt.Errorf("%w: expiresAt (%s) is before issuedAt (%s)", ErrAntiFreeze, idx.ExpiresAt, idx.IssuedAt)
	}
	if now.Before(idx.IssuedAt) {
		return fmt.Errorf("%w: index issued in future (issuedAt: %s, now: %s)", ErrAntiFreeze, idx.IssuedAt.Format(time.RFC3339), now.Format(time.RFC3339))
	}
	if now.After(idx.ExpiresAt) {
		return fmt.Errorf("%w: index expired (expiresAt: %s, now: %s)", ErrAntiFreeze, idx.ExpiresAt.Format(time.RFC3339), now.Format(time.RFC3339))
	}
	return nil
}

// ValidateSequence performs anti-rollback validation: monotonic sequence check (idx.Sequence >= lastSequence).
func (idx CatalogIndex) ValidateSequence(lastSequence int64) error {
	if idx.Sequence < lastSequence {
		return fmt.Errorf("%w: sequence %d is older than previously recorded sequence %d", ErrAntiRollback, idx.Sequence, lastSequence)
	}
	return nil
}

// FindPlugin looks up a plugin by its PluginID in the catalog.
func (idx CatalogIndex) FindPlugin(pluginID string) (RegistryPluginEntry, bool) {
	for _, p := range idx.Plugins {
		if p.PluginID == pluginID {
			return p, true
		}
	}
	return RegistryPluginEntry{}, false
}

// CanonicalJSON serializes the index into RFC 8785 Canonical JSON bytes.
func (idx CatalogIndex) CanonicalJSON() ([]byte, error) {
	return CanonicalJSON(idx)
}

// Validate validates the catalog index.
func (idx CatalogIndex) Validate() error {
	if idx.SchemaVersion <= 0 {
		return fmt.Errorf("%w: invalid schemaVersion %d", ErrInvalidIndex, idx.SchemaVersion)
	}
	if idx.Sequence < 0 {
		return fmt.Errorf("%w: negative sequence %d", ErrInvalidIndex, idx.Sequence)
	}
	if idx.IssuedAt.IsZero() || idx.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: missing timestamps", ErrInvalidIndex)
	}
	if idx.ExpiresAt.Before(idx.IssuedAt) {
		return fmt.Errorf("%w: expiresAt before issuedAt", ErrInvalidIndex)
	}
	for _, p := range idx.Plugins {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidIndex, err)
		}
	}
	return nil
}
