package binding

import (
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Irreversible owner-only SSH Job submission protocol upgrade. This cannot
// be invoked by a Profile, an MCP Tool, or a browser-supplied SSH command.
func (s *Service) ActivateSSHJobV2(raw string) (SSHJobAdmissionState, error) {
	id, err := sandboxdomain.ParseID(raw)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	state, err := s.runtime.Sandboxes.ActivateSSHJobV2(s.ctx, id, caller)
	if err != nil {
		return SSHJobAdmissionState{}, err
	}
	return projectSSHJobAdmission(state), nil
}
