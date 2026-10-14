package bootstrap

import (
	"context"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// SSHJobAdmissionState is Core-owned authority, independent of plugin-owned
// Task lifecycle. Pausing does not stop, cancel, replay or resolve any job.
type SSHJobAdmissionState struct {
	SandboxID  sandboxdomain.ID `json:"sandboxId"`
	Generation uint64           `json:"generation"`
	Paused     bool             `json:"paused"`
	Protocol   string           `json:"protocol"` // legacy_v1, durable_v2
}

func (s *Sandboxes) sshJobOwner(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (sandboxdomain.Sandbox, sandboxdomain.Run, error) {
	if s == nil || s.store == nil || s.sshRemote == nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, err
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, err
	}
	if item.Backend() != sandboxdomain.BackendSSHRemote || item.Spec().Binding != sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote) ||
		!sandboxapp.IsOwner(caller, item) || caller.Scope().Kind() != gov.ScopeCatalog ||
		caller.Principal().Kind() != gov.PrincipalLocalUser || caller.Origin() == gov.OriginMCP {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, sandboxdomain.ErrNotAuthorized
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, err
	}
	if run.State == sandboxdomain.StateDestroying || run.State == sandboxdomain.StateDestroyed {
		return sandboxdomain.Sandbox{}, sandboxdomain.Run{}, sandboxdomain.ErrConflict
	}
	return item, run, nil
}

func (s *Sandboxes) SSHJobAdmission(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SSHJobAdmissionState, error) {
	_, run, err := s.sshJobOwner(ctx, id, caller)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	paused, err := s.store.SSHJobSubmissionsPaused(ctx, id)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	protocol, err := s.store.SSHJobProtocol(ctx, id)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	version := "legacy_v1"
	if protocol == sandboxsqlite.SSHJobProtocolV2 {
		version = "durable_v2"
	}
	return SSHJobAdmissionState{SandboxID: id, Generation: run.Generation, Paused: paused, Protocol: version}, nil
}

// SetSSHJobSubmissionsPaused gates new Job submissions in SQLite across every
// Runtime using the same DB. The managed backend is never contacted. The
// authoritative CAS is performed by the Sandbox store after policy admission.
func (s *Sandboxes) SetSSHJobSubmissionsPaused(ctx context.Context, id sandboxdomain.ID, paused bool, caller gov.Context) (SSHJobAdmissionState, error) {
	item, run, err := s.sshJobOwner(ctx, id, caller)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	if err = s.admission.AdmitAction(ctx, caller, item, gov.ActionSandboxReconcile); err != nil {
		return SSHJobAdmissionState{}, err
	}
	if err = s.store.SetSSHJobSubmissionsPaused(ctx, id, run.Generation, paused, caller); err != nil {
		return SSHJobAdmissionState{}, err
	}
	return s.SSHJobAdmission(ctx, id, caller)
}

// ActivateSSHJobV2 is a one-way, audited owner-administrative resource
// transition. Core approves the exact Sandbox and pinned Run BEFORE SSH.
// The SSH Backend verifies real transport/fsync/flock capabilities; ONLY the
// sandbox admission protocol selection is stored in Core.
func (s *Sandboxes) ActivateSSHJobV2(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SSHJobAdmissionState, error) {
	item, run, err := s.sshJobOwner(ctx, id, caller)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	if run.State != sandboxdomain.StateReady {
		return SSHJobAdmissionState{}, sandboxdomain.ErrConflict
	}
	if err = s.admission.AdmitAction(ctx, caller, item, gov.ActionSandboxReconcile); err != nil {
		return SSHJobAdmissionState{}, err
	}
	paused, err := s.store.SSHJobSubmissionsPaused(ctx, id)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	if !paused {
		return SSHJobAdmissionState{}, sandboxdomain.ErrConflict
	}

	err = s.sshRemote.WithJobs(ctx, id, func(j *sshremote.Jobs) error {
		if e := j.Prepare(ctx); e != nil {
			return e
		}
		return j.CheckV2(ctx)
	})
	if err != nil {
		return SSHJobAdmissionState{}, err
	}

	// A competing Stop, Submit, resume or an older v1 writer is fenced here.
	if err = s.store.EnableSSHJobV2(ctx, run, caller); err != nil {
		return SSHJobAdmissionState{}, err
	}
	return s.SSHJobAdmission(ctx, id, caller)
}
