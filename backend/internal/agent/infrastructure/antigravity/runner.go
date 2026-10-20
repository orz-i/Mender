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
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const maxEventBytes = 1 << 20
const maxModelListingBytes = 64 << 10
const resumeHandshakeTimeout = 45 * time.Second

// CommandFactory is supplied by the trusted, allowlisted Mender launcher;
// the last booleans select read-only model inspection and authorized YOLO.
type CommandFactory func(context.Context, agentdomain.Agent, string, string, string, bool, bool) (*exec.Cmd, error)
type ResumeCommandFactory func(context.Context, agentdomain.Agent, string, string, string, string, bool) (*exec.Cmd, error)

type Runner struct {
	NewCommand       CommandFactory
	NewResumeCommand ResumeCommandFactory
}

type stageError string

func (e stageError) Error() string                 { return string(e) }
func (e stageError) SafeAgentFailureStage() string { return string(e) }

// ExecuteConfigured sends the prompt ONLY on stdin. No prompt, credential,
// tool arguments or native tool output are ever persisted in diagnostics.
func (r Runner) ExecuteConfigured(ctx context.Context, agent agentdomain.Agent, prompt string, cfg agentapplication.RunConfig) (result string, runErr error) {
	if cfg.Resume != nil && (r.NewResumeCommand == nil || !cfg.Resume.Valid() ||
		cfg.Resume.Backend != agentdomain.BackendAntigravity ||
		cfg.Resume.AgentID != agent.ID() || cfg.Resume.Isolation != "host" ||
		agent.Spec().Isolation != "host" || cfg.Resume.WorkspaceRoot != cfg.WorkspaceRoot ||
		!agentdomain.ValidAntigravityConversationID(cfg.Resume.NativeSessionID)) {
		return "", agentdomain.ErrResumeUnavailable
	}
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
	var cmd *exec.Cmd
	var err error
	if cfg.Resume != nil {
		cmd, err = r.NewResumeCommand(ctx, agent, cfg.WorkspaceRoot, cfg.Model, cfg.ReasoningEffort,
			cfg.Resume.NativeSessionID, cfg.AllowNativeYOLO)
	} else {
		cmd, err = r.NewCommand(ctx, agent, cfg.WorkspaceRoot, cfg.Model, cfg.ReasoningEffort, false, cfg.AllowNativeYOLO)
	}
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
	registeredProcess := false
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
		if registeredProcess && cfg.Observe != nil {
			if err := cfg.Observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventLeaderReaped}); err != nil {
				runErr = errors.Join(runErr, fmt.Errorf("%w: process exit evidence unavailable", agentdomain.ErrOutcomeUncertain))
			}
		}
	}()
	if cfg.Observe != nil {
		if err := cfg.Observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventProcessStarted,
			ProcessPID: cmd.Process.Pid}); err != nil {
			return "", stageError("process_launch")
		}
		registeredProcess = true
	}

	// New conversations may emit init after receiving the prompt. By contrast,
	// Agy v1.4.2 emits init BEFORE a prompt for --conversation, including when
	// it silently falls back to a new Session for an invalid/native-missing ID.
	// This pre-prompt identity gate is what makes explicit continuation safe.
	scanner := newStreamScanner(stdout)
	alreadyInitialized := false
	if cfg.Resume != nil {
		if err := scanResumeInit(ctx, cmd, scanner); err != nil {
			return "", err
		}
		var initFrame struct {
			Event          string `json:"event"`
			ConversationID string `json:"conversation_id"`
		}
		if json.Unmarshal(scanner.Bytes(), &initFrame) != nil || initFrame.Event != "init" ||
			initFrame.ConversationID != cfg.Resume.NativeSessionID {
			return "", agentdomain.ErrResumeUnavailable
		}
		// Atomically bind the same native SessionRef to this new Task before
		// sending anything actionable. Persistence failure here is pre-prompt
		// and must never trigger a fallback execution in a fresh conversation.
		if cfg.Observe != nil {
			if err := cfg.Observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventSessionReady,
				NativeSession: initFrame.ConversationID, ExistingSessionRef: cfg.Resume.SessionRef,
				CanLoad: true, CanResume: true}); err != nil {
				return "", fmt.Errorf("antigravity pre-prompt session evidence: %w", err)
			}
		}
		alreadyInitialized = true
	}

	// --input-format stream-json accepts one user message per NDJSON line.
	if err := json.NewEncoder(stdin).Encode(map[string]any{
		"event": "user", "message": map[string]any{"content": prompt},
	}); err != nil {
		// A partial stdin write may already have delivered an actionable
		// user turn; this cannot be treated as a safe pre-prompt failure.
		return "", fmt.Errorf("%w: Antigravity prompt transport uncertain", agentdomain.ErrOutcomeUncertain)
	}
	if err := stdin.Close(); err != nil {
		return "", fmt.Errorf("%w: Antigravity prompt close outcome uncertain", agentdomain.ErrOutcomeUncertain)
	}
	if cfg.Observe != nil {
		if err := cfg.Observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventPromptSent}); err != nil {
			return "", fmt.Errorf("%w: prompt dispatch evidence unavailable", agentdomain.ErrOutcomeUncertain)
		}
	}

	response, err := parseScannerObserved(scanner, cfg.Observe, alreadyInitialized, cfg.Resume)
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
	return parseStreamObserved(reader, nil)
}

