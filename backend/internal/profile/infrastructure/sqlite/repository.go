package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

func (s *Store) CreateProfile(ctx context.Context, profile profiledomain.Profile) error {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO profiles(id, slug, display_name, exposure_mode, tunnel_id)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(slug) DO NOTHING`,
		profile.ID().String(),
		profile.Slug().String(),
		profile.DisplayName(),
		string(profile.Mode()),
		profileTunnelID(profile),
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
	if len(profile.Bindings()) != 0 {
		if err := s.SaveProfile(ctx, profile); err != nil {
			return fmt.Errorf("create profile bindings: %w", err)
		}
	}
	return nil
}

func (s *Store) GetProfileByID(
	ctx context.Context,
	id profiledomain.ProfileID,
) (profiledomain.Profile, error) {
	var rawID, rawSlug, displayName, rawMode, rawTunnelID string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, slug, display_name, exposure_mode, tunnel_id
        FROM profiles
        WHERE id = ?`,
		id.String(),
	).Scan(&rawID, &rawSlug, &displayName, &rawMode, &rawTunnelID)
	if errors.Is(err, sql.ErrNoRows) {
		return profiledomain.Profile{}, profileapplication.ErrProfileNotFound
	}
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile by id: %w", err)
	}
	return s.restoreProfile(ctx, rawID, rawSlug, displayName, rawMode, rawTunnelID)
}

func (s *Store) GetProfileBySlug(
	ctx context.Context,
	slug profiledomain.Slug,
) (profiledomain.Profile, error) {
	var rawID, rawSlug, displayName, rawMode, rawTunnelID string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, slug, display_name, exposure_mode, tunnel_id
        FROM profiles
        WHERE slug = ?`,
		slug.String(),
	).Scan(&rawID, &rawSlug, &displayName, &rawMode, &rawTunnelID)
	if errors.Is(err, sql.ErrNoRows) {
		return profiledomain.Profile{}, profileapplication.ErrProfileNotFound
	}
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return s.restoreProfile(ctx, rawID, rawSlug, displayName, rawMode, rawTunnelID)
}

func (s *Store) ListProfiles(ctx context.Context) ([]profiledomain.Profile, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, slug, display_name, exposure_mode, tunnel_id
        FROM profiles
        ORDER BY slug, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}

	type row struct {
		id, slug, displayName, mode, tunnelID string
	}
	rawProfiles := make([]row, 0)
	for rows.Next() {
		var value row
		if err := rows.Scan(
			&value.id,
			&value.slug,
			&value.displayName,
			&value.mode,
			&value.tunnelID,
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
			raw.mode,
			raw.tunnelID,
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
        SET display_name = ?, exposure_mode = ?, tunnel_id = ?
        WHERE id = ?`,
		profile.DisplayName(),
		string(profile.Mode()),
		profileTunnelID(profile),
		profile.ID().String(),
	)
	if err != nil {
		return fmt.Errorf("save profile: update: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("save profile: rows affected: %w", err)
	}
	if affected == 0 {
		return profileapplication.ErrProfileNotFound
	}

	if _, err := tx.ExecContext(
		ctx,
		"DELETE FROM profile_tools WHERE profile_id = ?",
		profile.ID().String(),
	); err != nil {
		return fmt.Errorf("save profile: clear memberships: %w", err)
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

		direct := 0
		if binding.Direct() {
			direct = 1
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO profile_tools(
                profile_id,
                tool_id,
                direct,
                alias,
                title_override,
                description_override,
                policy_constraint,
                concurrency_constraint,
                credential_refs_json
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			profile.ID().String(),
			binding.ToolID().String(),
			direct,
			binding.Alias(),
			binding.TitleOverride(),
			binding.DescriptionOverride(),
			string(binding.PolicyConstraint()),
			string(binding.ConcurrencyConstraint()),
			string(credentialRefsJSON),
		); err != nil {
			return fmt.Errorf("save profile: insert binding %s: %w", binding.ToolID(), err)
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
	rawMode string,
	rawTunnelID string,
) (profiledomain.Profile, error) {
	id, err := profiledomain.ParseProfileID(rawID)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("parse profile id: %w", err)
	}
	slug, err := profiledomain.NewSlug(rawSlug)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("parse profile slug: %w", err)
	}
	var tunnelID tunneldomain.TunnelID
	if rawTunnelID != "" {
		tunnelID, err = tunneldomain.ParseTunnelID(rawTunnelID)
		if err != nil {
			return profiledomain.Profile{}, fmt.Errorf("parse profile tunnel id: %w", err)
		}
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
            tool_id,
            direct,
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
			direct                int
			alias                 string
			titleOverride         string
			descriptionOverride   string
			policyConstraint      string
			concurrencyConstraint string
			rawCredentialRefs     string
		)
		if err := rows.Scan(
			&rawToolID,
			&direct,
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
			Direct:                direct == 1,
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

	return profiledomain.NewProfileWithBindingsAndTunnel(
		id,
		slug,
		displayName,
		profiledomain.ExposureMode(rawMode),
		tunnelID,
		bindings,
	)
}

func profileTunnelID(profile profiledomain.Profile) string {
	if tunnelID, ok := profile.TunnelID(); ok {
		return tunnelID.String()
	}
	return ""
}
