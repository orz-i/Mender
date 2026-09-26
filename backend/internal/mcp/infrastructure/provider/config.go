package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

	if config.Environment != nil {
		command.Env = append([]string{}, os.Environ()...)
		keys := make([]string, 0, len(config.Environment))
		for key := range config.Environment {
			if key == "" || strings.ContainsRune(key, '=') || strings.ContainsRune(key, 0) {
				return nil, fmt.Errorf("connect MCP command: invalid environment key")
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			command.Env = append(command.Env, key+"="+config.Environment[key])
		}
	}

	return ConnectTransport(ctx, &mcp.CommandTransport{Command: command})
}

// ConnectHTTP connects to a remote MCP server over Streamable HTTP.
func ConnectHTTP(ctx context.Context, config HTTPConfig) (*Connection, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("connect MCP HTTP: parse endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("connect MCP HTTP: endpoint must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("connect MCP HTTP: endpoint must be absolute")
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           config.HTTPClient,
		MaxRetries:           config.MaxRetries,
		DisableStandaloneSSE: true,
	}
	return ConnectTransport(ctx, transport)
}
