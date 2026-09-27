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
	// ErrArgumentsNotObject indicates arguments cannot be sent as MCP Tool input.
	ErrArgumentsNotObject = errors.New("MCP tool arguments must be a JSON object")
)

// Execute implements Invocation's ToolExecutor port.
func (c *Connection) Execute(
	ctx context.Context,
	request invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver,
	_ invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	arguments := request.Arguments.Bytes()
	if !isJSONObject(arguments) {
		return invocationdomain.Outcome{}, ErrArgumentsNotObject
	}

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      request.Tool.SourceKey(),
		Arguments: json.RawMessage(arguments),
	})
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("call upstream MCP tool %q: %w", request.Tool.SourceKey(), err)
	}

	return invocationResult(result)
}

func invocationResult(result *mcp.CallToolResult) (invocationdomain.Outcome, error) {
	if result == nil {
		return invocationdomain.Outcome{}, fmt.Errorf("map MCP tool result: nil result")
	}
	if result.NeedsInput() {
		return inputRequiredResult(result)
	}
	if result.IsError {
		message := toolErrorSummary(result.Content)
		if message == "" {
			message = "upstream tool reported failure"
		}
		failed, err := invocationdomain.Failed(
			invocationdomain.FailureSourceTool,
			invocationdomain.FailureToolError,
			message,
		)
		if err != nil {
			return invocationdomain.Outcome{}, fmt.Errorf("map MCP tool failure: %w", err)
		}
		return failed, nil
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
		return invocationdomain.Outcome{}, fmt.Errorf("map MCP complete result: %w", err)
	}

	value, err := invocationdomain.NewJSONValue(raw)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("map MCP complete result: %w", err)
	}
	complete, err := invocationdomain.Complete(value)
	if err != nil {
		return invocationdomain.Outcome{}, fmt.Errorf("map MCP complete result: %w", err)
	}
	return complete, nil
}

func toolErrorSummary(content []mcp.Content) string {
	const maxSummaryRunes = 2048

	parts := make([]string, 0, len(content))
	totalRunes := 0
	for _, item := range content {
		text, ok := item.(*mcp.TextContent)
		if !ok {
			continue
		}
		value := strings.TrimSpace(text.Text)
		if value == "" {
			continue
		}
		runes := []rune(value)
		remaining := maxSummaryRunes - totalRunes
		if remaining <= 0 {
			break
		}
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		parts = append(parts, string(runes))
		totalRunes += len(runes)
		if totalRunes >= maxSummaryRunes {
			break
		}
	}
	return strings.Join(parts, " | ")
}

func inputRequiredResult(result *mcp.CallToolResult) (invocationdomain.Outcome, error) {
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
		return invocationdomain.Outcome{}, fmt.Errorf("map MCP input-required state: %w", err)
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
		return invocationdomain.Outcome{}, fmt.Errorf("map MCP input-required result: %w", err)
	}
	return mapped, nil
}

func isJSONObject(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return len(trimmed) >= 2 && trimmed[0] == '{' && trimmed[len(trimmed)-1] == '}'
}
