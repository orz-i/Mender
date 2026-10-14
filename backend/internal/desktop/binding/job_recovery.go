package binding

import (
	"time"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type SSHJobRecoveryRecord struct {
	TaskID      string `json:"taskId"`
	Phase       string `json:"phase"`
	State       string `json:"state"`
	Activity    string `json:"activity"`
	Fingerprint string `json:"fingerprint"`
	Recoverable bool   `json:"recoverable"`
}
type SSHJobRecoveryEvidence struct {
	Records     []SSHJobRecoveryRecord `json:"records"`
	InspectedAt string                 `json:"inspectedAt"`
	Verdict     string                 `json:"verdict"`
}
type SSHJobRecoveryView struct {
	SandboxID  string                 `json:"sandboxId"`
	Generation uint64                 `json:"generation"`
	Managed    bool                   `json:"managed"`
	Paused     bool                   `json:"paused"`
	Evidence   SSHJobRecoveryEvidence `json:"evidence"`
}

func projectSSHJobRecovery(v bootstrap.SSHJobRecoveryView) SSHJobRecoveryView {
	records := make([]SSHJobRecoveryRecord, 0, len(v.Evidence.Records))
	for _, e := range v.Evidence.Records {
		records = append(records, SSHJobRecoveryRecord{
			TaskID: e.TaskID, Phase: e.Phase, State: string(e.State), Activity: e.Activity,
			Fingerprint: e.Fingerprint, Recoverable: e.Recoverable,
		})
	}
	return SSHJobRecoveryView{
		SandboxID: v.SandboxID.String(), Generation: v.Generation, Managed: v.Managed, Paused: v.Paused,
		Evidence: SSHJobRecoveryEvidence{Records: records, InspectedAt: v.Evidence.InspectedAt.Format(time.RFC3339Nano), Verdict: v.Evidence.Verdict},
	}
}
func (s *Service) GetSSHJobRecovery(raw string) (SSHJobRecoveryView, error) {
	id, err := sandboxdomain.ParseID(raw)
	if err != nil {
		return SSHJobRecoveryView{}, err
	}
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SSHJobRecoveryView{}, err
	}
	view, err := s.runtime.Sandboxes.InspectSSHJobRecovery(s.ctx, id, caller)
	if err != nil {
		return SSHJobRecoveryView{}, err
	}
	return projectSSHJobRecovery(view), nil
}
