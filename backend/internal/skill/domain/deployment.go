package domain

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const maxDeploymentPathBytes = 4096

// DeploymentTarget identifies one supported Agent Skill host location.
type DeploymentTarget string

const (
	DeploymentAgentsProject DeploymentTarget = "agents-project"
	DeploymentGitHubProject DeploymentTarget = "github-project"
	DeploymentClaudeProject DeploymentTarget = "claude-project"
	DeploymentAgentsUser    DeploymentTarget = "agents-user"
	DeploymentCopilotUser   DeploymentTarget = "copilot-user"
)

// Validate checks one supported deployment target.
func (t DeploymentTarget) Validate() error {
	switch t {
	case DeploymentAgentsProject,
		DeploymentGitHubProject,
		DeploymentClaudeProject,
		DeploymentAgentsUser,
		DeploymentCopilotUser:
		return nil
	default:
		return fmt.Errorf("skill deployment target: unsupported value %q", t)
	}
}

// ProjectScoped reports whether the target requires a project root.
func (t DeploymentTarget) ProjectScoped() bool {
	switch t {
	case DeploymentAgentsProject, DeploymentGitHubProject, DeploymentClaudeProject:
		return true
	default:
		return false
	}
}

// Deployment records one exact materialized Skill installation.
type Deployment struct {
	id             DeploymentID
	installationID InstallationID
	target         DeploymentTarget
	root           string
	destination    string
	digest         string
	deployedAt     time.Time
}

// NewDeployment constructs a validated Skill deployment record.
func NewDeployment(
	id DeploymentID,
	installationID InstallationID,
	target DeploymentTarget,
	root string,
	destination string,
	digest string,
	deployedAt time.Time,
) (Deployment, error) {
	if _, err := ParseDeploymentID(id.String()); err != nil {
		return Deployment{}, err
	}
	if _, err := ParseInstallationID(installationID.String()); err != nil {
		return Deployment{}, err
	}
	if err := target.Validate(); err != nil {
		return Deployment{}, err
	}
	root = filepath.Clean(strings.TrimSpace(root))
	destination = filepath.Clean(strings.TrimSpace(destination))
	if root == "." || !filepath.IsAbs(root) {
		return Deployment{}, fmt.Errorf("skill deployment root: must be an absolute path")
	}
	if destination == "." || !filepath.IsAbs(destination) {
		return Deployment{}, fmt.Errorf("skill deployment destination: must be an absolute path")
	}
	if len(root) > maxDeploymentPathBytes || len(destination) > maxDeploymentPathBytes {
		return Deployment{}, fmt.Errorf(
			"skill deployment path: root and destination must be at most %d bytes",
			maxDeploymentPathBytes,
		)
	}
	relative, err := filepath.Rel(root, destination)
	if err != nil {
		return Deployment{}, fmt.Errorf("skill deployment destination: compare root: %w", err)
	}
	if relative == "." ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Deployment{}, fmt.Errorf("skill deployment destination: must remain below root")
	}
	if !digestPattern.MatchString(digest) {
		return Deployment{}, fmt.Errorf("skill deployment digest: must be lowercase sha256")
	}
	if deployedAt.IsZero() {
		return Deployment{}, fmt.Errorf("skill deployment deployed at: must not be zero")
	}
	return Deployment{
		id:             id,
		installationID: installationID,
		target:         target,
		root:           root,
		destination:    destination,
		digest:         digest,
		deployedAt:     deployedAt.UTC(),
	}, nil
}

// Validate checks persisted deployment invariants.
func (d Deployment) Validate() error {
	_, err := NewDeployment(
		d.id,
		d.installationID,
		d.target,
		d.root,
		d.destination,
		d.digest,
		d.deployedAt,
	)
	return err
}

func (d Deployment) ID() DeploymentID               { return d.id }
func (d Deployment) InstallationID() InstallationID { return d.installationID }
func (d Deployment) Target() DeploymentTarget       { return d.target }
func (d Deployment) Root() string                   { return d.root }
func (d Deployment) Destination() string            { return d.destination }
func (d Deployment) Digest() string                 { return d.digest }
func (d Deployment) DeployedAt() time.Time          { return d.deployedAt }

// DeploymentDrift classifies the current target content against the persisted
// exact deployment digest.
type DeploymentDrift string

const (
	DeploymentCurrent DeploymentDrift = "current"
	DeploymentDrifted DeploymentDrift = "drifted"
	DeploymentMissing DeploymentDrift = "missing"
)
