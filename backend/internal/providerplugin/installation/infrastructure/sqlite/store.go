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

	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	installationapplication "github.com/orz-i/Mender/backend/internal/providerplugin/installation/application"
	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
	_ "modernc.org/sqlite"
)

// Store persists external Provider plugin installation metadata.
type Store struct {
	db *sql.DB
}

// Open opens the plugin installation store and applies forward-only migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open plugin installation sqlite: path must not be empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open plugin installation sqlite: resolve path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open plugin installation sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open plugin installation sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open plugin installation sqlite: migrate: %w", err)
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

// Create persists one immutable plugin installation.
func (s *Store) Create(
	ctx context.Context,
	installation installationdomain.Installation,
) error {
	if err := installation.Validate(); err != nil {
		return fmt.Errorf("create plugin installation: %w", err)
	}
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO provider_plugin_installations(
			installation_id,
			plugin_id,
			package_version,
			source_kind,
			source_reference,
			digest,
			manifest_json,
			status,
			installed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		installation.ID().String(),
		installation.PluginID().String(),
		installation.PackageVersion(),
		string(installation.SourceKind()),
		installation.SourceReference(),
		installation.Digest(),
		string(installation.ManifestJSON()),
		string(installation.Status()),
		installation.InstalledAt().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return installationapplication.ErrInstallationConflict
		}
		return fmt.Errorf("create plugin installation: %w", err)
	}
	return nil
}

// Get returns one plugin installation by stable installation identity.
func (s *Store) Get(
	ctx context.Context,
	id installationdomain.InstallationID,
) (installationdomain.Installation, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			installation_id,
			plugin_id,
			package_version,
			source_kind,
			source_reference,
			digest,
			manifest_json,
			status,
			installed_at
		FROM provider_plugin_installations
		WHERE installation_id = ?`,
		id.String(),
	)
	installation, err := scanInstallation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return installationdomain.Installation{}, installationapplication.ErrInstallationNotFound
	}
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("get plugin installation: %w", err)
	}
	return installation, nil
}

// GetByDigest returns an installation for one exact immutable package digest.
func (s *Store) GetByDigest(
	ctx context.Context,
	digest string,
) (installationdomain.Installation, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT
			installation_id,
			plugin_id,
			package_version,
			source_kind,
			source_reference,
			digest,
			manifest_json,
			status,
			installed_at
		FROM provider_plugin_installations
		WHERE digest = ?`,
		digest,
	)
	installation, err := scanInstallation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return installationdomain.Installation{}, installationapplication.ErrInstallationNotFound
	}
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("get plugin installation by digest: %w", err)
	}
	return installation, nil
}

// List returns plugin installations ordered by InstallationID.
func (s *Store) List(
	ctx context.Context,
) ([]installationdomain.Installation, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			installation_id,
			plugin_id,
			package_version,
			source_kind,
			source_reference,
			digest,
			manifest_json,
			status,
			installed_at
		FROM provider_plugin_installations
		ORDER BY installation_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list plugin installations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]installationdomain.Installation, 0)
	for rows.Next() {
		item, scanErr := scanInstallation(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list plugin installations: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list plugin installations: rows: %w", err)
	}
	return items, nil
}

// SetStatus updates only the administrative availability of an installation.
func (s *Store) SetStatus(
	ctx context.Context,
	id installationdomain.InstallationID,
	status installationdomain.Status,
) error {
	if _, err := installationdomain.ParseInstallationID(id.String()); err != nil {
		return fmt.Errorf("set plugin installation status: %w", err)
	}
	if err := installationdomain.ValidateStatus(status); err != nil {
		return fmt.Errorf("set plugin installation status: %w", err)
	}
	result, err := s.db.ExecContext(
		ctx,
		"UPDATE provider_plugin_installations SET status = ? WHERE installation_id = ?",
		string(status),
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("set plugin installation status: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set plugin installation status: rows affected: %w", err)
	}
	if count == 0 {
		return installationapplication.ErrInstallationNotFound
	}
	return nil
}

// Delete removes plugin installation metadata. Reference guards belong to the application layer.
func (s *Store) Delete(
	ctx context.Context,
	id installationdomain.InstallationID,
) error {
	if _, err := installationdomain.ParseInstallationID(id.String()); err != nil {
		return fmt.Errorf("delete plugin installation: %w", err)
	}
	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM provider_plugin_installations WHERE installation_id = ?",
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("delete plugin installation: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete plugin installation: rows affected: %w", err)
	}
	if count == 0 {
		return installationapplication.ErrInstallationNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanInstallation(
	scanner rowScanner,
) (installationdomain.Installation, error) {
	var (
		rawID           string
		rawPluginID     string
		packageVersion  string
		rawSourceKind   string
		sourceReference string
		digest          string
		rawManifest     string
		rawStatus       string
		rawInstalledAt  string
	)
	if err := scanner.Scan(
		&rawID,
		&rawPluginID,
		&packageVersion,
		&rawSourceKind,
		&sourceReference,
		&digest,
		&rawManifest,
		&rawStatus,
		&rawInstalledAt,
	); err != nil {
		return installationdomain.Installation{}, err
	}

	id, err := installationdomain.ParseInstallationID(rawID)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("parse plugin installation id: %w", err)
	}
	pluginID, err := providerplugin.ParseID(rawPluginID)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("parse plugin id: %w", err)
	}
	installedAt, err := time.Parse(time.RFC3339Nano, rawInstalledAt)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("parse plugin installed_at: %w", err)
	}
	installation, err := installationdomain.NewInstallation(
		id,
		pluginID,
		packageVersion,
		installationdomain.SourceKind(rawSourceKind),
		sourceReference,
		digest,
		[]byte(rawManifest),
		installationdomain.Status(rawStatus),
		installedAt,
	)
	if err != nil {
		return installationdomain.Installation{}, fmt.Errorf("restore plugin installation: %w", err)
	}
	return installation, nil
}

func isUniqueConstraint(err error) bool {
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "unique") ||
		strings.Contains(value, "primary key")
}
