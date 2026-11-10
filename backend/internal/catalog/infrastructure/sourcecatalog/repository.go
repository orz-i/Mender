package sourcecatalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	application "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	providersqlite "github.com/orz-i/Mender/backend/internal/catalog/infrastructure/sqlite"
	suite "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

// ErrRetirementRequiresReview prevents a source refresh from silently deleting
// a ToolID that may still be selected by a Profile or referenced by Policy.
var ErrRetirementRequiresReview = errors.New("tool source snapshot would retire an existing tool")
var ErrSourceIdentityConflict = errors.New("tool source identity conflict")
var ErrUnverifiedSource = errors.New("tool source ownership is unverified")

// OwnerVerifier is host-injected, read-only, and not derived from a Plugin or
// a source label. Stage E additionally needs commit-point SQL ownership guards
// for Agent Sources, so a concurrent retirement cannot race this check.
type OwnerVerifier interface {
	VerifySourceOwner(context.Context, catalog.SourceRef) error
}

// Repository consumes the proposed typed Catalog schema. It does not create,
// upgrade or modify the schema when opened. bootstrap.Runtime never constructs
// this adapter until Stage E's cross-context migration is in place.
type Repository struct {
	// Provider CRUD is the same canonical repository in v2/v3, not a second
	// implementation or a fabricated Provider parent for Native/Agent Tools.
	*providersqlite.ProviderRepository
	db    *sql.DB
	owner OwnerVerifier
}

func NewRepository(ctx context.Context, db *sql.DB, owner OwnerVerifier) (*Repository, error) {
	if db == nil || owner == nil {
		return nil, fmt.Errorf("source catalog: database and verified owner required")
	}
	for _, probe := range []string{
		"SELECT source_kind, source_id, source_key FROM tools LIMIT 0",
		"SELECT tool_id, source_kind, source_id FROM tools_fts LIMIT 0",
		"SELECT revision FROM catalog_tool_revision WHERE id=1",
	} {
		if _, err := db.ExecContext(ctx, probe); err != nil {
			return nil, fmt.Errorf("source catalog: typed schema unavailable: %w", err)
		}
	}
	return &Repository{db: db, owner: owner, ProviderRepository: providersqlite.NewProviderRepository(db)}, nil
}

