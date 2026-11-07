// Package typedsupply is the future read-only Profile supplier for typed
// Tool sources. It accepts only Stage E migrated databases; it is NOT
// registered by the legacy bootstrap.Runtime or MCP server.
package typedsupply

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	_ "modernc.org/sqlite"
)

var ErrTypedSchemaRequired = errors.New("typed Profile supply requires the hard-cut SQLite schema")
var ErrInvalidSupply = errors.New("profile supply cannot be verified")

// Target is an exact requested execution scope, not an authorization grant.
// The native runtime must still perform Sandbox Grant, Policy, Workspace and
// durable lease checks at its own admission/commit boundaries.
type Target struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
	SandboxID   string `json:"sandboxId"`
}
type Tool struct {
	ToolID                string   `json:"toolId"`
	SourceID              string   `json:"sourceId"`
	SourceKey             string   `json:"sourceKey"`
	Name                  string   `json:"name"`
	PolicyConstraint      string   `json:"policyConstraint"`
	ConcurrencyConstraint string   `json:"concurrencyConstraint"`
	Title                 string   `json:"title"`
	Description           string   `json:"description"`
	Targets               []Target `json:"targets,omitempty"`
	Direct                bool     `json:"direct"`
}
type Source struct {
	SourceID   string `json:"sourceId"`
	SourceKind string `json:"sourceKind"`
	SourceName string `json:"sourceName"`
	Tools      []Tool `json:"tools"`
}
type Export struct {
	ToolID   string `json:"toolId"`
	Name     string `json:"name"`
	SourceID string `json:"sourceId"`
}
type Snapshot struct {
	ProfileID        string   `json:"profileId"`
	ProfileRevision  int64    `json:"profileRevision"`
	DeliveryRevision int64    `json:"deliveryRevision"`
	Mode             string   `json:"mode"`
	Sources          []Source `json:"sources"`
	Direct           []Export `json:"direct"`
}
type Reader struct{ db *sql.DB }

// Open opens an existing, already migrated SQLite database in query-only mode.
// It does not create a file, run migrations, activate Providers or Agents,
// connect Sandboxes, or mutate a Profile or its grants.
func Open(ctx context.Context, path string) (*Reader, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrTypedSchemaRequired
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, ErrTypedSchemaRequired
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}
	q := u.Query()
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	reader := &Reader{db: db}
	if err := reader.checkSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return reader, nil
}
func (r *Reader) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}
func (r *Reader) checkSchema(ctx context.Context) error {
	// Query on schema *versions* rather than "table exists": a legacy v2
	// catalog cannot be treated as an empty typed source inventory.
	for _, probe := range []struct {
		table   string
		version int64
	}{
		{"tool_hardcut_schema_migrations", 1},
		{"catalog_schema_migrations", 3},
		{"profile_schema_migrations", 8},
		{"mcp_delivery_schema_migrations", 2},
	} {
		var version int64
		// Table name is a fixed build-time literal, never a runtime identifier.
		if err := r.db.QueryRowContext(ctx, "SELECT MAX(version) FROM "+probe.table).Scan(&version); err != nil || version != probe.version {
			return fmt.Errorf("%w: %s version", ErrTypedSchemaRequired, probe.table)
		}
	}
	for _, probe := range []string{
		"SELECT source_kind,source_id,source_key FROM tools LIMIT 0",
		"SELECT workspace_id,sandbox_id FROM profile_tool_target_scopes LIMIT 0",
		"SELECT workspace_id,profile_id FROM workspace_profile_grants LIMIT 0",
		"SELECT sandbox_id,profile_id FROM sandbox_profile_grants LIMIT 0",
		"SELECT agent_id,profile_id FROM agent_grants LIMIT 0",
		"SELECT source_key FROM native_tool_contracts LIMIT 0",
		"SELECT profile_id,tool_id FROM mcp_delivery_direct_tools LIMIT 0",
	} {
		if _, err := r.db.ExecContext(ctx, probe); err != nil {
			return fmt.Errorf("%w: %w", ErrTypedSchemaRequired, err)
		}
	}
	return nil
}

