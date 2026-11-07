package application

import (
	"context"
	"errors"
	"slices"
	"testing"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type staticPlacementReader struct {
	items []PlacementCandidate
	err   error
	calls int
}

func (f *staticPlacementReader) ListWorkspacePlacements(ctx context.Context, id workspace.Ref) ([]PlacementCandidate, error) {
	f.calls++
	return f.items, f.err
}

func TestPlacementQueryRequiresExplicitIdentityAndNeverSelectsTarget(t *testing.T) {
	ref, _ := workspace.ParseRef("wrk_0123456789abcdef0123456789abcdef")
	first := "sbx_11111111111111111111111111111111"
	second := "sbx_22222222222222222222222222222222"
	reader := &staticPlacementReader{items: []PlacementCandidate{
		{Workspace: ref, SandboxID: second, TargetRoot: "/workspace"},
		{Workspace: ref, SandboxID: first, TargetRoot: "/workspace"},
	}}
	query, err := NewPlacementQuery(reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = query.List(context.Background(), ""); err == nil || reader.calls != 0 {
		t.Fatal("invalid ref must not access source")
	}
	candidates, err := query.List(context.Background(), ref)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("query candidates=%+v err=%v", candidates, err)
	}
	if candidates[0].SandboxID != first || candidates[1].SandboxID != second {
		t.Fatalf("query changed order unpredictably: %+v", candidates)
	}
	candidates[0].SandboxID = "tampered"
	if reader.items[1].SandboxID != first {
		t.Fatal("mutated source metadata")
	}
	// Query only returns candidates; it must not silently pick one of two.
	if slices.ContainsFunc(candidates, func(p PlacementCandidate) bool { return p.Workspace != ref }) {
		t.Fatal("query returned candidate for another workspace")
	}
}

func TestPlacementQueryFailsClosedOnUnverifiedCandidates(t *testing.T) {
	ref, _ := workspace.ParseRef("wrk_0123456789abcdef0123456789abcdef")
	other, _ := workspace.ParseRef("wrk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	valid := PlacementCandidate{Workspace: ref, SandboxID: "sbx_11111111111111111111111111111111", TargetRoot: "/workspace"}
	for _, invalid := range []PlacementCandidate{
		{Workspace: other, SandboxID: valid.SandboxID, TargetRoot: "/workspace"},
		{Workspace: ref, SandboxID: "bad", TargetRoot: "/workspace"},
		{Workspace: ref, SandboxID: "sbx_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", TargetRoot: "/workspace"},
		{Workspace: ref, SandboxID: valid.SandboxID, TargetRoot: ""},
		{Workspace: ref, SandboxID: valid.SandboxID, TargetRoot: " /workspace"},
	} {
		reader := &staticPlacementReader{items: []PlacementCandidate{valid, invalid}}
		query, _ := NewPlacementQuery(reader)
		if got, err := query.List(context.Background(), ref); err == nil || got != nil {
			t.Errorf("accepted invalid candidate %+v", invalid)
		}
	}
	sentinel := errors.New("lookup unavailable")
	reader := &staticPlacementReader{err: sentinel}
	query, _ := NewPlacementQuery(reader)
	if got, err := query.List(context.Background(), ref); got != nil || !errors.Is(err, sentinel) {
		t.Fatal("reader error must fail closed")
	}
	if _, err := NewPlacementQuery(nil); err == nil {
		t.Fatal("nil reader was accepted")
	}
}
