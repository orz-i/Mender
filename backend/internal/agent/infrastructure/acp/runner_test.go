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
	for scan.Scan() {
		var msg rpcMessage
		if err := json.Unmarshal(scan.Bytes(), &msg); err != nil {
			os.Exit(90)
		}
		switch msg.Method {
		case "initialize":
			version := 1
			if os.Getenv("ACP_FIXTURE_MODE") == "version2" {
				version = 2
			}
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"protocolVersion": version}})
		case "session/new":
			result := map[string]any{"sessionId": "fixture-1"}
			if os.Getenv("ACP_FIXTURE_MODE") == "models" {
				result["configOptions"] = []any{map[string]any{
					"id": "model", "category": "model", "type": "select", "currentValue": "fixture-one",
					"options": []any{map[string]string{"value": "fixture-one", "name": "Fixture One"}, map[string]string{"value": "fixture-two", "name": "Fixture Two"}},
				}, map[string]any{"id": "reasoning_effort", "category": "thought_level", "type": "select", "currentValue": "high",
					"options": []any{map[string]string{"value": "high", "name": "High"}, map[string]string{"value": "xhigh", "name": "Extra high"}}}}
			}
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
		case "session/prompt":
			mode := os.Getenv("ACP_FIXTURE_MODE")
			if mode == "models" {
				os.Exit(92)
			} // inspection must not send a prompt
			if mode == "invalid-json" {
				_, _ = fmt.Fprintln(writer, `{"jsonrpc":`)
				_ = writer.Flush()
				os.Exit(85)
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
						"toolCall": map[string]any{"toolCallId": "fixture-tool", "kind": "read", "title": "Do not persist", "locations": []any{map[string]string{"path": os.Getenv("ACP_FIXTURE_TARGET_PATH")}}},
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
			if mode == "permission" {
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
			sessionID := "fixture-1"
			if mode == "foreign-session" {
				sessionID = "other-session"
			}
			text := "fixture completed"
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
	return Runner{NewCommand: func(ctx context.Context, a agentdomain.Agent) (*exec.Cmd, error) {
		binary, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext( // #nosec G204 -- trusted current test executable and fixed test-only arguments.
			ctx, binary, "-test.run=^TestACPFixtureProcess$")
		cmd.Env = append(os.Environ(), "MENDER_ACP_TEST_HELPER=1", "ACP_FIXTURE_MODE="+mode, "ACP_FIXTURE_TARGET_PATH="+filepath.Join(a.Spec().WorkspaceRoot, "fixture.txt"))
		return cmd, nil
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
	for _, mode := range []string{"version2", "invalid-json", "foreign-session", "oversized"} {
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
