package bootstrap

import (
	"context"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	agentagy "github.com/orz-i/Mender/backend/internal/agent/infrastructure/antigravity"
)

// agentExecutionRouter is a transport router, never an Agent Loop. ACP owners
// keep ACP approvals; Antigravity owns its own headless native permissions.
type agentExecutionRouter struct {
	acp         agentacp.Runner
	antigravity agentagy.Runner
}

func (r agentExecutionRouter) ExecuteConfigured(ctx context.Context, a agentdomain.Agent, prompt string, cfg agentapplication.RunConfig) (string, error) {
	if a.Spec().Backend == agentdomain.BackendAntigravity {
		return r.antigravity.ExecuteConfigured(ctx, a, prompt, cfg)
	}
	return r.acp.ExecuteConfigured(ctx, a, prompt, cfg)
}

func (r agentExecutionRouter) InspectModels(ctx context.Context, a agentdomain.Agent) (agentapplication.ModelChoices, error) {
	if a.Spec().Backend == agentdomain.BackendAntigravity {
		return r.antigravity.InspectModels(ctx, a)
	}
	return r.acp.InspectModels(ctx, a)
}
