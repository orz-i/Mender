package binding

import (
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type SSHJobResolutionReceipt struct {
	SandboxID  string               `json:"sandboxId"`
	Generation uint64               `json:"generation"`
	Evidence   SSHJobRecoveryRecord `json:"evidence"`
}

func (s *Service) ResolveSSHJobRecovery(sandboxID, taskID, fingerprint string, generation uint64, reason string) (SSHJobResolutionReceipt, error) {
	id, err := sandboxdomain.ParseID(sandboxID)
	if err != nil {
		return SSHJobResolutionReceipt{}, err
	}
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SSHJobResolutionReceipt{}, err
	}
	receipt, err := s.runtime.Sandboxes.ResolveSSHJobRecovery(s.ctx, id, bootstrap.SSHJobResolutionRequest{
		TaskID: taskID, Fingerprint: fingerprint, Generation: generation, Reason: reason,
	}, caller)
	if err != nil {
		return SSHJobResolutionReceipt{}, err
	}
	return SSHJobResolutionReceipt{
		SandboxID: receipt.SandboxID.String(), Generation: receipt.Generation,
		Evidence: SSHJobRecoveryRecord{
			TaskID: receipt.Evidence.TaskID, Phase: receipt.Evidence.Phase,
			State: string(receipt.Evidence.State), Activity: receipt.Evidence.Activity,
			Fingerprint: receipt.Evidence.Fingerprint, Recoverable: receipt.Evidence.Recoverable,
		},
	}, nil
}
