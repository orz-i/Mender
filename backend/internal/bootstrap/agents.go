package bootstrap

import (
	"context"
	"time"

	agentapplication "github.com/orz-i/Mender/backend/internal/agent/application"
	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentacp "github.com/orz-i/Mender/backend/internal/agent/infrastructure/acp"
	agentagy "github.com/orz-i/Mender/backend/internal/agent/infrastructure/antigravity"
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
	service, err := agentapplication.NewService(store, runner, admit.Execute, owner, profileExists)
	if err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	return service, store, launcher, nil
}
