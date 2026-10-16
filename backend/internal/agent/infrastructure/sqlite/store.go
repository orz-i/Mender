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

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
	_ "modernc.org/sqlite"
)

// Store is the authoritative persistent Agent/Task/Grant inventory.
// It intentionally owns no executable and never resolves credentials.
type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("agent sqlite: missing path")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	slashed := filepath.ToSlash(absolute)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	u := url.URL{Scheme: "file", Path: slashed}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err == nil {
		err = migrate(ctx, db)
	}
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open agent sqlite: %w", err)
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) CreateAgent(ctx context.Context, a agentdomain.Agent) error {
	v := a.Spec()
	_, err := s.db.ExecContext(ctx, `INSERT INTO agents(`+agentColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		string(v.ID), v.Name, string(v.Backend), string(v.PluginID), string(v.ExtensionID),
		v.WorkspaceRoot, v.Isolation, v.OwnerKind, v.OwnerRef, v.CreatedAt.Format(time.RFC3339Nano), v.DefaultModel, v.DefaultReasoningEffort)
	if err != nil {
		return fmt.Errorf("create Agent: %w", err)
	}
	return nil
}
func scanAgent(scanner interface{ Scan(...any) error }) (agentdomain.Agent, error) {
	var spec agentdomain.Spec
	var id, backend, pluginID, extensionID, created string
	err := scanner.Scan(&id, &spec.Name, &backend, &pluginID, &extensionID,
		&spec.WorkspaceRoot, &spec.Isolation, &spec.OwnerKind, &spec.OwnerRef, &created, &spec.DefaultModel, &spec.DefaultReasoningEffort)
	if err != nil {
		return agentdomain.Agent{}, err
	}
	spec.ID = agentdomain.ID(id)
	spec.Backend = agentdomain.Backend(backend)
	spec.PluginID = platformplugin.ID(pluginID)
	spec.ExtensionID = platformplugin.ExtensionID(extensionID)
	if spec.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return agentdomain.Agent{}, err
	}
	return agentdomain.NewAgent(spec)
}

const agentColumns = `agent_id,name,backend,plugin_id,extension_id,workspace_root,isolation,owner_kind,owner_ref,created_at,default_model,default_reasoning_effort`

func (s *Store) GetAgent(ctx context.Context, id agentdomain.ID) (agentdomain.Agent, error) {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return agentdomain.Agent{}, err
	}
	a, err := scanAgent(s.db.QueryRowContext(ctx, `SELECT `+agentColumns+` FROM agents WHERE agent_id=?`, string(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return agentdomain.Agent{}, agentdomain.ErrNotFound
	}
	return a, err
}
func (s *Store) ListAgents(ctx context.Context) ([]agentdomain.Agent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+agentColumns+` FROM agents ORDER BY name,agent_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []agentdomain.Agent{}
	for rows.Next() {
		a, e := scanAgent(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
func (s *Store) DeleteAgent(ctx context.Context, id agentdomain.ID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE agent_id=?`, string(id))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "constraint failed") ||
			strings.Contains(err.Error(), "Agent is referenced by a Provider") {
			return fmt.Errorf("%w: Agent still referenced by task or Provider", agentdomain.ErrConflict)
		}
		return fmt.Errorf("delete Agent: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return agentdomain.ErrNotFound
	}
	return nil
}

const taskColumns = `task_id,agent_id,status,origin,principal_kind,principal_ref,scope_kind,scope_ref,generation,result_text,diagnostic,cancel_requested,created_at,updated_at,model,workspace_key,workspace_root,reasoning_effort`

