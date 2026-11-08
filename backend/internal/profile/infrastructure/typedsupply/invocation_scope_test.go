package typedsupply_test

import (
	"context"
	"errors"
	"testing"

	catalog "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
)

func scopeToolID(t *testing.T, raw string) catalog.ToolID {
	t.Helper()
	id, err := catalog.ParseToolID(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTypedInvocationScopePreservesBindingConstraintsAndCredentialRefs(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "scope", "hybrid")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.addTool(t, profileA, f.nativeReadID, "read")
	f.grantAgent(t, profileA)
	f.grantNative(t, profileA)
	f.exec(t, `UPDATE profile_tools SET policy_constraint='require_approval',
 concurrency_constraint='serial', credential_refs_json='["credential://github/operations"]'
 WHERE profile_id=? AND tool_id=?`, profileA, f.agentToolIDs[0])
	reader, err := supply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	agentID := scopeToolID(t, f.agentToolIDs[0])
	governance, found, err := reader.ResolveBindingGovernance(context.Background(), profileA, agentID)
	if err != nil || !found {
		t.Fatalf("Agent binding not resolved: %v found=%v", err, found)
	}
	if governance.PolicyConstraint() != invocation.BindingPolicyRequireApproval ||
		governance.ConcurrencyConstraint() != invocation.BindingConcurrencySerial {
		t.Fatalf("binding scope constraints lost: %+v", governance)
	}
	if got := governance.CredentialRefs(); len(got) != 1 || got[0].String() != "credential://github/operations" {
		t.Fatalf("binding credential allowlist lost: %+v", got)
	}
	native, found, err := reader.ResolveBindingGovernance(context.Background(), profileA, scopeToolID(t, f.nativeReadID))
	if err != nil || !found || native.PolicyConstraint() != invocation.BindingPolicyInherit ||
		len(native.CredentialRefs()) != 0 {
		t.Fatalf("native binding did not resolve exact inherited constraints: %+v %v found=%v", native, err, found)
	}
	f.exec(t, "UPDATE profile_tools SET policy_constraint='deny' WHERE profile_id=? AND tool_id=?", profileA, f.nativeReadID)
	disabled, found, err := reader.ResolveBindingGovernance(context.Background(), profileA, scopeToolID(t, f.nativeReadID))
	if err != nil || !found || disabled.PolicyConstraint() != invocation.BindingPolicyDeny {
		t.Fatalf("explicit deny was silently removed: %+v %v found=%v", disabled, err, found)
	}
}

func TestTypedInvocationScopeDeniesMissingGrantsAndRemovedBindings(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "scope", "direct")
	f.addProfile(t, profileB, "other", "direct")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.addTool(t, profileA, f.nativeReadID, "read")
	f.addTool(t, profileB, f.nativeReadID, "read")
	f.grantAgent(t, profileA)
	f.grantNative(t, profileA)
	reader, err := supply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	ctx := context.Background()
	agentID := scopeToolID(t, f.agentToolIDs[0])
	nativeID := scopeToolID(t, f.nativeReadID)
	for _, tc := range []struct {
		profile string
		id      catalog.ToolID
	}{
		{profileB, nativeID},
		{"prof_11111111111111111111111111111111", nativeID},
		{"not-a-profile", agentID},
	} {
		_, found, err := reader.ResolveBindingGovernance(ctx, tc.profile, tc.id)
		if err != nil || found {
			t.Fatalf("ungranted Profile %s allowed %s: %v found=%v", tc.profile, tc.id, err, found)
		}
	}
	f.exec(t, "DELETE FROM agent_grants WHERE profile_id=?", profileA)
	if _, found, err := reader.ResolveBindingGovernance(ctx, profileA, agentID); err != nil || found {
		t.Fatalf("revoked Agent retained invocation: %v found=%v", err, found)
	}
	f.exec(t, "DELETE FROM sandbox_profile_grants WHERE profile_id=?", profileA)
	if _, found, err := reader.ResolveBindingGovernance(ctx, profileA, nativeID); err != nil || found {
		t.Fatalf("revoked Sandbox retained invocation: %v found=%v", err, found)
	}
	f.exec(t, "DELETE FROM profile_tools WHERE profile_id=? AND tool_id=?", profileA, f.nativeReadID)
	if _, found, err := reader.ResolveBindingGovernance(ctx, profileA, nativeID); err != nil || found {
		t.Fatalf("deleted ToolBinding retained invocation: %v found=%v", err, found)
	}
}

func TestTypedInvocationScopeRejectsCorruptConstraintsAndCredentials(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "scope", "dynamic")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.grantAgent(t, profileA)
	reader, err := supply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	ctx := context.Background()
	id := scopeToolID(t, f.agentToolIDs[0])
	const updateCredentials = "UPDATE profile_tools SET credential_refs_json=? WHERE profile_id=? AND tool_id=?"
	const updatePolicy = "UPDATE profile_tools SET policy_constraint=? WHERE profile_id=? AND tool_id=?"
	const updateConcurrency = "UPDATE profile_tools SET concurrency_constraint=? WHERE profile_id=? AND tool_id=?"
	for _, tc := range []struct {
		query, label, value string
	}{
		{updateCredentials, "credential_refs_json", "null"},
		{updateCredentials, "credential_refs_json", "{bad-json"},
		{updateCredentials, "credential_refs_json", `["credential://github/operations","credential://github/operations"]`},
		{updateCredentials, "credential_refs_json", `["credential://github/operations","invalid"]`},
		{updatePolicy, "policy_constraint", "garbage"},
		{updateConcurrency, "concurrency_constraint", "garbage"},
	} {
		if _, err := f.db.Exec(tc.query, tc.value, profileA, f.agentToolIDs[0]); err != nil {
			// SQL-level rejection is also a valid fail-closed outcome.
			continue
		}
		_, found, err := reader.ResolveBindingGovernance(ctx, profileA, id)
		if !errors.Is(err, supply.ErrInvalidSupply) || found {
			t.Fatalf("%s=%q did not fail closed: %v found=%v", tc.label, tc.value, err, found)
		}
		f.exec(t, `UPDATE profile_tools SET credential_refs_json='[]',
 policy_constraint='inherit',concurrency_constraint='inherit' WHERE profile_id=? AND tool_id=?`, profileA, f.agentToolIDs[0])
	}
}

func TestTypedInvocationScopeRejectsLegacyWithoutTypedReader(t *testing.T) {
	var nilReader *supply.Reader
	_, found, err := nilReader.ResolveBindingGovernance(context.Background(), profileA, scopeToolID(t, "tool_11111111111111111111111111111111"))
	if !errors.Is(err, supply.ErrTypedSchemaRequired) || found {
		t.Fatalf("nil or legacy scope resolver must fail: %v found=%v", err, found)
	}
}
