package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ApprovalID is an opaque, non-authorizing lookup identifier. Only the
// locally authenticated Agent owner may decide a pending request.
type ApprovalID string

func NewApprovalID() (ApprovalID, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return ApprovalID("aapr_" + hex.EncodeToString(entropy[:])), nil
}
func ParseApprovalID(input string) (ApprovalID, error) {
	if len(input) != 37 || !strings.HasPrefix(input, "aapr_") {
		return "", fmt.Errorf("%w: ApprovalID", ErrInvalid)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(input, "aapr_")); err != nil {
		return "", fmt.Errorf("%w: ApprovalID", ErrInvalid)
	}
	return ApprovalID(input), nil
}

type ApprovalStatus string

const (
	ApprovalPending     ApprovalStatus = "pending"
	ApprovalAllowedOnce ApprovalStatus = "allowed_once"
	ApprovalRejected    ApprovalStatus = "rejected"
	ApprovalExpired     ApprovalStatus = "expired"
)

type ApprovalChoice string

const (
	ApprovalAllowOnce ApprovalChoice = "allow_once"
	ApprovalReject    ApprovalChoice = "reject"
)

func (c ApprovalChoice) Valid() bool { return c == ApprovalAllowOnce || c == ApprovalReject }

// Approval stores only bounded, reviewable ACP permission metadata. The tool
// call text, rawInput, command, diffs and prompt are NEVER persisted here.
// ACP session/option identities are internal and not serialized to upstreams.
type Approval struct {
	ID             ApprovalID     `json:"approvalId"`
	TaskID         TaskID         `json:"taskId"`
	AgentID        ID             `json:"agentId"`
	Generation     uint64         `json:"-"`
	SessionID      string         `json:"-"`
	ToolCallID     string         `json:"-"`
	Kind           string         `json:"kind"`
	Target         string         `json:"target,omitempty"`
	CanAllow       bool           `json:"canAllow"`
	AllowOptionID  string         `json:"-"`
	RejectOptionID string         `json:"-"`
	Status         ApprovalStatus `json:"status"`
	DecisionSource string         `json:"decisionSource,omitempty"` // owner or automatic policy
	CreatedAt      time.Time      `json:"createdAt"`
	ExpiresAt      time.Time      `json:"expiresAt"`
	ResolvedAt     time.Time      `json:"resolvedAt,omitempty"`
}

func (a Approval) Validate() error {
	if _, err := ParseApprovalID(string(a.ID)); err != nil {
		return err
	}
	if _, err := ParseTaskID(string(a.TaskID)); err != nil {
		return err
	}
	if _, err := ParseID(string(a.AgentID)); err != nil {
		return err
	}
	if a.Generation == 0 || a.CreatedAt.IsZero() || a.ExpiresAt.IsZero() || !a.ExpiresAt.After(a.CreatedAt) ||
		len(a.SessionID) < 1 || len(a.SessionID) > 512 || len(a.ToolCallID) < 1 || len(a.ToolCallID) > 128 ||
		len(a.AllowOptionID) < 1 || len(a.AllowOptionID) > 128 || len(a.RejectOptionID) < 1 || len(a.RejectOptionID) > 128 ||
		a.AllowOptionID == a.RejectOptionID {
		return fmt.Errorf("%w: malformed approval identity", ErrInvalid)
	}
	switch a.Kind {
	case "read", "edit", "execute", "other":
	default:
		return fmt.Errorf("%w: unsupported approval kind", ErrInvalid)
	}
	if len(a.Target) > 240 || strings.ContainsAny(a.Target, "\r\n\x00") ||
		filepath.IsAbs(a.Target) || strings.HasPrefix(a.Target, "../") ||
		a.Target == ".." || (a.Target != "" && filepath.Clean(a.Target) != a.Target) {
		return fmt.Errorf("%w: unsafe review target", ErrInvalid)
	}
	if a.CanAllow && (a.Target == "" || a.Kind != "read") {
		return fmt.Errorf("%w: unreviewable native action cannot be approved", ErrInvalid)
	}
	if a.Status != ApprovalPending {
		return fmt.Errorf("%w: approval must start pending", ErrInvalid)
	}
	return nil
}
