package executionguard

import (
	"context"
	"fmt"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// ProfileBindingSerial enforces ToolBinding-local serial execution when configured.
type ProfileBindingSerial struct {
	serial keyedSerial
}

func NewProfileBindingSerial() *ProfileBindingSerial {
	return &ProfileBindingSerial{}
}

func (g *ProfileBindingSerial) Acquire(
	ctx context.Context,
	request invocationapplication.BindingExecutionGuardRequest,
) (func(), error) {
	constraint := request.Governance.ConcurrencyConstraint()
	switch constraint {
	case invocationdomain.BindingConcurrencyInherit:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return func() {}, nil

	case invocationdomain.BindingConcurrencySerial:
		scope := request.Context.Scope()
		if scope.Kind() != governancedomain.ScopeProfile {
			return nil, fmt.Errorf(
				"profile binding serial guard: serial constraint requires profile scope",
			)
		}
		key := scope.Reference() + "|" + request.Tool.ID().String()
		return g.serial.acquire(ctx, key)

	default:
		return nil, fmt.Errorf(
			"profile binding serial guard: unknown concurrency constraint %q",
			constraint,
		)
	}
}
