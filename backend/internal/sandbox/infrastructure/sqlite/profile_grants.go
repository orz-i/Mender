package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	governancedomain "github.com/orz-i/Mender/backend/internal/governance/domain"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	sandboxapplication "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// HasProfileGrant is read-only pre-admission evidence, never sufficient alone
// for acquiring authority; ReserveExecution fences revocation atomically.
func (s *Store) HasProfileGrant(ctx context.Context, id sandboxdomain.ID, profileID string, capability sandboxdomain.Capability) (bool, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return false, err
	}
	if _, err := profiledomain.ParseProfileID(profileID); err != nil {
		return false, err
	}
	if err := capability.Validate(); err != nil {
		return false, err
	}
	var present int
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM sandbox_profile_grants WHERE sandbox_id=? AND profile_id=? AND capability=?)", id.String(), profileID, string(capability)).Scan(&present)
	if err != nil {
		return false, fmt.Errorf("lookup Sandbox Profile grant: %w", err)
	}
	return present == 1, nil
}

// ListProfileGrants only projects current grants to the verified local owner.
func (s *Store) ListProfileGrants(ctx context.Context, id sandboxdomain.ID, caller governancedomain.Context) ([]sandboxapplication.ProfileGrant, error) {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return nil, err
	}
	if err := caller.Validate(); err != nil {
		return nil, err
	}
	item, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !sandboxapplication.IsOwner(caller, item) {
		return nil, sandboxdomain.ErrNotAuthorized
	}
	rows, err := s.db.QueryContext(ctx, "SELECT profile_id,capability FROM sandbox_profile_grants WHERE sandbox_id=? ORDER BY profile_id,capability", id.String())
	if err != nil {
		return nil, fmt.Errorf("list Sandbox Profile grants: %w", err)
	}
	defer func() { _ = rows.Close() }()
	output := make([]sandboxapplication.ProfileGrant, 0)
	for rows.Next() {
		var profileID string
		var value sandboxdomain.Capability
		if err := rows.Scan(&profileID, &value); err != nil {
			return nil, fmt.Errorf("scan Sandbox Profile grant: %w", err)
		}
		if _, err := profiledomain.ParseProfileID(profileID); err != nil {
			return nil, err
		}
		if err := value.Validate(); err != nil {
			return nil, err
		}
		if len(output) == 0 || output[len(output)-1].ProfileID != profileID {
			output = append(output, sandboxapplication.ProfileGrant{ProfileID: profileID, Capabilities: []sandboxdomain.Capability{}})
		}
		output[len(output)-1].Capabilities = append(output[len(output)-1].Capabilities, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return output, nil
}

// ReplaceProfileGrants is a transactionally audited owner-only grant set
// replacement. An empty capability set revokes that Profile's complete access.
func (s *Store) ReplaceProfileGrants(ctx context.Context, id sandboxdomain.ID, profileID string, requested []sandboxdomain.Capability, caller governancedomain.Context) error {
	if _, err := sandboxdomain.ParseID(id.String()); err != nil {
		return err
	}
	if _, err := profiledomain.ParseProfileID(profileID); err != nil {
		return err
	}
	if err := caller.Validate(); err != nil {
		return err
	}
	if caller.Principal().Kind() != governancedomain.PrincipalLocalUser ||
		(caller.Origin() != governancedomain.OriginDesktop &&
			caller.Origin() != governancedomain.OriginManagementHTTP &&
			caller.Origin() != governancedomain.OriginCLI) ||
		caller.Scope().Kind() != governancedomain.ScopeCatalog {
		return sandboxdomain.ErrNotAuthorized
	}
	next := make(map[sandboxdomain.Capability]bool, len(requested))
	for _, capability := range requested {
		if err := capability.Validate(); err != nil {
			return err
		}
		if next[capability] {
			return fmt.Errorf("sandbox Profile grant: duplicate capability %q", capability)
		}
		next[capability] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace Sandbox Profile grants: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Take SQLite's writer reservation before reading the grant set. Concurrent
	// revocations otherwise race on a DEFERRED read-to-write lock upgrade.
	if _, err = tx.ExecContext(ctx, "UPDATE sandbox_profile_grants SET granted_at=granted_at WHERE sandbox_id=? AND profile_id=?", id.String(), profileID); err != nil {
		return fmt.Errorf("replace Sandbox Profile grants: acquire write reservation: %w", err)
	}
	var ownerKind, ownerRef, state string
	err = tx.QueryRowContext(ctx, "SELECT s.owner_kind,s.owner_ref,r.state FROM sandboxes s JOIN sandbox_runs r ON s.id=r.sandbox_id WHERE s.id=?", id.String()).Scan(&ownerKind, &ownerRef, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return sandboxdomain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("replace Sandbox Profile grants: inventory: %w", err)
	}
	if ownerKind != string(caller.Principal().Kind()) || ownerRef != caller.Principal().Reference() {
		return sandboxdomain.ErrNotAuthorized
	}
	if state == string(sandboxdomain.StateDestroyed) || state == string(sandboxdomain.StateDestroying) {
		return sandboxdomain.ErrConflict
	}
	rows, err := tx.QueryContext(ctx, "SELECT capability FROM sandbox_profile_grants WHERE sandbox_id=? AND profile_id=?", id.String(), profileID)
	if err != nil {
		return fmt.Errorf("replace Sandbox Profile grants: read: %w", err)
	}
	current := make(map[sandboxdomain.Capability]bool)
	for rows.Next() {
		var c sandboxdomain.Capability
		if err = rows.Scan(&c); err != nil {
			_ = rows.Close()
			return err
		}
		if err = c.Validate(); err != nil {
			_ = rows.Close()
			return err
		}
		current[c] = true
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	// Stable ordering ensures deterministic audit rows and repeatable tests.
	capabilities := []sandboxdomain.Capability{sandboxdomain.CapabilityRead, sandboxdomain.CapabilitySearch, sandboxdomain.CapabilityEdit, sandboxdomain.CapabilityShell}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, capability := range capabilities {
		had, want := current[capability], next[capability]
		if had == want {
			continue
		}
		action := "grant"
		if want {
			_, err = tx.ExecContext(ctx, "INSERT INTO sandbox_profile_grants(sandbox_id,profile_id,capability,granted_at) VALUES(?,?,?,?)", id.String(), profileID, string(capability), now)
		} else {
			action = "revoke"
			_, err = tx.ExecContext(ctx, "DELETE FROM sandbox_profile_grants WHERE sandbox_id=? AND profile_id=? AND capability=?", id.String(), profileID, string(capability))
		}
		if err != nil {
			return fmt.Errorf("replace Sandbox Profile grants: mutate: %w", err)
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO sandbox_profile_grant_events(sandbox_id,profile_id,capability,action,principal_kind,principal_ref,origin,occurred_at) VALUES(?,?,?,?,?,?,?,?)",
			id.String(), profileID, string(capability), action, string(caller.Principal().Kind()), caller.Principal().Reference(), string(caller.Origin()), now)
		if err != nil {
			return fmt.Errorf("replace Sandbox Profile grants: audit: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("replace Sandbox Profile grants: commit: %w", err)
	}
	return nil
}

// ListGrantEvents is bounded review-only audit evidence.
type GrantEvent struct {
	ProfileID  string
	Capability sandboxdomain.Capability
	Action     string
	OccurredAt string
}

func (s *Store) ListGrantEvents(ctx context.Context, id sandboxdomain.ID, caller governancedomain.Context, limit int) ([]GrantEvent, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("sandbox grant audit limit must be 1..100")
	}
	item, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !sandboxapplication.IsOwner(caller, item) {
		return nil, sandboxdomain.ErrNotAuthorized
	}
	rows, err := s.db.QueryContext(ctx, "SELECT profile_id,capability,action,occurred_at FROM sandbox_profile_grant_events WHERE sandbox_id=? ORDER BY event_id DESC LIMIT ?", id.String(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]GrantEvent, 0)
	for rows.Next() {
		var e GrantEvent
		if err := rows.Scan(&e.ProfileID, &e.Capability, &e.Action, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Keep SQL-backed grant listing deterministic even if callers construct maps.
func SortedCapabilities(grant sandboxapplication.ProfileGrant) []sandboxdomain.Capability {
	out := append([]sandboxdomain.Capability(nil), grant.Capabilities...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
