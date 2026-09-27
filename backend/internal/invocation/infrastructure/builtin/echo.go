// Package builtin contains deterministic local executors used to exercise the
// shared runtime without coupling it to a protocol adapter.
package builtin

import (
	"context"

	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// EchoExecutor returns the invocation arguments unchanged as a Complete result.
type EchoExecutor struct{}

func (EchoExecutor) Execute(
	ctx context.Context,
	request invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver,
) (invocationdomain.Outcome, error) {
	if err := ctx.Err(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	return invocationdomain.Complete(request.Arguments)
}