// Project loads only the exact Profile's selected ToolIDs in a consistent
// read transaction. It emits no Credential references or raw Task/Job/Policy
// material. Its result is DISCOVERY ONLY: an Invocation must revalidate
// grants, target scope, Policy, approval, and lease before executing a Tool.
func (r *Reader) Project(ctx context.Context, profileID string) (Snapshot, error) {
	if r == nil || r.db == nil {
		return Snapshot{}, ErrTypedSchemaRequired
	}
	if _, err := profile.ParseProfileID(profileID); err != nil {
		return Snapshot{}, ErrInvalidSupply
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = tx.Rollback() }()
	s := Snapshot{ProfileID: profileID, Sources: []Source{}, Direct: []Export{}}
	err = tx.QueryRowContext(ctx, "SELECT revision FROM profiles WHERE id=?", profileID).Scan(&s.ProfileRevision)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: Profile missing", ErrInvalidSupply)
	}
	err = tx.QueryRowContext(ctx, `SELECT mode,revision FROM mcp_delivery_configs WHERE profile_id=?`, profileID).Scan(&s.Mode, &s.DeliveryRevision)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: MCP delivery missing", ErrInvalidSupply)
	}
	if s.Mode != "direct" && s.Mode != "dynamic" && s.Mode != "hybrid" {
		return Snapshot{}, ErrInvalidSupply
	}
	selected, err := readDirectSubset(ctx, tx, profileID)
	if err != nil {
		return Snapshot{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT b.tool_id,b.alias,b.title_override,b.description_override,b.policy_constraint,b.concurrency_constraint,
 t.source_kind,t.source_id,t.source_key,t.name,t.title,t.description
 FROM profile_tools b LEFT JOIN tools t ON t.id=b.tool_id
 WHERE b.profile_id=? ORDER BY b.tool_id LIMIT 4097`, profileID)
	if err != nil {
		return Snapshot{}, err
	}
	type item struct {
		id, alias, titleOverride, descriptionOverride, policy, concurrency string
		source                                                             catalog.SourceRef
		key, name, title, description                                      string
	}
	found := []item{}
	for rows.Next() {
		var x item
		var kind, owner sql.NullString
		var key, name, title, desc sql.NullString
		if err := rows.Scan(&x.id, &x.alias, &x.titleOverride, &x.descriptionOverride, &x.policy, &x.concurrency,
			&kind, &owner, &key, &name, &title, &desc); err != nil {
			_ = rows.Close()
			return Snapshot{}, err
		}
		if !kind.Valid || !owner.Valid || !key.Valid || !name.Valid || !title.Valid || !desc.Valid {
			_ = rows.Close()
			return Snapshot{}, fmt.Errorf("%w: dangling Profile ToolBinding", ErrInvalidSupply)
		}
		if _, err := catalog.ParseToolID(x.id); err != nil {
			_ = rows.Close()
			return Snapshot{}, ErrInvalidSupply
		}
		source, err := catalog.NewSourceRef(catalog.SourceKind(kind.String), owner.String)
		if err != nil {
			_ = rows.Close()
			return Snapshot{}, ErrInvalidSupply
		}
		x.source = source
		x.key = key.String
		x.name = name.String
		x.title = title.String
		x.description = desc.String
		found = append(found, x)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return Snapshot{}, err
	}
	if len(found) > 4096 {
		return Snapshot{}, fmt.Errorf("%w: Profile Tool inventory too large", ErrInvalidSupply)
	}
	groups := map[catalog.SourceRef]*Source{}
	directNames := map[string]string{}
	for _, x := range found {
		if x.policy != "inherit" && x.policy != "require_approval" && x.policy != "deny" {
			return Snapshot{}, fmt.Errorf("%w: unknown binding policy constraint", ErrInvalidSupply)
		}
		if x.concurrency != "inherit" && x.concurrency != "serial" {
			return Snapshot{}, ErrInvalidSupply
		}
		if x.policy == "deny" {
			continue
		} // cannot advertise a deliberately disabled Tool
		label, namespace, valid, err := sourceIdentity(ctx, tx, profileID, x.source)
		if err != nil {
			return Snapshot{}, err
		}
		if !valid {
			continue
		}
		targets := []Target{}
		if x.source.Kind() == catalog.SourceNative {
			if !reviewedNativeTool(x.id, x.source, x.key) {
				return Snapshot{}, ErrInvalidSupply
			}
			targets, err = allowedTargets(ctx, tx, profileID, x.id, x.source, x.key)
			if err != nil {
				return Snapshot{}, err
			}
			if len(targets) == 0 {
				continue
			}
		}
		effectiveName := x.name
		if x.alias != "" {
			effectiveName = x.alias
		}
		if effectiveName == "" {
			return Snapshot{}, ErrInvalidSupply
		}
		title := x.title
		if x.titleOverride != "" {
			title = x.titleOverride
		}
		description := x.description
		if x.descriptionOverride != "" {
			description = x.descriptionOverride
		}
		isDirect := s.Mode == "direct" || (s.Mode == "hybrid" && selected[x.id])
		display := Tool{ToolID: x.id, SourceID: x.source.String(), SourceKey: x.key,
			Name: effectiveName, PolicyConstraint: x.policy, ConcurrencyConstraint: x.concurrency,
			Title: title, Description: description, Targets: targets, Direct: isDirect}
		group, ok := groups[x.source]
		if !ok {
			group = &Source{SourceID: x.source.String(), SourceKind: string(x.source.Kind()),
				SourceName: label, Tools: []Tool{}}
			groups[x.source] = group
		}
		group.Tools = append(group.Tools, display)
		if isDirect {
			name := exportName(namespace, effectiveName, x.id)
			if previous, collision := directNames[name]; collision && previous != x.id {
				return Snapshot{}, fmt.Errorf("%w: duplicate direct export %s", ErrInvalidSupply, name)
			}
			directNames[name] = x.id
			s.Direct = append(s.Direct, Export{ToolID: x.id, SourceID: x.source.String(), Name: name})
		}
	}
	for _, group := range groups {
		slices.SortFunc(group.Tools, func(a, b Tool) int { return strings.Compare(a.ToolID, b.ToolID) })
		s.Sources = append(s.Sources, *group)
	}
	slices.SortFunc(s.Sources, func(a, b Source) int { return strings.Compare(a.SourceID, b.SourceID) })
	slices.SortFunc(s.Direct, func(a, b Export) int { return strings.Compare(a.Name, b.Name) })
	return s, nil
}
func readDirectSubset(ctx context.Context, tx *sql.Tx, profileID string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT tool_id FROM mcp_delivery_direct_tools WHERE profile_id=?`, profileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}
func sourceIdentity(ctx context.Context, tx *sql.Tx, profileID string, source catalog.SourceRef) (label, namespace string, ok bool, err error) {
	switch source.Kind() {
	case catalog.SourceProvider:
		err = tx.QueryRowContext(ctx, `SELECT p.display_name,p.namespace
   FROM providers p JOIN provider_configs c ON c.provider_id=p.id WHERE p.id=?`, source.ID()).Scan(&label, &namespace)
	case catalog.SourceAgent:
		err = tx.QueryRowContext(ctx, `SELECT a.name FROM agents a
   JOIN agent_grants g ON g.agent_id=a.agent_id AND g.profile_id=?
   WHERE a.agent_id=?`, profileID, source.ID()).Scan(&label)
		namespace = "agent-" + strings.TrimPrefix(source.ID(), "agent_")
	case catalog.SourceNative:
		namespace = source.ID()
		switch namespace {
		case "workspace":
			label = "Workspace"
		case "jobs":
			label = "Jobs"
		default:
			return "", "", false, ErrInvalidSupply
		}
		return label, namespace, true, nil
	default:
		return "", "", false, ErrInvalidSupply
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if strings.TrimSpace(label) == "" || strings.TrimSpace(namespace) == "" {
		return "", "", false, ErrInvalidSupply
	}
	return label, namespace, true, nil
}
func reviewedNativeTool(id string, source catalog.SourceRef, key string) bool {
	entries, err := curated.PlannedNativeTools()
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Source == source && entry.SourceKey == key && entry.ToolID.String() == id {
			return true
		}
	}
	return false
}
func allowedTargets(ctx context.Context, tx *sql.Tx, pid, toolID string, source catalog.SourceRef, key string) ([]Target, error) {
	capability := map[string]string{
		"workspace.read": "file.read", "workspace.search": "file.search",
		"workspace.edit": "file.edit", "workspace.shell": "process.shell",
		"jobs.submit": "job.submit", "jobs.status": "job.observe", "jobs.logs": "job.observe",
		"jobs.result": "job.observe", "jobs.cancel": "job.cancel",
	}[key]
	if capability == "" {
		return nil, ErrInvalidSupply
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.workspace_id,s.sandbox_id FROM profile_tool_target_scopes s
 JOIN sandboxes sb ON sb.id=s.sandbox_id
 JOIN sandbox_profile_grants g ON g.sandbox_id=s.sandbox_id
   AND g.profile_id=s.profile_id AND g.capability=?
 WHERE s.profile_id=? AND s.tool_id=?
 ORDER BY s.workspace_id,s.sandbox_id`, capability, pid, toolID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Target{}
	for rows.Next() {
		var workspaceID, sandboxID string
		if err := rows.Scan(&workspaceID, &sandboxID); err != nil {
			return nil, err
		}
		if source.ID() == "workspace" {
			if workspaceID == "" {
				return nil, ErrInvalidSupply
			}
			var exists int
			err := tx.QueryRowContext(ctx, `SELECT COUNT(*)
    FROM workspace_placements p JOIN workspace_profile_grants g
      ON g.workspace_id=p.workspace_id AND g.profile_id=?
    WHERE p.workspace_id=? AND p.sandbox_id=?`, pid, workspaceID, sandboxID).Scan(&exists)
			if err != nil {
				return nil, err
			}
			if exists != 1 {
				continue
			}
		} else if workspaceID != "" {
			return nil, ErrInvalidSupply
		}
		if _, err := profile.NewExecutionTarget(workspaceID, sandboxID); err != nil {
			return nil, ErrInvalidSupply
		}
		out = append(out, Target{WorkspaceID: workspaceID, SandboxID: sandboxID})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 256 {
		return nil, ErrInvalidSupply
	}
	return out, nil
}

// exportName preserves the current namespace__machineName sanitization
// rules while changing the namespace to the true SourceRef owner.
func exportName(namespace, machineName, toolID string) string {
	raw := namespace + "__" + machineName
	var builder strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '.' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	name := builder.String()
	if len(name) <= 128 {
		return name
	}
	return name[:115] + "_" + strings.TrimPrefix(toolID, "tool_")[:12]
}
