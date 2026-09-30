package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

const (
	// ID is the stable built-in Workspace Provider plugin identity.
	ID providerplugin.ID = "io.mender.provider.workspace"

	// ConfigVersion is the persisted Workspace Provider configuration version.
	ConfigVersion = 1

	pluginVersion = "1.0.0"
	maxRootBytes  = 4096
)

type Config struct {
	Root string `json:"root"`
}

func Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{
		ID:            ID,
		Version:       pluginVersion,
		DisplayName:   "Workspace",
		Description:   "Expose governed local workspace read, search, edit, and shell capabilities",
		ConfigVersion: ConfigVersion,
		ConfigSchema:  append(json.RawMessage(nil), workspaceConfigSchema...),
		Capabilities:  []string{"workspace.read", "workspace.search", "workspace.edit", "workspace.shell"},
	}
}

// DecodeConfig strict-decodes and applies side-effect-free syntactic validation.
func DecodeConfig(raw json.RawMessage) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode Workspace plugin config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode Workspace plugin config: multiple JSON values")
		}
		return Config{}, fmt.Errorf("decode Workspace plugin config: trailing data: %w", err)
	}
	if err := config.ValidateSyntax(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// ValidateSyntax is safe for preflight Governance Fact extraction.
func (c Config) ValidateSyntax() error {
	root := strings.TrimSpace(c.Root)
	if root == "" {
		return fmt.Errorf("workspace plugin config: root must not be empty")
	}
	if root != c.Root {
		return fmt.Errorf("workspace plugin config: root must not contain surrounding whitespace")
	}
	if len(root) > maxRootBytes {
		return fmt.Errorf("workspace plugin config: root must be at most %d bytes", maxRootBytes)
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("workspace plugin config: root must be absolute")
	}
	return nil
}

// CanonicalRoot validates host filesystem state and returns a symlink-resolved root.
func (c Config) CanonicalRoot() (string, error) {
	if err := c.ValidateSyntax(); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(c.Root))
	if err != nil {
		return "", fmt.Errorf("workspace plugin config: resolve root: %w", err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("workspace plugin config: absolute root: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("workspace plugin config: stat root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace plugin config: root must be a directory")
	}
	return filepath.Clean(absolute), nil
}

func ValidateConfig(_ context.Context, raw json.RawMessage) error {
	config, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	_, err = config.CanonicalRoot()
	return err
}

func InspectConfig(
	_ context.Context,
	raw json.RawMessage,
) (providerplugin.ConfigMetadata, error) {
	config, err := DecodeConfig(raw)
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	root, err := config.CanonicalRoot()
	if err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	return providerplugin.NewConfigMetadataWithSummary(nil, root)
}
