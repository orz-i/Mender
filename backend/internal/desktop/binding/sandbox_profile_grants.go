package binding

import (
	"github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

func (s *Service) GetSandboxProfileGrants(rawID string) (SandboxProfileGrants, error) {
	id, err := domain.ParseID(rawID)
	if err != nil {
		return SandboxProfileGrants{}, err
	}
	caller, err := s.governanceContext()
	if err != nil {
		return SandboxProfileGrants{}, err
	}
	grants, err := s.runtime.Sandboxes.ListProfileGrants(s.ctx, id, caller)
	if err != nil {
		return SandboxProfileGrants{}, err
	}
	result := SandboxProfileGrants{SandboxID: rawID, Grants: make([]SandboxProfileGrant, 0, len(grants))}
	for _, grant := range grants {
		caps := make([]string, 0, len(grant.Capabilities))
		for _, capability := range grant.Capabilities {
			caps = append(caps, string(capability))
		}
		result.Grants = append(result.Grants, SandboxProfileGrant{ProfileID: grant.ProfileID, Capabilities: caps})
	}
	return result, nil
}

func (s *Service) PutSandboxProfileGrants(sandboxID, profileID string, capabilities []string) (SandboxProfileGrants, error) {
	id, err := domain.ParseID(sandboxID)
	if err != nil {
		return SandboxProfileGrants{}, err
	}
	caller, err := s.governanceContext()
	if err != nil {
		return SandboxProfileGrants{}, err
	}
	caps := make([]domain.Capability, 0, len(capabilities))
	for _, value := range capabilities {
		cap := domain.Capability(value)
		if err = cap.Validate(); err != nil {
			return SandboxProfileGrants{}, err
		}
		caps = append(caps, cap)
	}
	if err = s.runtime.Sandboxes.SetProfileGrants(s.ctx, id, profileID, caps, caller); err != nil {
		return SandboxProfileGrants{}, err
	}
	return s.GetSandboxProfileGrants(sandboxID)
}

func (s *Service) RevokeSandboxProfileGrants(sandboxID, profileID string) (SandboxProfileGrants, error) {
	return s.PutSandboxProfileGrants(sandboxID, profileID, nil)
}
func (s *Service) GetSandboxProfileGrantEvents(sandboxID string, limit int) (SandboxProfileGrantEvents, error) {
	id, err := domain.ParseID(sandboxID)
	if err != nil {
		return SandboxProfileGrantEvents{}, err
	}
	caller, err := s.governanceContext()
	if err != nil {
		return SandboxProfileGrantEvents{}, err
	}
	events, err := s.runtime.Sandboxes.ListProfileGrantEvents(s.ctx, id, caller, limit)
	if err != nil {
		return SandboxProfileGrantEvents{}, err
	}
	result := SandboxProfileGrantEvents{SandboxID: sandboxID, Events: make([]SandboxProfileGrantEvent, 0, len(events))}
	for _, event := range events {
		result.Events = append(result.Events, SandboxProfileGrantEvent{ProfileID: event.ProfileID, Capability: string(event.Capability), Action: event.Action, OccurredAt: event.OccurredAt})
	}
	return result, nil
}
