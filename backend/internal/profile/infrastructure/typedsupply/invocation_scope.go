package typedsupply

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	credential "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	profile "github.com/orz-i/Mender/backend/internal/profile/domain"
	suite "github.com/orz-i/Mender/backend/internal/toolsuite/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
)

// ResolveBindingGovernance makes the existing, query-only typed Profile Reader
// implement canonical Invocation's ProfileScopeResolver port. This is NOT a
// second authorization engine: Sandbox/Workspace target scope, live grants,
// Policy/approval and atomic lease are still independently enforced at
// placement and execution. It never activates a Provider or Agent.
func (r *Reader) ResolveBindingGovernance(ctx context.Context, profileReference string, toolID catalog.ToolID,
) (invocationdomain.ScopedBindingGovernance, bool, error) {
	denied := invocationdomain.ScopedBindingGovernance{}
	if r == nil || r.db == nil {
		return denied, false, ErrTypedSchemaRequired
	}
	if _, err := profile.ParseProfileID(profileReference); err != nil {
		return denied, false, nil
	}
	if _, err := catalog.ParseToolID(toolID.String()); err != nil {
		return denied, false, nil
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return denied, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var policy, concurrency, credentialJSON, kind, owner, key string
	err = tx.QueryRowContext(ctx, `SELECT b.policy_constraint, b.concurrency_constraint,
 b.credential_refs_json, t.source_kind, t.source_id, t.source_key
 FROM profile_tools b
 JOIN tools t ON t.id=b.tool_id
 JOIN profiles p ON p.id=b.profile_id
 WHERE b.profile_id=? AND b.tool_id=?`, profileReference, toolID.String()).
		Scan(&policy, &concurrency, &credentialJSON, &kind, &owner, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return denied, false, nil
	}
	if err != nil {
		return denied, false, fmt.Errorf("typed Profile binding: %w", err)
	}
	source, err := catalog.NewSourceRef(catalog.SourceKind(kind), owner)
	if err != nil {
		return denied, false, ErrInvalidSupply
	}
	_, _, visible, err := sourceIdentity(ctx, tx, profileReference, source)
	if err != nil {
		return denied, false, err
	}
	if !visible {
		return denied, false, nil
	}
	switch source.Kind() {
	case catalog.SourceAgent:
		definition, ok := curated.Find(suite.AgentDelegation)
		if !ok || !slices.Contains(definition.ToolSourceKeys, key) {
			return denied, false, ErrInvalidSupply
		}
	case catalog.SourceNative:
		if !reviewedNativeTool(toolID.String(), source, key) {
			return denied, false, ErrInvalidSupply
		}
		targets, err := allowedTargets(ctx, tx, profileReference, toolID.String(), source, key)
		if err != nil {
			return denied, false, err
		}
		if len(targets) == 0 {
			return denied, false, nil
		}
	}
	// Credentials must come from the authoritative ToolBinding row. The
	// source-first discovery projection intentionally omits them.
	if !strings.HasPrefix(strings.TrimSpace(credentialJSON), "[") {
		return denied, false, ErrInvalidSupply
	}
	var rawRefs []string
	if err := json.Unmarshal([]byte(credentialJSON), &rawRefs); err != nil {
		return denied, false, ErrInvalidSupply
	}
	refs := make([]credential.Reference, 0, len(rawRefs))
	for _, raw := range rawRefs {
		ref, err := credential.ParseReference(raw)
		if err != nil {
			return denied, false, ErrInvalidSupply
		}
		refs = append(refs, ref)
	}
	// Reuse Profile's actual validation, including credential count,
	// uniqueness and the exact constraints; don't invent future defaults.
	if policy == "" || concurrency == "" {
		return denied, false, ErrInvalidSupply
	}
	binding, err := profile.NewToolBinding(profile.ToolBindingSpec{
		ToolID: toolID, PolicyConstraint: profile.PolicyConstraint(policy),
		ConcurrencyConstraint: profile.ConcurrencyConstraint(concurrency),
		CredentialRefs:        refs,
	})
	if err != nil {
		return denied, false, ErrInvalidSupply
	}
	governance, err := invocationdomain.NewScopedBindingGovernanceWithCredentials(
		invocationdomain.BindingPolicyConstraint(binding.PolicyConstraint()),
		invocationdomain.BindingConcurrencyConstraint(binding.ConcurrencyConstraint()),
		binding.CredentialRefs(),
	)
	if err != nil {
		return denied, false, ErrInvalidSupply
	}
	if err := tx.Commit(); err != nil {
		return denied, false, err
	}
	return governance, true, nil
}

var _ invocation.ProfileScopeResolver = (*Reader)(nil)
