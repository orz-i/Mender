package bootstrap

import (
	"context"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sshremote "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sshremote"
)

// SSHJobRecoveryView joins Core-owned resource metadata with unmodified,
// bounded SSH-plugin evidence. Neither Core nor adapters interpret a v1
// fingerprint as permission to release a lifecycle obligation.
type SSHJobRecoveryView struct {
	SandboxID  sandboxdomain.ID         `json:"sandboxId"`
	Generation uint64                   `json:"generation"`
	Managed    bool                     `json:"managed"`
	Paused     bool                     `json:"paused"`
	Evidence   sshremote.RecoveryReport `json:"evidence"`
}

// InspectSSHJobRecovery is an owner-only administrative read, not a Model
// Tool or Profile grant. Policy admission is recorded before credentials,
// network or plugin observations. It does not acquire an exclusive lease.
func (s *Sandboxes) InspectSSHJobRecovery(ctx context.Context, id sandboxdomain.ID, caller gov.Context) (SSHJobRecoveryView, error) {
	if s == nil || s.store == nil || s.sshRemote == nil {
		return SSHJobRecoveryView{}, sandboxdomain.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return SSHJobRecoveryView{}, err
	}
	item, err := s.store.Get(ctx, id)
	if err != nil {
		return SSHJobRecoveryView{}, err
	}
	if item.Backend() != sandboxdomain.BackendSSHRemote ||
		item.Spec().Binding != sandboxdomain.BuiltinBinding(sandboxdomain.BackendSSHRemote) ||
		!sandboxapp.IsOwner(caller, item) || caller.Scope().Kind() != gov.ScopeCatalog ||
		caller.Principal().Kind() != gov.PrincipalLocalUser || caller.Origin() == gov.OriginMCP {
		return SSHJobRecoveryView{}, sandboxdomain.ErrNotAuthorized
	}
	run, err := s.store.GetRun(ctx, id)
	if err != nil {
		return SSHJobRecoveryView{}, err
	}
	if run.State == sandboxdomain.StateDestroyed || run.State == sandboxdomain.StateDestroying {
		return SSHJobRecoveryView{}, sandboxdomain.ErrConflict
	}
	if err = s.admission.AdmitAction(ctx, caller, item, gov.ActionSandboxReconcile); err != nil {
		return SSHJobRecoveryView{}, err
	}
	managed, err := s.store.SSHJobManaged(ctx, id)
	if err != nil {
		return SSHJobRecoveryView{}, err
	}
	report := sshremote.RecoveryReport{Records: []sshremote.RecoveryEvidence{}, InspectedAt: time.Now().UTC(), Verdict: "not_enrolled"}
	if managed {
		report, err = s.sshRemote.InspectJobRecovery(ctx, id)
		if err != nil {
			return SSHJobRecoveryView{}, err
		}
	}
	return SSHJobRecoveryView{SandboxID: id, Generation: run.Generation, Managed: managed, Evidence: report}, nil
}
