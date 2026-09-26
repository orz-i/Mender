package router

import (
	"context"
	"errors"
	"testing"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestRouterSelectsExecutorByProvider(t *testing.T) {
	t.Parallel()

	router := New()
	tool := testTool(t)
	executor := stubExecutor{}

	if err := router.Register(tool.ProviderID(), executor); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := router.ExecutorFor(context.Background(), tool)
	if err != nil {
		t.Fatalf("ExecutorFor() error = %v", err)
	}
	if _, ok := got.(stubExecutor); !ok {
		t.Fatalf("ExecutorFor() returned %T", got)
	}
}

func TestRouterReturnsExecutorNotFound(t *testing.T) {
	t.Parallel()

	_, err := New().ExecutorFor(context.Background(), testTool(t))
	if !errors.Is(err, invocationapplication.ErrExecutorNotFound) {
		t.Fatalf("ExecutorFor() error = %v", err)
	}
}

type stubExecutor struct{}

func (stubExecutor) Execute(
	context.Context,
	invocationapplication.ExecutionRequest,
	invocationapplication.CredentialResolver,
) (invocationdomain.Result, error) {
	value, _ := invocationdomain.NewJSONValue([]byte("null"))
	return invocationdomain.Complete(value)
}

func testTool(t *testing.T) catalogdomain.Tool {
	t.Helper()

	toolID, _ := catalogdomain.ParseToolID("tool_00000000000000000000000000000001")
	providerID, _ := catalogdomain.ParseProviderID("prov_00000000000000000000000000000001")
	schema, _ := catalogdomain.NewSchema([]byte(`{"type":"object"}`))
	tool, err := catalogdomain.NewTool(
		toolID,
		providerID,
		catalogdomain.ToolSpec{
			SourceKey:   "echo",
			Name:        "echo",
			InputSchema: schema,
		},
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}
