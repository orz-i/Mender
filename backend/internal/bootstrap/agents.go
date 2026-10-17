package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	policydomain "github.com/orz-i/Mender/backend/internal/policy/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// AgentAvailability does not launch a CLI, access an account or resolve credentials.
func (r *Runtime) AgentAvailability(backend agentdomain.Backend) (string, error) {
	if r == nil || r.agentLauncher == nil {
		return "", agentdomain.ErrUnavailable
	}
	return r.agentLauncher.Availability(backend)
}

// newAgents composes a host-reviewed set of local ACP Agent backends. External
// plugin packages cannot silently acquire the native process authority.
func newAgents(ctx context.Context, dbPath string, owner gov.Principal,
	profiles *Profiles, policies *Policies, governance *RuntimeGovernance,
	fixtureRunner agentapplication.Executor,
) (*agentapplication.Service, *agentsqlite.Store, *agentacp.LocalFactory, error) {
	store, err := agentsqlite.Open(ctx, dbPath)
	if err != nil {
		return nil, nil, nil, err
	}
	// Only expired prior-run tasks can be fenced into unknown. A live
	// 10-minute task is never reclassified merely because another Runtime
	// opened the same database; no task is automatically replayed.
	if _, err := store.ReconcileExpired(ctx, time.Now().Add(-(agentapplication.TaskTimeout + 2*time.Minute))); err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	launcher, err := agentacp.NewLocalFactory(governance.processIsolation)
	if err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	admit := func(ctx context.Context, caller gov.Context, a agentdomain.Agent, action gov.Action) error {
		if err := caller.Validate(); err != nil {
			return agentdomain.ErrForbidden
		}
		// The AgentID is a reviewed policy resource, never model-provided
		// identity, arbitrary exec path or prompt content.
		decision, err := policies.Evaluate.Execute(ctx, policyapplication.EvaluateRequest{
			Action: action, Origin: caller.Origin(), PrincipalKind: caller.Principal().Kind(),
			PrincipalReference: caller.Principal().Reference(), ScopeKind: caller.Scope().Kind(),
			ScopeReference: caller.Scope().Reference(), FactKnowledge: policydomain.FactKnown,
			Resources: []policyapplication.ResourceFact{
				{Kind: "agent_id", Value: string(a.ID())},
				{Kind: "agent_backend", Value: string(a.Spec().Backend)},
				{Kind: "agent_isolation", Value: a.Spec().Isolation},
			},
		})
		policyKind := "failed"
		policyReason := "policy_evaluation_error"
		status := "failed"
		if err == nil {
			policyKind = string(decision.Kind())
			policyReason = decision.ReasonCode()
			if decision.Kind() == policydomain.DecisionAllow {
				status = "admitted"
			} else {
				status = "denied"
			}
		}
		recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		recordErr := store.RecordAdmission(recordCtx, agentsqlite.AdmissionEvent{
			AgentID: a.ID(), Action: string(action),
			PrincipalKind: string(caller.Principal().Kind()), PrincipalRef: caller.Principal().Reference(),
			Origin: string(caller.Origin()), ScopeKind: string(caller.Scope().Kind()),
			ScopeRef: caller.Scope().Reference(), PolicyKind: policyKind, PolicyReason: policyReason, Status: status,
		})
		recordCancel()
		if err != nil {
			return errors.Join(fmt.Errorf("agent admission: %w", err), recordErr)
		}
		if recordErr != nil {
			return fmt.Errorf("agent admission audit unavailable: %w", recordErr)
		}
		if decision.Kind() != policydomain.DecisionAllow {
			return agentdomain.ErrForbidden
		}
		return nil
	}
	profileExists := func(ctx context.Context, id string) error {
		parsed, err := profiledomain.ParseProfileID(id)
		if err != nil {
			return err
		}
		_, err = profiles.GetByID.Execute(ctx, parsed)
		return err
	}
	runner := fixtureRunner
	if runner == nil {
		runner = agentacp.Runner{NewCommand: launcher.Command, NewCommandInWorkspace: launcher.CommandInWorkspace, NewCommandWithConfig: launcher.CommandWithConfig}
	}
	service, err := agentapplication.NewService(store, runner, admit, owner, profileExists)
	if err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	return service, store, launcher, nil
}
