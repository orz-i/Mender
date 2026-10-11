package bootstrap

import (
	"context"
	"fmt"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	sandboxsqlite "github.com/orz-i/Mender/backend/internal/sandbox/infrastructure/sqlite"
)

// ListProfileGrants returns the owner's explicit Profile grants, not a service
// Principal's implied permissions.
func (s *Sandboxes) ListProfileGrants(ctx context.Context, id sandboxdomain.ID, caller governancedomain.Context) ([]sandboxapplication.ProfileGrant, error) {
	if s == nil || s.store == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return s.store.ListProfileGrants(ctx, id, caller)
}

func (s *Sandboxes) ListProfileGrantEvents(ctx context.Context, id sandboxdomain.ID, caller governancedomain.Context, limit int) ([]sandboxsqlite.GrantEvent, error) {
	if s == nil || s.store == nil {
		return nil, sandboxdomain.ErrBackendUnavailable
	}
	return s.store.ListGrantEvents(ctx, id, caller, limit)
}

// SetProfileGrants replaces this single Profile's bounded capability set.
// Zero capabilities deliberately revoke; the request cannot grant lifecycle.
func (s *Sandboxes) SetProfileGrants(ctx context.Context, id sandboxdomain.ID, profileID string, capabilities []sandboxdomain.Capability, caller governancedomain.Context) error {
	if s == nil || s.store == nil || s.profileExists == nil {
		return sandboxdomain.ErrBackendUnavailable
	}
	parsed, err := profiledomain.ParseProfileID(profileID)
	if err != nil {
		return err
	}
	if err = caller.Validate(); err != nil {
		return err
	}
	if err = s.profileExists(ctx, parsed); err != nil {
		return fmt.Errorf("grant Sandbox access: Profile not found: %w", err)
	}
	return s.store.ReplaceProfileGrants(ctx, id, profileID, capabilities, caller)
}
