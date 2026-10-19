package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func TestACPFixtureProcess(t *testing.T) {
	if os.Getenv("MENDER_ACP_TEST_HELPER") != "1" {
		return
	}
	scan := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	send := func(msg any) { b, _ := json.Marshal(msg); _, _ = fmt.Fprintln(writer, string(b)); _ = writer.Flush() }
	var createdSessions int
	currentSessionID := "fixture-1"
	chosenModel := "fixture-one"
	chosenEffort := "high"
	// Deterministic ACP option fixtures: the first session may hide a
	// configured choice and the second may advertise it, but no prompt
	// is ever allowed while a configured choice remains unconfirmed.
	choiceOptions := func(mode string) []any {
		modelAvailable, effortAvailable := true, true
		switch mode {
		case "choice-first-model-missing", "choice-refresh-rejected", "choice-invalid-refreshed-session":
			modelAvailable = createdSessions > 1
		case "choice-always-model-missing", "choice-always-model-missing-mcp-http":
			modelAvailable = false
		case "choice-first-effort-missing":
			effortAvailable = createdSessions > 1
		case "choice-always-effort-missing":
			effortAvailable = false
		}
		models := []map[string]string{{"value": "fixture-one", "name": "Fixture One"}}
		if modelAvailable {
			models = append(models, map[string]string{"value": "fixture-two", "name": "Fixture Two"})
		}
		efforts := []map[string]string{{"value": "high", "name": "High"}}
		if effortAvailable {
			efforts = append(efforts, map[string]string{"value": "xhigh", "name": "Extra High"})
		}
		return []any{
			map[string]any{"id": "model", "category": "model", "type": "select", "currentValue": chosenModel, "options": models},
			map[string]any{"id": "reasoning_effort", "category": "thought_level", "type": "select", "currentValue": chosenEffort, "options": efforts},
		}
	}
	for scan.Scan() {
		var msg rpcMessage
		if err := json.Unmarshal(scan.Bytes(), &msg); err != nil {
			os.Exit(90)
		}
		switch msg.Method {
		case "initialize":
			if os.Getenv("ACP_FIXTURE_MODE") == "reject-initialize" {
				send(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"error": map[string]any{"code": -32001,
						"message": "FAKE_PRIVATE_BEARER_42 /Users/private/.ssh/id_test"}})
				continue
			}
			version := 1
			if os.Getenv("ACP_FIXTURE_MODE") == "version2" {
				version = 2
			}
			hello := map[string]any{"protocolVersion": version}
			mode := os.Getenv("ACP_FIXTURE_MODE")
			if mode == "session-capabilities" || mode == "acp-resume" || mode == "acp-mismatch" || mode == "acp-resume-rejected" {
				hello["agentCapabilities"] = map[string]any{
					"loadSession":         true,
					"sessionCapabilities": map[string]any{"resume": map[string]any{}},
				}
			}
			if mode == "acp-load" {
				hello["agentCapabilities"] = map[string]any{"loadSession": true}
			}
			if mode := os.Getenv("ACP_FIXTURE_MODE"); mode == "mcp-http" || mode == "choice-always-model-missing-mcp-http" {
				hello["agentCapabilities"] = map[string]any{"mcpCapabilities": map[string]any{"http": true}}
			}
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": hello})
		case "session/new":
			if strings.HasPrefix(os.Getenv("ACP_FIXTURE_MODE"), "acp-") {
				os.Exit(179)
			}
			createdSessions++
			currentSessionID = fmt.Sprintf("fixture-%d", createdSessions)
			if os.Getenv("ACP_FIXTURE_MODE") == "reject-session-new" {
				send(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"error": map[string]any{"code": -32002,
						"message": "FAKE_PRIVATE_BEARER_42 /Users/private/.ssh/id_test"}})
				continue
			}
			if mode := os.Getenv("ACP_FIXTURE_MODE"); mode == "mcp-http" || mode == "choice-always-model-missing-mcp-http" {
				var supply struct {
					MCPServers []struct {
						Type    string `json:"type"`
						Name    string `json:"name"`
						URL     string `json:"url"`
						Headers []struct {
							Name  string `json:"name"`
							Value string `json:"value"`
						} `json:"headers"`
					} `json:"mcpServers"`
				}
				if json.Unmarshal(msg.Params, &supply) != nil || len(supply.MCPServers) != 1 {
					os.Exit(160)
				}
				server := supply.MCPServers[0]
				if server.Type != "http" || server.Name != "mender-task-tools" ||
					server.URL != "http://127.0.0.1:12345/mcp" ||
					len(server.Headers) != 1 || server.Headers[0].Name != "Authorization" ||
					server.Headers[0].Value != "Bearer "+strings.Repeat("a", 64) {
					os.Exit(161)
				}
			}
			mode := os.Getenv("ACP_FIXTURE_MODE")
			if mode == "choice-refresh-rejected" && createdSessions > 1 {
				send(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"error": map[string]any{"code": -32001, "message": "REDACTED_UNTRUSTED_SESSION_ERROR"}})
				continue
			}
			if (mode == "choice-once" || mode == "choice-unconfirmed") && createdSessions > 1 {
				// A healthy/unconfirmed option must not create another session.
				os.Exit(130)
			}
			result := map[string]any{"sessionId": currentSessionID}
			if mode == "choice-invalid-refreshed-session" && createdSessions > 1 {
				result["sessionId"] = ""
			}
			if mode == "models" {
				result["configOptions"] = []any{map[string]any{
					"id": "model", "category": "model", "type": "select", "currentValue": "fixture-one",
					"options": []any{map[string]string{"value": "fixture-one", "name": "Fixture One"}, map[string]string{"value": "fixture-two", "name": "Fixture Two"}},
				}, map[string]any{"id": "reasoning_effort", "category": "thought_level", "type": "select", "currentValue": "high",
					"options": []any{map[string]string{"value": "high", "name": "High"}, map[string]string{"value": "xhigh", "name": "Extra high"}}}}
			}
			if strings.HasPrefix(mode, "choice-") {
				result["configOptions"] = choiceOptions(mode)
			}
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
		case "session/resume", "session/load":
			mode := os.Getenv("ACP_FIXTURE_MODE")
			if mode != "acp-resume" && mode != "acp-load" && mode != "acp-mismatch" && mode != "acp-resume-rejected" {
				os.Exit(181)
			}
			if (mode == "acp-load" && msg.Method != "session/load") ||
				(mode != "acp-load" && msg.Method != "session/resume") {
				os.Exit(182)
			}
			var req struct {
				SessionID  string `json:"sessionId"`
				Cwd        string `json:"cwd"`
				MCPServers []any  `json:"mcpServers"`
			}
			if json.Unmarshal(msg.Params, &req) != nil || req.SessionID != "fixture-1" || req.Cwd == "" || len(req.MCPServers) != 0 {
				os.Exit(183)
			}
			if mode == "acp-resume-rejected" {
				send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": -32001, "message": "SECRET_IN_UNTRUSTED_REPLY"}})
				continue
			}
			currentSessionID = "fixture-1"
			reply := map[string]any{}
			if mode == "acp-mismatch" {
				reply["sessionId"] = "wrong-session"
			}
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": reply})
		case "session/set_config_option":
			mode := os.Getenv("ACP_FIXTURE_MODE")
			if !strings.HasPrefix(mode, "choice-") {
				os.Exit(131)
			}
			var params struct {
				SessionID string `json:"sessionId"`
				ConfigID  string `json:"configId"`
				Value     string `json:"value"`
			}
			if json.Unmarshal(msg.Params, &params) != nil || params.SessionID != currentSessionID {
				os.Exit(132)
			}
			switch params.ConfigID {
			case "model":
				if params.Value != "fixture-two" {
					os.Exit(133)
				}
				chosenModel = params.Value
			case "reasoning_effort":
				if params.Value != "high" && params.Value != "xhigh" {
					os.Exit(134)
				}
				chosenEffort = params.Value
			default:
				os.Exit(135)
			}
			options := choiceOptions(mode)
			if mode == "choice-unconfirmed" {
				// Incorrect currentValue must fail closed, never retry a prompt.
				options = []any{map[string]string{"id": params.ConfigID, "type": "select", "currentValue": "fixture-one"}}
			}
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"configOptions": options}})
		case "session/prompt":
			mode := os.Getenv("ACP_FIXTURE_MODE")
			if mode == "choice-always-model-missing" || mode == "choice-always-model-missing-mcp-http" || mode == "choice-always-effort-missing" ||
				mode == "choice-unconfirmed" || mode == "choice-refresh-rejected" ||
				mode == "choice-invalid-refreshed-session" {
				// Missing, rejected or unconfirmed owner choices cannot prompt.
				os.Exit(136)
			}
			if mode == "reject-prompt" {
				send(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
					"error": map[string]any{"code": -32003,
						"message": "FAKE_PRIVATE_BEARER_42 /Users/private/.ssh/id_test"}})
				continue
			}
			if mode == "models" {
				os.Exit(92)
			} // inspection must not send a prompt
			if mode == "invalid-json" {
				_, _ = fmt.Fprintln(writer, `{"jsonrpc":`)
				_ = writer.Flush()
				os.Exit(85)
			}
			if mode == "close-prompt-early" {
				// The subprocess closes stdout without answering session/prompt.
				os.Exit(86)
			}
			if mode == "oversized-acp-frame" {
				// Exceed the scanner frame bound without retaining any frame data.
				_, _ = fmt.Fprintln(writer, strings.Repeat("x", maxFrameBytes+1))
				_ = writer.Flush()
				os.Exit(87)
			}
			if mode == "hang" {
				// Cancellation test: block until process is killed by caller.
				for {
					time.Sleep(time.Second)
				}
			}
			if mode == "client-file" {
				send(map[string]any{"jsonrpc": "2.0", "id": 96, "method": "fs/read_text_file", "params": map[string]string{"path": "/etc/passwd"}})
				if !scan.Scan() {
					os.Exit(81)
				}
				var response rpcMessage
				if err := json.Unmarshal(scan.Bytes(), &response); err != nil || response.Error == nil || response.Error.Code != -32601 {
					os.Exit(82)
				}
			}
			if mode == "approve-once" || mode == "reject-once" || mode == "approved-client-file" {
				send(map[string]any{"jsonrpc": "2.0", "id": 197, "method": "session/request_permission",
					"params": map[string]any{"sessionId": "fixture-1",
						"toolCall": map[string]any{"toolCallId": "fixture-tool", "kind": "read", "title": "Do not persist", "locations": []any{map[string]string{"path": os.Getenv("ACP_FIXTURE_TARGET_PATH")}}, "rawInput": map[string]string{"path": os.Getenv("ACP_FIXTURE_TARGET_PATH")}},
						"options": []any{
							map[string]string{"optionId": "offered-once", "kind": "allow_once"},
							map[string]string{"optionId": "offered-always", "kind": "allow_always"},
							map[string]string{"optionId": "offered-reject", "kind": "reject_once"},
						},
					},
				})
				if !scan.Scan() {
					os.Exit(151)
				}
				var response struct {
					Result struct {
						Outcome struct {
							Outcome  string `json:"outcome"`
							OptionID string `json:"optionId"`
						} `json:"outcome"`
					} `json:"result"`
				}
				if json.Unmarshal(scan.Bytes(), &response) != nil || response.Result.Outcome.Outcome != "selected" {
					os.Exit(152)
				}
				selected := response.Result.Outcome.OptionID
				if selected == "offered-always" || (mode == "reject-once" && selected != "offered-reject") || (mode != "reject-once" && selected != "offered-once") {
					os.Exit(153)
				}
				if mode == "approved-client-file" {
					send(map[string]any{"jsonrpc": "2.0", "id": 198, "method": "fs/write_text_file",
						"params": map[string]string{"path": "/tmp/mender-not-authorized", "content": "write forbidden"}})
					if !scan.Scan() {
						os.Exit(154)
					}
					var denied rpcMessage
					if json.Unmarshal(scan.Bytes(), &denied) != nil || denied.Error == nil || denied.Error.Code != -32601 {
						os.Exit(155)
					}
				}
			}
			if mode == "permission" || mode == "invalid-permission" {
				send(map[string]any{"jsonrpc": "2.0", "id": 97, "method": "session/request_permission", "params": map[string]any{
					"sessionId": "fixture-1", "options": []any{map[string]string{"optionId": "allow_once", "name": "Allow", "kind": "allow_once"}},
				}})
				if !scan.Scan() {
					os.Exit(81)
				}
				var response rpcMessage
				if err := json.Unmarshal(scan.Bytes(), &response); err != nil {
					os.Exit(82)
				}
				var value struct {
					Outcome struct {
						Outcome string `json:"outcome"`
					} `json:"outcome"`
				}
				if err := json.Unmarshal(response.Result, &value); err != nil || value.Outcome.Outcome != "cancelled" {
					os.Exit(83)
				}
			}
			if mode == "approve-once" {
				// Real ACP tool results are not Agent text. OpenCode v2 emits
				// non-text tool_call/tool_call_update content arrays here.
				for _, category := range []string{"tool_call", "tool_call_update"} {
					send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
						"sessionId": "fixture-1", "update": map[string]any{
							"sessionUpdate": category, "toolCallId": "fixture-tool",
							"content": []any{map[string]string{"type": "text", "text": "UNTRUSTED_TOOL_BODY_MUST_NOT_BE_RESULT"}},
						},
					}})
				}
			}
			if mode == "bad-text-chunk" {
				send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
					"sessionId": "fixture-1", "update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       []any{map[string]string{"type": "text", "text": "not a single content object"}},
					},
				}})
			}
			sessionID := "fixture-1"
			if strings.HasPrefix(mode, "choice-") {
				sessionID = currentSessionID
			}
			if mode == "foreign-session" {
				sessionID = "other-session"
			}
			text := "fixture completed"
			if strings.HasPrefix(mode, "choice-") {
				text = fmt.Sprintf("fixture completed after %d ACP sessions", createdSessions)
			}
			if mode == "oversized" {
				text = strings.Repeat("X", agentdomain.MaxResultBytes+1)
			}
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": sessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": text}},
			}})
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]string{"stopReason": "end_turn"}})
		default:
			os.Exit(84)
		}
	}
	os.Exit(0)
}

