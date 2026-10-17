package bootstrap

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// taskScopedMCPServer is a short-lived local execution proxy, not a new
// Provider, Tool catalog or independently authenticated Profile. Only the
// runtime that issued its random Task capability may serve its Tools.
type taskScopedMCPServer struct {
	endpoint string
	token    string
	server   *http.Server
	listener net.Listener
	owner    *Runtime
	stop     chan struct{}
	once     sync.Once
}

func (s *taskScopedMCPServer) close() error {
	if s == nil {
		return nil
	}
	var err error
	s.once.Do(func() {
		close(s.stop)
		s.owner.closeAgentTaskToolCapability(s.token)
		// http.Server.Close already owns and closes its listener.
		err = s.server.Close()
		s.owner.agentToolGateway.mu.Lock()
		delete(s.owner.agentToolGateway.servers, s.token)
		s.owner.agentToolGateway.mu.Unlock()
	})
	return err
}
func (r *Runtime) closeAgentTaskToolMCPServers() {
	r.agentToolGateway.mu.Lock()
	// Close creation and invocation atomically before enumerating listeners.
	// No new server may register between snapshot and shutdown.
	r.agentToolGateway.closed = true
	clear(r.agentToolGateway.tickets)
	servers := make([]*taskScopedMCPServer, 0, len(r.agentToolGateway.servers))
	for _, item := range r.agentToolGateway.servers {
		servers = append(servers, item)
	}
	r.agentToolGateway.mu.Unlock()
	for _, item := range servers {
		_ = item.close()
	}
}

// openAgentTaskToolMCPServer is used only by an admitted Runtime/ACP Task
// coordinator. No Model-facing Tool or management endpoint can mint a token.
// The task must be running, have current Profile grants and be bound to one
// ready, restricted_host local Workspace Sandbox with the EXACT admitted cwd.
func (r *Runtime) openAgentTaskToolMCPServer(ctx context.Context,
	taskID agentdomain.TaskID, sandboxID sandboxdomain.ID, requested []catalogdomain.ToolID) (*taskScopedMCPServer, error) {
	token, err := r.openAgentTaskToolCapability(ctx, taskID, sandboxID, requested)
	if err != nil {
		return nil, err
	}
	cleanup := func() { r.closeAgentTaskToolCapability(token) }
	scoped := mcp.NewServer(&mcp.Implementation{Name: "mender-task-tools", Version: "1.0.0"}, nil)
	names := map[string]bool{}
	for _, id := range requested {
		tool, err := r.Catalog.store.GetTool(ctx, id)
		if err != nil {
			cleanup()
			return nil, err
		}
		source := tool.SourceKey()
		name := "mender_" + strings.TrimPrefix(source, "workspace.")
		if name == "mender_" || names[name] {
			cleanup()
			return nil, fmt.Errorf("ambiguous task Tool projection")
		}
		names[name] = true
		if len(tool.InputSchema().Bytes()) == 0 {
			cleanup()
			return nil, fmt.Errorf("invalid Task Tool schema")
		}
		description := tool.Description()
		scoped.AddTool(&mcp.Tool{Name: name, Title: tool.Title(),
			Description: description, InputSchema: json.RawMessage(tool.InputSchema().Bytes())},
			func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				if request == nil {
					return nil, errors.New("missing bounded Tool request")
				}
				result, err := r.invokeAgentTaskTool(ctx, token, id, request.Params.Arguments)
				if err != nil {
					return nil, errors.New("task tool invocation denied")
				}
				return mcpTaskToolResult(result.Outcome)
			})
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cleanup()
		return nil, err
	}
	endpoint := "http://" + listener.Addr().String() + "/mcp"
	// No Host/Origin variation, arbitrary network listener, generic Profile
	// exposure, wildcard bearer or browser cross-origin access.
	hostname := listener.Addr().String()
	protocol := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return scoped },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Host != hostname || req.URL.Path != "/mcp" || (req.Header.Get("Origin") != "" && req.Header.Get("Origin") != "http://"+hostname) {
			http.Error(w, "invalid local Task origin", http.StatusForbidden)
			return
		}
		supplied := req.Header.Get("Authorization")
		expected := "Bearer " + token
		if len(supplied) != len(expected) || subtle.ConstantTimeCompare([]byte(supplied), []byte(expected)) != 1 {
			http.Error(w, "Task capability required", http.StatusUnauthorized)
			return
		}
		if req.ContentLength > 256<<10 {
			http.Error(w, "Task MCP request too large", http.StatusRequestEntityTooLarge)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, 256<<10)
		protocol.ServeHTTP(w, req)
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 16 << 10}
	item := &taskScopedMCPServer{endpoint: endpoint, token: token, server: server, listener: listener, owner: r, stop: make(chan struct{})}
	r.agentToolGateway.mu.Lock()
	if r.agentToolGateway.closed {
		r.agentToolGateway.mu.Unlock()
		_ = listener.Close()
		cleanup()
		return nil, errors.New("agent task tool gateway closed")
	}
	if r.agentToolGateway.servers == nil {
		r.agentToolGateway.servers = map[string]*taskScopedMCPServer{}
	}
	expires := r.agentToolGateway.tickets[token].expiresAt
	r.agentToolGateway.servers[token] = item
	r.agentToolGateway.mu.Unlock()
	go func() { _ = server.Serve(listener) }()
	// Short-lived transport is torn down when its owning task terminates,
	// is cancelled, exceeds its deadline, or the opening context closes.
	// A Task cancellation is also enforced synchronously for every tool call.
	go func() {
		deadline := time.NewTimer(time.Until(expires))
		ticker := time.NewTicker(time.Second)
		defer deadline.Stop()
		defer ticker.Stop()
		for {
			select {
			case <-item.stop:
				return
			case <-ctx.Done():
			case <-deadline.C:
			case <-ticker.C:
				r.agentToolGateway.mu.Lock()
				_, valid := r.agentToolGateway.tickets[token]
				r.agentToolGateway.mu.Unlock()
				if valid && r.Agents.TaskLocallyActive(taskID) {
					continue
				}
			}
			_ = item.close()
			return
		}
	}()
	return item, nil
}

func mcpTaskToolResult(outcome invocationdomain.Outcome) (*mcp.CallToolResult, error) {
	switch outcome.Kind() {
	case invocationdomain.OutcomeKindComplete:
		raw := outcome.Content().Bytes()
		var structured any
		if json.Unmarshal(raw, &structured) != nil {
			return nil, errors.New("governed Task result invalid")
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}, StructuredContent: structured,
		}, nil
	case invocationdomain.OutcomeKindRejected:
		return mcpTaskToolError("rejected", string(outcome.RejectionCode()))
	case invocationdomain.OutcomeKindFailed:
		return mcpTaskToolError("failed", string(outcome.FailureCode()))
	default:
		return mcpTaskToolError("unavailable", "unsupported_task_tool_result")
	}
}
func mcpTaskToolError(kind, code string) (*mcp.CallToolResult, error) {
	response := map[string]any{"kind": kind, "code": code}
	raw, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{
		IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}, StructuredContent: response,
	}, nil
}
