package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func (s *Store) CreateProfile(ctx context.Context, profile profiledomain.Profile) error {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO profiles(id, slug, display_name, exposure_mode)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(slug) DO NOTHING`,
		profile.ID().String(),
		profile.Slug().String(),
		profile.DisplayName(),
		string(profile.Mode()),
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
	if len(profile.Selections()) != 0 {
		if err := s.SaveProfile(ctx, profile); err != nil {
			return fmt.Errorf("create profile selections: %w", err)
		}
	}
	return nil
}

func (s *Store) GetProfileByID(
	ctx context.Context,
	id profiledomain.ProfileID,
) (profiledomain.Profile, error) {
	var rawID, rawSlug, displayName, rawMode string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, slug, display_name, exposure_mode
        FROM profiles
        WHERE id = ?`,
		id.String(),
	).Scan(&rawID, &rawSlug, &displayName, &rawMode)
	if errors.Is(err, sql.ErrNoRows) {
		return profiledomain.Profile{}, profileapplication.ErrProfileNotFound
	}
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile by id: %w", err)
	}
	return s.restoreProfile(ctx, rawID, rawSlug, displayName, rawMode)
}

func (s *Store) GetProfileBySlug(
	ctx context.Context,
	slug profiledomain.Slug,
) (profiledomain.Profile, error) {
	var rawID, rawSlug, displayName, rawMode string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, slug, display_name, exposure_mode
        FROM profiles
        WHERE slug = ?`,
		slug.String(),
	).Scan(&rawID, &rawSlug, &displayName, &rawMode)
	if errors.Is(err, sql.ErrNoRows) {
		return profiledomain.Profile{}, profileapplication.ErrProfileNotFound
	}
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return s.restoreProfile(ctx, rawID, rawSlug, displayName, rawMode)
}

func (s *Store) ListProfiles(ctx context.Context) ([]profiledomain.Profile, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, slug, display_name, exposure_mode
        FROM profiles
        ORDER BY slug, id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}

	type row struct {
		id, slug, displayName, mode string
	}
	rawProfiles := make([]row, 0)
	for rows.Next() {
		var value row
		if err := rows.Scan(&value.id, &value.slug, &value.displayName, &value.mode); err != nil {
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
		profile, err := s.restoreProfile(ctx, raw.id, raw.slug, raw.displayName, raw.mode)
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
        SET display_name = ?, exposure_mode = ?
        WHERE id = ?`,
		profile.DisplayName(),
		string(profile.Mode()),
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

	for _, selection := range profile.Selections() {
		direct := 0
		if selection.Direct {
			direct = 1
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO profile_tools(profile_id, tool_id, direct)
            VALUES (?, ?, ?)`,
			profile.ID().String(),
			selection.ToolID.String(),
			direct,
		); err != nil {
			return fmt.Errorf("save profile: insert membership %s: %w", selection.ToolID, err)
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
		`SELECT tool_id, direct
        FROM profile_tools
        WHERE profile_id = ?
        ORDER BY tool_id`,
		rawID,
	)
	if err != nil {
		return profiledomain.Profile{}, fmt.Errorf("load profile memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	selections := make([]profiledomain.ToolSelection, 0)
	for rows.Next() {
		var rawToolID string
		var direct int
		if err := rows.Scan(&rawToolID, &direct); err != nil {
			return profiledomain.Profile{}, fmt.Errorf("load profile memberships: scan: %w", err)
		}
		toolID, err := catalogdomain.ParseToolID(rawToolID)
		if err != nil {
			return profiledomain.Profile{}, fmt.Errorf("parse profile tool id: %w", err)
		}
		selections = append(selections, profiledomain.ToolSelection{
			ToolID: toolID,
			Direct: direct == 1,
		})
	}
	if err := rows.Err(); err != nil {
		return profiledomain.Profile{}, fmt.Errorf("load profile memberships: rows: %w", err)
	}

	return profiledomain.NewProfile(
		id,
		slug,
		displayName,
		profiledomain.ExposureMode(rawMode),
		selections,
	)
}
