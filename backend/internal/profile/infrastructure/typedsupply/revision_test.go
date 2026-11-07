package typedsupply_test

import (
	"context"
	"errors"
	"testing"

	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
)

func TestFutureSQLNativeScopeCannotBeGrantedWithoutSandboxCapability(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "guarded", "direct")
	f.addTool(t, profileA, f.nativeReadID, "read")
	_, err := f.db.Exec(`INSERT INTO profile_tool_target_scopes(profile_id,tool_id,workspace_id,sandbox_id)
 VALUES(?,?,?,?)`, profileA, f.nativeReadID, f.workspaceID, f.sandboxID)
	if err == nil {
		t.Fatal("native target scope accepted without an existing Sandbox Profile grant")
	}
	reader, err := supply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	first, err := reader.Project(context.Background(), profileA)
	if err != nil || len(first.Sources) != 0 {
		t.Fatalf("unapproved scope was published: %+v %v", first, err)
	}
	f.grantNative(t, profileA)
	second, err := reader.Project(context.Background(), profileA)
	if err != nil || len(second.Sources) != 1 {
		t.Fatalf("explicit native scope missing: %+v %v", second, err)
	}
	if second.ProfileRevision <= first.ProfileRevision || second.DeliveryRevision <= first.DeliveryRevision {
		t.Fatalf("native grant/target changes failed to bump revisions: %+v -> %+v", first, second)
	}
	source, ok := second.Source("native:workspace")
	if !ok || len(source.Tools) != 1 {
		t.Fatal("native source missing")
	}
	source.Tools[0].Targets[0].SandboxID = "sbx_ffffffffffffffffffffffffffffffff"
	stable, ok := second.Source("native:workspace")
	if !ok || stable.Tools[0].Targets[0].SandboxID != f.sandboxID {
		t.Fatal("Source accessor leaked mutable target scope")
	}
	page, err := second.Browse("tools", "native:workspace", "read", "", 10)
	if err != nil || len(page.Tools) != 1 {
		t.Fatal(err)
	}
	page.Tools[0].Targets[0].SandboxID = "sbx_ffffffffffffffffffffffffffffffff"
	checked, err := second.Browse("tools", "native:workspace", "read", "", 10)
	if err != nil || checked.Tools[0].Targets[0].SandboxID != f.sandboxID {
		t.Fatal("Browse accessor leaked mutable target scope")
	}
	f.exec(t, "DELETE FROM sandbox_profile_grants WHERE sandbox_id=? AND profile_id=?", f.sandboxID, profileA)
	revoked, err := reader.Project(context.Background(), profileA)
	if err != nil || len(revoked.Sources) != 0 {
		t.Fatalf("Sandbox revocation did not deny future native discovery: %+v %v", revoked, err)
	}
	if revoked.ProfileRevision <= second.ProfileRevision || revoked.DeliveryRevision <= second.DeliveryRevision {
		t.Fatal("Sandbox revocation failed to invalidate delivery/profile revisions")
	}
}

func TestFutureSQLAgentGrantRevisionAndNoAutomaticProfileExposure(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "agent-supply", "direct")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	reader, err := supply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	initial, err := reader.Project(context.Background(), profileA)
	if err != nil || len(initial.Sources) != 0 {
		t.Fatal("Agent source exposed before grant", err)
	}
	f.grantAgent(t, profileA)
	granted, err := reader.Project(context.Background(), profileA)
	if err != nil || len(granted.Sources) != 1 {
		t.Fatal("Agent source missing after grant", err)
	}
	if granted.ProfileRevision <= initial.ProfileRevision || granted.DeliveryRevision <= initial.DeliveryRevision {
		t.Fatal("Agent grant did not bump source supply revision")
	}
	f.exec(t, "DELETE FROM agent_grants WHERE agent_id=? AND profile_id=?", f.agentID, profileA)
	revoked, err := reader.Project(context.Background(), profileA)
	if err != nil || len(revoked.Sources) != 0 {
		t.Fatalf("Agent source persisted after revocation: %+v %v", revoked, err)
	}
	if revoked.ProfileRevision <= granted.ProfileRevision || revoked.DeliveryRevision <= granted.DeliveryRevision {
		t.Fatal("Agent grant revoke did not bump revision")
	}
	// Old Catalog or a typo must never be a fallback for the typed reader.
	if _, err := reader.Project(context.Background(), "prof_missing"); !errors.Is(err, supply.ErrInvalidSupply) {
		t.Fatalf("unknown profile accepted: %v", err)
	}
}
