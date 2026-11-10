package bootstrap

import (
	"context"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	agentagy "github.com/orz-i/Mender/backend/internal/agent/infrastructure/antigravity"
	agentworkspace "github.com/orz-i/Mender/backend/internal/agent/infrastructure/localworkspace"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

// AgentAvailability does not launch a CLI, access an account or resolve credentials.
func (r *Runtime) AgentAvailability(backend agentdomain.Backend) (string, error) {
	if r == nil || r.agentLauncher == nil {
		return "", agentdomain.ErrUnavailable
	}
	return r.agentLauncher.Availability(backend)
}

// newTypedAgents reuses the existing ACP Task/Session and approval service on
// an already attested v3 schema. Opening a store does NOT reinterpret unknown
// tasks, invalidate pending approvals or resume/replay an old process. Recovery
// decisions remain explicit, after the exclusive runtime cutover boundary.
func newTypedAgents(ctx context.Context, dbPath string, owner gov.Principal,
	profiles *Profiles, policies *Policies, governance *RuntimeGovernance,
	fixtureRunner agentapplication.Executor,
) (*agentapplication.Service, *agentsqlite.Store, *agentacp.LocalFactory, error) {
	store, err := agentsqlite.OpenTyped(ctx, dbPath)
	if err != nil {
		return nil, nil, nil, err
	}
	// Fence only overdue prior-run tasks. Preserve unknown outcomes and never replay.
	if _, err := store.ReconcileExpired(ctx, time.Now().UTC().Add(-12*time.Minute)); err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	return composeAgentService(ctx, store, owner, profiles, policies, governance, fixtureRunner)
}

// composeAgentService is the single Agent/ACP Task Service composer for v3.
// Store opening is inert; only the official Runtime may reconcile overdue
// Tasks and pending approvals after explicit ownership and generation checks.
func composeAgentService(ctx context.Context, store *agentsqlite.Store, owner gov.Principal,
	profiles *Profiles, policies *Policies, governance *RuntimeGovernance,
	fixtureRunner agentapplication.Executor,
) (*agentapplication.Service, *agentsqlite.Store, *agentacp.LocalFactory, error) {
	launcher, err := agentacp.NewLocalFactory(governance.processIsolation)
	if err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	admit := agentapplication.NewAdmit(policies.Evaluate, store)
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
		runner = agentExecutionRouter{
			acp:         agentacp.Runner{NewCommand: launcher.Command, NewCommandInWorkspace: launcher.CommandInWorkspace, NewCommandWithConfig: launcher.CommandWithConfig},
			antigravity: agentagy.Runner{NewCommand: launcher.AntigravityCommand, NewResumeCommand: launcher.AntigravityResumeCommand},
		}
	}
	service, err := agentapplication.NewService(store, runner, admit.Execute, owner, profileExists, agentworkspace.New())
	if err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	return service, store, launcher, nil
}
