package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
)

const delegationPluginID = "io.mender.provider.agent-delegation"

// GetToolFacade is a metadata-only lookup. A missing row is an explicitly
// unbound resource; reads never create Providers or run Agent processes.
func (s *Store) GetToolFacade(ctx context.Context, id agentdomain.ID) (string, error) {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return "", err
	}
	var providerID string
	err := s.db.QueryRowContext(ctx,
		"SELECT provider_id FROM agent_tool_facades WHERE agent_id=?", string(id)).Scan(&providerID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get Agent tool facade: %w", err)
	}
	return providerID, nil
}

// ClaimToolFacade pins one actual Agent Delegation Provider to one Agent.
// The same association can be retried, but changing it is never implicit.
func (s *Store) ClaimToolFacade(ctx context.Context, id agentdomain.ID, providerID string) error {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return err
	}
	if providerID == "" {
		return agentdomain.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var current string
	err = tx.QueryRowContext(ctx, "SELECT provider_id FROM agent_tool_facades WHERE agent_id=?", string(id)).Scan(&current)
	if err == nil {
		if current != providerID {
			return agentdomain.ErrConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var pluginID, configJSON string
	err = tx.QueryRowContext(ctx,
		"SELECT plugin_id,config_json FROM provider_configs WHERE provider_id=? AND plugin_installation_id IS NULL",
		providerID).Scan(&pluginID, &configJSON)
	if err != nil {
		return fmt.Errorf("%w: Agent facade Provider missing configuration: %v", agentdomain.ErrConflict, err)
	}
	var config struct {
		AgentID string `json:"agentId"`
	}
	if pluginID != delegationPluginID || json.Unmarshal([]byte(configJSON), &config) != nil || config.AgentID != string(id) {
		return fmt.Errorf("%w: Agent facade Provider targets a different resource", agentdomain.ErrConflict)
	}
	if _, err = tx.ExecContext(ctx,
		"INSERT INTO agent_tool_facades(agent_id,provider_id,suite_id,created_at) VALUES(?,?,?,?)",
		string(id), providerID, "agent.delegation", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("%w: claim Agent facade: %v", agentdomain.ErrConflict, err)
	}
	return tx.Commit()
}

// IsToolFacadeProvider prevents normal Provider CRUD from detaching a suite
// that belongs to an Agent. The Agent owner must perform the retirement.
func (s *Store) IsToolFacadeProvider(ctx context.Context, providerID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		"SELECT 1 FROM agent_tool_facades WHERE provider_id=?", providerID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ReleaseToolFacade is a guarded owner-driven detach used only while retiring
// an unexposed, unused Provider. Provider config deletion is SQL-protected
// until this association is removed.
func (s *Store) ReleaseToolFacade(ctx context.Context, id agentdomain.ID, providerID string) error {
	if _, err := agentdomain.ParseID(string(id)); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM agent_tool_facades WHERE agent_id=? AND provider_id=?", string(id), providerID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return agentdomain.ErrConflict
	}
	return nil
}
