package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"
	_ "modernc.org/sqlite"
)

var (
	// ErrNotFound indicates no persisted Provider plugin configuration exists.
	ErrNotFound = errors.New("provider config: not found")

	importNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

// Store persists generic Provider plugin configuration.
type Store struct {
	db *sql.DB
}

// Open opens the Provider plugin configuration store and applies known schema
// migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open provider config sqlite: path must not be empty")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open provider config sqlite: resolve path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open provider config sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open provider config sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open provider config sqlite: migrate: %w", err)
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

// Save creates or replaces one Provider configuration.
func (s *Store) Save(
	ctx context.Context,
	config providerplugin.Configuration,
) error {
	if err := config.ValidateForPersistence(); err != nil {
		return fmt.Errorf("save provider config: %w", err)
	}
	credentialRefsJSON, connectionSummary, err := encodeConfigMetadata(config.Metadata)
	if err != nil {
		return fmt.Errorf("save provider config: metadata: %w", err)
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO provider_configs(
			provider_id, plugin_id, plugin_installation_id, config_version, revision, config_json,
			credential_refs_json, connection_summary
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_id) DO UPDATE SET
			plugin_id = excluded.plugin_id,
			plugin_installation_id = excluded.plugin_installation_id,
			config_version = excluded.config_version,
			revision = excluded.revision,
			config_json = excluded.config_json,
			credential_refs_json = excluded.credential_refs_json,
			connection_summary = excluded.connection_summary`,
		config.ProviderID.String(),
		config.PluginID.String(),
		nullableInstallationID(config.InstallationID),
		config.ConfigVersion,
		config.Revision,
		string(config.Config),
		credentialRefsJSON,
		connectionSummary,
	)
	if err != nil {
		return fmt.Errorf("save provider config: %w", err)
	}
	return nil
}

// GetProviderConfig loads one Provider configuration and implements
// providerplugin.ConfigurationSource.
func (s *Store) GetProviderConfig(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (providerplugin.Configuration, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT provider_id, plugin_id, plugin_installation_id, config_version, revision, config_json,
			credential_refs_json, connection_summary
		FROM provider_configs
		WHERE provider_id = ?`,
		providerID.String(),
	)
	config, err := scanConfiguration(row)
	if errors.Is(err, sql.ErrNoRows) {
		return providerplugin.Configuration{}, ErrNotFound
	}
	if err != nil {
		return providerplugin.Configuration{}, fmt.Errorf("get provider config: %w", err)
	}
	return config, nil
}

// List returns all Provider configurations ordered by ProviderID.
func (s *Store) List(
	ctx context.Context,
) ([]providerplugin.Configuration, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT provider_id, plugin_id, plugin_installation_id, config_version, revision, config_json,
			credential_refs_json, connection_summary
		FROM provider_configs
		ORDER BY provider_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list provider configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	configs := make([]providerplugin.Configuration, 0)
	for rows.Next() {
		config, scanErr := scanConfiguration(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list provider configs: %w", scanErr)
		}
		configs = append(configs, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list provider configs: rows: %w", err)
	}
	return configs, nil
}

// Delete removes one persisted Provider configuration.
func (s *Store) Delete(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) error {
	if _, err := catalogdomain.ParseProviderID(providerID.String()); err != nil {
		return fmt.Errorf("delete provider config: %w", err)
	}
	if _, err := s.db.ExecContext(
		ctx,
		"DELETE FROM provider_configs WHERE provider_id = ?",
		providerID.String(),
	); err != nil {
		return fmt.Errorf("delete provider config: %w", err)
	}
	return nil
}

// ImportOnce atomically imports a named compatibility snapshot exactly once.
//
// Existing generic Provider rows win over imported rows. This lets a migration
// resume safely without overwriting a configuration that has already moved to
// the generic store.
func (s *Store) ImportOnce(
	ctx context.Context,
	name string,
	configs []providerplugin.Configuration,
) error {
	name = strings.TrimSpace(name)
	if !importNamePattern.MatchString(name) {
		return fmt.Errorf("import provider configs: invalid import name %q", name)
	}
	for _, config := range configs {
		if err := config.ValidateForPersistence(); err != nil {
			return fmt.Errorf("import provider configs: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("import provider configs: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var applied int
	if err := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM provider_config_imports WHERE name = ?",
		name,
	).Scan(&applied); err != nil {
		return fmt.Errorf("import provider configs: check marker: %w", err)
	}
	if applied != 0 {
		return nil
	}

	for _, config := range configs {
		credentialRefsJSON, connectionSummary, metadataErr := encodeConfigMetadata(config.Metadata)
		if metadataErr != nil {
			return fmt.Errorf("import provider configs: metadata %s: %w", config.ProviderID, metadataErr)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO provider_configs(
				provider_id, plugin_id, plugin_installation_id, config_version, revision, config_json,
				credential_refs_json, connection_summary
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(provider_id) DO NOTHING`,
			config.ProviderID.String(),
			config.PluginID.String(),
			nullableInstallationID(config.InstallationID),
			config.ConfigVersion,
			config.Revision,
			string(config.Config),
			credentialRefsJSON,
			connectionSummary,
		); err != nil {
			return fmt.Errorf(
				"import provider configs: insert %s: %w",
				config.ProviderID,
				err,
			)
		}
	}
	if _, err := tx.ExecContext(
		ctx,
		"INSERT INTO provider_config_imports(name, applied_at) VALUES (?, ?)",
		name,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("import provider configs: record marker: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("import provider configs: commit: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanConfiguration(
	scanner rowScanner,
) (providerplugin.Configuration, error) {
	var (
		rawProviderID     string
		rawPluginID       string
		rawInstallationID sql.NullString
		configVersion     int
		revision          string
		rawConfig         string
		rawCredentialRefs sql.NullString
		rawSummary        sql.NullString
	)
	if err := scanner.Scan(
		&rawProviderID,
		&rawPluginID,
		&rawInstallationID,
		&configVersion,
		&revision,
		&rawConfig,
		&rawCredentialRefs,
		&rawSummary,
	); err != nil {
		return providerplugin.Configuration{}, err
	}
	providerID, err := catalogdomain.ParseProviderID(rawProviderID)
	if err != nil {
		return providerplugin.Configuration{}, fmt.Errorf("parse provider id: %w", err)
	}
	pluginID, err := providerplugin.ParseID(rawPluginID)
	if err != nil {
		return providerplugin.Configuration{}, fmt.Errorf("parse plugin id: %w", err)
	}
	var installationID *providerplugin.InstallationID
	if rawInstallationID.Valid {
		parsed, parseErr := providerplugin.ParseInstallationID(rawInstallationID.String)
		if parseErr != nil {
			return providerplugin.Configuration{}, fmt.Errorf("parse plugin installation id: %w", parseErr)
		}
		installationID = &parsed
	}
	config := providerplugin.Configuration{
		ProviderID:     providerID,
		PluginID:       pluginID,
		InstallationID: installationID,
		ConfigVersion:  configVersion,
		Revision:       revision,
		Config:         []byte(rawConfig),
	}
	if rawCredentialRefs.Valid {
		metadata, metadataErr := decodeConfigMetadata(
			rawCredentialRefs.String,
			rawSummary.String,
		)
		if metadataErr != nil {
			return providerplugin.Configuration{}, fmt.Errorf(
				"parse provider config metadata: %w",
				metadataErr,
			)
		}
		config.Metadata = &metadata
	} else if rawSummary.Valid {
		return providerplugin.Configuration{}, fmt.Errorf(
			"parse provider config metadata: connection summary exists without Credential metadata marker",
		)
	}
	if err := config.Validate(); err != nil {
		return providerplugin.Configuration{}, err
	}
	return config, nil
}

func encodeConfigMetadata(metadata *providerplugin.ConfigMetadata) (any, any, error) {
	if metadata == nil {
		return nil, nil, nil
	}
	normalized, err := providerplugin.NewConfigMetadataWithSummary(
		metadata.CredentialRefs,
		metadata.ConnectionSummary,
	)
	if err != nil {
		return nil, nil, err
	}
	refs := make([]string, 0, len(normalized.CredentialRefs))
	for _, ref := range normalized.CredentialRefs {
		refs = append(refs, ref.String())
	}
	raw, err := json.Marshal(refs)
	if err != nil {
		return nil, nil, err
	}
	return string(raw), normalized.ConnectionSummary, nil
}

func decodeConfigMetadata(rawRefs, connectionSummary string) (providerplugin.ConfigMetadata, error) {
	var values []string
	if err := json.Unmarshal([]byte(rawRefs), &values); err != nil {
		return providerplugin.ConfigMetadata{}, err
	}
	refs := make([]credentialdomain.Reference, 0, len(values))
	for _, value := range values {
		ref, err := credentialdomain.ParseReference(value)
		if err != nil {
			return providerplugin.ConfigMetadata{}, err
		}
		refs = append(refs, ref)
	}
	return providerplugin.NewConfigMetadataWithSummary(refs, connectionSummary)
}

func nullableInstallationID(id *providerplugin.InstallationID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

var _ providerplugin.ConfigurationSource = (*Store)(nil)
