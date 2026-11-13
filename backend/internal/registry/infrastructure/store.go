package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/orz-i/Mender/backend/internal/registry/domain"
	_ "modernc.org/sqlite"
)

// IndexStore provides SQLite caching and persistence for registry indexes and plugin offerings.
type IndexStore struct {
	db *sql.DB
}

// OpenIndexStore opens or creates an SQLite database at dbPath and initializes the schema.
func OpenIndexStore(ctx context.Context, dbPath string) (*IndexStore, error) {
	trimmed := strings.TrimSpace(dbPath)
	if trimmed == "" {
		return nil, fmt.Errorf("index store: path must not be empty")
	}

	var dsn string
	if trimmed == ":memory:" {
		dsn = "file::memory:?cache=shared&_pragma=busy_timeout(5000)"
	} else {
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return nil, fmt.Errorf("index store: resolve path: %w", err)
		}
		slashed := filepath.ToSlash(abs)
		if !strings.HasPrefix(slashed, "/") {
			slashed = "/" + slashed
		}
		u := url.URL{Scheme: "file", Path: slashed}
		q := u.Query()
		q.Set("mode", "rwc")
		q.Add("_pragma", "busy_timeout(5000)")
		q.Add("_pragma", "foreign_keys(1)")
		q.Add("_pragma", "journal_mode(WAL)")
		u.RawQuery = q.Encode()
		dsn = u.String()
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("index store: open db: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &IndexStore{db: db}
	if err := store.initSchema(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("index store: init schema: %w", err)
	}
	return store, nil
}

// NewIndexStoreWithDB wraps an existing SQLite connection.
func NewIndexStoreWithDB(ctx context.Context, db *sql.DB) (*IndexStore, error) {
	store := &IndexStore{db: db}
	if err := store.initSchema(ctx); err != nil {
		return nil, fmt.Errorf("index store: init schema: %w", err)
	}
	return store, nil
}

func (s *IndexStore) initSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS registry_catalog (
			registry_id TEXT PRIMARY KEY,
			sequence INTEGER NOT NULL,
			issued_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			raw_envelope TEXT NOT NULL,
			cached_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS registry_plugins (
			registry_id TEXT NOT NULL,
			plugin_id TEXT NOT NULL,
			version TEXT NOT NULL,
			display_name TEXT NOT NULL,
			description TEXT,
			category TEXT,
			targets_json TEXT NOT NULL,
			cached_at TEXT NOT NULL,
			PRIMARY KEY (registry_id, plugin_id, version)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_registry_plugins_lookup
			ON registry_plugins(registry_id, plugin_id);`,
	}
	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the underlying database handle.
func (s *IndexStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveEnvelope persists a validated SignatureEnvelope and indexes all its plugins within a transaction.
func (s *IndexStore) SaveEnvelope(ctx context.Context, registryID string, envelope domain.SignatureEnvelope) error {
	if strings.TrimSpace(registryID) == "" {
		return fmt.Errorf("index store: registryID must not be empty")
	}

	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("index store: marshal envelope: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("index store: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	// Save or replace catalog entry
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO registry_catalog(registry_id, sequence, issued_at, expires_at, raw_envelope, cached_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(registry_id) DO UPDATE SET
			sequence = excluded.sequence,
			issued_at = excluded.issued_at,
			expires_at = excluded.expires_at,
			raw_envelope = excluded.raw_envelope,
			cached_at = excluded.cached_at`,
		registryID,
		envelope.Payload.Sequence,
		envelope.Payload.IssuedAt.Format(time.RFC3339Nano),
		envelope.Payload.ExpiresAt.Format(time.RFC3339Nano),
		string(envelopeJSON),
		nowStr,
	)
	if err != nil {
		return fmt.Errorf("index store: insert catalog: %w", err)
	}

	// Remove old plugins for this registry to replace with new snapshot
	if _, err := tx.ExecContext(ctx, `DELETE FROM registry_plugins WHERE registry_id = ?`, registryID); err != nil {
		return fmt.Errorf("index store: clear old plugins: %w", err)
	}

	// Insert plugin entries
	insertStmt, err := tx.PrepareContext(ctx, `INSERT INTO registry_plugins(
		registry_id, plugin_id, version, display_name, description, category, targets_json, cached_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("index store: prepare insert plugin: %w", err)
	}
	defer func() { _ = insertStmt.Close() }()

	for _, p := range envelope.Payload.Plugins {
		targetsJSON, err := json.Marshal(p.Targets)
		if err != nil {
			return fmt.Errorf("index store: marshal targets for %s: %w", p.PluginID, err)
		}
		_, err = insertStmt.ExecContext(
			ctx,
			registryID,
			p.PluginID,
			p.Version,
			p.DisplayName,
			p.Description,
			p.Category,
			string(targetsJSON),
			nowStr,
		)
		if err != nil {
			return fmt.Errorf("index store: insert plugin %s: %w", p.PluginID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("index store: commit tx: %w", err)
	}
	return nil
}

// GetLatestSequence returns the latest recorded sequence number for anti-rollback checking.
// Returns 0 if no record exists yet.
func (s *IndexStore) GetLatestSequence(ctx context.Context, registryID string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT sequence FROM registry_catalog WHERE registry_id = ?`,
		registryID,
	).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("index store: query sequence: %w", err)
	}
	return seq, nil
}

