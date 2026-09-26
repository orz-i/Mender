package configstore

import (
	"errors"
	"fmt"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/headertemplate"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/transportpolicy"
)

type Transport string

const (
	TransportHTTP  Transport = "http"
	TransportStdio Transport = "stdio"
)

var ErrNotFound = errors.New("MCP provider config: not found")

type Config struct {
	ProviderID catalogdomain.ProviderID
	Transport  Transport
	Endpoint   string
	Headers    map[string]string
	Executable string
	Args       []string
	Directory  string
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
		if c.Executable != "" || len(c.Args) != 0 || c.Directory != "" {
			return fmt.Errorf("MCP provider config: HTTP config must not contain stdio fields")
		}
	case TransportStdio:
		if strings.TrimSpace(c.Executable) == "" {
			return fmt.Errorf("MCP provider config: stdio executable must not be empty")
		}
		if c.Endpoint != "" || len(c.Headers) != 0 {
			return fmt.Errorf("MCP provider config: stdio config must not contain HTTP fields")
		}
	default:
		return fmt.Errorf("MCP provider config: unknown transport %q", c.Transport)
	}

	return nil
}