// UpsertSourceSnapshot upserts stable canonical ToolIDs in one transaction.
// Metadata refreshes never reassign a source/key, implicitly revoke a ToolID,
// or grant a Profile. The source's owner and host-reviewed operation family
// must already be proven; this is not a source-management API.
func (r *Repository) UpsertSourceSnapshot(ctx context.Context, source catalog.SourceRef, tools []catalog.Tool) error {
	if r == nil || r.db == nil || r.owner == nil {
		return ErrUnverifiedSource
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnverifiedSource, err)
	}
	if err := r.owner.VerifySourceOwner(ctx, source); err != nil {
		return fmt.Errorf("%w: %w", ErrUnverifiedSource, err)
	}
	if err := validateSnapshot(source, tools); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("source catalog: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, "SELECT id, source_key, created_at, updated_at FROM tools WHERE source_kind=? AND source_id=?",
		string(source.Kind()), source.ID())
	if err != nil {
		return fmt.Errorf("source catalog: inspect previous snapshot: %w", err)
	}
	type previousTool struct {
		id               string
		created, updated time.Time
	}
	previous := map[string]previousTool{}
	for rows.Next() {
		var id, key, rawCreated, rawUpdated string
		if err = rows.Scan(&id, &key, &rawCreated, &rawUpdated); err != nil {
			_ = rows.Close()
			return err
		}
		created, err := time.Parse(time.RFC3339Nano, rawCreated)
		if err != nil {
			_ = rows.Close()
			return err
		}
		updated, err := time.Parse(time.RFC3339Nano, rawUpdated)
		if err != nil {
			_ = rows.Close()
			return err
		}
		previous[key] = previousTool{id: id, created: created, updated: updated}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	desired := make(map[string]catalog.ToolID, len(tools))
	for _, tool := range tools {
		desired[tool.SourceKey()] = tool.ID()
	}
	for key, old := range previous {
		if desiredID, ok := desired[key]; !ok {
			return fmt.Errorf("%w: source %s key %q id %s", ErrRetirementRequiresReview, source, key, old.id)
		} else if desiredID.String() != old.id {
			return fmt.Errorf("%w: source %s key %q changed %s to %s", ErrSourceIdentityConflict, source, key, old.id, desiredID)
		}
	}
	for _, tool := range tools {
		if old, exists := previous[tool.SourceKey()]; exists &&
			(!tool.CreatedAt().Equal(old.created) || tool.UpdatedAt().Before(old.updated)) {
			return fmt.Errorf("%w: Tool timestamps changed or regressed for %s", ErrSourceIdentityConflict, tool.ID())
		}
		if err := upsertTool(ctx, tx, tool); err != nil {
			return fmt.Errorf("source catalog: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM tools_fts WHERE tool_id=?", tool.ID().String()); err != nil {
			return fmt.Errorf("source catalog: remove old search row: %w", err)
		}
		if err := indexTool(ctx, tx, tool); err != nil {
			return fmt.Errorf("source catalog: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("source catalog: commit: %w", err)
	}
	return nil
}

func validateSnapshot(source catalog.SourceRef, tools []catalog.Tool) error {
	seenKeys := map[string]bool{}
	seenIDs := map[catalog.ToolID]bool{}
	reviewedNative := map[string]catalog.Tool{}
	if source.Kind() == catalog.SourceNative {
		entries, err := curated.PlannedNativeCatalogTools(time.Now().UTC())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.SourceRef() == source {
				reviewedNative[entry.SourceKey()] = entry
			}
		}
	}
	agentOperations := map[string]bool{}
	if source.Kind() == catalog.SourceAgent {
		def, ok := curated.Find(suite.AgentDelegation)
		if !ok {
			return ErrUnverifiedSource
		}
		for _, key := range def.ToolSourceKeys {
			agentOperations[key] = true
		}
	}
	for _, tool := range tools {
		if tool.SourceRef() != source || tool.ID() == "" || tool.SourceKey() == "" ||
			seenKeys[tool.SourceKey()] || seenIDs[tool.ID()] {
			return ErrSourceIdentityConflict
		}
		seenKeys[tool.SourceKey()], seenIDs[tool.ID()] = true, true
		if source.Kind() == catalog.SourceNative {
			reviewed, ok := reviewedNative[tool.SourceKey()]
			if !ok || !sameReviewedNativeContract(tool, reviewed) {
				return fmt.Errorf("%w: unreviewed native Tool contract for %s", ErrUnverifiedSource, tool.SourceKey())
			}
		}
		if source.Kind() == catalog.SourceAgent && !agentOperations[tool.SourceKey()] {
			return fmt.Errorf("%w: unknown Agent operation %s", ErrUnverifiedSource, tool.SourceKey())
		}
	}
	// A native source must have precisely the complete pinned operation set.
	// Omitting an operation must not retire previously published capabilities.
	if source.Kind() == catalog.SourceNative && len(tools) != len(reviewedNative) {
		return fmt.Errorf("%w: incomplete native Tool source", ErrUnverifiedSource)
	}
	return nil
}

func sameReviewedNativeContract(actual, reviewed catalog.Tool) bool {
	return actual.ID() == reviewed.ID() && actual.SourceRef() == reviewed.SourceRef() &&
		actual.SourceKey() == reviewed.SourceKey() && actual.Name() == reviewed.Name() &&
		actual.Title() == reviewed.Title() && actual.Description() == reviewed.Description() &&
		bytes.Equal(actual.InputSchema().Bytes(), reviewed.InputSchema().Bytes()) &&
		actual.OutputSchema() != nil && reviewed.OutputSchema() != nil &&
		bytes.Equal(actual.OutputSchema().Bytes(), reviewed.OutputSchema().Bytes()) &&
		slices.Equal(actual.Tags(), reviewed.Tags())
}

func upsertTool(ctx context.Context, tx *sql.Tx, tool catalog.Tool) error {
	tags, err := json.Marshal(tool.Tags())
	if err != nil {
		return err
	}
	var out []byte
	if output := tool.OutputSchema(); output != nil {
		out = output.Bytes()
	}
	source := tool.SourceRef()
	// Conflict by ID can refresh metadata only if source/key are unchanged;
	// a second source cannot adopt/reassign an existing ToolID.
	result, err := tx.ExecContext(ctx, `INSERT INTO tools(
			id,source_kind,source_id,source_key,name,title,description,input_schema,output_schema,tags_json,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,title=excluded.title,description=excluded.description,
			input_schema=excluded.input_schema,output_schema=excluded.output_schema,
			tags_json=excluded.tags_json,updated_at=excluded.updated_at
		WHERE tools.source_kind=excluded.source_kind
		  AND tools.source_id=excluded.source_id AND tools.source_key=excluded.source_key`,
		tool.ID().String(), string(source.Kind()), source.ID(), tool.SourceKey(),
		tool.Name(), tool.Title(), tool.Description(), tool.InputSchema().Bytes(), out,
		string(tags), tool.CreatedAt().UTC().Format(time.RFC3339Nano),
		tool.UpdatedAt().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrSourceIdentityConflict
	}
	return nil
}

func indexTool(ctx context.Context, tx *sql.Tx, tool catalog.Tool) error {
	source := tool.SourceRef()
	tags := strings.Join(tool.Tags(), " ")
	_, err := tx.ExecContext(ctx, `INSERT INTO tools_fts (
		tool_id,source_kind,source_id,name,title,description,tags
	) VALUES(?,?,?,?,?,?,?)`, tool.ID().String(), string(source.Kind()),
		source.ID(), tool.Name(), tool.Title(), tool.Description(), tags)
	return err
}

const sourceColumns = `id,source_kind,source_id,source_key,name,title,description,
	input_schema,output_schema,tags_json,created_at,updated_at`

const sourceJoinedColumns = `tools.id,tools.source_kind,tools.source_id,tools.source_key,
	tools.name,tools.title,tools.description,tools.input_schema,
	tools.output_schema,tools.tags_json,tools.created_at,tools.updated_at`

type rowScanner interface{ Scan(...any) error }

func scanSourceTool(row rowScanner) (catalog.Tool, error) {
	var id, kind, sourceID, key, name, title, description, tagsJSON, created, updated string
	var input, output []byte
	if err := row.Scan(&id, &kind, &sourceID, &key, &name, &title, &description,
		&input, &output, &tagsJSON, &created, &updated); err != nil {
		return catalog.Tool{}, err
	}
	toolID, err := catalog.ParseToolID(id)
	if err != nil {
		return catalog.Tool{}, err
	}
	source, err := catalog.NewSourceRef(catalog.SourceKind(kind), sourceID)
	if err != nil {
		return catalog.Tool{}, err
	}
	in, err := catalog.NewSchema(input)
	if err != nil {
		return catalog.Tool{}, err
	}
	var out *catalog.Schema
	if len(output) != 0 {
		schema, err := catalog.NewSchema(output)
		if err != nil {
			return catalog.Tool{}, err
		}
		out = &schema
	}
	var tags []string
	if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
		return catalog.Tool{}, err
	}
	start, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return catalog.Tool{}, err
	}
	end, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return catalog.Tool{}, err
	}
	return catalog.RestoreToolFromSource(toolID, source, catalog.ToolSpec{
		SourceKey: key, Name: name, Title: title, Description: description,
		InputSchema: in, OutputSchema: out, Tags: tags,
	}, start, end)
}

