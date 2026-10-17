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
	"strings"
	"sync"

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
}

func (s *stream) send(message any) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
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
		case msg, ok := <-s.updates:
			if !ok {
				return nil, errors.New("ACP server closed before response")
			}
			if msg.JSONRPC != "2.0" {
				return nil, errors.New("unsupported ACP JSON-RPC version")
			}
			if len(msg.ID) > 0 && msg.Method != "" {
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
		var result strings.Builder
		var outputOverflow bool
		onUpdate := func(msg rpcMessage) error {
			if msg.Method != "session/update" {
				return nil
			}
			var params struct {
				SessionID string `json:"sessionId"`
				Update    struct {
					SessionUpdate string `json:"sessionUpdate"`
					Content       struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"update"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				return errors.New("ACP malformed session update")
			}
			if params.SessionID != sessionID {
				return errors.New("ACP update from unexpected SessionID")
			}
			if params.Update.SessionUpdate == "agent_message_chunk" && params.Update.Content.Type == "text" {
				if len(params.Update.Content.Text) > agentdomain.MaxResultBytes-result.Len() {
					outputOverflow = true
					return errors.New("ACP Agent result exceeds limit")
				}
				_, _ = result.WriteString(params.Update.Content.Text)
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
) (string, error) {
	if r.NewCommand == nil {
		return "", errors.New("ACP process launcher unavailable")
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
	var err error
	if config.WorkspaceRoot == agent.Spec().WorkspaceRoot {
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
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.Unmarshal(init, &hello); err != nil || hello.ProtocolVersion != 1 {
		return "", errors.New("ACP Agent does not support stable protocol v1")
	}
	session, err := wire.call(ctx, 2, "session/new", map[string]any{
		"cwd": config.WorkspaceRoot, "mcpServers": []any{},
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