func newStreamScanner(reader io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 16<<10), maxEventBytes)
	return scanner
}

// Resume is a two-phase protocol. The first native init must arrive within a
// small bound, independent of the user's 10/20/30-minute execution budget.
// CLI authentication/network stalls must not hold an unverified Session open
// for the whole Task. Killing the process cannot send a business prompt.
func scanResumeInit(ctx context.Context, cmd *exec.Cmd, scanner *bufio.Scanner) error {
	received := make(chan bool, 1)
	go func() { received <- scanner.Scan() }()
	timer := time.NewTimer(resumeHandshakeTimeout)
	defer timer.Stop()
	select {
	case ok := <-received:
		if !ok {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return agentdomain.ErrResumeUnavailable
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		if cmd.Cancel != nil {
			_ = cmd.Cancel()
		} else if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return agentdomain.ErrResumeUnavailable
	}
}

func parseStreamObserved(reader io.Reader, observe func(agentdomain.ExecutionEvent) error) (string, error) {
	return parseScannerObserved(newStreamScanner(reader), observe, false, nil)
}

func resumedProtocolError(resume *agentdomain.SessionContinuation) error {
	if resume != nil {
		// A new prompt has already been dispatched to an exactly validated
		// Session. Broken/missing native terminal evidence leaves the outcome
		// uncertain even if a client process has exited.
		return fmt.Errorf("%w: resumed native Agent protocol lost terminal proof", agentdomain.ErrOutcomeUncertain)
	}
	return stageError("headless_protocol")
}

func parseScannerObserved(scanner *bufio.Scanner, observe func(agentdomain.ExecutionEvent) error,
	gotInit bool, resume *agentdomain.SessionContinuation) (string, error) {
	var result string
	gotResult := false
	lastProgress := time.Time{}
	for scanner.Scan() {
		var frame struct {
			Event          string          `json:"event"`
			ConversationID string          `json:"conversation_id"`
			Result         json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			return "", resumedProtocolError(resume)
		}
		switch frame.Event {
		case "init":
			if gotInit || gotResult {
				return "", resumedProtocolError(resume)
			}
			gotInit = true
			if frame.ConversationID != "" && observe != nil {
				validated := agentdomain.ValidAntigravityConversationID(frame.ConversationID)
				if err := observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventSessionReady,
					NativeSession: frame.ConversationID, CanLoad: validated, CanResume: validated}); err != nil {
					return "", fmt.Errorf("%w: native session evidence unavailable", agentdomain.ErrOutcomeUncertain)
				}
			}
		case "step_update":
			if !gotInit || gotResult {
				return "", resumedProtocolError(resume)
			}
			if observe != nil && time.Since(lastProgress) >= 5*time.Second {
				if err := observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventProgress}); err != nil {
					return "", fmt.Errorf("%w: native progress evidence unavailable", agentdomain.ErrOutcomeUncertain)
				}
				lastProgress = time.Now()
			}
		case "result":
			if !gotInit || gotResult || len(frame.Result) == 0 {
				return "", resumedProtocolError(resume)
			}
			if resume != nil && frame.ConversationID != "" &&
				frame.ConversationID != resume.NativeSessionID {
				// The new prompt has already reached the native conversation.
				// A mismatched terminal result cannot prove its actual outcome.
				return "", fmt.Errorf("%w: resumed conversation terminal identity mismatch", agentdomain.ErrOutcomeUncertain)
			}
			gotResult = true
			var terminal struct {
				Status   string `json:"status"`
				Response string `json:"response"`
			}
			if err := json.Unmarshal(frame.Result, &terminal); err != nil {
				return "", resumedProtocolError(resume)
			}
			switch terminal.Status {
			case "SUCCESS":
				if len(terminal.Response) > agentdomain.MaxResultBytes {
					return "", stageError("result")
				}
				result = terminal.Response
				if observe != nil {
					if err := observe(agentdomain.ExecutionEvent{Kind: agentdomain.EventTerminalSeen}); err != nil {
						return "", fmt.Errorf("%w: native terminal evidence unavailable", agentdomain.ErrOutcomeUncertain)
					}
				}
			case "WAITING":
				return "", stageError("headless_permission")
			default:
				return "", stageError("headless_rejected")
			}
		default:
			// Unknown types cannot be silently interpreted as a success.
			return "", resumedProtocolError(resume)
		}
	}
	if err := scanner.Err(); err != nil {
		if resume != nil {
			return "", fmt.Errorf("%w: resumed Agent output channel ended without terminal proof", agentdomain.ErrOutcomeUncertain)
		}
		return "", stageError("headless_protocol")
	}
	if !gotInit || !gotResult {
		if resume != nil {
			return "", fmt.Errorf("%w: resumed Agent did not provide native terminal proof", agentdomain.ErrOutcomeUncertain)
		}
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
