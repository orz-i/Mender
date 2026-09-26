package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/orz-i/Mender/backend/internal/mcp/infrastructure/transportpolicy"
)

// CommandConfig configures a local upstream MCP Provider over stdio.
type CommandConfig struct {
	Executable  string
	Args        []string
	Directory   string
	Environment map[string]string
	Stderr      io.Writer
}

// HTTPConfig configures a remote upstream MCP Provider over Streamable HTTP.
type HTTPConfig struct {
	Endpoint   string
	Headers    map[string]string
	HTTPClient *http.Client
	MaxRetries int
}

// ConnectCommand starts and connects to a local MCP server without invoking a shell.
func ConnectCommand(ctx context.Context, config CommandConfig) (*Connection, error) {
	executable := strings.TrimSpace(config.Executable)
	if executable == "" {
		return nil, fmt.Errorf("connect MCP command: executable must not be empty")
	}

	//nolint:gosec // executable and argv are explicit provider configuration; no shell is invoked.
	command := exec.CommandContext(ctx, executable, config.Args...)
	command.Dir = config.Directory
	if config.Stderr != nil {
		command.Stderr = config.Stderr
	} else {
		command.Stderr = os.Stderr
	}

	environment, err := commandEnvironment(os.Environ(), config.Environment)
	if err != nil {
		return nil, err
	}
	command.Env = environment

	return ConnectTransport(ctx, &mcp.CommandTransport{Command: command})
}

// ConnectHTTP connects to a remote MCP server over Streamable HTTP.
func ConnectHTTP(ctx context.Context, config HTTPConfig) (*Connection, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if err := transportpolicy.ValidateHTTPEndpoint(endpoint); err != nil {
		return nil, fmt.Errorf("connect MCP HTTP: %w", err)
	}

	httpClient, err := httpClientWithHeaders(
		config.HTTPClient,
		endpoint,
		config.Headers,
	)
	if err != nil {
		return nil, fmt.Errorf("connect MCP HTTP: %w", err)
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           httpClient,
		MaxRetries:           config.MaxRetries,
		DisableStandaloneSSE: true,
	}
	return ConnectTransport(ctx, transport)
}
