package domain

import (
	"fmt"
	"time"
)

// ApprovalMode governs *cooperative* ACP requests, never actual native OS
// operations or canonical Mender Tool invocations.
type ApprovalMode string

const (
	ApprovalManual        ApprovalMode = "manual"
	ApprovalWorkspaceRead ApprovalMode = "workspace_read"
	ApprovalTrusted       ApprovalMode = "trusted"
)
const MaxTrustedDuration = 24 * time.Hour

func (m ApprovalMode) Valid() bool {
	return m == ApprovalManual || m == ApprovalWorkspaceRead || m == ApprovalTrusted
}

type ApprovalPolicy struct {
	AgentID       ID           `json:"agentId"`
	WorkspaceKey  string       `json:"workspaceKey"`
	WorkspaceRoot string       `json:"-"`
	Mode          ApprovalMode `json:"mode"`
	ExpiresAt     time.Time    `json:"expiresAt,omitempty"`
	UpdatedAt     time.Time    `json:"updatedAt,omitempty"`
}

func (p ApprovalPolicy) Validate(now time.Time) error {
	if _, err := ParseID(string(p.AgentID)); err != nil {
		return err
	}
	if p.Mode != ApprovalWorkspaceRead && p.Mode != ApprovalTrusted {
		return fmt.Errorf("%w: invalid stored approval mode", ErrInvalid)
	}
	if p.WorkspaceKey != "" {
		if err := ValidateWorkspaceKey(p.WorkspaceKey); err != nil {
			return err
		}
	}
	if p.WorkspaceRoot == "" || p.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: invalid approval policy root", ErrInvalid)
	}
	if p.Mode == ApprovalWorkspaceRead && !p.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: read approval mode must have no expiration", ErrInvalid)
	}
	if p.Mode == ApprovalTrusted && (p.ExpiresAt.IsZero() || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(MaxTrustedDuration+time.Minute))) {
		return fmt.Errorf("%w: trusted approval must expire within 24 hours", ErrInvalid)
	}
	return nil
}
func (p ApprovalPolicy) Allows(kind string, reviewable bool, now time.Time) bool {
	if !p.ExpiresAt.IsZero() && !now.Before(p.ExpiresAt) {
		return false
	}
	switch p.Mode {
	case ApprovalWorkspaceRead:
		return p.ExpiresAt.IsZero() && kind == "read" && reviewable
	case ApprovalTrusted:
		return !p.ExpiresAt.IsZero() && now.Before(p.ExpiresAt) &&
			(kind == "read" || kind == "edit" || kind == "execute")
	default:
		return false
	}
}

// PendingApproval carries only local-owner dashboard metadata.
// No ACP raw inputs, command text, prompt, secret or bearer is present.
type PendingApproval struct {
	Approval     Approval `json:"approval"`
	AgentName    string   `json:"agentName"`
	WorkspaceKey string   `json:"workspaceKey"`
}
