package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

var (
	// ErrToolResult indicates that an upstream MCP tool returned isError=true.
	ErrToolResult = errors.New("MCP upstream tool returned an error result")
	// ErrArgumentsNotObject indicates arguments cannot be sent as MCP Tool input.
	ErrArgumentsNotObject = errors.New("MCP tool arguments must be a JSON object")
)

// Execute implements Invocation's ToolExecutor port.
func (c *Connection) Execute(
	ctx context.Context,
	request invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver,
) (invocationdomain.Result, error) {
	arguments := request.Arguments.Bytes()
	if !isJSONObject(arguments) {
		return invocationdomain.Result{}, ErrArgumentsNotObject
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      request.Tool.SourceKey(),
		Arguments: json.RawMessage(arguments),
	})
	if err != nil {
		return invocationdomain.Result{}, fmt.Errorf("call upstream MCP tool %q: %w", request.Tool.SourceKey(), err)
	}

	return invocationResult(result)
}

func invocationResult(result *mcp.CallToolResult) (invocationdomain.Result, error) {
	if result == nil {
		return invocationdomain.Result{}, fmt.Errorf("map MCP tool result: nil result")
	}
	if result.NeedsInput() {
		return inputRequiredResult(result)
	}
	if result.IsError {
		return invocationdomain.Result{}, ErrToolResult
	}

	var raw []byte
	var err error
	if result.StructuredContent != nil {
		raw, err = json.Marshal(result.StructuredContent)
	} else if len(result.Content) != 0 {
		raw, err = json.Marshal(result.Content)
	} else {
		raw = []byte("null")
	}
	if err != nil {
		return invocationdomain.Result{}, fmt.Errorf("map MCP complete result: %w", err)
	}

	value, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Result{}, fmt.Errorf("map MCP complete result: %w", err)
	}
	complete, err := invocationdomain.Complete(value)
	if err != nil {
		return invocationdomain.Result{}, fmt.Errorf("map MCP complete result: %w", err)
	}
	return complete, nil
}

func inputRequiredResult(result *mcp.CallToolResult) (invocationdomain.Result, error) {
	ids := make([]string, 0, len(result.InputRequests))
	for id := range result.InputRequests {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	state := struct {
		RequestState string   `json:"requestState,omitempty"`
		RequestIDs   []string `json:"requestIds,omitempty"`
	}{
		RequestState: result.RequestState,
		RequestIDs:   ids,
	}
	rawState, err := json.Marshal(state)
	if err != nil {
		return invocationdomain.Result{}, fmt.Errorf("map MCP input-required state: %w", err)
	}

	requestID := "mcp_" + base64.RawURLEncoding.EncodeToString(rawState)
	message := "Upstream MCP tool requires additional input"
	if len(ids) > 0 {
		message += " for request(s): " + strings.Join(ids, ", ")
	}

	mapped, err := invocationdomain.InputRequired(
		requestID,
		message,
		[]byte(`{"type":"object","additionalProperties":true}`),
	)
	if err != nil {
		return invocationdomain.Result{}, fmt.Errorf("map MCP input-required result: %w", err)
	}
	return mapped, nil
}

func isJSONObject(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}