func fixtureAgent(t *testing.T) agentdomain.Agent {
	t.Helper()
	id, err := agentdomain.NewID()
	if err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Fixture", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: canonicalRoot,
		Isolation: "restricted_host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Spec().WorkspaceRoot, "fixture.txt"), []byte("synthetic local fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	return a
}
func fixtureRunner(mode string) Runner {
	factory := func(ctx context.Context, a agentdomain.Agent) (*exec.Cmd, error) {
		binary, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext( // #nosec G204 -- trusted current test executable and fixed test-only arguments.
			ctx, binary, "-test.run=^TestACPFixtureProcess$")
		cmd.Env = append(os.Environ(), "MENDER_ACP_TEST_HELPER=1", "ACP_FIXTURE_MODE="+mode, "ACP_FIXTURE_TARGET_PATH="+filepath.Join(a.Spec().WorkspaceRoot, "fixture.txt"))
		return cmd, nil
	}
	return Runner{NewCommand: factory, NewCommandWithConfig: func(ctx context.Context, a agentdomain.Agent, _ agentapplication.RunConfig) (*exec.Cmd, error) {
		return factory(ctx, a)
	}}
}

// Synthetic ACP fixtures use the same explicitly admitted root contract
// as production Agent task snapshots; no runtime compatibility fallback.
func executeFixture(ctx context.Context, mode string, a agentdomain.Agent, prompt string) (string, error) {
	return fixtureRunner(mode).ExecuteConfigured(ctx, a, prompt,
		agentapplication.RunConfig{WorkspaceRoot: a.Spec().WorkspaceRoot})
}
func TestACPRejectsMissingAdmittedRootBeforeStartingProcess(t *testing.T) {
	launched := false
	r := Runner{NewCommand: func(context.Context, agentdomain.Agent) (*exec.Cmd, error) {
		launched = true
		return nil, errors.New("should not be invoked")
	}}
	_, err := r.ExecuteConfigured(context.Background(), fixtureAgent(t), "safe task",
		agentapplication.RunConfig{})
	if !errors.Is(err, agentdomain.ErrInvalid) || launched {
		t.Fatalf("unreviewed default-root fallback accepted: launched=%t err=%v", launched, err)
	}
}
func TestACPStableHandshakeAndNoApproval(t *testing.T) {
	for _, mode := range []string{"completed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			got, err := executeFixture(ctx, mode, fixtureAgent(t), "task")
			if err != nil || got != "fixture completed" {
				t.Fatalf("result %q error %v", got, err)
			}
		})
	}
}
func TestACPFailClosedOnClientPermissionAndFilesystemRequests(t *testing.T) {
	for _, mode := range []string{"permission", "client-file"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			result, err := executeFixture(ctx, mode, fixtureAgent(t), "task")
			if !errors.Is(err, errClientPermissionDenied) || result != "" {
				t.Fatalf("ACP %s did not fail closed, result %q, error %v", mode, result, err)
			}
		})
	}
}

