package bootstrap

import (
	"context"
	"time"

	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
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
	item, run, err := s.sshJobOwner(ctx, id, caller)
	if err != nil {
		return SSHJobRecoveryView{}, err
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
	paused, err := s.store.SSHJobSubmissionsPaused(ctx, id)
	if err != nil {
		return SSHJobRecoveryView{}, err
	}
	return SSHJobRecoveryView{SandboxID: id, Generation: run.Generation, Managed: managed, Paused: paused, Evidence: report}, nil
}
