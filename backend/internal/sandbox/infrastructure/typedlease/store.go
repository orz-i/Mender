// Package typedlease is a future-only atomic native Workspace lease
// reservation for ADR-0064. It never opens a process or Sandbox backend,
// and is NOT wired to the current bootstrap Runtime.
package typedlease

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandboxapp "github.com/orz-i/Mender/backend/internal/sandbox/application"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

var ErrUnadmitted = errors.New("native Workspace lease: Invocation not admitted")

// AdmissionVerifier is an exact receipt check issued by the current canonical
// Invocation pipeline. A caller-supplied ToolID, sandboxId or matching Facts
// is NOT a substitute for approved Policy/Approval/Runtime Budget.
type AdmissionVerifier interface {
	VerifyAdmittedAttempt(context.Context, invocation.InvocationID, governance.Context,
		catalog.ToolID, invocation.GovernanceFacts) error
}

// Store shares the same physical SQLite database as the existing Sandbox
// durable lease table. No duplicate lease ledger or scheduler is introduced.
// It can ONLY be constructed against the Stage E target schema.
type Store struct {
	db       *sql.DB
	verifier AdmissionVerifier
}

func New(ctx context.Context, db *sql.DB, verifier AdmissionVerifier) (*Store, error) {
	if db == nil || verifier == nil {
		return nil, ErrUnadmitted
	}
	for _, probe := range []struct {
		table   string
		version int
	}{
		{"tool_hardcut_schema_migrations", 1},
		{"catalog_schema_migrations", 3},
		{"profile_schema_migrations", 8},
		{"sandbox_schema_migrations", 20},
	} {
		var v int
		if err := db.QueryRowContext(ctx, "SELECT MAX(version) FROM "+probe.table).Scan(&v); err != nil || v != probe.version {
			return nil, fmt.Errorf("native Workspace lease: typed schema unavailable: %s", probe.table)
		}
	}
	return &Store{db: db, verifier: verifier}, nil
}

// Request is constructed only from a frozen, host-verified stage C placement
// and an already approved Invocation. Every identity must match the operation
// reviewed by Policy. Only local_workspace is currently attested: this API
// must never assume Docker/SSH/Daytona roots match a host directory.
type Request struct {
	InvocationID  invocation.InvocationID
	Caller        governance.Context
	ToolID        catalog.ToolID
	Operation     workspace.Operation
	Workspace     workspace.Ref
	Sandbox       sandbox.ID
	Generation    uint64
	Isolation     sandbox.IsolationClass
	ApprovedFacts invocation.GovernanceFacts
}

func (r Request) Validate() error {
	if err := r.Caller.Validate(); err != nil {
		return err
	}
	if _, ok := sandboxapp.ProfileServiceID(r.Caller); !ok {
		return sandbox.ErrNotAuthorized
	}
	if _, err := invocation.ParseInvocationID(r.InvocationID.String()); err != nil {
		return ErrUnadmitted
	}
	if _, err := catalog.ParseToolID(r.ToolID.String()); err != nil {
		return err
	}
	if err := r.Workspace.Validate(); err != nil {
		return err
	}
	if _, err := sandbox.ParseID(r.Sandbox.String()); err != nil {
		return err
	}
	if err := r.Isolation.Validate(); err != nil {
		return err
	}
	if r.Generation == 0 {
		return sandbox.ErrConflict
	}
	if err := validateOperation(r.Operation, r.ToolID, r.ApprovedFacts, r.Workspace, r.Sandbox); err != nil {
		return err
	}
	return nil
}
func validateOperation(op workspace.Operation, id catalog.ToolID,
	approved invocation.GovernanceFacts, ws workspace.Ref, sb sandbox.ID) error {
	key, _ := operationCapability(op)
	effect := map[string]string{
		"workspace.read":   "filesystem.read",
		"workspace.search": "filesystem.search",
		"workspace.edit":   "filesystem.write",
		"workspace.shell":  "process.execute",
	}[key]
	if key == "" || effect == "" {
		return sandbox.ErrCapabilityUnavailable
	}
	expectedID := catalog.ToolID("")
	defs, err := curated.PlannedNativeTools()
	if err != nil {
		return err
	}
	for _, def := range defs {
		if def.SourceKey == key {
			expectedID = def.ToolID
			break
		}
	}
	if expectedID == "" || expectedID != id {
		return sandbox.ErrCapabilityUnavailable
	}
	if approved.Validate() != nil || approved.Knowledge() != invocation.FactKnowledgeKnown {
		return ErrUnadmitted
	}
	effectMatches := 0
	for _, v := range approved.Effects() {
		if string(v) == effect {
			effectMatches++
		}
	}
	if effectMatches != 1 {
		return ErrUnadmitted
	}
	wsSeen, sbSeen, pathSeen := 0, 0, 0
	for _, v := range approved.Resources() {
		switch v.Kind() {
		case "workspace_id":
			wsSeen++
			if v.Value() != ws.String() {
				return ErrUnadmitted
			}
		case "sandbox_id":
			sbSeen++
			if v.Value() != sb.String() {
				return ErrUnadmitted
			}
		case "workspace_path":
			pathSeen++
			normalized, err := workspace.NormalizeRelativePath(v.Value(), op == workspace.Search || op == workspace.Shell)
			if err != nil || normalized != v.Value() {
				return ErrUnadmitted
			}
		}
	}
	if wsSeen != 1 || sbSeen != 1 || pathSeen != 1 {
		return ErrUnadmitted
	}
	return nil
}
func operationCapability(op workspace.Operation) (string, string) {
	switch op {
	case workspace.Read:
		return "workspace.read", "file.read"
	case workspace.Search:
		return "workspace.search", "file.search"
	case workspace.Edit:
		return "workspace.edit", "file.edit"
	case workspace.Shell:
		return "workspace.shell", "process.shell"
	}
	return "", ""
}

