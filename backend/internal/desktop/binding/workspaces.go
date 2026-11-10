package binding

import (
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	wsapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

// WorkspaceRecord is the desktop projection of one reusable logical work
// context; it is not a Sandbox Tool source or an execution grant.
type WorkspaceRecord struct {
	WorkspaceID  string `json:"workspaceId"`
	DisplayName  string `json:"displayName"`
	VerifiedRoot string `json:"verifiedRoot"`
	CreatedAt    string `json:"createdAt"`
}
type WorkspaceList struct {
	Workspaces []WorkspaceRecord `json:"workspaces"`
}
type WorkspacePlacement struct {
	WorkspaceID string `json:"workspaceId"`
	SandboxID   string `json:"sandboxId"`
	BackendRoot string `json:"backendRoot"`
}
type WorkspacePlacementList struct {
	Placements []WorkspacePlacement `json:"placements"`
}
type WorkspaceProfileGrants struct {
	ProfileIDs []string `json:"profileIds"`
}
type ProfileToolTarget struct {
	ProfileID   string `json:"profileId"`
	ToolID      string `json:"toolId"`
	WorkspaceID string `json:"workspaceId"`
	SandboxID   string `json:"sandboxId"`
}
type ProfileToolTargets struct {
	Targets []ProfileToolTarget `json:"targets"`
}

func projectWorkspace(value wsapp.Record) WorkspaceRecord {
	return WorkspaceRecord{WorkspaceID: value.ID.String(), DisplayName: value.DisplayName, VerifiedRoot: value.VerifiedRoot, CreatedAt: value.CreatedAt}
}
func projectPlacement(value wsapp.PlacementRecord) WorkspacePlacement {
	return WorkspacePlacement{WorkspaceID: value.WorkspaceID.String(), SandboxID: value.SandboxID, BackendRoot: value.BackendRoot}
}
func projectTarget(value wsapp.TargetRecord) ProfileToolTarget {
	return ProfileToolTarget{ProfileID: value.ProfileID, ToolID: value.ToolID.String(), WorkspaceID: value.WorkspaceID, SandboxID: value.SandboxID}
}
func (s *Service) ListWorkspaces() (WorkspaceList, error) {
	caller, e := s.governanceContext()
	if e != nil {
		return WorkspaceList{}, e
	}
	items, e := s.runtime.Workspaces.List(s.ctx, caller)
	if e != nil {
		return WorkspaceList{}, e
	}
	out := WorkspaceList{Workspaces: make([]WorkspaceRecord, 0, len(items))}
	for _, v := range items {
		out.Workspaces = append(out.Workspaces, projectWorkspace(v))
	}
	return out, nil
}
func (s *Service) CreateWorkspace(displayName, verifiedRoot string) (WorkspaceRecord, error) {
	caller, e := s.governanceContext()
	if e != nil {
		return WorkspaceRecord{}, e
	}
	item, e := s.runtime.Workspaces.RegisterLocal(s.ctx, caller, displayName, verifiedRoot)
	if e != nil {
		return WorkspaceRecord{}, e
	}
	return projectWorkspace(item), nil
}
func (s *Service) GetWorkspace(id string) (WorkspaceRecord, error) {
	ref, e := workspace.ParseRef(id)
	if e != nil {
		return WorkspaceRecord{}, e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return WorkspaceRecord{}, e
	}
	v, e := s.runtime.Workspaces.Get(s.ctx, caller, ref)
	if e != nil {
		return WorkspaceRecord{}, e
	}
	return projectWorkspace(v), nil
}
func (s *Service) DeleteWorkspace(id string) error {
	ref, e := workspace.ParseRef(id)
	if e != nil {
		return e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return e
	}
	return s.runtime.Workspaces.Delete(s.ctx, caller, ref)
}
func (s *Service) ListWorkspacePlacements(id string) (WorkspacePlacementList, error) {
	ref, e := workspace.ParseRef(id)
	if e != nil {
		return WorkspacePlacementList{}, e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return WorkspacePlacementList{}, e
	}
	items, e := s.runtime.Workspaces.Placements(s.ctx, caller, ref)
	if e != nil {
		return WorkspacePlacementList{}, e
	}
	out := WorkspacePlacementList{Placements: make([]WorkspacePlacement, 0, len(items))}
	for _, v := range items {
		out.Placements = append(out.Placements, projectPlacement(v))
	}
	return out, nil
}
func (s *Service) LinkWorkspaceSandbox(workspaceID, sandboxID string) error {
	ref, e := workspace.ParseRef(workspaceID)
	if e != nil {
		return e
	}
	id, e := sandbox.ParseID(sandboxID)
	if e != nil {
		return e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return e
	}
	return s.runtime.Workspaces.LinkLocal(s.ctx, caller, ref, id.String())
}
func (s *Service) UnlinkWorkspaceSandbox(workspaceID, sandboxID string) error {
	ref, e := workspace.ParseRef(workspaceID)
	if e != nil {
		return e
	}
	id, e := sandbox.ParseID(sandboxID)
	if e != nil {
		return e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return e
	}
	return s.runtime.Workspaces.Unlink(s.ctx, caller, ref, id.String())
}
func (s *Service) ListWorkspaceProfileGrants(workspaceID string) (WorkspaceProfileGrants, error) {
	ref, e := workspace.ParseRef(workspaceID)
	if e != nil {
		return WorkspaceProfileGrants{}, e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return WorkspaceProfileGrants{}, e
	}
	ids, e := s.runtime.Workspaces.Grants(s.ctx, caller, ref)
	if e != nil {
		return WorkspaceProfileGrants{}, e
	}
	return WorkspaceProfileGrants{ProfileIDs: ids}, nil
}
func (s *Service) GrantWorkspaceProfile(workspaceID, profileID string) error {
	ref, e := workspace.ParseRef(workspaceID)
	if e != nil {
		return e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return e
	}
	return s.runtime.Workspaces.Grant(s.ctx, caller, ref, profileID)
}
func (s *Service) RevokeWorkspaceProfile(workspaceID, profileID string) error {
	ref, e := workspace.ParseRef(workspaceID)
	if e != nil {
		return e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return e
	}
	return s.runtime.Workspaces.Revoke(s.ctx, caller, ref, profileID)
}
func (s *Service) profileID(slug string) (string, error) {
	value, e := profile.NewSlug(slug)
	if e != nil {
		return "", e
	}
	p, e := s.runtime.Profiles.Get.Execute(s.ctx, value)
	if e != nil {
		return "", e
	}
	return p.ID().String(), nil
}
func (s *Service) ListProfileToolTargets(slug string) (ProfileToolTargets, error) {
	id, e := s.profileID(slug)
	if e != nil {
		return ProfileToolTargets{}, e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return ProfileToolTargets{}, e
	}
	items, e := s.runtime.Workspaces.Targets(s.ctx, caller, id)
	if e != nil {
		return ProfileToolTargets{}, e
	}
	out := ProfileToolTargets{Targets: make([]ProfileToolTarget, 0, len(items))}
	for _, v := range items {
		out.Targets = append(out.Targets, projectTarget(v))
	}
	return out, nil
}
func (s *Service) PutProfileToolTarget(slug, toolID, workspaceID, sandboxID string) (ProfileToolTarget, error) {
	pid, e := s.profileID(slug)
	if e != nil {
		return ProfileToolTarget{}, e
	}
	tid, e := catalog.ParseToolID(toolID)
	if e != nil {
		return ProfileToolTarget{}, e
	}
	sid, e := sandbox.ParseID(sandboxID)
	if e != nil {
		return ProfileToolTarget{}, e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return ProfileToolTarget{}, e
	}
	t := wsapp.TargetRecord{ProfileID: pid, ToolID: tid, WorkspaceID: workspaceID, SandboxID: sid.String()}
	if e = s.runtime.Workspaces.PutTarget(s.ctx, caller, t); e != nil {
		return ProfileToolTarget{}, e
	}
	return projectTarget(t), nil
}
func (s *Service) DeleteProfileToolTarget(slug, toolID, workspaceID, sandboxID string) error {
	pid, e := s.profileID(slug)
	if e != nil {
		return e
	}
	tid, e := catalog.ParseToolID(toolID)
	if e != nil {
		return e
	}
	sid, e := sandbox.ParseID(sandboxID)
	if e != nil {
		return e
	}
	caller, e := s.governanceContext()
	if e != nil {
		return e
	}
	return s.runtime.Workspaces.DeleteTarget(s.ctx, caller, wsapp.TargetRecord{ProfileID: pid, ToolID: tid, WorkspaceID: workspaceID, SandboxID: sid.String()})
}
