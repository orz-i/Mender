package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

// TaskDisposition reads only explicit owner review events. Missing rows are
// intentionally not inferred from other completed Tasks or the filesystem.
func (s *Store) TaskDisposition(ctx context.Context, id agentdomain.TaskID) (*agentdomain.UnknownTaskDisposition, error) {
	if _, err := agentdomain.ParseTaskID(string(id)); err != nil {
		return nil, err
	}
	var handling, occurred string
	err := s.db.QueryRowContext(ctx, `SELECT handling,occurred_at
		FROM agent_unknown_dispositions WHERE task_id=?
		ORDER BY event_id DESC LIMIT 1`, string(id)).Scan(&handling, &occurred)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	when, err := time.Parse(time.RFC3339Nano, occurred)
	if err != nil {
		return nil, fmt.Errorf("invalid Agent disposition timestamp: %w", err)
	}
	return &agentdomain.UnknownTaskDisposition{Handling: handling, RecordedAt: when}, nil
}

// RecordUnknownDisposition appends a reversible, immutable audit event for
// owner-reported handling. It never changes Task status, generation, result
// or execution evidence; the original unknown is always unknown.
func (s *Store) RecordUnknownDisposition(ctx context.Context, id agentdomain.TaskID, handling string) error {
	if _, err := agentdomain.ParseTaskID(string(id)); err != nil {
		return err
	}
	if handling != agentdomain.UnknownHandledElsewhere && handling != agentdomain.UnknownNeedsReview {
		return agentdomain.ErrInvalid
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO agent_unknown_dispositions(task_id,handling,occurred_at)
		SELECT ?,?,?
		WHERE EXISTS(SELECT 1 FROM agent_tasks WHERE task_id=? AND status='unknown')
		  AND COALESCE((SELECT handling FROM agent_unknown_dispositions
			  WHERE task_id=? ORDER BY event_id DESC LIMIT 1),'needs_review')!=?`,
		string(id), handling, time.Now().UTC().Format(time.RFC3339Nano),
		string(id), string(id), handling)
	if err != nil {
		return fmt.Errorf("record unknown AgentTask disposition: %w", err)
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
