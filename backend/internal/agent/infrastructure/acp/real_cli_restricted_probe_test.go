package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	processisolation "github.com/orz-i/Mender/backend/internal/runtimegovernance/infrastructure/processisolation"
)

// This is an explicitly operator-triggered no-prompt diagnostic only.
// It runs the *production* restricted-host command plan, retaining no
// credentials, stdout payload, raw stderr or bearer in test output.
func TestPhase46OptInRealRestrictedACPInitialize(t *testing.T) {
	backend := agentdomain.Backend(os.Getenv("MENDER_PHASE46_REAL_ACP_PROBE"))
	if backend == "" {
		t.Skip("real external CLI startup is opt-in")
	}
	if backend != agentdomain.BackendOpenCode && backend != agentdomain.BackendGrok {
		t.Fatal("expected opencode or grok")
	}
	if runtime.GOOS != "darwin" || !processisolation.New().Info().RestrictedHostAvailable {
		t.Skip("macOS Seatbelt unavailable")
	}
	factory, err := NewLocalFactory(processisolation.New())
	if err != nil {
		t.Fatal(err)
	}
	spec := fixtureAgent(t).Spec()
	spec.Backend = backend
	spec.PluginID = backend.PluginID()
	control := os.Getenv("MENDER_PHASE46_REAL_ACP_CONTROL")
	if control == "host" {
		// No-prompt/no-tool diagnostic only; never used for governed supply.
		spec.Isolation = "host"
	}
	agent, err := agentdomain.NewAgent(spec)
	if err != nil {
		t.Fatal(err)
	}
	timeout := 8 * time.Second
	if os.Getenv("MENDER_PHASE46_REAL_ACP_SESSION_PROBE") == "1" {
		timeout = 23 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	mode := "governed_read_only_root"
	var command *exec.Cmd
	if control == "1" || control == "host" {
		// Non-governed startup controls receive NO Task capability and
		// can never prove the restricted real Task acceptance gate.
		mode = "ordinary_restricted_control"
		if control == "host" {
			mode = "host_no_prompt_control"
		}
		command, err = factory.CommandInWorkspace(ctx, agent, spec.WorkspaceRoot)
	} else {
		command, err = factory.CommandWithConfig(ctx, agent, agentapp.RunConfig{
			WorkspaceRoot: spec.WorkspaceRoot,
			ToolSupply: agentapp.ToolSupply{Delivery: agentapp.ToolDeliveryMCPHTTP,
				Endpoint: "http://127.0.0.1:12345/mcp", BearerToken: strings.Repeat("a", 64)},
		})
	}
	if err != nil {
		t.Fatalf("unable to construct production read-only-root command plan: %v", err)
	}
	if control != "host" && command.Path != "/usr/bin/sandbox-exec" {
		t.Fatal("restricted real ACP probe lacks Seatbelt confinement")
	}
	// No secrets or real HOME reads: this test-only variant changes the
	// process's HOME variable to a brand new empty directory in the existing
	// Seatbelt-permitted temporary area; actual isolation stays unchanged.
	if os.Getenv("MENDER_PHASE46_REAL_ACP_EMPTY_HOME") == "1" {
		fakeHome := t.TempDir()
		for i, item := range command.Env {
			if strings.HasPrefix(item, "HOME=") {
				command.Env[i] = "HOME=" + fakeHome
			}
		}
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var privateStderr bytes.Buffer
	command.Stderr = &privateStderr
	if err := command.Start(); err != nil {
		t.Fatalf("Seatbelt child process could not start (%s)", classifyRestrictedProcessError(err.Error()))
	}
	line := make(chan []byte, 8)
	go func() {
		scan := bufio.NewScanner(stdout)
		scan.Buffer(make([]byte, 4096), 65536)
		for scan.Scan() {
			frame := append([]byte(nil), scan.Bytes()...)
			select {
			case line <- frame:
			case <-ctx.Done():
				close(line)
				return
			}
		}
		close(line)
	}()
	_, _ = fmt.Fprintln(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientCapabilities":{},"clientInfo":{"name":"mender","title":"Mender","version":"dev"}}}`)
	stage := "no_acp_initialize_reply"
	select {
	case raw, ok := <-line:
		if ok {
			var parsed struct {
				ID     json.RawMessage `json:"id"`
				Result struct {
					ProtocolVersion   int `json:"protocolVersion"`
					AgentCapabilities struct {
						MCPCapabilities struct {
							HTTP bool `json:"http"`
						} `json:"mcpCapabilities"`
					} `json:"agentCapabilities"`
				} `json:"result"`
			}
			if json.Unmarshal(raw, &parsed) == nil && parsed.Result.ProtocolVersion == 1 {
				if parsed.Result.AgentCapabilities.MCPCapabilities.HTTP {
					stage = "acp_v1_http_advertised"
				} else {
					stage = "acp_v1_without_http"
				}
			} else {
				stage = "unexpected_first_frame"
			}
		}
	case <-ctx.Done():
		stage = "initialize_timeout"
	}
	if stage == "acp_v1_http_advertised" && os.Getenv("MENDER_PHASE46_REAL_ACP_SESSION_PROBE") == "1" {
		// A no-prompt session/new only: no model inference or Tool bearer.
		_, _ = fmt.Fprintf(stdin, "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"session/new\",\"params\":{\"cwd\":%q,\"mcpServers\":[]}}\n", spec.WorkspaceRoot)
		stage = "session_new_unanswered"
		for stage == "session_new_unanswered" {
			select {
			case raw, ok := <-line:
				if !ok {
					stage = "session_new_closed"
					break
				}
				var response struct {
					ID     json.RawMessage `json:"id"`
					Result struct {
						SessionID string `json:"sessionId"`
					} `json:"result"`
					Error *struct {
						Code int `json:"code"`
					} `json:"error"`
				}
				if json.Unmarshal(raw, &response) == nil && string(response.ID) == "2" {
					if response.Error != nil {
						stage = "session_new_acp_rejected"
					} else if response.Result.SessionID != "" {
						stage = "session_new_completed"
					} else {
						stage = "session_new_malformed"
					}
				}
			case <-ctx.Done():
				stage = "session_new_timeout"
			}
		}
	}
	cancel()
	_ = stdin.Close()
	if command.Process != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	exitCategory := "no_exit_error"
	if waitErr != nil {
		if errors.Is(waitErr, exec.ErrNotFound) {
			exitCategory = "binary_not_found"
		} else if strings.Contains(waitErr.Error(), "signal") {
			exitCategory = "terminated_by_test"
		} else {
			exitCategory = "child_exited_nonzero"
		}
	}
	plan := "sandbox-exec"
	if control == "host" {
		plan = "host_no_mcp_bearer"
	}
	t.Logf("real_backend=%s, plan=%s, mode=%s, stage=%s, stderr_category=%s, exit_category=%s, stderr_bytes=%d, stderr_markers=%s",
		backend, plan, mode, stage, classifyRestrictedProcessError(privateStderr.String()), exitCategory, privateStderr.Len(), restrictedErrorMarkers(privateStderr.String()))
	expected := "acp_v1_http_advertised"
	if os.Getenv("MENDER_PHASE46_REAL_ACP_SESSION_PROBE") == "1" {
		expected = "session_new_completed"
	}
	if stage != expected {
		t.Fatalf("real ACP readiness stage incomplete; no permission broadening or host fallback")
	}
}

// Only report membership in a fixed, public vocabulary; diagnostic stdout
// must never expose unbounded CLI stderr, user paths or account material.
func restrictedErrorMarkers(raw string) string {
	value := strings.ToLower(raw)
	known := []string{"auth", "home", "sandbox", "config", "data", "path", "cache", "read", "write", "access", "error", "failed", "denied", "permission", "runtime", "bun", "node", "sqlite", "network", "symlink", "file", "directory", "executable", "environment", "storage", "keychain", "operation"}
	matched := []string{}
	for _, word := range known {
		if strings.Contains(value, word) {
			matched = append(matched, word)
		}
	}
	return strings.Join(matched, ",")
}

func classifyRestrictedProcessError(raw string) string {
	value := strings.ToLower(raw)
	switch {
	case strings.Contains(value, "operation not permitted") || strings.Contains(value, "sandbox violation"):
		return "os_isolation_denied"
	case strings.Contains(value, "permission denied") || strings.Contains(value, "eacces"):
		return "filesystem_access_denied"
	case strings.Contains(value, "cannot find module") || strings.Contains(value, "module not found"):
		return "runtime_module_unavailable"
	case strings.Contains(value, "enoent") || strings.Contains(value, "no such file"):
		return "runtime_dependency_missing"
	case strings.Contains(value, "authentication") || strings.Contains(value, "not logged in"):
		return "cli_auth_unavailable"
	case strings.Contains(value, "read-only") || strings.Contains(value, "erofs"):
		return "workspace_root_read_only"
	case strings.Contains(value, "no space left") || strings.Contains(value, "enospc"):
		return "runtime_storage_full"
	case strings.Contains(value, "dyld") || strings.Contains(value, "dynamic linker"):
		return "runtime_dynamic_linker_error"
	case strings.Contains(value, "exception") || strings.Contains(value, "stack trace"):
		return "runtime_exception"
	case strings.Contains(value, "panic") || strings.Contains(value, "fatal"):
		return "cli_internal_failure"
	case strings.TrimSpace(value) == "":
		return "no_stderr"
	default:
		return "unclassified_stderr"
	}
}