func TestACPRejectsInvalidProtocolAndUnboundedOrForeignOutput(t *testing.T) {
	for _, mode := range []string{"version2", "invalid-json", "foreign-session", "oversized", "bad-text-chunk"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			result, err := executeFixture(ctx, mode, fixtureAgent(t), "task")
			if err == nil || result != "" {
				t.Fatalf("untrusted ACP %s accepted with result %q, error %v", mode, result, err)
			}
		})
	}
}
func TestACPCancellationKillsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := executeFixture(ctx, "hang", fixtureAgent(t), "task")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want cancellation, got %v", err)
	}
}
func TestACPRejectsBadPrompts(t *testing.T) {
	ctx := context.Background()
	a := fixtureAgent(t)
	for _, input := range []string{"", strings.Repeat("X", agentdomain.MaxPromptBytes+1)} {
		if _, err := executeFixture(ctx, "completed", a, input); !errors.Is(err, agentdomain.ErrInvalid) {
			t.Fatal("accepted unsafe prompt", err)
		}
	}
}
func TestACPTaskToolSupplyUsesNegotiatedHTTPWithoutBackendAssumptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	// The fixture intentionally identifies as Grok. The ACP transport
	// negotiation is protocol-based, never a hardcoded OpenCode branch.
	agent := fixtureAgent(t)
	result, err := fixtureRunner("mcp-http").ExecuteConfigured(ctx, agent, "bounded task",
		agentapplication.RunConfig{
			WorkspaceRoot: agent.Spec().WorkspaceRoot,
			ToolSupply: agentapplication.ToolSupply{
				Delivery:    agentapplication.ToolDeliveryMCPHTTP,
				Endpoint:    "http://127.0.0.1:12345/mcp",
				BearerToken: strings.Repeat("a", 64),
			},
		})
	if err != nil || result != "fixture completed" {
		t.Fatalf("advertised MCP HTTP Tool delivery failed: %q %v", result, err)
	}
}
func TestACPTaskToolSupplyFailsClosedWhenHTTPIsNotAdvertised(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	agent := fixtureAgent(t)
	result, err := fixtureRunner("completed").ExecuteConfigured(ctx, agent, "bounded task",
		agentapplication.RunConfig{
			WorkspaceRoot: agent.Spec().WorkspaceRoot,
			ToolSupply: agentapplication.ToolSupply{
				Delivery:    agentapplication.ToolDeliveryMCPHTTP,
				Endpoint:    "http://127.0.0.1:12345/mcp",
				BearerToken: strings.Repeat("a", 64),
			},
		})
	if !errors.Is(err, agentdomain.ErrUnavailable) || result != "" {
		t.Fatalf("unadvertised HTTP tool supply silently downgraded: %q %v", result, err)
	}
}
func TestACPRejectsInvalidToolSupplyBeforeLaunchingAnyAgent(t *testing.T) {
	launched := false
	agent := fixtureAgent(t)
	runner := Runner{NewCommand: func(context.Context, agentdomain.Agent) (*exec.Cmd, error) {
		launched = true
		return nil, errors.New("should not launch")
	}}
	_, err := runner.ExecuteConfigured(context.Background(), agent, "bounded task",
		agentapplication.RunConfig{
			WorkspaceRoot: agent.Spec().WorkspaceRoot,
			ToolSupply: agentapplication.ToolSupply{
				Delivery:    agentapplication.ToolDeliveryMCPHTTP,
				Endpoint:    "https://untrusted.example/mcp",
				BearerToken: strings.Repeat("a", 64),
			},
		})
	if err == nil || launched {
		t.Fatalf("unsafe Task MCP endpoint permitted: launched=%t err=%v", launched, err)
	}
}

func TestACPTaskSupplyNeverFallsBackToUnrestrictedCommand(t *testing.T) {
	agent := fixtureAgent(t)
	launched := false
	runner := Runner{NewCommand: func(context.Context, agentdomain.Agent) (*exec.Cmd, error) {
		launched = true
		return nil, errors.New("unrestricted launcher must not be invoked")
	}}
	_, err := runner.ExecuteConfigured(context.Background(), agent, "governed task",
		agentapplication.RunConfig{WorkspaceRoot: agent.Spec().WorkspaceRoot,
			ToolSupply: agentapplication.ToolSupply{
				Delivery:    agentapplication.ToolDeliveryMCPHTTP,
				Endpoint:    "http://127.0.0.1:12345/mcp",
				BearerToken: strings.Repeat("a", 64),
			},
		})
	if !errors.Is(err, agentdomain.ErrUnavailable) || launched {
		t.Fatalf("unsafe host fallback when configured command is unavailable: launched=%t err=%v", launched, err)
	}
}
