package binding

import (
	"github.com/orz-i/Mender/backend/internal/bootstrap"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type SSHJobAdmissionState struct {
	SandboxID  string `json:"sandboxId"`
	Generation uint64 `json:"generation"`
	Paused     bool   `json:"paused"`
	Protocol   string `json:"protocol"`
}

func projectSSHJobAdmission(s bootstrap.SSHJobAdmissionState) SSHJobAdmissionState {
	return SSHJobAdmissionState{SandboxID: s.SandboxID.String(), Generation: s.Generation, Paused: s.Paused, Protocol: s.Protocol}
}
func (s *Service) GetSSHJobAdmission(raw string) (SSHJobAdmissionState, error) {
	id, err := sandboxdomain.ParseID(raw)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	state, err := s.runtime.Sandboxes.SSHJobAdmission(s.ctx, id, caller)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	return projectSSHJobAdmission(state), nil
}
func (s *Service) SetSSHJobAdmission(raw string, paused bool) (SSHJobAdmissionState, error) {
	id, err := sandboxdomain.ParseID(raw)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	state, err := s.runtime.Sandboxes.SetSSHJobSubmissionsPaused(s.ctx, id, paused, caller)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	return projectSSHJobAdmission(state), nil
}
