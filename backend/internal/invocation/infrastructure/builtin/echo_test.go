package builtin

import (
	"bytes"
	"context"
	"testing"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

func TestEchoExecutorReturnsArguments(t *testing.T) {
	t.Parallel()

	arguments, _ := invocationdomain.NewJSONValue([]byte(`{"echo":"value"}`))
	result, err := (EchoExecutor{}).Execute(
		context.Background(),
		invocationapplication.ExecutionRequest{Arguments: arguments},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Kind() != invocationdomain.ResultKindComplete {
		t.Fatalf("result kind = %q", result.Kind())
	}
	if !bytes.Equal(result.Content().Bytes(), arguments.Bytes()) {
		t.Fatalf("result = %s, arguments = %s", result.Content().Bytes(), arguments.Bytes())
	}
}