// Reserve is a single SQLite INSERT...SELECT admission. It checks the
// **current** Profile ToolBinding, exact Profile TargetScope, Workspace grant,
// Sandbox capability grant, native operation, isolation, backend root, Ready
// Run generation and unique durable lease under the same SQLite write lock.
// If revocation commits before this INSERT, zero rows can be reserved.
// A successful reservation still does not open a backend or authorize actual
// invocation; the caller must release this token on any later failure.
func (s *Store) Reserve(ctx context.Context, r Request) (string, error) {
	if s == nil || s.db == nil || s.verifier == nil {
		return "", ErrUnadmitted
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := r.Validate(); err != nil {
		return "", err
	}
	profileID, _ := sandboxapp.ProfileServiceID(r.Caller)
	if err := s.verifier.VerifyAdmittedAttempt(ctx, r.InvocationID, r.Caller, r.ToolID, r.ApprovedFacts); err != nil {
		return "", fmt.Errorf("%w: %w", ErrUnadmitted, err)
	}
	sourceKey, capability := operationCapability(r.Operation)
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(random[:])
	subject, scope := r.Caller.Principal(), r.Caller.Scope()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `INSERT INTO sandbox_execution_leases
 (sandbox_id,lease_token,generation,principal_kind,principal_ref,scope_kind,scope_ref,acquired_at)
 SELECT r.sandbox_id,?,?,?,?,?,?,? FROM sandbox_runs r
 JOIN sandboxes sb ON sb.id=r.sandbox_id
 JOIN tools t ON t.id=?
 JOIN profile_tools b ON b.tool_id=t.id AND b.profile_id=?
 JOIN profile_tool_target_scopes p ON p.profile_id=b.profile_id
    AND p.tool_id=b.tool_id AND p.workspace_id=? AND p.sandbox_id=r.sandbox_id
 JOIN workspace_placements w ON w.workspace_id=p.workspace_id AND w.sandbox_id=r.sandbox_id
 JOIN workspace_profile_grants wg ON wg.workspace_id=p.workspace_id
    AND wg.profile_id=b.profile_id
 JOIN sandbox_profile_grants sg ON sg.sandbox_id=r.sandbox_id
    AND sg.profile_id=b.profile_id AND sg.capability=?
 WHERE r.sandbox_id=? AND r.generation=? AND r.state='ready'
   AND sb.backend='local_workspace' AND sb.local_root=w.backend_root
   AND sb.isolation=? AND t.source_kind='native'
   AND t.source_id='workspace' AND t.source_key=?
   AND b.policy_constraint!='deny'`,
		token, r.Generation, string(subject.Kind()), subject.Reference(), string(scope.Kind()), scope.Reference(), now,
		r.ToolID.String(), profileID, r.Workspace.String(), capability, r.Sandbox.String(),
		r.Generation, string(r.Isolation), sourceKey)
	if err != nil {
		return "", fmt.Errorf("native Workspace reserve: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if count != 1 {
		return "", sandbox.ErrNotAuthorized
	}
	return token, nil
}

// Release requires the same SandboxID, durable token and exact Profile owner.
// It never changes a task outcome or grants a new tool.
func (s *Store) Release(ctx context.Context, caller governance.Context, sandboxID sandbox.ID, token string) error {
	if s == nil || s.db == nil {
		return sandbox.ErrBackendUnavailable
	}
	if err := caller.Validate(); err != nil {
		return err
	}
	if _, ok := sandboxapp.ProfileServiceID(caller); !ok {
		return sandbox.ErrNotAuthorized
	}
	if _, err := sandbox.ParseID(sandboxID.String()); err != nil {
		return err
	}
	if len(token) != 32 {
		return sandbox.ErrConflict
	}
	if _, err := hex.DecodeString(token); err != nil {
		return sandbox.ErrConflict
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM sandbox_execution_leases
  WHERE sandbox_id=? AND lease_token=? AND principal_kind=?
    AND principal_ref=? AND scope_kind=? AND scope_ref=?`,
		sandboxID.String(), token, string(caller.Principal().Kind()), caller.Principal().Reference(),
		string(caller.Scope().Kind()), caller.Scope().Reference())
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sandbox.ErrConflict
	}
	return nil
}