// GetEnvelope retrieves the stored SignatureEnvelope for a registry ID.
func (s *IndexStore) GetEnvelope(ctx context.Context, registryID string) (*domain.SignatureEnvelope, error) {
	var rawEnvelope string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT raw_envelope FROM registry_catalog WHERE registry_id = ?`,
		registryID,
	).Scan(&rawEnvelope)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrInvalidIndex
	}
	if err != nil {
		return nil, fmt.Errorf("index store: query envelope: %w", err)
	}

	var envelope domain.SignatureEnvelope
	if err := json.Unmarshal([]byte(rawEnvelope), &envelope); err != nil {
		return nil, fmt.Errorf("index store: unmarshal envelope: %w", err)
	}
	return &envelope, nil
}

// ListPlugins returns all indexed plugins for a given registry ID.
func (s *IndexStore) ListPlugins(ctx context.Context, registryID string) ([]domain.RegistryPluginEntry, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT plugin_id, version, display_name, description, category, targets_json
		FROM registry_plugins
		WHERE registry_id = ?
		ORDER BY plugin_id, version`,
		registryID,
	)
	if err != nil {
		return nil, fmt.Errorf("index store: query plugins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	entries := make([]domain.RegistryPluginEntry, 0)
	for rows.Next() {
		var (
			p           domain.RegistryPluginEntry
			targetsJSON string
		)
		if err := rows.Scan(&p.PluginID, &p.Version, &p.DisplayName, &p.Description, &p.Category, &targetsJSON); err != nil {
			return nil, fmt.Errorf("index store: scan plugin: %w", err)
		}
		if targetsJSON != "" {
			if err := json.Unmarshal([]byte(targetsJSON), &p.Targets); err != nil {
				return nil, fmt.Errorf("index store: unmarshal targets for %s: %w", p.PluginID, err)
			}
		}
		entries = append(entries, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("index store: rows err: %w", err)
	}
	return entries, nil
}

// GetPlugin returns the newest version of a plugin from the registry.
func (s *IndexStore) GetPlugin(ctx context.Context, registryID string, pluginID string) (*domain.RegistryPluginEntry, error) {
	plugins, err := s.ListPlugins(ctx, registryID)
	if err != nil {
		return nil, err
	}
	var best *domain.RegistryPluginEntry
	for i := range plugins {
		p := &plugins[i]
		if p.PluginID == pluginID {
			if best == nil || p.Version > best.Version {
				best = p
			}
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrPluginNotFound, pluginID)
	}
	return best, nil
}

// GetPluginVersion returns the specific version of a plugin.
func (s *IndexStore) GetPluginVersion(ctx context.Context, registryID string, pluginID string, version string) (*domain.RegistryPluginEntry, error) {
	var (
		p           domain.RegistryPluginEntry
		targetsJSON string
	)
	err := s.db.QueryRowContext(
		ctx,
		`SELECT plugin_id, version, display_name, description, category, targets_json
		FROM registry_plugins
		WHERE registry_id = ? AND plugin_id = ? AND version = ?`,
		registryID, pluginID, version,
	).Scan(&p.PluginID, &p.Version, &p.DisplayName, &p.Description, &p.Category, &targetsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s@%s", domain.ErrPluginNotFound, pluginID, version)
	}
	if err != nil {
		return nil, fmt.Errorf("index store: query plugin version: %w", err)
	}
	if targetsJSON != "" {
		if err := json.Unmarshal([]byte(targetsJSON), &p.Targets); err != nil {
			return nil, fmt.Errorf("index store: unmarshal targets: %w", err)
		}
	}
	return &p, nil
}
