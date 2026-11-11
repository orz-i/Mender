package delegationprovider

import (
	"context"
	"fmt"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"

	agentapp "github.com/orz-i/Mender/backend/internal/agent/application"
	agent "github.com/orz-i/Mender/backend/internal/agent/domain"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

// OwnedExecutor exposes a configured Agent directly as an Agent-owned Tool
// source, without creating an internal Provider. It delegates to the already
// audited Instance.Execute Task operations, so Task/Session/ACP/unknown/resume
// have exactly one business implementation.
type OwnedExecutor struct {
	service *agentapp.Service
	agentID agent.ID
}

func NewOwnedExecutor(service *agentapp.Service, id agent.ID) (*OwnedExecutor, error) {
	if service == nil {
		return nil, fmt.Errorf("agent as Tool executor: task service unavailable")
	}
	if _, err := agent.ParseID(string(id)); err != nil {
		return nil, err
	}
	return &OwnedExecutor{service: service, agentID: id}, nil
}

func (e *OwnedExecutor) Execute(ctx context.Context, req invocationapp.ExecutionRequest,
	credentials invocationapp.CredentialResolver, progress invocationapp.ProgressReporter,
) (invocation.Outcome, error) {
	if e == nil || e.service == nil {
		return invocation.Outcome{}, agent.ErrUnavailable
	}
	source, err := catalog.AgentSource(string(e.agentID))
	if err != nil || req.Tool.SourceRef() != source || req.Context.Validate() != nil {
		return invocation.Outcome{}, agent.ErrForbidden
	}
	// Instance has no provider-specific execution semantics: every operation
	// already passes this exact AgentID and the authenticated caller into
	// TaskService, where grants and Task ownership are rechecked.
	return (&Instance{service: e.service, agentID: e.agentID}).Execute(ctx, req, credentials, progress)
}

// Extract derives the same AgentID/effect facts as the existing Task tools,
// without Provider configuration, process activation or mutable workspace
// information. The TaskService independently rechecks grants and ownership.
func (e *OwnedExecutor) Extract(_ context.Context, tool catalog.Tool,
	_ invocation.JSONValue, caller governance.Context,
) (invocationapp.GovernanceFactExtraction, error) {
	if e == nil || e.service == nil || caller.Validate() != nil {
		return invocationapp.GovernanceFactExtraction{}, agent.ErrForbidden
	}
	source, err := catalog.AgentSource(string(e.agentID))
	if err != nil || tool.SourceRef() != source {
		return invocationapp.GovernanceFactExtraction{}, agent.ErrForbidden
	}
	facts, err := reviewedAgentToolFacts(string(e.agentID), tool.SourceKey())
	if err != nil {
		return invocationapp.GovernanceFactExtraction{}, err
	}
	return invocationapp.GovernanceFactExtraction{Facts: facts}, nil
}

var _ invocationapp.ToolExecutor = (*OwnedExecutor)(nil)
var _ invocationapp.GovernanceFactExtractor = (*OwnedExecutor)(nil)
