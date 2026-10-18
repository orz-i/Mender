// Package acp executes one bounded Agent Client Protocol v1 session using
// stdin/stdout JSON-RPC. The Agent owns its internal loop; Mender owns the
// process, admission, timeout, output limits and permissions.
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const maxFrameBytes = 1 << 20

var errClientPermissionDenied = errors.New("ACP Agent requested a client capability without authorization")

// CommandFactory must be a trusted, host-owned process selector; no prompt or
// model-controlled argument may select the command, cwd, sandbox or env.
type CommandFactory func(context.Context, agentdomain.Agent) (*exec.Cmd, error)

type Runner struct {
	NewCommand            CommandFactory
	NewCommandInWorkspace func(context.Context, agentdomain.Agent, string) (*exec.Cmd, error)
	// Required for task-supplied governed Tools: only a verified restrictive
	// process plan can receive an MCP bearer. No fallback to old host launcher.
	NewCommandWithConfig func(context.Context, agentdomain.Agent, agentapplication.RunConfig) (*exec.Cmd, error)
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type stream struct {
	sendMu  sync.Mutex
	stdin   io.Writer
	updates <-chan rpcMessage

	// Only configured for the live session/prompt; initialize, session/new,
	// model inspection and all unrelated ACP client calls stay fail-closed.
	permissionSession   string
	permissionRoot      string
	permissionRequester agentapplication.PermissionRequester
	permissionErrors    chan error
	seenRequests        map[string]struct{} // raw RPC and ToolCallID replay guard
	pendingPermissions  atomic.Int32
}

func (s *stream) send(message any) error {
	return s.sendLocked(context.Background(), message)
}

// sendPermission rechecks Task cancellation while holding the same write
// mutex used by every ACP frame. In particular, a permission queued behind
// a slow/blocked frame must not send a late allow_once after cancellation.
// This is NOT atomic with an asynchronous cancellation that occurs after the
// check but while the OS pipe write is already in progress, nor proof the
// external Agent received or executed the permission.
func (s *stream) sendPermission(ctx context.Context, message any) error {
	return s.sendLocked(ctx, message)
}

func (s *stream) sendLocked(ctx context.Context, message any) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err = s.stdin.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write ACP request: %w", err)
	}
	return nil
}
func (s *stream) call(ctx context.Context, id int, method string, params any, notify func(rpcMessage) error) (json.RawMessage, error) {
	if err := s.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-s.permissionErrors:
			return nil, err
		case msg, ok := <-s.updates:
			if !ok {
				return nil, errors.New("ACP server closed before response")
			}
			if msg.JSONRPC != "2.0" {
				return nil, errors.New("unsupported ACP JSON-RPC version")
			}
			if len(msg.ID) > 0 && msg.Method != "" {
				if msg.Method == "session/request_permission" && s.permissionRequester != nil {
					if err := s.dispatchPermission(ctx, msg); err != nil {
						return nil, err
					}
					continue
				}
				if err := s.respondToAgent(msg); err != nil {
					return nil, err
				}
				continue
			}
			if msg.Method != "" {
				if err := notify(msg); err != nil {
					return nil, err
				}
				continue
			}
			if len(msg.ID) == 0 {
				return nil, errors.New("ACP response missing request id")
			}
			var got int
			if err := json.Unmarshal(msg.ID, &got); err != nil || got != id {
				return nil, errors.New("ACP response ID mismatch")
			}
			if msg.Error != nil {
				return nil, fmt.Errorf("ACP %s failed (%d)", method, msg.Error.Code)
			}
			if len(msg.Result) == 0 {
				return nil, fmt.Errorf("ACP %s: missing result", method)
			}
			if s.pendingPermissions.Load() != 0 {
				return nil, errors.New("ACP returned while a permission request is unresolved")
			}
			return msg.Result, nil
		}
	}
}
func (s *stream) respondToAgent(msg rpcMessage) error {
	switch msg.Method {
	case "session/request_permission":
		// Never select an approval option or infer permission from a tool
		// argument. A trusted operator response path may be added later.
		if err := s.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID), "result": map[string]any{
			"outcome": map[string]string{"outcome": "cancelled"},
		}}); err != nil {
			return err
		}
		return errClientPermissionDenied
	default:
		// Deny client-side filesystem, terminal, elicitation, or unrecognized
		// ACP requests. Not implementing them is not implicit authority.
		if err := s.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(msg.ID),
			"error": map[string]any{"code": -32601, "message": "client capability not authorized"}}); err != nil {
			return err
		}
		return errClientPermissionDenied
	}
}

