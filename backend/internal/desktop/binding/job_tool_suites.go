package binding

import "github.com/orz-i/Mender/backend/internal/sandbox/domain"

func (s *Service) GetSandboxJobTools(id string) (SandboxToolSuite, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SandboxToolSuite{}, err
	}
	parsed, err := domain.ParseID(id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	value, err := s.runtime.SandboxJobTools(s.ctx, parsed, caller)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	return projectSandboxToolSuite(value), nil
}
func (s *Service) EnsureSandboxJobTools(id string) (SandboxToolSuite, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return SandboxToolSuite{}, err
	}
	parsed, err := domain.ParseID(id)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	value, err := s.runtime.EnsureSandboxJobTools(s.ctx, parsed, caller)
	if err != nil {
		return SandboxToolSuite{}, err
	}
	return projectSandboxToolSuite(value), nil
}
func (s *Service) RemoveSandboxJobTools(id string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	parsed, err := domain.ParseID(id)
	if err != nil {
		return err
	}
	return s.runtime.RemoveSandboxJobTools(s.ctx, parsed, caller)
}
