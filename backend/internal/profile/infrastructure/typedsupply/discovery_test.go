package typedsupply_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	supply "github.com/orz-i/Mender/backend/internal/profile/infrastructure/typedsupply"
)

func TestTypedSourceDiscoveryPagesOnlyProfileVisibleTools(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "discover", "hybrid")
	f.addTool(t, profileA, f.nativeReadID, "read")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.grantNative(t, profileA)
	f.grantAgent(t, profileA)
	reader, err := supply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	snapshot, err := reader.Project(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	first, err := snapshot.Browse("sources", "", "", "", 1)
	if err != nil || len(first.Sources) != 1 || first.NextCursor == "" {
		t.Fatalf("source-first pagination broken: %+v %v", first, err)
	}
	next, err := snapshot.Browse("sources", "", "", first.NextCursor, 1)
	if err != nil || len(next.Sources) != 1 || next.NextCursor != "" ||
		next.Sources[0].SourceID == first.Sources[0].SourceID {
		t.Fatalf("next Source cursor leaked or repeated: %+v %v", next, err)
	}
	exact, ok := snapshot.Source("native:workspace")
	if !ok || len(exact.Tools) != 1 || exact.Tools[0].SourceID != "native:workspace" {
		t.Fatalf("typed native source missing: %+v", exact)
	}
	if unknown, ok := snapshot.Source("provider:prov_99999999999999999999999999999999"); ok || unknown.SourceID != "" {
		t.Fatalf("unbound source inferred: %+v", unknown)
	}
	byTool, err := snapshot.Browse("tools", "native:workspace", "read", "", 10)
	if err != nil || len(byTool.Tools) != 1 || byTool.Tools[0].ToolID != f.nativeReadID {
		t.Fatalf("source-local browse returned unauthorized Tools: %+v %v", byTool, err)
	}
	hidden, err := snapshot.Browse("tools", "provider:prov_99999999999999999999999999999999", "read", "", 10)
	if err != nil || len(hidden.Tools) != 0 || len(hidden.Sources) != 0 {
		t.Fatalf("profile-hidden source disclosed: %+v %v", hidden, err)
	}
	if _, err := snapshot.Browse("tools", "", "", "", 10); !errors.Is(err, supply.ErrInvalidSupply) {
		t.Fatalf("unbounded browse accepted: %v", err)
	}
}

func TestTypedDiscoveryCursorRejectedAfterGrantRevocation(t *testing.T) {
	f := futureFixture(t)
	f.addProfile(t, profileA, "revocable", "direct")
	f.addTool(t, profileA, f.agentToolIDs[0], "delegate")
	f.addTool(t, profileA, f.nativeReadID, "read")
	f.grantAgent(t, profileA)
	f.grantNative(t, profileA)
	reader, err := supply.Open(context.Background(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	before, err := reader.Project(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	first, err := before.Browse("sources", "", "", "", 1)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("expected cursor before revoke: %+v %v", first, err)
	}
	fingerprint, err := before.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, "DELETE FROM sandbox_profile_grants WHERE profile_id=?", profileA)
	after, err := reader.Project(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	newFingerprint, err := after.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint == newFingerprint {
		t.Fatal("Sandbox grant revoke did not invalidate source fingerprint")
	}
	if _, err := after.Browse("sources", "", "", first.NextCursor, 1); !errors.Is(err, supply.ErrStaleCursor) {
		t.Fatalf("stale Cursor accepted after grant revoke: %v", err)
	}
	for _, tc := range []struct {
		view, source, query, cursor string
		limit                       int
	}{
		{"sources", "", "", first.NextCursor, 2},
		{"tools", "", "read", first.NextCursor, 1},
		{"sources", "", "", "@@@", 1},
		{"sources", "", "", strings.Repeat("a", 4097), 1},
	} {
		if _, err := before.Browse(tc.view, tc.source, tc.query, tc.cursor, tc.limit); err == nil {
			t.Fatalf("wrong-scope/corrupt cursor accepted: %+v", tc)
		}
	}
}

func TestTypedDiscoveryCursorRejectsSchemaAndForgedFields(t *testing.T) {
	bogus := supply.Snapshot{ProfileID: profileA, Mode: "direct", Sources: []supply.Source{}, Direct: []supply.Export{}}
	if _, err := bogus.Browse("tools", "provider:prov_invalid", "read", "", 10); !errors.Is(err, supply.ErrInvalidSupply) {
		t.Fatalf("untrusted ProviderID accepted: %v", err)
	}
	if _, err := bogus.Browse("sources", "", "", "", 0); !errors.Is(err, supply.ErrInvalidSupply) {
		t.Fatal(err)
	}
	if _, err := bogus.Browse("sources", "", "", "", 101); !errors.Is(err, supply.ErrInvalidSupply) {
		t.Fatal(err)
	}
	fingerprint, err := bogus.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if len(fingerprint) != 64 {
		t.Fatal("non-SHA256 fingerprint")
	}
	encoded, err := json.Marshal(bogus)
	if err != nil || strings.Contains(string(encoded), "providerId") {
		t.Fatalf("typed supply leaked legacy ProviderID: %s %v", encoded, err)
	}
}
