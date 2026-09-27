package configstore

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/headertemplate"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/transportpolicy"
)

type Transport string

const (
	TransportHTTP  Transport = "http"
	TransportStdio Transport = "stdio"
)

var (
	ErrNotFound = errors.New("MCP provider config: not found")

	environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type CredentialHeader struct {
	Name      string
	Scheme    string
	Reference credentialdomain.Reference
}

type CredentialEnvironment struct {
	Name      string
	Reference credentialdomain.Reference
}

type Config struct {
	ProviderID            catalogdomain.ProviderID
	Transport             Transport
	Endpoint              string
	Headers               map[string]string
	CredentialHeaders     []CredentialHeader
	Executable            string
	Args                  []string
	Directory             string
	CredentialEnvironment []CredentialEnvironment
}

func (c Config) Validate() error {
	if _, err := catalogdomain.ParseProviderID(c.ProviderID.String()); err != nil {
		return fmt.Errorf("MCP provider config: %w", err)
	}

	switch c.Transport {
	case TransportHTTP:
		if strings.TrimSpace(c.Endpoint) == "" {
			return fmt.Errorf("MCP provider config: HTTP endpoint must not be empty")
		}
		if err := transportpolicy.ValidateHTTPEndpoint(c.Endpoint); err != nil {
			return fmt.Errorf("MCP provider config: %w", err)
		}
		if err := headertemplate.Validate(c.Headers); err != nil {
			return fmt.Errorf("MCP provider config: %w", err)
		}
		if err := validateCredentialHeaders(c.Headers, c.CredentialHeaders); err != nil {
			return fmt.Errorf("MCP provider config: %w", err)
		}
		if c.Executable != "" || len(c.Args) != 0 || c.Directory != "" ||
			len(c.CredentialEnvironment) != 0 {
			return fmt.Errorf("MCP provider config: HTTP config must not contain stdio fields")
		}
	case TransportStdio:
		if strings.TrimSpace(c.Executable) == "" {
			return fmt.Errorf("MCP provider config: stdio executable must not be empty")
		}
		if c.Endpoint != "" || len(c.Headers) != 0 || len(c.CredentialHeaders) != 0 {
			return fmt.Errorf("MCP provider config: stdio config must not contain HTTP fields")
		}
		if err := validateCredentialEnvironment(c.CredentialEnvironment); err != nil {
			return fmt.Errorf("MCP provider config: %w", err)
		}
	default:
		return fmt.Errorf("MCP provider config: unknown transport %q", c.Transport)
	}

	return nil
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
			return fmt.Errorf("stdio credential environment name %q is invalid", binding.Name)
		}
		if _, err := credentialdomain.ParseReference(binding.Reference.String()); err != nil {
			return fmt.Errorf("stdio credential environment %q: %w", name, err)
		}
		key := strings.ToUpper(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("stdio credential environment %q is configured more than once", name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (c Config) RequiredCredentialRefs() []credentialdomain.Reference {
	refs := make([]credentialdomain.Reference, 0, len(c.CredentialHeaders)+len(c.CredentialEnvironment))
	seen := make(map[credentialdomain.Reference]struct{}, cap(refs))
	for _, binding := range c.CredentialHeaders {
		if _, exists := seen[binding.Reference]; exists {
			continue
		}
		seen[binding.Reference] = struct{}{}
		refs = append(refs, binding.Reference)
	}
	for _, binding := range c.CredentialEnvironment {
		if _, exists := seen[binding.Reference]; exists {
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
