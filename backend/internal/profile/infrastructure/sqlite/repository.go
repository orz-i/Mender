package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

func (s *Store) CreateProfile(ctx context.Context, profile profiledomain.Profile) error {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO profiles(id, slug, display_name)
        VALUES (?, ?, ?)
        ON CONFLICT(slug) DO NOTHING`,
		profile.ID().String(),
		profile.Slug().String(),
		profile.DisplayName(),
	)
	if err != nil {
		return fmt.Errorf("create profile: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("create profile: rows affected: %w", err)
	}
	if affected == 0 {
		return profileapplication.ErrSlugConflict
	}
	if len(profile.Bindings()) != 0 || len(profile.Skills()) != 0 {
		if err := s.SaveProfile(ctx, profile); err != nil {
			return fmt.Errorf("create profile bindings: %w", err)
		}
	}
	return nil
}

// GetProfileRevision performs a single read-only committed revision lookup.
// A full aggregate is only hydrated by consumers when this version changes.
func (s *Store) GetProfileRevision(ctx context.Context, id profiledomain.ProfileID) (uint64, error) {
	var revision uint64
	err := s.db.QueryRowContext(ctx, `SELECT revision FROM profiles WHERE id=?`, id.String()).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, profileapplication.ErrProfileNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get profile revision: %w", err)
	}
	return revision, nil
}

func (s *Store) GetProfileByID(
	ctx context.Context,
	id profiledomain.ProfileID,
) (profiledomain.Profile, error) {
	var rawID, rawSlug, displayName string
	var revision uint64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, slug, display_name, revision
        FROM profiles
        WHERE id = ?`,
		id.String(),
	).Scan(&rawID, &rawSlug, &displayName, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return profiledomain.Profile{}, profileapplication.ErrProfileNotFound
	}
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile by id: %w", err)
	}
	return s.restoreProfile(ctx, rawID, rawSlug, displayName, revision)
}

