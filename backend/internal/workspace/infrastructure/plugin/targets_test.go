package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type fakeTargets struct {
	providerID string
	key        string
	target     sandboxdomain.ID
}

func (f fakeTargets) ResolveWorkspaceTarget(_ context.Context, providerID, key string) (sandboxdomain.ID, error) {
	if providerID != f.providerID || key != f.key {
		return "", sandboxdomain.ErrNotAuthorized
	}
	return f.target, nil
}
func TestWorkspaceToolTargetAliasMakesTrustedSandboxFactAndStripsExecutorArgs(t *testing.T) {
	ctx := context.Background()
	tool := workspaceTestTool(t, sourceKeyRead)
	alternate := sandboxdomain.ID("sbx_12345678901234567890123456789012")
	f := fakeTargets{providerID: tool.ProviderID().String(), key: "project-b", target: alternate}
	raw, err := invocationdomain.NewJSONValue([]byte(`{"workspace":"project-b","path":"notes.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	target, clean, err := resolveSelection(ctx, f, f.providerID, sandboxdomain.ID(testSandboxID), raw)
	if err != nil || target != alternate {
		t.Fatalf("resolved=%s err=%v", target, err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(clean.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["workspace"]; exists || decoded["path"] != "notes.txt" {
		t.Fatalf("alias forwarded to backend: %s", clean.Bytes())
	}
	p := New(nil, f)
	facts, err := p.ExtractGovernanceFacts(ctx, json.RawMessage(`{"sandboxId":"`+testSandboxID+`"}`), tool, raw)
	if err != nil {
		t.Fatal(err)
	}
	resources := facts.Resources()
	if len(resources) != 2 || resources[0].Value() != "notes.txt" || resources[1].Kind() != "sandbox_id" || resources[1].Value() != alternate.String() {
		t.Fatalf("facts missing target: %+v", resources)
	}
	for _, payload := range []string{
		`{"workspace":"other","path":"notes.txt"}`,
		`{"workspace":"default","path":"notes.txt"}`,
		`{"workspace":"sbx_12345678901234567890123456789012","path":"notes.txt"}`,
		`{"workspace":"../escape","path":"notes.txt"}`,
	} {
		arg, err := invocationdomain.NewJSONValue([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := resolveSelection(ctx, f, f.providerID, sandboxdomain.ID(testSandboxID), arg); err == nil {
			t.Fatalf("unregistered/invalid workspace accepted: %s", payload)
		}
	}
	_, _, err = resolveSelection(ctx, f, "another-provider", sandboxdomain.ID(testSandboxID), raw)
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("cross-provider alias was accepted: %v", err)
	}
}

func TestWorkspaceAliasRebindingCannotChangeApprovedTarget(t *testing.T) {
	ctx := context.Background()
	instance, router := openWorkspaceTestInstance(t, t.TempDir())
	tool := workspaceTestTool(t, sourceKeyRead)
	first := sandboxdomain.ID("sbx_12345678901234567890123456789012")
	second := sandboxdomain.ID("sbx_abcdefabcdefabcdefabcdefabcdefab")
	target := &fakeTargets{providerID: tool.ProviderID().String(), key: "project-b", target: first}
	instance.targets = target
	instance.providerID = target.providerID
	args, err := invocationdomain.NewJSONValue([]byte(`{"workspace":"project-b","path":"proof.txt"}`))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := New(nil, target).ExtractGovernanceFacts(ctx,
		json.RawMessage(`{"sandboxId":"`+testSandboxID+`"}`), tool, args)
	if err != nil {
		t.Fatal(err)
	}
	if !admittedSandboxTarget(facts, first) || admittedSandboxTarget(facts, second) {
		t.Fatal("preflight facts do not pin an exact Sandbox")
	}
	// Simulate another authorized owner changing a target between Policy and
	// execution. The second resolution must fail before acquiring either Sandbox.
	target.target = second
	beforeResolve, beforeAcquire := router.resolves, router.acquires
	_, err = instance.Execute(ctx, invocationapp.ExecutionRequest{
		Tool: tool, Arguments: args, Context: fixtureCaller(t), AdmittedFacts: facts,
	}, nil, nil)
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("changed Sandbox escaped Policy fact fencing: %v", err)
	}
	if router.resolves != beforeResolve || router.acquires != beforeAcquire {
		t.Fatal("mismatched target reached Sandbox router")
	}
	// Direct execution of an additional target without policy-preflight facts
	// is equally forbidden, even though the alias exists.
	_, err = instance.Execute(ctx, invocationapp.ExecutionRequest{
		Tool: tool, Arguments: args, Context: fixtureCaller(t),
	}, nil, nil)
	if !errors.Is(err, sandboxdomain.ErrNotAuthorized) {
		t.Fatalf("additional target without approved facts: %v", err)
	}
}
