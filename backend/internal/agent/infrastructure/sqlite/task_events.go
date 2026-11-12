package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

func (s *Store) ensureTaskEventsTable(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS agent_task_events (
		event_id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		seq INTEGER NOT NULL,
		event_type TEXT NOT NULL,
		payload TEXT NOT NULL,
		created_at TEXT NOT NULL,
		UNIQUE(task_id, seq)
	);
	CREATE INDEX IF NOT EXISTS idx_agent_task_events_task_seq ON agent_task_events(task_id, seq);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) RecordTaskEvent(ctx context.Context, event agentdomain.TaskEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	if event.ID == "" {
		id, err := agentdomain.NewTaskEventID()
		if err != nil {
			return err
		}
		event.ID = id
	}
	if event.Seq <= 0 {
		var nextSeq sql.NullInt64
		err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM agent_task_events WHERE task_id=?`, string(event.TaskID)).Scan(&nextSeq)
		if err != nil {
			return fmt.Errorf("calculate event seq: %w", err)
		}
		event.Seq = nextSeq.Int64
	}
	if err := event.Validate(); err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("%w: invalid event payload: %v", agentdomain.ErrInvalid, err)
	}
	if event.Payload == nil {
		payloadBytes = []byte("{}")
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO agent_task_events (event_id, task_id, seq, event_type, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		string(event.ID),
		string(event.TaskID),
		event.Seq,
		string(event.Type),
		string(payloadBytes),
		formatChronologicalTime(event.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("record task event: %w", err)
	}
	return nil
}

func (s *Store) ListTaskEvents(ctx context.Context, taskID agentdomain.TaskID) ([]agentdomain.TaskEvent, error) {
	if _, err := agentdomain.ParseTaskID(string(taskID)); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, task_id, seq, event_type, payload, created_at
		FROM agent_task_events
		WHERE task_id=?
		ORDER BY seq ASC, created_at ASC`,
		string(taskID),
	)
	if err != nil {
		return nil, fmt.Errorf("list task events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := []agentdomain.TaskEvent{}
	for rows.Next() {
		var eventID, tID, eventType, payloadStr, createdAtStr string
		var seq int64
		if err := rows.Scan(&eventID, &tID, &seq, &eventType, &payloadStr, &createdAtStr); err != nil {
			return nil, fmt.Errorf("scan task event: %w", err)
		}
		payload := map[string]any{}
		if len(payloadStr) > 0 {
			_ = json.Unmarshal([]byte(payloadStr), &payload)
		}
		createdAt, err := time.Parse(time.RFC3339Nano, createdAtStr)
		if err != nil {
			createdAt, _ = time.Parse("2006-01-02T15:04:05.000000000Z07:00", createdAtStr)
		}
		events = append(events, agentdomain.TaskEvent{
			ID:        agentdomain.TaskEventID(eventID),
			TaskID:    agentdomain.TaskID(tID),
			Seq:       seq,
			Type:      agentdomain.TaskEventType(eventType),
			Payload:   payload,
			CreatedAt: createdAt,
		})
	}
	return events, rows.Err()
}
