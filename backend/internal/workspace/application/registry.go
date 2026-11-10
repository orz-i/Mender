package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

var (
	ErrRegistryDenied   = errors.New("workspace registry: owner authorization required")
	ErrRegistryInvalid  = errors.New("workspace registry: invalid resource or scope")
	ErrRegistryConflict = errors.New("workspace registry: resource in use or incompatible")
	ErrRegistryMissing  = errors.New("workspace registry: resource not found")
)

// Record is a shared logical Workspace, not an implicit mount or tool grant.
type Record struct {
	ID           workspace.Ref `json:"workspaceId"`
	DisplayName  string        `json:"displayName"`
	VerifiedRoot string        `json:"verifiedRoot"`
	CreatedAt    string        `json:"createdAt"`
}
type PlacementRecord struct {
	WorkspaceID workspace.Ref `json:"workspaceId"`
	SandboxID   string        `json:"sandboxId"`
	BackendRoot string        `json:"backendRoot"`
}
type TargetRecord struct {
	ProfileID   string         `json:"profileId"`
	ToolID      catalog.ToolID `json:"toolId"`
	WorkspaceID string         `json:"workspaceId"`
	SandboxID   string         `json:"sandboxId"`
}

// RegistryStore owns atomic, cross-context SQLite reference checks; it must
// never infer authorization from a user supplied display name or host path.
type RegistryStore interface {
	Register(context.Context, string, string, Record) error
	List(context.Context, string, string) ([]Record, error)
	Get(context.Context, string, string, workspace.Ref) (Record, error)
	Delete(context.Context, string, string, workspace.Ref) error
	Link(context.Context, string, string, workspace.Ref, string, string) error
	Unlink(context.Context, string, string, workspace.Ref, string) error
	Placements(context.Context, string, string, workspace.Ref) ([]PlacementRecord, error)
	Grants(context.Context, string, string, workspace.Ref) ([]string, error)
	Grant(context.Context, string, string, workspace.Ref, string) error
	Revoke(context.Context, string, string, workspace.Ref, string) error
	PutTarget(context.Context, string, string, TargetRecord) error
	DeleteTarget(context.Context, string, string, TargetRecord) error
	Targets(context.Context, string, string, string) ([]TargetRecord, error)
}

// Registry is the sole owner-facing configuration boundary for native Workspace
// placement and exact Profile/Tool TargetScope. Grant operations never synthesize
// a Profile ToolBinding or Sandbox capability permission.
type Registry struct {
	store     RegistryStore
	sandboxes SandboxLookup
	roots     RootVerifier
}

// SandboxBinding is the verified metadata a Workspace consumer needs; the
// Sandbox aggregate is owned by its own context, not imported into Workspace.
type SandboxBinding struct {
	Backend   string
	LocalRoot string
	OwnerKind string
	OwnerRef  string
}
type SandboxLookup interface {
	GetSandbox(context.Context, string) (SandboxBinding, error)
}

// RootVerifier is implemented by a side-effect-free local filesystem adapter.
type RootVerifier interface {
	CanonicalLocalRoot(context.Context, string) (string, error)
}

