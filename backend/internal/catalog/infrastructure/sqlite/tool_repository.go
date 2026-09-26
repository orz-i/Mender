package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

const toolSelectColumns = `
	id,
	provider_id,
	source_key,
	name,
	title,
	description,
	input_schema,
	output_schema,
	tags_json,
	created_at,
	updated_at
`

// ListProviderTools returns the current Tool snapshot for one Provider.
func (s *Store) ListProviderTools(
	ctx context.Context,
	providerID domain.ProviderID,
) ([]domain.Tool, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT "+toolSelectColumns+` FROM tools
		WHERE provider_id = ?
		ORDER BY source_key, id`,
		providerID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list provider tools: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	tools := make([]domain.Tool, 0)
	for rows.Next() {
		tool, scanErr := scanTool(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list provider tools: %w", scanErr)
		}
		tools = append(tools, tool)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list provider tools: rows: %w", err)
	}

	return tools, nil
}

// ReplaceProviderTools atomically replaces one Provider's Tool snapshot and FTS rows.
func (s *Store) ReplaceProviderTools(
	ctx context.Context,
	providerID domain.ProviderID,
	tools []domain.Tool,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace provider tools: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(
		ctx,
		"DELETE FROM tools_fts WHERE provider_id = ?",
		providerID.String(),
	); err != nil {
		return fmt.Errorf("replace provider tools: clear search index: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		"DELETE FROM tools WHERE provider_id = ?",
		providerID.String(),
	); err != nil {
		return fmt.Errorf("replace provider tools: clear snapshot: %w", err)
	}

	for _, tool := range tools {
		if tool.ProviderID() != providerID {
			return fmt.Errorf(
				"replace provider tools: tool %s belongs to provider %s",
				tool.ID(),
				tool.ProviderID(),
			)
		}

		if err := insertTool(ctx, tx, tool); err != nil {
			return fmt.Errorf("replace provider tools: %w", err)
		}
		if err := insertToolSearchRow(ctx, tx, tool); err != nil {
			return fmt.Errorf("replace provider tools: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace provider tools: commit: %w", err)
	}
	return nil
}

// GetTool loads one canonical Tool by stable Mender identity.
func (s *Store) GetTool(
	ctx context.Context,
	id domain.ToolID,
) (domain.Tool, error) {
	row := s.db.QueryRowContext(
		ctx,
		"SELECT "+toolSelectColumns+" FROM tools WHERE id = ?",
		id.String(),
	)

	tool, err := scanTool(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Tool{}, application.ErrToolNotFound
	}
	if err != nil {
		return domain.Tool{}, fmt.Errorf("get tool: %w", err)
	}
	return tool, nil
}

type sqlExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertTool(ctx context.Context, executor sqlExecutor, tool domain.Tool) error {
	tags, err := json.Marshal(tool.Tags())
	if err != nil {
		return fmt.Errorf("encode tags for tool %s: %w", tool.ID(), err)
	}

	var output []byte
	if schema := tool.OutputSchema(); schema != nil {
		output = schema.Bytes()
	}

	if _, err := executor.ExecContext(
		ctx,
		`INSERT INTO tools(
			id, provider_id, source_key, name, title, description,
			input_schema, output_schema, tags_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tool.ID().String(),
		tool.ProviderID().String(),
		tool.SourceKey(),
		tool.Name(),
		tool.Title(),
		tool.Description(),
		tool.InputSchema().Bytes(),
		output,
		string(tags),
		formatTime(tool.CreatedAt()),
		formatTime(tool.UpdatedAt()),
	); err != nil {
		return fmt.Errorf("insert tool %s: %w", tool.ID(), err)
	}

	return nil
}

func insertToolSearchRow(
	ctx context.Context,
	executor sqlExecutor,
	tool domain.Tool,
) error {
	tags := ""
	for i, tag := range tool.Tags() {
		if i > 0 {
			tags += " "
		}
		tags += tag
	}

	if _, err := executor.ExecContext(
		ctx,
		`INSERT INTO tools_fts(
			tool_id, provider_id, name, title, description, tags
		) VALUES (?, ?, ?, ?, ?, ?)`,
		tool.ID().String(),
		tool.ProviderID().String(),
		tool.Name(),
		tool.Title(),
		tool.Description(),
		tags,
	); err != nil {
		return fmt.Errorf("index tool %s: %w", tool.ID(), err)
	}
	return nil
}

func scanTool(scanner rowScanner) (domain.Tool, error) {
	var (
		rawID         string
		rawProviderID string
		sourceKey     string
		name          string
		title         string
		description   string
		inputBytes    []byte
		outputBytes   []byte
		rawTags       string
		rawCreated    string
		rawUpdated    string
	)
	if err := scanner.Scan(
		&rawID,
		&rawProviderID,
		&sourceKey,
		&name,
		&title,
		&description,
		&inputBytes,
		&outputBytes,
		&rawTags,
		&rawCreated,
		&rawUpdated,
	); err != nil {
		return domain.Tool{}, err
	}

	id, err := domain.ParseToolID(rawID)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("parse tool id: %w", err)
	}
	providerID, err := domain.ParseProviderID(rawProviderID)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("parse tool provider id: %w", err)
	}
	inputSchema, err := domain.NewSchema(inputBytes)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("parse tool input schema: %w", err)
	}

	var outputSchema *domain.Schema
	if len(outputBytes) != 0 {
		schema, schemaErr := domain.NewSchema(outputBytes)
		if schemaErr != nil {
			return domain.Tool{}, fmt.Errorf("parse tool output schema: %w", schemaErr)
		}
		outputSchema = &schema
	}

	var tags []string
	if err := json.Unmarshal([]byte(rawTags), &tags); err != nil {
		return domain.Tool{}, fmt.Errorf("parse tool tags: %w", err)
	}

	createdAt, err := parseTime(rawCreated)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("parse tool created_at: %w", err)
	}
	updatedAt, err := parseTime(rawUpdated)
	if err != nil {
		return domain.Tool{}, fmt.Errorf("parse tool updated_at: %w", err)
	}

	return domain.RestoreTool(
		id,
		providerID,
		domain.ToolSpec{
			SourceKey:    sourceKey,
			Name:         name,
			Title:        title,
			Description:  description,
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
			Tags:         tags,
		},
		createdAt,
		updatedAt,
	)
}