func (r *Repository) GetTool(ctx context.Context, id catalog.ToolID) (catalog.Tool, error) {
	if _, err := catalog.ParseToolID(id.String()); err != nil {
		return catalog.Tool{}, err
	}
	tool, err := scanSourceTool(r.db.QueryRowContext(ctx, "SELECT "+sourceColumns+" FROM tools WHERE id=?", id.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return catalog.Tool{}, application.ErrToolNotFound
	}
	return tool, err
}

func (r *Repository) ListSourceTools(ctx context.Context, source catalog.SourceRef) ([]catalog.Tool, error) {
	if err := source.Validate(); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+sourceColumns+
		" FROM tools WHERE source_kind=? AND source_id=? ORDER BY source_key,id", string(source.Kind()), source.ID())
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tools := []catalog.Tool{}
	for rows.Next() {
		item, err := scanSourceTool(rows)
		if err != nil {
			return nil, err
		}
		tools = append(tools, item)
	}
	return tools, rows.Err()
}

// SourceCount is a read-only admin inventory projection, not a grant/Source
// aggregate. A Profile-scoped adapter must still filter exact bound ToolIDs.
type SourceCount struct {
	Source catalog.SourceRef
	Count  int
}

func (r *Repository) ListSourceCounts(ctx context.Context) ([]SourceCount, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT source_kind,source_id,COUNT(*)
		FROM tools GROUP BY source_kind,source_id ORDER BY source_kind,source_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []SourceCount{}
	for rows.Next() {
		var kind, id string
		var n int
		if err := rows.Scan(&kind, &id, &n); err != nil {
			return nil, err
		}
		source, err := catalog.NewSourceRef(catalog.SourceKind(kind), id)
		if err != nil {
			return nil, err
		}
		out = append(out, SourceCount{Source: source, Count: n})
	}
	return out, rows.Err()
}

