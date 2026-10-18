package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

type stagedFailureFixture struct {
	stage  string
	detail string
	cause  error
}

func (e stagedFailureFixture) Error() string { return e.detail }
func (e stagedFailureFixture) SafeAgentFailureStage() string {
	return e.stage
}
func (e stagedFailureFixture) Unwrap() error { return e.cause }

type diagnosticRunnerFixture struct{ err error }

func (f diagnosticRunnerFixture) ExecuteConfigured(context.Context, agentdomain.Agent, string, RunConfig) (string, error) {
	return "UNTRUSTED_AGENT_BODY_MUST_NOT_SURVIVE_FAILURE", f.err
}

func TestAgentFailureDiagnosticStrictAllowlist(t *testing.T) {
	secret := strings.Join([]string{"Bearer", "mender_private_token_98765", "at /Users/private/.ssh/secret"}, " ")
	cases := []struct {
		stage, diagnostic string
	}{
		{"process_launch", "Agent process could not start"},
		{"initialize", "ACP initialization failed"},
		{"session_create", "ACP session creation failed"},
		{"configuration", "Agent model or reasoning configuration failed"},
		{"configuration_model_unavailable", "Configured Agent model not advertised by ACP session"},
		{"configuration_reasoning_unavailable", "Configured Agent reasoning effort not advertised by ACP session"},
		{"configuration_confirmation_failed", "ACP configuration change not confirmed"},
		{"configuration_request_failed", "ACP configuration request failed"},
		{"permission", "Agent permission request was rejected"},
		{"permission_request_invalid", "ACP permission request is invalid"},
		{"permission_client_capability_unavailable", "Agent requested an unsupported ACP client capability"},
		{"prompt_rejected", "Agent rejected the ACP prompt"},
		{"prompt_transport", "ACP prompt transport failed"},
		{"prompt_stream_closed", "ACP prompt stream closed before completion"},
		{"prompt_malformed_frame", "ACP prompt frame was malformed"},
		{"prompt_read_failed", "ACP prompt stream read failed"},
		{"prompt_write_failed", "ACP prompt request write failed"},
		{"protocol", "ACP protocol validation failed"},
		{"result", "Agent returned an invalid completion"},
		{"../../" + secret, "Agent execution failed"},
		{"", "Agent execution failed"},
	}
	for _, test := range cases {
		t.Run(test.diagnostic+"-"+test.stage[:min(len(test.stage), 6)], func(t *testing.T) {
			failure := fmt.Errorf("wrapped untrusted failure: %w", stagedFailureFixture{
				stage: test.stage, detail: secret, cause: errors.New(secret),
			})
			got := agentFailureDiagnostic(failure)
			if got != test.diagnostic || strings.Contains(got, secret) ||
				strings.Contains(got, "/Users/") || strings.Contains(got, "Bearer") {
				t.Fatalf("unsafe Stage diagnostic: %q", got)
			}
		})
	}
	if diagnostic := agentFailureDiagnostic(errors.New(secret)); diagnostic != "Agent execution failed" {
		t.Fatal("unknown errors must remain generic")
	}
}

func TestAgentTaskPersistsOnlySafeFailureStageAndNoRawOutput(t *testing.T) {
	secret := strings.Join([]string{"Bearer", "TOKEN_TEST_777", "/Users/private/.ssh"}, " ")
	for _, test := range []struct{ stage, expected string }{
		{"prompt_rejected", "Agent rejected the ACP prompt"},
		{"session_create", "ACP session creation failed"},
		{"permission", "Agent permission request was rejected"},
		{"permission_request_invalid", "ACP permission request is invalid"},
		{"permission_client_capability_unavailable", "Agent requested an unsupported ACP client capability"},
		{"configuration_model_unavailable", "Configured Agent model not advertised by ACP session"},
		{"configuration_reasoning_unavailable", "Configured Agent reasoning effort not advertised by ACP session"},
		{"configuration_confirmation_failed", "ACP configuration change not confirmed"},
		{"configuration_request_failed", "ACP configuration request failed"},
		{"prompt_stream_closed", "ACP prompt stream closed before completion"},
		{"prompt_malformed_frame", "ACP prompt frame was malformed"},
		{"prompt_read_failed", "ACP prompt stream read failed"},
		{"prompt_write_failed", "ACP prompt request write failed"},
		{"exploit\n" + secret, "Agent execution failed"},
	} {
		t.Run(test.expected+"-"+test.stage[:min(6, len(test.stage))], func(t *testing.T) {
			svc, ownerCtx := newFixtureService(t, diagnosticRunnerFixture{err: stagedFailureFixture{
				stage: test.stage, detail: secret,
			}})
			ctx := context.Background()
			agent, err := svc.Create(ctx, CreateRequest{
				Name: "Safe diagnostic fixture", Backend: agentdomain.BackendGrok,
				WorkspaceRoot: t.TempDir(),
			}, ownerCtx)
			if err != nil {
				t.Fatal(err)
			}
			task, err := svc.Submit(ctx, agent.ID(), "bounded diagnostic test", ownerCtx)
			if err != nil {
				t.Fatal(err)
			}
			got := waitTask(t, svc, task.ID, ownerCtx, agentdomain.TaskFailed)
			if got.Diagnostic != test.expected || got.Result != "" ||
				strings.Contains(got.Diagnostic, secret) {
				t.Fatalf("Task leaked error or body: status=%s diagnostic=%q result=%q",
					got.Status, got.Diagnostic, got.Result)
			}
		})
	}
}
