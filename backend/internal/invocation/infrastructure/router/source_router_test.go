package router

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

type labelledExecutor struct{ id string }

func (e *labelledExecutor) Execute(_ context.Context, _ invocation.ExecutionRequest, _ invocation.CredentialResolver, _ invocation.ProgressReporter) (invocationdomain.Outcome, error) {
	value, _ := invocationdomain.NewJSONValue([]byte("null"))
	return invocationdomain.Complete(value)
}

func sourceTool(t *testing.T, id string, src catalog.SourceRef, key string) catalog.Tool {
	t.Helper()
	toolID, err := catalog.ParseToolID(id)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := catalog.NewSchema([]byte(`{}`))
	tool, err := catalog.NewToolFromSource(toolID, src, catalog.ToolSpec{
		SourceKey: key, Name: "invoke", InputSchema: input,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func TestTypedRouterUsesExactAgentSourceNotAgentBackend(t *testing.T) {
	ctx := context.Background()
	firstSource, _ := catalog.AgentSource("agent_11111111111111111111111111111111")
	secondSource, _ := catalog.AgentSource("agent_22222222222222222222222222222222")
	firstTool := sourceTool(t, "tool_11111111111111111111111111111111", firstSource, "agent.delegate")
	secondTool := sourceTool(t, "tool_22222222222222222222222222222222", secondSource, "agent.delegate")
	firstExecutor := &labelledExecutor{"OpenCode-A"}
	secondExecutor := &labelledExecutor{"OpenCode-B"}
	router := New()
	if err := router.RegisterSource(firstSource, firstExecutor); err != nil {
		t.Fatal(err)
	}
	if _, err := router.ExecutorFor(ctx, secondTool); !errors.Is(err, invocation.ErrExecutorNotFound) {
		t.Fatalf("second Agent borrowed first Agent executor: %v", err)
	}
	if err := router.RegisterSource(secondSource, secondExecutor); err != nil {
		t.Fatal(err)
	}
	if got, err := router.ExecutorFor(ctx, firstTool); err != nil || got != firstExecutor {
		t.Fatalf("first Agent route=%v, err=%v", got, err)
	}
	if got, err := router.ExecutorFor(ctx, secondTool); err != nil || got != secondExecutor {
		t.Fatalf("second Agent route=%v, err=%v", got, err)
	}
	unknownOperation := sourceTool(t, "tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", firstSource, "agent.unsupported")
	if _, err := router.ExecutorFor(ctx, unknownOperation); !errors.Is(err, invocation.ErrExecutorNotFound) {
		t.Fatalf("unknown operation used Agent route: %v", err)
	}
	if err := router.UnregisterSource(firstSource); err != nil {
		t.Fatal(err)
	}
	if router.HasSource(firstSource) || !router.HasSource(secondSource) {
		t.Fatal("unregister cross-deleted another Agent")
	}
}

func TestTypedRouterNativeToolIDMustMatchHostReviewedOperation(t *testing.T) {
	ctx := context.Background()
	router := New()
	native, _ := catalog.NativeSource("workspace")
	executor := &labelledExecutor{"workspace"}
	if err := router.RegisterSource(native, executor); err != nil {
		t.Fatal(err)
	}
	entries, err := curated.PlannedNativeTools()
	if err != nil {
		t.Fatal(err)
	}
	var real catalog.Tool
	for _, entry := range entries {
		if entry.Source == native && entry.SourceKey == "workspace.read" {
			real = sourceTool(t, entry.ToolID.String(), entry.Source, entry.SourceKey)
			break
		}
	}
	if real.ID() == "" {
		t.Fatal("no canonical native read Tool")
	}
	if got, err := router.ExecutorFor(ctx, real); err != nil || got != executor {
		t.Fatalf("canonical native Tool not routed: %v %v", got, err)
	}
	for _, forged := range []catalog.Tool{
		sourceTool(t, "tool_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", native, "workspace.read"),
		sourceTool(t, real.ID().String(), native, "workspace.shell"),
		sourceTool(t, real.ID().String(), native, "workspace.unknown"),
	} {
		if got, err := router.ExecutorFor(ctx, forged); got != nil || !errors.Is(err, invocation.ErrExecutorNotFound) {
			t.Fatalf("unreviewed Tool gained native executor: %s %s %v", forged.ID(), forged.SourceKey(), err)
		}
	}
	other, _ := catalog.NativeSource("jobs")
	if router.HasSource(other) {
		t.Fatal("jobs received workspace route")
	}
}

func TestTypedRouterRefusesInvalidRegistrationAndConcurrentCrossSourceConfusion(t *testing.T) {
	router := New()
	if err := router.RegisterSource(catalog.SourceRef{}, &labelledExecutor{"invalid"}); err == nil {
		t.Fatal("invalid source registered")
	}
	source, _ := catalog.AgentSource("agent_11111111111111111111111111111111")
	if err := router.RegisterSource(source, nil); err == nil {
		t.Fatal("nil executor registered")
	}
	if err := router.UnregisterSource(catalog.SourceRef{}); err == nil {
		t.Fatal("invalid source removal accepted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = router.RegisterSource(source, &labelledExecutor{"same"})
			_ = router.HasSource(source)
		}()
	}
	wg.Wait()
	if !router.HasSource(source) {
		t.Fatal("concurrent writes lost source")
	}
}
