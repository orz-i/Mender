package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// v14 stores per-task execution provenance without changing historical Task
// outcomes or making native SessionIDs accessible to Profile callers.
// Earlier unknown Tasks deliberately have no inferred AgentSession.
func migrateExecutionEvidenceV14(ctx context.Context, db *sql.DB) error {
	var version sql.NullInt64
	if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM agent_schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version.Valid && version.Int64 >= 14 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range []string{
		`CREATE TABLE agent_native_sessions (
			session_ref TEXT PRIMARY KEY,
			task_id TEXT NOT NULL UNIQUE REFERENCES agent_tasks(task_id) ON DELETE RESTRICT,
			agent_id TEXT NOT NULL REFERENCES agents(agent_id) ON DELETE RESTRICT,
			backend TEXT NOT NULL,
			native_session_id TEXT NOT NULL,
			workspace_root TEXT NOT NULL,
			isolation TEXT NOT NULL,
			principal_kind TEXT NOT NULL,
			principal_ref TEXT NOT NULL,
			scope_kind TEXT NOT NULL,
			scope_ref TEXT NOT NULL,
			load_supported INTEGER NOT NULL CHECK(load_supported IN (0,1)),
			resume_supported INTEGER NOT NULL CHECK(resume_supported IN (0,1)),
			storage_verified INTEGER NOT NULL DEFAULT 0 CHECK(storage_verified=0),
			created_at TEXT NOT NULL,
			CHECK(length(native_session_id)>0 AND length(native_session_id)<=512),
			CHECK(length(session_ref)=38 AND substr(session_ref,1,6)='asess_')
		)`,
		`CREATE TRIGGER agent_native_session_no_update
			BEFORE UPDATE ON agent_native_sessions
			BEGIN SELECT RAISE(ABORT,'native AgentSession immutable'); END`,
		`CREATE TRIGGER agent_native_session_no_delete
			BEFORE DELETE ON agent_native_sessions
			BEGIN SELECT RAISE(ABORT,'native AgentSession immutable'); END`,
		`CREATE TABLE agent_execution_attempts (
			task_id TEXT PRIMARY KEY REFERENCES agent_tasks(task_id) ON DELETE RESTRICT,
			generation INTEGER NOT NULL CHECK(generation>0),
			runtime_instance TEXT NOT NULL,
			phase TEXT NOT NULL CHECK(phase IN (
				'reserved','process_started','session_ready','prompt_sent',
				'progress','terminal_seen','leader_reaped')),
			process_pid INTEGER NOT NULL DEFAULT 0 CHECK(process_pid>=0),
			process_started_at TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			last_observed_at TEXT NOT NULL,
			last_worker_heartbeat_at TEXT NOT NULL DEFAULT '',
			last_progress_at TEXT NOT NULL DEFAULT '',
			leader_reaped_at TEXT NOT NULL DEFAULT '',
			session_ref TEXT REFERENCES agent_native_sessions(session_ref) ON DELETE RESTRICT,
			event_count INTEGER NOT NULL DEFAULT 0 CHECK(event_count>=0 AND event_count<=32)
		)`,
		`CREATE TABLE agent_execution_processes (
			task_id TEXT NOT NULL REFERENCES agent_execution_attempts(task_id) ON DELETE RESTRICT,
			process_generation INTEGER NOT NULL CHECK(process_generation BETWEEN 1 AND 2),
			runtime_instance TEXT NOT NULL,
			process_pid INTEGER NOT NULL CHECK(process_pid>0),
			started_at TEXT NOT NULL,
			leader_reaped_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY(task_id,process_generation)
		)`,
		`CREATE TABLE agent_execution_events (
			event_id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL REFERENCES agent_execution_attempts(task_id) ON DELETE RESTRICT,
			generation INTEGER NOT NULL,
			event_kind TEXT NOT NULL CHECK(event_kind IN (
				'attempt_reserved','process_started','session_ready',
				'prompt_sent','terminal_seen','leader_reaped')),
			occurred_at TEXT NOT NULL
		)`,
		`CREATE INDEX agent_execution_events_task ON agent_execution_events(task_id,event_id)`,
		`CREATE TRIGGER agent_execution_event_no_update
			BEFORE UPDATE ON agent_execution_events
			BEGIN SELECT RAISE(ABORT,'Agent evidence event immutable'); END`,
		`CREATE TRIGGER agent_execution_event_no_delete
			BEFORE DELETE ON agent_execution_events
			BEGIN SELECT RAISE(ABORT,'Agent evidence event immutable'); END`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("agent execution evidence v14: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO agent_schema_migrations(version,applied_at) VALUES(14,?)",
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("agent v14 migration record: %w", err)
	}
	return tx.Commit()
}