// SearchSource is for the host's Catalog management view. It is NOT an MCP
// Profile search; profile source discovery must additionally join current
// ToolBindings before returning counts, names or schema.
func (r *Repository) SearchSource(ctx context.Context, source catalog.SourceRef, query string, limit int) ([]catalog.Tool, error) {
	if err := source.Validate(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("source search: limit must be 1..100")
	}
	expr, err := normalizedFTSQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+sourceJoinedColumns+` FROM tools
		JOIN tools_fts ON tools_fts.tool_id=tools.id
		WHERE tools_fts MATCH ? AND tools_fts.source_kind=? AND tools_fts.source_id=?
		ORDER BY bm25(tools_fts),tools.source_key,tools.id LIMIT ?`,
		expr, string(source.Kind()), source.ID(), limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []catalog.Tool{}
	for rows.Next() {
		item, err := scanSourceTool(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func normalizedFTSQuery(query string) (string, error) {
	if len(query) > 256 || len(strings.Fields(query)) > 12 {
		return "", fmt.Errorf("source search: query exceeds bounded size")
	}
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "", fmt.Errorf("source search: query is required")
	}
	quoted := make([]string, 0, len(parts))
	for _, token := range parts {
		quoted = append(quoted, "\""+strings.ReplaceAll(token, "\"", "\"\"")+"\"")
	}
	return strings.Join(quoted, " AND "), nil
}

// RetireOwnedAgent atomically removes a no-longer-referenced Agent and its
// canonical ToolIDs. The caller must have passed Agent ownership admission.
// Database triggers enforce Profile/ToolID/Policy integrity at the commit
// point; Task history, unknown outcomes and explicit Agent grants block
// retirement. Audit history is intentionally left immutable.
func (r *Repository) RetireOwnedAgent(ctx context.Context, source catalog.SourceRef) error {
	if r == nil || r.db == nil || r.owner == nil || source.Kind() != catalog.SourceAgent {
		return ErrUnverifiedSource
	}
	if err := r.owner.VerifySourceOwner(ctx, source); err != nil {
		return fmt.Errorf("retire owned Agent: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var present int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agents WHERE agent_id=?)`, source.ID()).Scan(&present); err != nil || present != 1 {
		return fmt.Errorf("retire owned Agent: source no longer exists or is unverified: %w", ErrUnverifiedSource)
	}
	for _, query := range []string{
		`SELECT EXISTS(SELECT 1 FROM agent_tasks WHERE agent_id=?)`,
		`SELECT EXISTS(SELECT 1 FROM agent_grants WHERE agent_id=?)`,
		`SELECT EXISTS(SELECT 1 FROM policy_rules r,json_each(CASE WHEN json_valid(r.match_json) THEN r.match_json ELSE '{}' END,'$.agentIds') v WHERE v.value=?)`,
	} {
		var referenced int
		if err = tx.QueryRowContext(ctx, query, source.ID()).Scan(&referenced); err != nil {
			return err
		}
		if referenced != 0 {
			return fmt.Errorf("retire owned Agent: resource still referenced: %w", ErrRetirementRequiresReview)
		}
	}
	for _, query := range []string{
		`DELETE FROM tools_fts WHERE tool_id IN (SELECT id FROM tools WHERE source_kind='agent' AND source_id=?)`,
		`DELETE FROM tools WHERE source_kind='agent' AND source_id=?`,
		`DELETE FROM agents WHERE agent_id=?`,
	} {
		if _, err = tx.ExecContext(ctx, query, source.ID()); err != nil {
			return fmt.Errorf("retire owned Agent: atomic retirement: %w", err)
		}
	}
	return tx.Commit()
}
