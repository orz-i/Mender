package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
	_ "modernc.org/sqlite"
)

const installationColumns = "installation_id, skill_id, name, description, source_kind, source_reference, digest, version_hint, trusted, enabled, installed_at"

// Store persists managed Skill installation metadata.
type Store struct {
	db *sql.DB
}

// Open opens the Skill installation store and applies forward-only migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open skill installation sqlite: path must not be empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open skill installation sqlite: resolve path: %w", err)
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	u := url.URL{Scheme: "file", Path: slashed}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open skill installation sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open skill installation sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open skill installation sqlite: migrate: %w", err)
	}
	return store, nil
}

// Close releases the SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Create persists one immutable Skill installation.
func (s *Store) Create(
	ctx context.Context,
	installation skilldomain.Installation,
) error {
	if err := installation.Validate(); err != nil {
		return fmt.Errorf("create skill installation: %w", err)
	}
	_, err := s.db.ExecContext(
		ctx,
		"INSERT INTO skill_installations("+installationColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		installation.ID().String(),
		installation.SkillID().String(),
		installation.Name(),
		installation.Description(),
		string(installation.SourceKind()),
		installation.SourceReference(),
		installation.Digest(),
		installation.VersionHint(),
		boolInt(installation.Trusted()),
		boolInt(installation.Enabled()),
		installation.InstalledAt().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return skillapplication.ErrInstallationConflict
		}
		return fmt.Errorf("create skill installation: %w", err)
	}
	return nil
}

// Get returns one installation by exact installation identity.
func (s *Store) Get(
	ctx context.Context,
	id skilldomain.InstallationID,
) (skilldomain.Installation, error) {
	if _, err := skilldomain.ParseInstallationID(id.String()); err != nil {
		return skilldomain.Installation{}, err
	}
	row := s.db.QueryRowContext(
		ctx,
		"SELECT "+installationColumns+" FROM skill_installations WHERE installation_id = ?",
		id.String(),
	)
	item, err := scanInstallation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return skilldomain.Installation{}, skillapplication.ErrInstallationNotFound
	}
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("get skill installation: %w", err)
	}
	return item, nil
}

// GetBySkillDigest resolves an exact snapshot in one logical Skill lineage.
func (s *Store) GetBySkillDigest(
	ctx context.Context,
	skillID skilldomain.SkillID,
	digest string,
) (skilldomain.Installation, error) {
	row := s.db.QueryRowContext(
		ctx,
		"SELECT "+installationColumns+" FROM skill_installations WHERE skill_id = ? AND digest = ?",
		skillID.String(),
		digest,
	)
	item, err := scanInstallation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return skilldomain.Installation{}, skillapplication.ErrInstallationNotFound
	}
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("get skill installation by digest: %w", err)
	}
	return item, nil
}

// List returns managed installations in deterministic management order.
func (s *Store) List(ctx context.Context) ([]skilldomain.Installation, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT "+installationColumns+" FROM skill_installations ORDER BY name, installed_at, installation_id",
	)
	if err != nil {
		return nil, fmt.Errorf("list skill installations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]skilldomain.Installation, 0)
	for rows.Next() {
		item, scanErr := scanInstallation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list skill installations: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list skill installations: rows: %w", err)
	}
	return items, nil
}

// SetState persists trust and enablement for one exact installation.
func (s *Store) SetState(
	ctx context.Context,
	id skilldomain.InstallationID,
	trusted bool,
	enabled bool,
) error {
	if enabled && !trusted {
		return fmt.Errorf("set skill installation state: enabled installation must be trusted")
	}
	result, err := s.db.ExecContext(
		ctx,
		"UPDATE skill_installations SET trusted = ?, enabled = ? WHERE installation_id = ?",
		boolInt(trusted),
		boolInt(enabled),
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("set skill installation state: %w", err)
	}
	return requireAffected(result)
}

// Delete removes one installation metadata row.
func (s *Store) Delete(
	ctx context.Context,
	id skilldomain.InstallationID,
) error {
	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM skill_installations WHERE installation_id = ?",
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("delete skill installation: %w", err)
	}
	return requireAffected(result)
}

type rowScanner interface {
	Scan(...any) error
}

func scanInstallation(scanner rowScanner) (skilldomain.Installation, error) {
	var (
		rawID           string
		rawSkillID      string
		name            string
		description     string
		rawSourceKind   string
		sourceReference string
		digest          string
		versionHint     string
		trusted         int
		enabled         int
		rawInstalledAt  string
	)
	if err := scanner.Scan(
		&rawID,
		&rawSkillID,
		&name,
		&description,
		&rawSourceKind,
		&sourceReference,
		&digest,
		&versionHint,
		&trusted,
		&enabled,
		&rawInstalledAt,
	); err != nil {
		return skilldomain.Installation{}, err
	}
	id, err := skilldomain.ParseInstallationID(rawID)
	if err != nil {
		return skilldomain.Installation{}, err
	}
	skillID, err := skilldomain.ParseSkillID(rawSkillID)
	if err != nil {
		return skilldomain.Installation{}, err
	}
	installedAt, err := time.Parse(time.RFC3339Nano, rawInstalledAt)
	if err != nil {
		return skilldomain.Installation{}, fmt.Errorf("parse installed_at: %w", err)
	}
	return skilldomain.NewInstallation(
		id,
		skillID,
		name,
		description,
		skilldomain.SourceKind(rawSourceKind),
		sourceReference,
		digest,
		versionHint,
		trusted != 0,
		enabled != 0,
		installedAt,
	)
}

func requireAffected(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("skill installation rows affected: %w", err)
	}
	if count == 0 {
		return skillapplication.ErrInstallationNotFound
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func isUniqueConstraint(err error) bool {
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "unique") ||
		strings.Contains(value, "primary key")
}
