// Package manifest validates new generic Mender plugin packages. The v1
// Provider-only manifest belongs to the one-time installation migration.
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

	"github.com/orz-i/Mender/backend/internal/plugin"
	"golang.org/x/mod/semver"
)

const (
	FileName         = "mender-plugin.json"
	CurrentVersion   = 2
	MaxManifestBytes = 64 * 1024
)

var platformPattern = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9_]+$`)

type Manifest struct {
	ManifestVersion int                `json:"manifestVersion"`
	PluginID        string             `json:"pluginId"`
	Version         string             `json:"version"`
	DisplayName     string             `json:"displayName"`
	Description     string             `json:"description,omitempty"`
	Entrypoints     map[string]string  `json:"entrypoints"`
	Extensions      []plugin.Extension `json:"extensions"`
}

func Parse(raw []byte) (Manifest, error) {
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("plugin manifest: empty or oversized document")
	}
	var m Manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("plugin manifest: decode: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Manifest{}, fmt.Errorf("plugin manifest: trailing data")
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (m Manifest) Validate() error {
	if m.ManifestVersion != CurrentVersion {
		return fmt.Errorf("plugin manifest: unsupported version %d", m.ManifestVersion)
	}
	if _, err := plugin.ParseID(m.PluginID); err != nil {
		return fmt.Errorf("plugin manifest: %w", err)
	}
	if m.Version == "" || len(m.Version) > 128 || m.Version != strings.TrimSpace(m.Version) || !semver.IsValid("v"+m.Version) {
		return fmt.Errorf("plugin manifest: invalid package version")
	}
	if m.DisplayName == "" || m.DisplayName != strings.TrimSpace(m.DisplayName) || !utf8.ValidString(m.DisplayName) || len(m.DisplayName) > 160 {
		return fmt.Errorf("plugin manifest: invalid display name")
	}
	if !utf8.ValidString(m.Description) || len(m.Description) > 4096 {
		return fmt.Errorf("plugin manifest: invalid description")
	}
	if len(m.Entrypoints) == 0 || len(m.Entrypoints) > 32 {
		return fmt.Errorf("plugin manifest: invalid entrypoint count")
	}
	for target, entry := range m.Entrypoints {
		if !platformPattern.MatchString(target) || !safeEntrypoint(entry) {
			return fmt.Errorf("plugin manifest: invalid entrypoint for %q", target)
		}
	}
	// Multi-extension packages are deliberately deferred until their process
	// version negotiation and partial disable semantics can be enforced.
	if len(m.Extensions) != 1 {
		return fmt.Errorf("plugin manifest: exactly one extension is supported")
	}
	if err := m.Extensions[0].Validate(); err != nil {
		return fmt.Errorf("plugin manifest: %w", err)
	}
	return nil
}

func (m Manifest) Entrypoint(goos, goarch string) (string, error) {
	value, ok := m.Entrypoints[goos+"/"+goarch]
	if !ok || !safeEntrypoint(value) {
		return "", fmt.Errorf("plugin manifest: unsupported platform %s/%s", goos, goarch)
	}
	return value, nil
}

func safeEntrypoint(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) ||
		strings.ContainsAny(value, "\\\x00") || path.IsAbs(value) {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}