func NewRegistry(store RegistryStore, sandboxes SandboxLookup, roots RootVerifier) (*Registry, error) {
	if store == nil || sandboxes == nil || roots == nil {
		return nil, ErrRegistryInvalid
	}
	return &Registry{store: store, sandboxes: sandboxes, roots: roots}, nil
}
func registryOwner(caller gov.Context) (string, string, error) {
	if caller.Validate() != nil || caller.Principal().Kind() != gov.PrincipalLocalUser || caller.Scope().Kind() != gov.ScopeCatalog {
		return "", "", ErrRegistryDenied
	}
	switch caller.Origin() {
	case gov.OriginCLI, gov.OriginManagementHTTP, gov.OriginDesktop:
	default:
		return "", "", ErrRegistryDenied
	}
	return string(caller.Principal().Kind()), caller.Principal().Reference(), nil
}
func (s *Registry) RegisterLocal(ctx context.Context, caller gov.Context, name, root string) (Record, error) {
	kind, owner, err := registryOwner(caller)
	if err != nil {
		return Record{}, err
	}
	if name == "" || name != strings.TrimSpace(name) || len(name) > 160 {
		return Record{}, ErrRegistryInvalid
	}
	canonical, err := s.roots.CanonicalLocalRoot(ctx, root)
	if err != nil || canonical != root {
		return Record{}, ErrRegistryInvalid
	}
	var raw [16]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return Record{}, err
	}
	id, err := workspace.ParseRef("wrk_" + hex.EncodeToString(raw[:]))
	if err != nil {
		return Record{}, err
	}
	record := Record{ID: id, DisplayName: name, VerifiedRoot: root, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err = s.store.Register(ctx, kind, owner, record); err != nil {
		return Record{}, err
	}
	return record, nil
}
func (s *Registry) List(ctx context.Context, caller gov.Context) ([]Record, error) {
	k, o, e := registryOwner(caller)
	if e != nil {
		return nil, e
	}
	return s.store.List(ctx, k, o)
}
func (s *Registry) Get(ctx context.Context, caller gov.Context, id workspace.Ref) (Record, error) {
	k, o, e := registryOwner(caller)
	if e != nil {
		return Record{}, e
	}
	if id.Validate() != nil {
		return Record{}, ErrRegistryInvalid
	}
	return s.store.Get(ctx, k, o, id)
}
func (s *Registry) Delete(ctx context.Context, caller gov.Context, id workspace.Ref) error {
	k, o, e := registryOwner(caller)
	if e != nil {
		return e
	}
	if id.Validate() != nil {
		return ErrRegistryInvalid
	}
	return s.store.Delete(ctx, k, o, id)
}
func (s *Registry) LinkLocal(ctx context.Context, caller gov.Context, id workspace.Ref, sandboxID string) error {
	k, o, e := registryOwner(caller)
	if e != nil {
		return e
	}
	if id.Validate() != nil || invalidSandboxID(sandboxID) {
		return ErrRegistryInvalid
	}
	record, e := s.store.Get(ctx, k, o, id)
	if e != nil {
		return e
	}
	item, e := s.sandboxes.GetSandbox(ctx, sandboxID)
	if e != nil {
		return e
	}
	if item.OwnerKind != k || item.OwnerRef != o {
		return ErrRegistryDenied
	}
	if item.Backend != "local_workspace" || item.LocalRoot != record.VerifiedRoot {
		return ErrRegistryConflict
	}
	canonical, e := s.roots.CanonicalLocalRoot(ctx, record.VerifiedRoot)
	if e != nil || canonical != record.VerifiedRoot {
		return ErrRegistryConflict
	}
	return s.store.Link(ctx, k, o, id, sandboxID, canonical)
}
func (s *Registry) Unlink(ctx context.Context, caller gov.Context, id workspace.Ref, sandboxID string) error {
	k, o, e := registryOwner(caller)
	if e != nil {
		return e
	}
	if id.Validate() != nil || invalidSandboxID(sandboxID) {
		return ErrRegistryInvalid
	}
	return s.store.Unlink(ctx, k, o, id, sandboxID)
}
func (s *Registry) Placements(ctx context.Context, caller gov.Context, id workspace.Ref) ([]PlacementRecord, error) {
	k, o, e := registryOwner(caller)
	if e != nil {
		return nil, e
	}
	if id.Validate() != nil {
		return nil, ErrRegistryInvalid
	}
	return s.store.Placements(ctx, k, o, id)
}
func (s *Registry) Grants(ctx context.Context, caller gov.Context, id workspace.Ref) ([]string, error) {
	k, o, e := registryOwner(caller)
	if e != nil {
		return nil, e
	}
	if id.Validate() != nil {
		return nil, ErrRegistryInvalid
	}
	return s.store.Grants(ctx, k, o, id)
}
func (s *Registry) Grant(ctx context.Context, caller gov.Context, id workspace.Ref, profileID string) error {
	k, o, e := registryOwner(caller)
	if e != nil {
		return e
	}
	if id.Validate() != nil {
		return ErrRegistryInvalid
	}
	return s.store.Grant(ctx, k, o, id, profileID)
}
func (s *Registry) Revoke(ctx context.Context, caller gov.Context, id workspace.Ref, profileID string) error {
	k, o, e := registryOwner(caller)
	if e != nil {
		return e
	}
	if id.Validate() != nil {
		return ErrRegistryInvalid
	}
	return s.store.Revoke(ctx, k, o, id, profileID)
}
func (s *Registry) PutTarget(ctx context.Context, caller gov.Context, target TargetRecord) error {
	k, o, e := registryOwner(caller)
	if e != nil {
		return e
	}
	if invalidToolID(target.ToolID) || invalidSandboxID(target.SandboxID) {
		return ErrRegistryInvalid
	}
	if target.WorkspaceID != "" {
		if _, e = workspace.ParseRef(target.WorkspaceID); e != nil {
			return ErrRegistryInvalid
		}
	}
	return s.store.PutTarget(ctx, k, o, target)
}
func (s *Registry) DeleteTarget(ctx context.Context, caller gov.Context, target TargetRecord) error {
	k, o, e := registryOwner(caller)
	if e != nil {
		return e
	}
	if invalidToolID(target.ToolID) || invalidSandboxID(target.SandboxID) {
		return ErrRegistryInvalid
	}
	return s.store.DeleteTarget(ctx, k, o, target)
}
func (s *Registry) Targets(ctx context.Context, caller gov.Context, profileID string) ([]TargetRecord, error) {
	k, o, e := registryOwner(caller)
	if e != nil {
		return nil, e
	}
	return s.store.Targets(ctx, k, o, profileID)
}

func invalidSandboxID(id string) bool {
	return len(id) != 36 || !strings.HasPrefix(id, "sbx_") || !validLowerHex(id[4:])
}
func validLowerHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
func invalidToolID(id catalog.ToolID) bool {
	_, err := catalog.ParseToolID(id.String())
	return err != nil
}