// ExecuteConfigured returns only the final agent_message_chunk text;
// reasoning and tool execution details are not retained. Cancellation
// interrupts the complete process group, not just the Task state.
func (r Runner) ExecuteConfigured(ctx context.Context, agent agentdomain.Agent, prompt string, config agentapplication.RunConfig) (string, error) {
	if len(prompt) == 0 || len(prompt) > agentdomain.MaxPromptBytes || strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("%w: invalid Task prompt", agentdomain.ErrInvalid)
	}
	return r.withSession(ctx, agent, config, func(wire *stream, sessionID string, session json.RawMessage, readErrors <-chan error) (string, error) {
		if err := applySessionChoices(ctx, wire, sessionID, session, config.Model, config.ReasoningEffort); err != nil {
			return "", err
		}
		// This callback is installed only for this admitted Task's ACP
		// session. It never approves a request itself; the owner decision
		// is awaited asynchronously while the reader keeps processing
		// cancellation and unrelated notifications.
		wire.permissionSession = sessionID
		wire.permissionRoot = config.WorkspaceRoot
		wire.permissionRequester = config.PermissionRequester
		wire.permissionErrors = make(chan error, 2)
		wire.seenRequests = map[string]struct{}{}
		var result strings.Builder
		var outputOverflow bool
		onUpdate := func(msg rpcMessage) error {
			if msg.Method != "session/update" {
				return nil
			}
			// ACP updates have heterogeneous content shapes. In particular,
			// OpenCode emits tool_call/tool_call_update content arrays after
			// approval. Do not try to decode those arrays as text chunks,
			// and never retain raw native Tool metadata or file contents.
			var params struct {
				SessionID string `json:"sessionId"`
				Update    struct {
					SessionUpdate string          `json:"sessionUpdate"`
					Content       json.RawMessage `json:"content"`
				} `json:"update"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return errors.New("ACP malformed session update")
			}
			if params.SessionID != sessionID {
				return errors.New("ACP update from unexpected SessionID")
			}
			if params.Update.SessionUpdate != "agent_message_chunk" {
				return nil
			}
			var chunk struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if len(params.Update.Content) == 0 ||
				json.Unmarshal(params.Update.Content, &chunk) != nil {
				return errors.New("ACP malformed Agent text chunk")
			}
			if chunk.Type == "text" {
				if len(chunk.Text) > agentdomain.MaxResultBytes-result.Len() {
					outputOverflow = true
					return errors.New("ACP Agent result exceeds limit")
				}
				_, _ = result.WriteString(chunk.Text)
			}
			return nil
		}
		outcome, err := wire.call(ctx, 3, "session/prompt", map[string]any{
			"sessionId": sessionID, "prompt": []any{map[string]string{"type": "text", "text": prompt}},
		}, onUpdate)
		if err != nil {
			if outputOverflow {
				return "", errors.New("ACP Agent result exceeds limit")
			}
			return "", annotateRead(err, readErrors)
		}
		var completed struct {
			StopReason string `json:"stopReason"`
		}
		if err := json.Unmarshal(outcome, &completed); err != nil {
			return "", errors.New("ACP invalid prompt outcome")
		}
		if completed.StopReason != "end_turn" {
			return "", fmt.Errorf("ACP Agent did not complete task (stop reason %q)", completed.StopReason)
		}
		return result.String(), nil
	})
}

// withSession owns all child-process cleanup and ACP v1 initialization.
// Inspection never sends a prompt; execution is bounded by the caller's context.
func (r Runner) withSession(ctx context.Context, agent agentdomain.Agent, config agentapplication.RunConfig,
	visit func(*stream, string, json.RawMessage, <-chan error) (string, error),
) (result string, err error) {
	if r.NewCommand == nil {
		return "", errors.New("ACP process launcher unavailable")
	}
	if err := config.ToolSupply.Validate(); err != nil {
		return "", fmt.Errorf("invalid Agent tool supply: %w", err)
	}
	// The per-task command has no persistent Agent session. Cancel its context
	// when the prompt ends so the owned process group and scanner are drained.
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	// Every task and no-prompt inspector provides an explicit admitted root.
	// Never silently substitute the Agent default when a caller loses the
	// immutable workspace snapshot; that would execute in the wrong project.
	if config.WorkspaceRoot == "" {
		return "", fmt.Errorf("%w: ACP execution requires an admitted workspace root", agentdomain.ErrInvalid)
	}
	var cmd *exec.Cmd
	if !config.ToolSupply.IsZero() {
		if agent.Spec().Isolation != "restricted_host" || r.NewCommandWithConfig == nil {
			return "", fmt.Errorf("%w: task Tool supply requires a verified restricted Agent process plan", agentdomain.ErrUnavailable)
		}
		cmd, err = r.NewCommandWithConfig(runCtx, agent, config)
	} else if config.WorkspaceRoot == agent.Spec().WorkspaceRoot {
		cmd, err = r.NewCommand(runCtx, agent)
	} else if r.NewCommandInWorkspace != nil {
		cmd, err = r.NewCommandInWorkspace(runCtx, agent, config.WorkspaceRoot)
	} else {
		return "", agentdomain.ErrUnavailable
	}
	if err != nil {
		return "", err
	}
	if cmd == nil {
		return "", errors.New("ACP launcher returned no command")
	}
	// For a governed macOS task, HOME is an empty private per-process scratch
	// directory under the existing temporary area, not the Runtime user HOME.
	// Register cleanup before process cleanup so it runs after cmd.Wait().
	if !config.ToolSupply.IsZero() && runtime.GOOS == "darwin" {
		release, prepareErr := scopeGovernedAgentHome(cmd)
		if prepareErr != nil {
			return "", prepareErr
		}
		defer func() {
			if releaseErr := release(); releaseErr != nil {
				err = errors.Join(err, fmt.Errorf("remove private governed Agent HOME: %w", releaseErr))
			}
		}()
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	// Stderr is intentionally not copied to task results (may contain
	// prompts, auth metadata, local paths or other private information).
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start ACP Agent: %w", err)
	}
	defer func() {
		// CommandContext uses the Runtime process-isolation Cancel hook:
		// on Unix this kills the whole process group, not only the leader.
		stop()
		_ = stdin.Close()
		if cmd.Cancel != nil {
			_ = cmd.Cancel()
		} else if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	messages := make(chan rpcMessage, 32)
	readErrors := make(chan error, 1)
	go func() {
		defer close(messages)
		scan := bufio.NewScanner(stdout)
		scan.Buffer(make([]byte, 8192), maxFrameBytes)
		for scan.Scan() {
			var message rpcMessage
			if err := json.Unmarshal(scan.Bytes(), &message); err != nil {
				readErrors <- errors.New("ACP server sent invalid JSON")
				return
			}
			select {
			case messages <- message:
			case <-runCtx.Done():
				return
			}
		}
		if err := scan.Err(); err != nil {
			readErrors <- fmt.Errorf("ACP stream terminated: %w", err)
		}
	}()
	wire := stream{stdin: stdin, updates: messages}
	// Pre-session notifications are not retained; client requests continue
	// to be rejected by the shared stream's fail-closed handler.
	noUpdates := func(rpcMessage) error { return nil }
	init, err := wire.call(ctx, 1, "initialize", map[string]any{
		"protocolVersion": 1, "clientCapabilities": map[string]any{},
		"clientInfo": map[string]string{"name": "mender", "title": "Mender", "version": "dev"},
	}, noUpdates)
	if err != nil {
		return "", annotateRead(err, readErrors)
	}
	var hello struct {
		ProtocolVersion   int `json:"protocolVersion"`
		AgentCapabilities struct {
			MCPCapabilities struct {
				HTTP bool `json:"http"`
			} `json:"mcpCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err := json.Unmarshal(init, &hello); err != nil || hello.ProtocolVersion != 1 {
		return "", errors.New("ACP Agent does not support stable protocol v1")
	}
	servers := []any{}
	if !config.ToolSupply.IsZero() {
		// ACP v1 only. HTTP is OPTIONAL for every backend; no assumption
		// based on a Grok/OpenCode/Codex name and no host-side fallback.
		if !hello.AgentCapabilities.MCPCapabilities.HTTP {
			return "", fmt.Errorf("%w: Agent does not advertise ACP v1 HTTP MCP support", agentdomain.ErrUnavailable)
		}
		servers = append(servers, map[string]any{
			"type": "http", "name": "mender-task-tools", "url": config.ToolSupply.Endpoint,
			"headers": []map[string]string{{"name": "Authorization", "value": "Bearer " + config.ToolSupply.BearerToken}},
		})
	}
	session, err := wire.call(ctx, 2, "session/new", map[string]any{
		"cwd": config.WorkspaceRoot, "mcpServers": servers,
	}, noUpdates)
	if err != nil {
		return "", annotateRead(err, readErrors)
	}
	var created struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(session, &created); err != nil || created.SessionID == "" || len(created.SessionID) > 512 {
		return "", errors.New("ACP Agent returned invalid SessionID")
	}
	return visit(&wire, created.SessionID, session, readErrors)
}

func annotateRead(err error, streamErrors <-chan error) error {
	select {
	case streamErr := <-streamErrors:
		return errors.Join(err, streamErr)
	default:
		return err
	}
}
