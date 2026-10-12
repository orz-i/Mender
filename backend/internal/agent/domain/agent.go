// Package domain defines Agent resources and delegated task identity.
// Mender does not own an Agent reasoning loop.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	platformplugin "github.com/orz-i/Mender/backend/internal/plugin"
)

var (
	ErrNotFound    = errors.New("agent resource not found")
	ErrConflict    = errors.New("agent resource conflict")
	ErrForbidden   = errors.New("agent operation not authorized")
	ErrUnavailable = errors.New("agent backend unavailable")
	ErrInvalid     = errors.New("invalid agent resource")
)

const (
	CodexPluginID = "io.mender.agent.codex"
	GrokPluginID  = "io.mender.agent.grok"
)

type ID string

func ParseID(raw string) (ID, error) {
	if len(raw) != 38 || !strings.HasPrefix(raw, "agent_") {
		return "", fmt.Errorf("%w: AgentID", ErrInvalid)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(raw, "agent_")); err != nil {
		return "", fmt.Errorf("%w: AgentID: %w", ErrInvalid, err)
	}
	return ID(raw), nil
}

func NewID() (ID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ID("agent_" + hex.EncodeToString(b[:])), nil
}

type Backend string

const (
	BackendCodex Backend = "codex"
	BackendGrok  Backend = "grok"
)

func (b Backend) PluginID() platformplugin.ID {
	switch b {
	case BackendCodex:
		return platformplugin.ID(CodexPluginID)
	case BackendGrok:
		return platformplugin.ID(GrokPluginID)
	default:
		return ""
	}
}

type Spec struct {
	ID            ID                         `json:"agentId"`
	Name          string                     `json:"name"`
	Backend       Backend                    `json:"backend"`
	PluginID      platformplugin.ID          `json:"pluginId"`
	ExtensionID   platformplugin.ExtensionID `json:"extensionId"`
	WorkspaceRoot string                     `json:"workspaceRoot"`
	Isolation     string                     `json:"isolation"`
	OwnerKind     string                     `json:"ownerKind"`
	OwnerRef      string                     `json:"ownerRef"`
	CreatedAt     time.Time                  `json:"createdAt"`
}

// Agent is a configured Resource bound to one built-in Backend, not a Provider.
type Agent struct{ spec Spec }

func NewAgent(spec Spec) (Agent, error) {
	if _, err := ParseID(string(spec.ID)); err != nil {
		return Agent{}, err
	}
	if spec.Backend.PluginID() == "" || spec.PluginID != spec.Backend.PluginID() ||
		spec.ExtensionID != "main" {
		return Agent{}, fmt.Errorf("%w: backend binding must select exact built-in Agent plugin", ErrInvalid)
	}
	if spec.Name == "" || strings.TrimSpace(spec.Name) != spec.Name || len(spec.Name) > 120 {
		return Agent{}, fmt.Errorf("%w: invalid display name", ErrInvalid)
	}
	if spec.WorkspaceRoot == "" || !filepath.IsAbs(spec.WorkspaceRoot) ||
		filepath.Clean(spec.WorkspaceRoot) != spec.WorkspaceRoot {
		return Agent{}, fmt.Errorf("%w: workspace root must be canonical absolute path", ErrInvalid)
	}
	switch spec.Isolation {
	case "host", "restricted_host":
	default:
		return Agent{}, fmt.Errorf("%w: unsupported isolation mode", ErrInvalid)
	}
	if (spec.OwnerKind != "local_user" && spec.OwnerKind != "service") ||
		spec.OwnerRef == "" || strings.TrimSpace(spec.OwnerRef) != spec.OwnerRef ||
		len(spec.OwnerRef) > 255 {
		return Agent{}, fmt.Errorf("%w: invalid owner", ErrInvalid)
	}
	if spec.CreatedAt.IsZero() {
		return Agent{}, fmt.Errorf("%w: missing creation time", ErrInvalid)
	}
	spec.CreatedAt = spec.CreatedAt.UTC()
	return Agent{spec: spec}, nil
}

func (a Agent) Spec() Spec { return a.spec }
func (a Agent) ID() ID     { return a.spec.ID }
func (a Agent) OwnedBy(kind, ref string) bool {
	return a.spec.OwnerKind == kind && a.spec.OwnerRef == ref
}
