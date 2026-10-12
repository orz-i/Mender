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
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"protocolVersion": 1}})
		case "session/new":
			send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": map[string]any{"sessionId": "fixture-1"}})
		case "session/prompt":
			if os.Getenv("ACP_FIXTURE_MODE") == "hang" {
				// Cancellation test: block until process is killed by caller.
				for {
					time.Sleep(time.Second)
				}
			}
			if os.Getenv("ACP_FIXTURE_MODE") == "permission" {
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
			send(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{
				"sessionId": "fixture-1", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "fixture completed"}},
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
	a, err := agentdomain.NewAgent(agentdomain.Spec{ID: id, Name: "Fixture", Backend: agentdomain.BackendGrok,
		PluginID: agentdomain.BackendGrok.PluginID(), ExtensionID: "main", WorkspaceRoot: filepath.Clean(t.TempDir()),
		Isolation: "restricted_host", OwnerKind: "local_user", OwnerRef: "os:501", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func fixtureRunner(mode string) Runner {
	return Runner{NewCommand: func(ctx context.Context, _ agentdomain.Agent) (*exec.Cmd, error) {
		binary, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cmd := exec.CommandContext( // #nosec G204 -- trusted current test executable and fixed test-only arguments.
			ctx, binary, "-test.run=^TestACPFixtureProcess$")
		cmd.Env = append(os.Environ(), "MENDER_ACP_TEST_HELPER=1", "ACP_FIXTURE_MODE="+mode)
		return cmd, nil
	}}
}
func TestACPStableHandshakeAndNoApproval(t *testing.T) {
	for _, mode := range []string{"completed", "permission"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			got, err := fixtureRunner(mode).Execute(ctx, fixtureAgent(t), "task")
			if err != nil || got != "fixture completed" {
				t.Fatalf("result %q error %v", got, err)
			}
		})
	}
}
func TestACPCancellationKillsProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := fixtureRunner("hang").Execute(ctx, fixtureAgent(t), "task")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want cancellation, got %v", err)
	}
}
func TestACPRejectsBadPrompts(t *testing.T) {
	ctx := context.Background()
	a := fixtureAgent(t)
	for _, input := range []string{"", strings.Repeat("X", agentdomain.MaxPromptBytes+1)} {
		if _, err := fixtureRunner("completed").Execute(ctx, a, input); !errors.Is(err, agentdomain.ErrInvalid) {
			t.Fatal("accepted unsafe prompt", err)
		}
	}
}
