package bootstrap

import (
	"context"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	suite "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

func (r *Runtime) sandboxJobCapabilitySnapshot(ctx context.Context, id sandbox.ID) (bool, string, error) {
	run, err := r.Sandboxes.GetRun(ctx, id)
	if err != nil {
		return false, "", err
	}
	contracts, err := r.Sandboxes.VerifiedSSHJobContracts(ctx, id)
	if err != nil {
		return false, "Only a verified SSH Remote backend provides Durable Jobs", nil
	}
	definition, ok := curated.Find(suite.SandboxJobs)
	if !ok {
		return false, "Reviewed Jobs suite not registered", suite.ErrUnavailable
	}
	if err = suite.RequireContracts(definition, suite.SandboxResource, contracts); err != nil {
		return false, "Required SSH Job guarantees unavailable", nil
	}
	if run.State != sandbox.StateReady {
		return false, "Submission requires a Ready SSH Sandbox; recovery may remain observable when Stopped", nil
	}
	return true, "", nil
}

// Native Job operations are common to all compatible SSH Sandbox targets. This
// is a capability projection; it does not create a Provider, Job or grant.
func (r *Runtime) SandboxJobTools(ctx context.Context, id sandbox.ID, caller gov.Context) (SandboxToolSuite, error) {
	if err := r.sandboxToolOwner(caller); err != nil {
		return SandboxToolSuite{}, err
	}
	available, issue, err := r.sandboxJobCapabilitySnapshot(ctx, id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	return r.listNativeSandboxTools(ctx, id, caller, suite.SandboxJobs, "jobs", available, issue)
}
