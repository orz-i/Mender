// Package antigravity integrates the official Antigravity CLI headless protocol.
// It does not implement ACP or the Agent's reasoning loop. No native-tool
// permission request is forwarded to Mender's ACP approval inbox.
package antigravity

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const maxEventBytes = 1 << 20
const maxModelListingBytes = 64 << 10

// CommandFactory is supplied by the trusted, allowlisted Mender launcher;
// the last booleans select read-only model inspection and authorized YOLO.
type CommandFactory func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error)

type Runner struct{ NewCommand CommandFactory }

type stageError string

func (e stageError) Error() string                 { return string(e) }
func (e stageError) SafeAgentFailureStage() string { return string(e) }

// ExecuteConfigured sends the prompt ONLY on stdin. No prompt, credential,
// tool arguments or native tool output are ever persisted in diagnostics.
func (r Runner) ExecuteConfigured(ctx context.Context, agent agentdomain.Agent, prompt string, cfg agentapplication.RunConfig) (string, error) {
	if agent.Spec().Backend != agentdomain.BackendAntigravity || r.NewCommand == nil {
		return "", stageError("process_launch")
	}
	if err := cfg.ToolSupply.Validate(); err != nil {
		return "", stageError("configuration")
	}
	// The official headless protocol cannot receive scoped Mender MCP bearer
	// tokens. Refuse BEFORE process start instead of falling back to Host tools.
	if !cfg.ToolSupply.IsZero() {
		return "", stageError("headless_tool_supply")
	}
	if cfg.WorkspaceRoot == "" || strings.TrimSpace(prompt) == "" || len(prompt) > agentdomain.MaxPromptBytes {
		return "", stageError("configuration")
	}
	cmd, err := r.NewCommand(ctx, agent, cfg.WorkspaceRoot, cfg.Model, cfg.ReasoningEffort, false, cfg.AllowNativeYOLO)
	if err != nil || cmd == nil {
		return "", stageError("process_launch")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", stageError("process_launch")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return "", stageError("process_launch")
	}
	// Antigravity stderr contains error details and account diagnostics.
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return "", stageError("process_launch")
	}
	waited := false
	defer func() {
		_ = stdin.Close()
		if !waited {
			if cmd.Cancel != nil {
				_ = cmd.Cancel()
			} else if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
		}
	}()
	// --input-format stream-json accepts one user message per NDJSON line.
	if err := json.NewEncoder(stdin).Encode(map[string]any{
		"event": "user", "message": map[string]any{"content": prompt},
	}); err != nil {
		return "", stageError("prompt_transport")
	}
	if err := stdin.Close(); err != nil {
		return "", stageError("prompt_transport")
	}

	response, err := parseStream(stdout)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	waitErr := cmd.Wait()
	waited = true
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if waitErr != nil {
		return "", stageError("headless_rejected")
	}
	return response, nil
}

func parseStream(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 16<<10), maxEventBytes)
	var result string
	gotResult := false
	gotInit := false
	for scanner.Scan() {
		var frame struct {
			Event  string          `json:"event"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return "", stageError("headless_protocol")
		}
		switch frame.Event {
		case "init":
			if gotInit || gotResult {
				return "", stageError("headless_protocol")
			}
			gotInit = true
		case "step_update":
			if !gotInit || gotResult {
				return "", stageError("headless_protocol")
			}
		case "result":
			if !gotInit || gotResult || len(frame.Result) == 0 {
				return "", stageError("headless_protocol")
			}
			gotResult = true
			var terminal struct {
				Status   string `json:"status"`
				Response string `json:"response"`
			}
			if err := json.Unmarshal(frame.Result, &terminal); err != nil {
				return "", stageError("headless_protocol")
			}
			switch terminal.Status {
			case "SUCCESS":
				if len(terminal.Response) > agentdomain.MaxResultBytes {
					return "", stageError("result")
				}
				result = terminal.Response
			case "WAITING":
				return "", stageError("headless_permission")
			default:
				return "", stageError("headless_rejected")
			}
		default:
			// Unknown types cannot be silently interpreted as a success.
			return "", stageError("headless_protocol")
		}
	}
	if err := scanner.Err(); err != nil {
		return "", stageError("headless_protocol")
	}
	if !gotInit || !gotResult {
		return "", stageError("headless_protocol")
	}
	return result, nil
}

// InspectModels is an explicit no-prompt, owner-only action. CLI v1.4.2
// reports one `<slug>\\t<name>` per line; it does not accept `--output-format`
// on the models subcommand. Model choices are not inferred from a marketing list.
func (r Runner) InspectModels(ctx context.Context, agent agentdomain.Agent) (agentapplication.ModelChoices, error) {
	if agent.Spec().Backend != agentdomain.BackendAntigravity || r.NewCommand == nil {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
	}
	cmd, err := r.NewCommand(ctx, agent, agent.Spec().WorkspaceRoot, "", "", true, false)
	if err != nil || cmd == nil {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
	}
	cmd.Stderr = io.Discard
	var output bytes.Buffer
	cmd.Stdout = &limitedWriter{writer: &output, limit: maxModelListingBytes}
	if err := cmd.Run(); err != nil {
		return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
	}
	return parseModels(output.String())
}

type limitedWriter struct {
	writer io.Writer
	limit  int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		return 0, errors.New("output exceeds limit")
	}
	n, err := w.writer.Write(p)
	w.limit -= n
	return n, err
}

func parseModels(output string) (agentapplication.ModelChoices, error) {
	result := agentapplication.ModelChoices{Models: []agentapplication.ModelChoice{}}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) != 2 {
			return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
		}
		slug, name := fields[0], strings.TrimSpace(fields[1])
		if err := agentdomain.ValidateModel(slug); err != nil || slug == "" || seen[slug] || name == "" || len(name) > 120 || len(result.Models) >= 64 {
			return agentapplication.ModelChoices{}, agentdomain.ErrUnavailable
		}
		seen[slug] = true
		result.Models = append(result.Models, agentapplication.ModelChoice{ID: slug, Name: name})
	}
	if len(result.Models) == 0 {
		return agentapplication.ModelChoices{}, fmt.Errorf("%w: empty Antigravity model list", agentdomain.ErrUnavailable)
	}
	return result, nil
}
