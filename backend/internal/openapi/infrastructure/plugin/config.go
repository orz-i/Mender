package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/orz-i/Mender/backend/internal/providerhttp"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

const (
	// ID is the stable built-in OpenAPI Provider plugin identity.
	ID providerplugin.ID = "io.mender.provider.openapi"

	// ConfigVersion is the current persisted OpenAPI plugin configuration version.
	ConfigVersion = 1

	pluginVersion            = "1.0.0"
	maxOperationFilterValues = 2048
	maxOperationSourceKeyLen = 1024
	maxSecurityBindings      = 256
	maxSecuritySchemeNameLen = 255
)

// SourceKind identifies how an OpenAPI Description is loaded.
type SourceKind string

const (
	SourceURL  SourceKind = "url"
	SourceFile SourceKind = "file"
)

// Config is the non-secret persisted OpenAPI Provider configuration.
type Config struct {
	Source      SourceConfig                          `json:"source"`
	ServerURL   string                                `json:"serverUrl,omitempty"`
	Operations  OperationFilter                       `json:"operations,omitempty"`
	Credentials map[string]credentialdomain.Reference `json:"credentials,omitempty"`
}

// SourceConfig identifies one explicit OpenAPI Description source.
type SourceConfig struct {
	Kind SourceKind `json:"kind"`
	URL  string     `json:"url,omitempty"`
	Path string     `json:"path,omitempty"`
}

// OperationFilter selects canonical OpenAPI operation SourceKeys.
type OperationFilter struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// Descriptor returns safe metadata for the built-in OpenAPI plugin.
func Descriptor() providerplugin.Descriptor {
	return providerplugin.Descriptor{
		ID:            ID,
		Version:       pluginVersion,
		DisplayName:   "OpenAPI",
		Description:   "Expose supported OpenAPI operations as governed Mender Tools",
		ConfigVersion: ConfigVersion,
	}
}

// DecodeConfig strict-decodes and validates plugin-owned configuration.
func DecodeConfig(raw json.RawMessage) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode OpenAPI plugin config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode OpenAPI plugin config: multiple JSON values")
		}
		return Config{}, fmt.Errorf("decode OpenAPI plugin config: trailing data: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks generic OpenAPI Provider configuration without loading the
// source document or resolving any Credential.
func (c Config) Validate() error {
	if err := c.Source.Validate(); err != nil {
		return err
	}
	if err := validateServerURL(c.ServerURL); err != nil {
		return err
	}
	if err := c.Operations.Validate(); err != nil {
		return err
	}
	if err := validateCredentialBindings(c.Credentials); err != nil {
		return err
	}
	return nil
}

// Validate checks Description source invariants.
func (s SourceConfig) Validate() error {
	switch s.Kind {
	case SourceURL:
		if strings.TrimSpace(s.Path) != "" {
			return fmt.Errorf("OpenAPI plugin config: URL source must not contain file path")
		}
		value := strings.TrimSpace(s.URL)
		if value == "" {
			return fmt.Errorf("OpenAPI plugin config: URL source must provide url")
		}
		if err := providerhttp.ValidateEndpoint(value); err != nil {
			return fmt.Errorf("OpenAPI plugin config: source URL: %w", err)
		}
		parsed, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("OpenAPI plugin config: source URL: %w", err)
		}
		if parsed.Fragment != "" {
			return fmt.Errorf("OpenAPI plugin config: source URL must not contain fragment")
		}

	case SourceFile:
		if strings.TrimSpace(s.URL) != "" {
			return fmt.Errorf("OpenAPI plugin config: file source must not contain url")
		}
		path := strings.TrimSpace(s.Path)
		if path == "" {
			return fmt.Errorf("OpenAPI plugin config: file source must provide path")
		}
		if !filepath.IsAbs(path) {
			return fmt.Errorf("OpenAPI plugin config: file source path must be absolute")
		}

	default:
		return fmt.Errorf("OpenAPI plugin config: unknown source kind %q", s.Kind)
	}
	return nil
}

// Validate checks include/exclude filter invariants.
func (f OperationFilter) Validate() error {
	if len(f.Include) > maxOperationFilterValues {
		return fmt.Errorf(
			"OpenAPI plugin config: include contains more than %d SourceKeys",
			maxOperationFilterValues,
		)
	}
	if len(f.Exclude) > maxOperationFilterValues {
		return fmt.Errorf(
			"OpenAPI plugin config: exclude contains more than %d SourceKeys",
			maxOperationFilterValues,
		)
	}

	include, err := normalizedSourceKeySet("include", f.Include)
	if err != nil {
		return err
	}
	exclude, err := normalizedSourceKeySet("exclude", f.Exclude)
	if err != nil {
		return err
	}
	for value := range include {
		if _, exists := exclude[value]; exists {
			return fmt.Errorf(
				"OpenAPI plugin config: SourceKey %q appears in both include and exclude",
				value,
			)
		}
	}
	return nil
}

// RequiredCredentialRefs returns configured Credential References uniquely and
// deterministically.
func (c Config) RequiredCredentialRefs() []credentialdomain.Reference {
	result := make([]credentialdomain.Reference, 0, len(c.Credentials))
	seen := make(map[credentialdomain.Reference]struct{}, len(c.Credentials))
	for _, ref := range c.Credentials {
		if _, exists := seen[ref]; exists {
			continue
		}
		seen[ref] = struct{}{}
		result = append(result, ref)
	}
	slices.SortFunc(result, func(a, b credentialdomain.Reference) int {
		return strings.Compare(a.String(), b.String())
	})
	return result
}

func validateServerURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := providerhttp.ValidateEndpoint(raw); err != nil {
		return fmt.Errorf("OpenAPI plugin config: server URL: %w", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("OpenAPI plugin config: server URL: %w", err)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("OpenAPI plugin config: server URL must not contain query or fragment")
	}
	return nil
}

func normalizedSourceKeySet(
	field string,
	values []string,
) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			return nil, fmt.Errorf(
				"OpenAPI plugin config: %s must not contain empty SourceKeys",
				field,
			)
		}
		if len(value) > maxOperationSourceKeyLen {
			return nil, fmt.Errorf(
				"OpenAPI plugin config: %s SourceKey %q is too long",
				field,
				value,
			)
		}
		if _, exists := result[value]; exists {
			return nil, fmt.Errorf(
				"OpenAPI plugin config: %s contains duplicate SourceKey %q",
				field,
				value,
			)
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func validateCredentialBindings(
	bindings map[string]credentialdomain.Reference,
) error {
	if len(bindings) > maxSecurityBindings {
		return fmt.Errorf(
			"OpenAPI plugin config: credentials contains more than %d bindings",
			maxSecurityBindings,
		)
	}
	for rawName, rawRef := range bindings {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return fmt.Errorf("OpenAPI plugin config: credential scheme name must not be empty")
		}
		if name != rawName {
			return fmt.Errorf(
				"OpenAPI plugin config: credential scheme name %q must not contain surrounding whitespace",
				rawName,
			)
		}
		if len(name) > maxSecuritySchemeNameLen {
			return fmt.Errorf(
				"OpenAPI plugin config: credential scheme name %q is too long",
				name,
			)
		}
		if _, err := credentialdomain.ParseReference(rawRef.String()); err != nil {
			return fmt.Errorf(
				"OpenAPI plugin config: credential scheme %q: %w",
				name,
				err,
			)
		}
	}
	return nil
}
