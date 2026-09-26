package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open MCP config sqlite: path must not be empty")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open MCP config sqlite: resolve path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open MCP config sqlite: %w", err)
	}

	store := &Store{db: db}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open MCP config sqlite: ping: %w", err)
	}
	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open MCP config sqlite: migrate: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Save(ctx context.Context, config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}
	argsJSON, err := json.Marshal(config.Args)
	if err != nil {
		return fmt.Errorf("save MCP provider config: encode args: %w", err)
	}
	headers := config.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return fmt.Errorf("save MCP provider config: encode headers: %w", err)
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO mcp_provider_configs(
			provider_id, transport, endpoint, executable, args_json, directory, headers_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_id) DO UPDATE SET
			transport = excluded.transport,
			endpoint = excluded.endpoint,
			executable = excluded.executable,
			args_json = excluded.args_json,
			directory = excluded.directory,
			headers_json = excluded.headers_json`,
		config.ProviderID.String(),
		string(config.Transport),
		config.Endpoint,
		config.Executable,
		string(argsJSON),
		config.Directory,
		string(headersJSON),
	)
	if err != nil {
		return fmt.Errorf("save MCP provider config: %w", err)
	}
	return nil
}

func (s *Store) Get(
	ctx context.Context,
	providerID catalogdomain.ProviderID,
) (Config, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT provider_id, transport, endpoint, executable, args_json, directory, headers_json
		FROM mcp_provider_configs
		WHERE provider_id = ?`,
		providerID.String(),
	)
	config, err := scanConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, ErrNotFound
	}
	if err != nil {
		return Config{}, fmt.Errorf("get MCP provider config: %w", err)
	}
	return config, nil
}

func (s *Store) List(ctx context.Context) ([]Config, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT provider_id, transport, endpoint, executable, args_json, directory, headers_json
		FROM mcp_provider_configs
		ORDER BY provider_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list MCP provider configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	configs := make([]Config, 0)
	for rows.Next() {
		config, scanErr := scanConfig(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list MCP provider configs: %w", scanErr)
		}
		configs = append(configs, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list MCP provider configs: rows: %w", err)
	}
	return configs, nil
}

func (s *Store) Delete(ctx context.Context, providerID catalogdomain.ProviderID) error {
	_, err := s.db.ExecContext(
		ctx,
		"DELETE FROM mcp_provider_configs WHERE provider_id = ?",
		providerID.String(),
	)
	if err != nil {
		return fmt.Errorf("delete MCP provider config: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanConfig(scanner rowScanner) (Config, error) {
	var rawProviderID, rawTransport, endpoint, executable, rawArgs, directory, rawHeaders string
	if err := scanner.Scan(
		&rawProviderID,
		&rawTransport,
		&endpoint,
		&executable,
		&rawArgs,
		&directory,
		&rawHeaders,
	); err != nil {
		return Config{}, err
	}

	providerID, err := catalogdomain.ParseProviderID(rawProviderID)
	if err != nil {
		return Config{}, fmt.Errorf("parse provider id: %w", err)
	}
	var args []string
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return Config{}, fmt.Errorf("parse args: %w", err)
	}
	headers := make(map[string]string)
	if err := json.Unmarshal([]byte(rawHeaders), &headers); err != nil {
		return Config{}, fmt.Errorf("parse headers: %w", err)
	}

	config := Config{
		ProviderID: providerID,
		Transport:  Transport(rawTransport),
		Endpoint:   endpoint,
		Headers:    headers,
		Executable: executable,
		Args:       args,
		Directory:  directory,
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}
