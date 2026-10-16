package binding

type WorkspaceToolTarget struct {
	Key       string `json:"key"`
	SandboxID string `json:"sandboxId"`
}
type WorkspaceToolTargets struct {
	Workspaces []WorkspaceToolTarget `json:"workspaces"`
}

func (s *Service) ListWorkspaceToolTargets(providerID string) (WorkspaceToolTargets, error) {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return WorkspaceToolTargets{}, err
	}
	items, err := s.runtime.ListWorkspaceToolTargets(s.ctx, providerID, caller)
	if err != nil {
		return WorkspaceToolTargets{}, err
	}
	result := WorkspaceToolTargets{Workspaces: make([]WorkspaceToolTarget, 0, len(items))}
	for _, item := range items {
		result.Workspaces = append(result.Workspaces, WorkspaceToolTarget{Key: item.Key, SandboxID: item.SandboxID})
	}
	return result, nil
}
func (s *Service) BindWorkspaceToolTarget(providerID, key, sandboxID string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	return s.runtime.BindWorkspaceToolTarget(s.ctx, providerID, key, sandboxID, caller)
}
func (s *Service) UnbindWorkspaceToolTarget(providerID, key string) error {
	caller, err := s.agentOwnerContext()
	if err != nil {
		return err
	}
	return s.runtime.UnbindWorkspaceToolTarget(s.ctx, providerID, key, caller)
}
