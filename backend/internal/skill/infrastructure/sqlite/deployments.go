package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	skillapplication "github.com/orz-i/Mender/backend/internal/skill/application"
	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

const deploymentColumns = "deployment_id, installation_id, target, root, destination, digest, deployed_at"

// CreateDeployment persists one exact Skill deployment.
func (s *Store) CreateDeployment(
	ctx context.Context,
	deployment skilldomain.Deployment,
) error {
	if err := deployment.Validate(); err != nil {
		return fmt.Errorf("create skill deployment: %w", err)
	}
	_, err := s.db.ExecContext(
		ctx,
		"INSERT INTO skill_deployments("+deploymentColumns+") VALUES (?, ?, ?, ?, ?, ?, ?)",
		deployment.ID().String(),
		deployment.InstallationID().String(),
		string(deployment.Target()),
		deployment.Root(),
		deployment.Destination(),
		deployment.Digest(),
		deployment.DeployedAt().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return skillapplication.ErrDeploymentConflict
		}
		return fmt.Errorf("create skill deployment: %w", err)
	}
	return nil
}

// GetDeployment returns one exact deployment record.
func (s *Store) GetDeployment(
	ctx context.Context,
	id skilldomain.DeploymentID,
) (skilldomain.Deployment, error) {
	if _, err := skilldomain.ParseDeploymentID(id.String()); err != nil {
		return skilldomain.Deployment{}, err
	}
	row := s.db.QueryRowContext(
		ctx,
		"SELECT "+deploymentColumns+" FROM skill_deployments WHERE deployment_id = ?",
		id.String(),
	)
	item, err := scanDeployment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return skilldomain.Deployment{}, skillapplication.ErrDeploymentNotFound
	}
	if err != nil {
		return skilldomain.Deployment{}, fmt.Errorf("get skill deployment: %w", err)
	}
	return item, nil
}

// ListDeployments returns deployments in deterministic target/destination order.
func (s *Store) ListDeployments(
	ctx context.Context,
) ([]skilldomain.Deployment, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT "+deploymentColumns+" FROM skill_deployments ORDER BY target, destination, deployment_id",
	)
	if err != nil {
		return nil, fmt.Errorf("list skill deployments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]skilldomain.Deployment, 0)
	for rows.Next() {
		item, scanErr := scanDeployment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("list skill deployments: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list skill deployments: rows: %w", err)
	}
	return items, nil
}

// ListDeploymentsByInstallation returns exact deployment references to one
// installation.
func (s *Store) ListDeploymentsByInstallation(
	ctx context.Context,
	id skilldomain.InstallationID,
) ([]skilldomain.Deployment, error) {
	rows, err := s.db.QueryContext(
		ctx,
		"SELECT "+deploymentColumns+" FROM skill_deployments WHERE installation_id = ? ORDER BY target, destination, deployment_id",
		id.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list skill deployments by installation: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]skilldomain.Deployment, 0)
	for rows.Next() {
		item, scanErr := scanDeployment(rows)
		if scanErr != nil {
			return nil, fmt.Errorf(
				"list skill deployments by installation: %w",
				scanErr,
			)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"list skill deployments by installation: rows: %w",
			err,
		)
	}
	return items, nil
}

// DeleteDeployment removes one exact deployment metadata record.
func (s *Store) DeleteDeployment(
	ctx context.Context,
	id skilldomain.DeploymentID,
) error {
	result, err := s.db.ExecContext(
		ctx,
		"DELETE FROM skill_deployments WHERE deployment_id = ?",
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("delete skill deployment: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete skill deployment rows affected: %w", err)
	}
	if count == 0 {
		return skillapplication.ErrDeploymentNotFound
	}
	return nil
}

func scanDeployment(scanner rowScanner) (skilldomain.Deployment, error) {
	var (
		rawID             string
		rawInstallationID string
		rawTarget         string
		root              string
		destination       string
		digest            string
		rawDeployedAt     string
	)
	if err := scanner.Scan(
		&rawID,
		&rawInstallationID,
		&rawTarget,
		&root,
		&destination,
		&digest,
		&rawDeployedAt,
	); err != nil {
		return skilldomain.Deployment{}, err
	}
	id, err := skilldomain.ParseDeploymentID(rawID)
	if err != nil {
		return skilldomain.Deployment{}, err
	}
	installationID, err := skilldomain.ParseInstallationID(rawInstallationID)
	if err != nil {
		return skilldomain.Deployment{}, err
	}
	deployedAt, err := time.Parse(time.RFC3339Nano, rawDeployedAt)
	if err != nil {
		return skilldomain.Deployment{}, fmt.Errorf("parse deployed_at: %w", err)
	}
	item, err := skilldomain.NewDeployment(
		id,
		installationID,
		skilldomain.DeploymentTarget(rawTarget),
		strings.TrimSpace(root),
		strings.TrimSpace(destination),
		digest,
		deployedAt,
	)
	if err != nil {
		return skilldomain.Deployment{}, err
	}
	return item, nil
}