func (s *Store) GetProfileBySlug(
	ctx context.Context,
	slug profiledomain.Slug,
) (profiledomain.Profile, error) {
	var rawID, rawSlug, displayName string
	var revision uint64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, slug, display_name, revision
        FROM profiles
        WHERE slug = ?`,
		slug.String(),
	).Scan(&rawID, &rawSlug, &displayName, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return profiledomain.Profile{}, profileapplication.ErrProfileNotFound
	}
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return s.restoreProfile(ctx, rawID, rawSlug, displayName, revision)
}

func (s *Store) ListProfiles(ctx context.Context) ([]profiledomain.Profile, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, slug, display_name, revision
        FROM profiles
        ORDER BY slug, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}

	type row struct {
		id, slug, displayName string
		revision              uint64
	}
	rawProfiles := make([]row, 0)
	for rows.Next() {
		var value row
		if err := rows.Scan(
			&value.id,
			&value.slug,
			&value.displayName,
			&value.revision,
		); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("list profiles: scan: %w", err)
		}
		rawProfiles = append(rawProfiles, value)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("list profiles: rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("list profiles: close rows: %w", err)
	}

	profiles := make([]profiledomain.Profile, 0, len(rawProfiles))
	for _, raw := range rawProfiles {
		profile, err := s.restoreProfile(
			ctx,
			raw.id,
			raw.slug,
			raw.displayName,
			raw.revision,
		)
		if err != nil {
			return nil, fmt.Errorf("list profiles: restore %s: %w", raw.slug, err)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (s *Store) SaveProfile(ctx context.Context, profile profiledomain.Profile) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save profile: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(
		ctx,
		`UPDATE profiles
        SET display_name = ?, revision = revision + 1
        WHERE id = ? AND revision = ?`,
		profile.DisplayName(),
		profile.ID().String(),
		profile.Revision(),
	)
	if err != nil {
		return fmt.Errorf("save profile: update: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("save profile: rows affected: %w", err)
	}
	if affected == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM profiles WHERE id=?)", profile.ID().String()).Scan(&exists); err != nil {
			return fmt.Errorf("save profile: check stale revision: %w", err)
		}
		if !exists {
			return profileapplication.ErrProfileNotFound
		}
		return profileapplication.ErrRevisionConflict
	}

	// Preserve independently owned MCP direct selections for unchanged
	// members. Only true removals may cascade into the delivery relationship.
	retained := make(map[string]struct{}, len(profile.Bindings()))
	for _, binding := range profile.Bindings() {
		retained[binding.ToolID().String()] = struct{}{}
	}
	rows, err := tx.QueryContext(ctx, "SELECT tool_id FROM profile_tools WHERE profile_id=?", profile.ID().String())
	if err != nil {
		return fmt.Errorf("save profile: list previous bindings: %w", err)
	}
	var removed []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		if _, keep := retained[id]; !keep {
			removed = append(removed, id)
		}
	}
	rowsErr, closeErr := rows.Err(), rows.Close()
	if rowsErr != nil {
		return rowsErr
	}
	if closeErr != nil {
		return closeErr
	}
	for _, id := range removed {
		if _, err := tx.ExecContext(ctx, "DELETE FROM profile_tools WHERE profile_id=? AND tool_id=?", profile.ID().String(), id); err != nil {
			return fmt.Errorf("save profile: remove membership: %w", err)
		}
	}

	for _, binding := range profile.Bindings() {
		credentialRefs := make([]string, 0, len(binding.CredentialRefs()))
		for _, ref := range binding.CredentialRefs() {
			credentialRefs = append(credentialRefs, ref.String())
		}
		credentialRefsJSON, err := json.Marshal(credentialRefs)
		if err != nil {
			return fmt.Errorf("save profile: encode credential refs: %w", err)
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO profile_tools(
                profile_id,
                tool_id,
                alias,
                title_override,
                description_override,
                policy_constraint,
                concurrency_constraint,
                credential_refs_json
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(profile_id,tool_id) DO UPDATE SET
                alias=excluded.alias, title_override=excluded.title_override,
                description_override=excluded.description_override,
                policy_constraint=excluded.policy_constraint,
                concurrency_constraint=excluded.concurrency_constraint,
                credential_refs_json=excluded.credential_refs_json`,
			profile.ID().String(),
			binding.ToolID().String(),
			binding.Alias(),
			binding.TitleOverride(),
			binding.DescriptionOverride(),
			string(binding.PolicyConstraint()),
			string(binding.ConcurrencyConstraint()),
			string(credentialRefsJSON),
		); err != nil {
			if strings.Contains(err.Error(), "Profile Tool reference unavailable") {
				return fmt.Errorf("%w: %s", profileapplication.ErrToolReferenceConflict, binding.ToolID())
			}
			return fmt.Errorf("save profile: insert binding %s: %w", binding.ToolID(), err)
		}
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM profile_skills WHERE profile_id = ?", profile.ID().String()); err != nil {
		return fmt.Errorf("save profile: clear skill memberships: %w", err)
	}
	for _, id := range profile.Skills() {
		if _, err := tx.ExecContext(ctx, "INSERT INTO profile_skills(profile_id, installation_id) VALUES (?, ?)", profile.ID().String(), id.String()); err != nil {
			if strings.Contains(err.Error(), "profile skill installation not deployable") {
				return fmt.Errorf("save profile: insert skill %s: %w", id, profileapplication.ErrSkillNotDeployable)
			}
			return fmt.Errorf("save profile: insert skill %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save profile: commit: %w", err)
	}
	return nil
}