func scanTask(scanner interface{ Scan(...any) error }) (agentdomain.Task, error) {
	var t agentdomain.Task
	var id, agentID, state, created, updated string
	var cancel int
	if err := scanner.Scan(&id, &agentID, &state, &t.Origin, &t.PrincipalKind, &t.PrincipalRef,
		&t.ScopeKind, &t.ScopeRef, &t.Generation, &t.Result, &t.Diagnostic, &cancel, &created, &updated,
		&t.Model, &t.WorkspaceKey, &t.WorkspaceRoot, &t.ReasoningEffort); err != nil {
		return t, err
	}
	t.ID = agentdomain.TaskID(id)
	t.AgentID = agentdomain.ID(agentID)
	t.Status = agentdomain.TaskStatus(state)
	t.CancelRequested = cancel != 0
	var err error
	if t.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return t, err
	}
	if t.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return t, err
	}
	return t, t.Validate()
}
func (s *Store) CreateTask(ctx context.Context, t agentdomain.Task) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if t.Status != agentdomain.TaskQueued || t.CancelRequested {
		return fmt.Errorf("%w: new Task must be queued", agentdomain.ErrInvalid)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_tasks(`+taskColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		string(t.ID), string(t.AgentID), string(t.Status), t.Origin, t.PrincipalKind, t.PrincipalRef,
		t.ScopeKind, t.ScopeRef, t.Generation, t.Result, t.Diagnostic, 0,
		t.CreatedAt.UTC().Format(time.RFC3339Nano), t.UpdatedAt.UTC().Format(time.RFC3339Nano),
		t.Model, t.WorkspaceKey, t.WorkspaceRoot, t.ReasoningEffort)
	return err
}

// CreateAdmittedTask atomically fences Profile grant revocation and per-Agent
// concurrency in the same INSERT decision. Caller has already passed Policy.
func (s *Store) CreateAdmittedTask(ctx context.Context, t agentdomain.Task, maxActive int, profileID string) error {
	if err := t.Validate(); err != nil {
		return err
	}
	if t.Status != agentdomain.TaskQueued || maxActive < 1 || maxActive > 32 {
		return agentdomain.ErrInvalid
	}
	// These limits are an independent, cross-Runtime task budget. Each
	// condition is checked in the INSERT statement under SQLite's writer
	// serialization, never via a separate racy count-then-insert.
	const globalTaskLimit = 8
	const principalTaskLimit = 2
	result, err := s.db.ExecContext(ctx, `INSERT INTO agent_tasks(`+taskColumns+`)
	SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
	WHERE (SELECT COUNT(*) FROM agent_tasks WHERE agent_id=? AND status IN ('queued','running')) < ?
	  AND (SELECT COUNT(*) FROM agent_tasks WHERE status IN ('queued','running')) < ?
	  AND (SELECT COUNT(*) FROM agent_tasks WHERE principal_kind=? AND principal_ref=?
	       AND status IN ('queued','running')) < ?
	  AND (?='' OR EXISTS(SELECT 1 FROM agent_grants WHERE agent_id=? AND profile_id=?))
	  AND (?='' OR EXISTS(SELECT 1 FROM agent_workspaces WHERE agent_id=? AND workspace_key=? AND workspace_root=?))
	  AND (?='' OR ?='' OR EXISTS(SELECT 1 FROM agent_workspace_grants WHERE agent_id=? AND workspace_key=? AND profile_id=?))`,
		string(t.ID), string(t.AgentID), string(t.Status), t.Origin, t.PrincipalKind, t.PrincipalRef,
		t.ScopeKind, t.ScopeRef, t.Generation, t.Result, t.Diagnostic, 0,
		t.CreatedAt.UTC().Format(time.RFC3339Nano), t.UpdatedAt.UTC().Format(time.RFC3339Nano),
		t.Model, t.WorkspaceKey, t.WorkspaceRoot, t.ReasoningEffort,
		string(t.AgentID), maxActive, globalTaskLimit,
		t.PrincipalKind, t.PrincipalRef, principalTaskLimit,
		profileID, string(t.AgentID), profileID,
		t.WorkspaceKey, string(t.AgentID), t.WorkspaceKey, t.WorkspaceRoot,
		profileID, t.WorkspaceKey, string(t.AgentID), t.WorkspaceKey, profileID)
	if err != nil {
		return fmt.Errorf("reserve Agent task: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrConflict
	}
	return nil
}
func (s *Store) GetTask(ctx context.Context, id agentdomain.TaskID) (agentdomain.Task, error) {
	if _, err := agentdomain.ParseTaskID(string(id)); err != nil {
		return agentdomain.Task{}, err
	}
	t, err := scanTask(s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM agent_tasks WHERE task_id=?`, string(id)))
	if errors.Is(err, sql.ErrNoRows) {
		return agentdomain.Task{}, agentdomain.ErrNotFound
	}
	return t, err
}
func (s *Store) ListTasks(ctx context.Context, agentID agentdomain.ID) ([]agentdomain.Task, error) {
	if _, err := agentdomain.ParseID(string(agentID)); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM agent_tasks WHERE agent_id=? ORDER BY created_at DESC LIMIT 100`, string(agentID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tasks := []agentdomain.Task{}
	for rows.Next() {
		t, e := scanTask(rows)
		if e != nil {
			return nil, e
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// Transition is fenced on the TaskID, generation, old status, and cancellation
// state in a single SQLite update. Never overwrite a completed task.
func (s *Store) Transition(ctx context.Context, id agentdomain.TaskID, generation uint64, from, to agentdomain.TaskStatus, result, diagnostic string) error {
	if err := agentdomain.NextStatus(from, to); err != nil {
		return err
	}
	if len(result) > agentdomain.MaxResultBytes || len(diagnostic) > 1024 {
		return agentdomain.ErrInvalid
	}
	res, err := s.db.ExecContext(ctx, `UPDATE agent_tasks SET status=?,result_text=?,diagnostic=?,updated_at=?
		WHERE task_id=? AND generation=? AND status=? AND (cancel_requested=0 OR ?!='running')`,
		string(to), result, diagnostic, time.Now().UTC().Format(time.RFC3339Nano), string(id), generation, string(from), string(to))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return agentdomain.ErrConflict
	}
	return nil
}

// ReconcileExpired fences tasks from prior Runtime runs only after the full
// hard task timeout plus an operator-reviewed recovery margin has passed.
// The outcome remains unknown, never completed/cancelled. Fencing generation
// prevents a late old worker from overwriting the recovered record; no task
// prompt is replayed and no process is started by reconciliation.
func (s *Store) ReconcileExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, agentdomain.ErrInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE agent_tasks SET
		status='unknown',generation=generation+1,result_text='',
		diagnostic='Agent outcome unknown after execution deadline',
		updated_at=?
		WHERE status IN ('queued','running') AND updated_at < ?`,
		time.Now().UTC().Format(time.RFC3339Nano),
		cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("reconcile stale Agent tasks: %w", err)
	}
	return result.RowsAffected()
}

func (s *Store) RequestCancel(ctx context.Context, id agentdomain.TaskID, generation uint64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE agent_tasks SET cancel_requested=1,updated_at=?
		WHERE task_id=? AND generation=? AND status IN ('queued','running')`,
		time.Now().UTC().Format(time.RFC3339Nano), string(id), generation)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return agentdomain.ErrConflict
	}
	return nil
}
func (s *Store) GrantProfile(ctx context.Context, agentID agentdomain.ID, profileID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_grants VALUES(?,?,?) ON CONFLICT DO NOTHING`, string(agentID), profileID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) RevokeProfile(ctx context.Context, agentID agentdomain.ID, profileID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_grants WHERE agent_id=? AND profile_id=?`, string(agentID), profileID)
	return err
}
func (s *Store) HasProfileGrant(ctx context.Context, agentID agentdomain.ID, profileID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_grants WHERE agent_id=? AND profile_id=?)`, string(agentID), profileID).Scan(&exists)
	return exists, err
}
func (s *Store) ListProfileGrants(ctx context.Context, agentID agentdomain.ID) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT profile_id FROM agent_grants WHERE agent_id=? ORDER BY profile_id`, string(agentID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		items = append(items, id)
	}
	return items, rows.Err()
}
