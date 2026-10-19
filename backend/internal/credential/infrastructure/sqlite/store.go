package sqlite

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

	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open credential sqlite: path must not be empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open credential sqlite: resolve path: %w", err)
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
		return nil, fmt.Errorf("open credential sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open credential sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open credential sqlite: migrate: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Create(
	ctx context.Context,
	credential credentialdomain.Credential,
) error {
	oauthMetadata, err := encodeOAuthMetadata(credential)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO credentials(
			reference,
			display_name,
			source,
			environment_variable,
			created_at,
			updated_at,
			oauth_metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		credential.Reference().String(),
		credential.DisplayName(),
		string(credential.Source()),
		credential.EnvironmentVariable(),
		credential.CreatedAt().Format(time.RFC3339Nano),
		credential.UpdatedAt().Format(time.RFC3339Nano),
		oauthMetadata,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(strings.ToLower(err.Error()), "primary key") {
			return credentialapplication.ErrReferenceConflict
		}
		return fmt.Errorf("create credential metadata: %w", err)
	}
	return nil
}

func (s *Store) Save(
	ctx context.Context,
	credential credentialdomain.Credential,
) error {
	oauthMetadata, err := encodeOAuthMetadata(credential)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE credentials
		SET display_name = ?,
			source = ?,
			environment_variable = ?,
			created_at = ?,
			updated_at = ?,
			oauth_metadata = ?
		WHERE reference = ?`,
		credential.DisplayName(),
		string(credential.Source()),
		credential.EnvironmentVariable(),
		credential.CreatedAt().Format(time.RFC3339Nano),
		credential.UpdatedAt().Format(time.RFC3339Nano),
		oauthMetadata,
		credential.Reference().String(),
	)
	if err != nil {
		return fmt.Errorf("save credential metadata: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("save credential metadata: rows affected: %w", err)
	}
	if count == 0 {
		return credentialapplication.ErrCredentialNotFound
	}
	return nil
}

func (s *Store) Get(
	ctx context.Context,
	ref credentialdomain.Reference,
) (credentialdomain.Credential, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT reference, display_name, source, environment_variable, created_at, updated_at, oauth_metadata
		FROM credentials
		WHERE reference = ?`,
		ref.String(),
	)
	credential, err := scanCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return credentialdomain.Credential{}, credentialapplication.ErrCredentialNotFound
	}
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("get credential metadata: %w", err)
	}
	return credential, nil
}

func (s *Store) List(
	ctx context.Context,
) ([]credentialdomain.Credential, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT reference, display_name, source, environment_variable, created_at, updated_at, oauth_metadata
		FROM credentials
		ORDER BY reference`,
	)
	if err != nil {
		return nil, fmt.Errorf("list credential metadata: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]credentialdomain.Credential, 0)
	for rows.Next() {
		item, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("list credential metadata: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list credential metadata: rows: %w", err)
	}
	return items, nil
}

func (s *Store) Delete(
	ctx context.Context,
	ref credentialdomain.Reference,
) error {
	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM credentials WHERE reference = ?",
		ref.String(),
	)
	if err != nil {
		return fmt.Errorf("delete credential metadata: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete credential metadata: rows affected: %w", err)
	}
	if count == 0 {
		return credentialapplication.ErrCredentialNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanCredential(scanner rowScanner) (credentialdomain.Credential, error) {
	var rawReference, displayName, rawSource, environmentVariable string
	var rawCreatedAt, rawUpdatedAt, rawOAuth string
	if err := scanner.Scan(
		&rawReference,
		&displayName,
		&rawSource,
		&environmentVariable,
		&rawCreatedAt,
		&rawUpdatedAt,
		&rawOAuth,
	); err != nil {
		return credentialdomain.Credential{}, err
	}
	ref, err := credentialdomain.ParseReference(rawReference)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("parse credential reference: %w", err)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, rawCreatedAt)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("parse credential created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, rawUpdatedAt)
	if err != nil {
		return credentialdomain.Credential{}, fmt.Errorf("parse credential updated_at: %w", err)
	}
	credential, err := credentialdomain.RestoreCredential(ref, displayName, credentialdomain.Source(rawSource), environmentVariable, createdAt, updatedAt)
	if err != nil {
		return credentialdomain.Credential{}, err
	}
	if credential.Source() == credentialdomain.SourceOAuth {
		var metadata credentialdomain.OAuthMetadata
		if err := json.Unmarshal([]byte(rawOAuth), &metadata); err != nil {
			return credentialdomain.Credential{}, fmt.Errorf("decode OAuth credential metadata: %w", err)
		}
		return credential.WithOAuthMetadata(metadata)
	}
	return credential, nil
}

func encodeOAuthMetadata(credential credentialdomain.Credential) (string, error) {
	metadata := credential.OAuthMetadata()
	if credential.Source() != credentialdomain.SourceOAuth {
		if metadata != nil {
			return "", fmt.Errorf("non-OAuth credential has OAuth metadata")
		}
		return "", nil
	}
	if metadata == nil {
		return "", fmt.Errorf("OAuth credential metadata is required")
	}
	blob, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode OAuth credential metadata: %w", err)
	}
	return string(blob), nil
}