func (s *Store) restoreProfile(
	ctx context.Context,
	rawID string,
	rawSlug string,
	displayName string,
	revision uint64,
) (profiledomain.Profile, error) {
	id, err := profiledomain.ParseProfileID(rawID)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("parse profile id: %w", err)
	}
	slug, err := profiledomain.NewSlug(rawSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("parse profile slug: %w", err)
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
            tool_id,
            alias,
            title_override,
            description_override,
            policy_constraint,
            concurrency_constraint,
            credential_refs_json
        FROM profile_tools
        WHERE profile_id = ?
        ORDER BY tool_id`,
		rawID,
	)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("load profile memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	bindings := make([]profiledomain.ToolBinding, 0)
	for rows.Next() {
		var (
			rawToolID             string
			alias                 string
			titleOverride         string
			descriptionOverride   string
			policyConstraint      string
			concurrencyConstraint string
			rawCredentialRefs     string
		)
		if err := rows.Scan(
			&rawToolID,
			&alias,
			&titleOverride,
			&descriptionOverride,
			&policyConstraint,
			&concurrencyConstraint,
			&rawCredentialRefs,
		); err != nil {
			return profiledomain.Profile{}, fmt.Errorf("load profile bindings: scan: %w", err)
		}
		toolID, err := catalogdomain.ParseToolID(rawToolID)
		if err != nil {
			return profiledomain.Profile{}, fmt.Errorf("parse profile tool id: %w", err)
		}
		var rawRefs []string
		if err := json.Unmarshal([]byte(rawCredentialRefs), &rawRefs); err != nil {
			return profiledomain.Profile{}, fmt.Errorf("parse profile credential refs: %w", err)
		}
		credentialRefs := make([]credentialdomain.Reference, 0, len(rawRefs))
		for _, rawRef := range rawRefs {
			ref, err := credentialdomain.ParseReference(rawRef)
			if err != nil {
				return profiledomain.Profile{}, fmt.Errorf("parse profile credential ref: %w", err)
			}
			credentialRefs = append(credentialRefs, ref)
		}

		binding, err := profiledomain.NewToolBinding(profiledomain.ToolBindingSpec{
			ToolID:                toolID,
			Alias:                 alias,
			TitleOverride:         titleOverride,
			DescriptionOverride:   descriptionOverride,
			PolicyConstraint:      profiledomain.PolicyConstraint(policyConstraint),
			ConcurrencyConstraint: profiledomain.ConcurrencyConstraint(concurrencyConstraint),
			CredentialRefs:        credentialRefs,
		})
		if err != nil {
			return profiledomain.Profile{}, fmt.Errorf("restore profile tool binding %s: %w", rawToolID, err)
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("load profile memberships: rows: %w", err)
	}

	profile, err := profiledomain.NewProfileWithBindings(
		id,
		slug,
		displayName,
		bindings,
	)
	if err != nil {
		return profiledomain.Profile{}, err
	}
	skillRows, err := s.db.QueryContext(ctx, "SELECT installation_id FROM profile_skills WHERE profile_id = ? ORDER BY installation_id", rawID)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("load profile skills: %w", err)
	}
	var installed []skilldomain.InstallationID
	for skillRows.Next() {
		var rawSkillID string
		if err := skillRows.Scan(&rawSkillID); err != nil {
			_ = skillRows.Close()
			return profiledomain.Profile{}, fmt.Errorf("load profile skill: %w", err)
		}
		id, err := skilldomain.ParseInstallationID(rawSkillID)
		if err != nil {
			_ = skillRows.Close()
			return profiledomain.Profile{}, fmt.Errorf("parse profile skill: %w", err)
		}
		installed = append(installed, id)
	}
	rowsErr := skillRows.Err()
	closeErr := skillRows.Close()
	if rowsErr != nil {
		return profiledomain.Profile{}, fmt.Errorf("load profile skills: rows: %w", rowsErr)
	}
	if closeErr != nil {
		return profiledomain.Profile{}, fmt.Errorf("load profile skills: close: %w", closeErr)
	}
	profile, err = profile.ReplaceSkills(installed)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("restore profile skills: %w", err)
	}
	return profile.WithRevision(revision), nil
}

// ListSkillProfiles reports explicit Profile references to an exact installation.
// This is a usage lookup, never a content/permission grant.
func (s *Store) ListSkillProfiles(ctx context.Context, id skilldomain.InstallationID) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.slug FROM profile_skills ps
		JOIN profiles p ON p.id = ps.profile_id
		WHERE ps.installation_id = ? ORDER BY p.slug`, id.String())
	if err != nil {
		return nil, fmt.Errorf("list skill profile uses: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		result = append(result, slug)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
