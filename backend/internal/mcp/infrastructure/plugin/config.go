package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/headertemplate"
	"github.com/orz-i/Mender/backend/internal/providerhttp"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
)

const (
	// ID is the stable built-in MCP Provider plugin identity.
	ID providerplugin.ID = "io.mender.provider.mcp"

	// ConfigVersion is the current persisted MCP plugin configuration version.
	ConfigVersion = 1
)

// TransportKind identifies one MCP client transport variant.
type TransportKind string

const (
	TransportStreamableHTTP TransportKind = "streamable-http"
	TransportStdio          TransportKind = "stdio"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Config is the non-secret persisted configuration owned by the MCP plugin.
type Config struct {
	Transport TransportConfig `json:"transport"`
}

// TransportConfig is a discriminated MCP transport configuration.
//
// Fields not belonging to Kind must be empty.
type TransportConfig struct {
	Kind TransportKind `json:"kind"`

	Endpoint           string                     `json:"endpoint,omitempty"`
	Headers            map[string]string          `json:"headers,omitempty"`
	CredentialHeaders  []CredentialHeader         `json:"credentialHeaders,omitempty"`
	OAuthCredentialRef credentialdomain.Reference `json:"oauthCredentialRef,omitempty"`

	Executable            string                  `json:"executable,omitempty"`
	Args                  []string                `json:"args,omitempty"`
	Directory             string                  `json:"directory,omitempty"`
	CredentialEnvironment []CredentialEnvironment `json:"credentialEnvironment,omitempty"`
}

// CredentialHeader places one inventory-backed secret in an HTTP header.
type CredentialHeader struct {
	Name      string                     `json:"name"`
	Scheme    string                     `json:"scheme,omitempty"`
	Reference credentialdomain.Reference `json:"credentialRef"`
}

// CredentialEnvironment places one inventory-backed secret in a child process
// environment entry.
type CredentialEnvironment struct {
	Name      string                     `json:"name"`
	Reference credentialdomain.Reference `json:"credentialRef"`
}

// DecodeConfig parses strict plugin-owned JSON and validates transport
// invariants.
func DecodeConfig(raw json.RawMessage) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode MCP plugin config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode MCP plugin config: multiple JSON values")
		}
		return Config{}, fmt.Errorf("decode MCP plugin config: trailing data: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate checks transport-specific configuration without activating a
// Provider.
func (c Config) Validate() error {
	switch c.Transport.Kind {
	case TransportStreamableHTTP:
		if strings.TrimSpace(c.Transport.Endpoint) == "" {
			return fmt.Errorf("MCP plugin config: HTTP endpoint must not be empty")
		}
		if err := providerhttp.ValidateEndpoint(c.Transport.Endpoint); err != nil {
			return fmt.Errorf("MCP plugin config: %w", err)
		}
		if err := headertemplate.Validate(c.Transport.Headers); err != nil {
			return fmt.Errorf("MCP plugin config: %w", err)
		}
		if err := validateCredentialHeaders(
			c.Transport.Headers,
			c.Transport.CredentialHeaders,
		); err != nil {
			return fmt.Errorf("MCP plugin config: %w", err)
		}
		if c.Transport.OAuthCredentialRef != "" {
			if _, err := credentialdomain.ParseReference(c.Transport.OAuthCredentialRef.String()); err != nil {
				return fmt.Errorf("MCP plugin config: OAuth credential: %w", err)
			}
			for name := range c.Transport.Headers {
				if strings.EqualFold(name, "Authorization") {
					return fmt.Errorf("MCP plugin config: OAuth cannot share an Authorization header")
				}
			}
			for _, header := range c.Transport.CredentialHeaders {
				if strings.EqualFold(header.Name, "Authorization") {
					return fmt.Errorf("MCP plugin config: OAuth cannot share an Authorization credential header")
				}
			}
		}
		if c.Transport.Executable != "" ||
			len(c.Transport.Args) != 0 ||
			c.Transport.Directory != "" ||
			len(c.Transport.CredentialEnvironment) != 0 {
			return fmt.Errorf(
				"MCP plugin config: streamable-http transport must not contain stdio fields",
			)
		}

	case TransportStdio:
		if strings.TrimSpace(c.Transport.Executable) == "" {
			return fmt.Errorf("MCP plugin config: stdio executable must not be empty")
		}
		if c.Transport.Endpoint != "" ||
			len(c.Transport.Headers) != 0 ||
			len(c.Transport.CredentialHeaders) != 0 ||
			c.Transport.OAuthCredentialRef != "" {
			return fmt.Errorf(
				"MCP plugin config: stdio transport must not contain HTTP fields",
			)
		}
		if err := validateCredentialEnvironment(c.Transport.CredentialEnvironment); err != nil {
			return fmt.Errorf("MCP plugin config: %w", err)
		}

	default:
		return fmt.Errorf(
			"MCP plugin config: unknown transport %q",
			c.Transport.Kind,
		)
	}
	return nil
}

// RequiredCredentialRefs returns unique Provider authentication references in
// deterministic order.
func (c Config) RequiredCredentialRefs() []credentialdomain.Reference {
	refs := make(
		[]credentialdomain.Reference,
		0,
		len(c.Transport.CredentialHeaders)+len(c.Transport.CredentialEnvironment)+1,
	)
	seen := make(map[credentialdomain.Reference]struct{}, cap(refs))
	if c.Transport.OAuthCredentialRef != "" {
		refs = append(refs, c.Transport.OAuthCredentialRef)
		seen[c.Transport.OAuthCredentialRef] = struct{}{}
	}
	for _, binding := range c.Transport.CredentialHeaders {
		if _, ok := seen[binding.Reference]; ok {
			continue
		}
		seen[binding.Reference] = struct{}{}
		refs = append(refs, binding.Reference)
	}
	for _, binding := range c.Transport.CredentialEnvironment {
		if _, ok := seen[binding.Reference]; ok {
			continue
		}
		seen[binding.Reference] = struct{}{}
		refs = append(refs, binding.Reference)
	}
	slices.SortFunc(refs, func(a, b credentialdomain.Reference) int {
		return strings.Compare(a.String(), b.String())
	})
	return refs
}

func validateCredentialHeaders(
	staticHeaders map[string]string,
	bindings []CredentialHeader,
) error {
	seen := make(map[string]struct{}, len(staticHeaders)+len(bindings))
	for name := range staticHeaders {
		normalized, err := headertemplate.ValidateName(name)
		if err != nil {
			return err
		}
		seen[strings.ToLower(normalized)] = struct{}{}
	}
	for _, binding := range bindings {
		name, err := headertemplate.ValidateName(binding.Name)
		if err != nil {
			return err
		}
		if _, err := headertemplate.ValidateScheme(binding.Scheme); err != nil {
			return err
		}
		if _, err := credentialdomain.ParseReference(binding.Reference.String()); err != nil {
			return fmt.Errorf("credential header %q: %w", name, err)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("HTTP header %q is configured more than once", name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateCredentialEnvironment(bindings []CredentialEnvironment) error {
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		name := strings.TrimSpace(binding.Name)
		if !environmentNamePattern.MatchString(name) {
			return fmt.Errorf(
				"stdio credential environment name %q is invalid",
				binding.Name,
			)
		}
		if _, err := credentialdomain.ParseReference(binding.Reference.String()); err != nil {
			return fmt.Errorf("stdio credential environment %q: %w", name, err)
		}
		key := strings.ToUpper(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf(
				"stdio credential environment %q is configured more than once",
				name,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}
