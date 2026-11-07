package application

import (
	"context"
	"errors"
	"testing"

	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

type selectionInspector struct {
	allowed map[string]bool
	seen    []string
	err     error
}

func (s *selectionInspector) InspectPlacement(_ context.Context, operation workspace.Operation, candidate PlacementCandidate) (bool, error) {
	s.seen = append(s.seen, candidate.SandboxID)
	if s.err != nil {
		return false, s.err
	}
	return s.allowed[candidate.SandboxID], nil
}

func TestSelectWorkspacePlacementRequiresUniqueAuthorizedTarget(t *testing.T) {
	ref, _ := workspace.ParseRef("wrk_0123456789abcdef0123456789abcdef")
	a := "sbx_11111111111111111111111111111111"
	b := "sbx_22222222222222222222222222222222"
	q, _ := NewPlacementQuery(&staticPlacementReader{items: []PlacementCandidate{
		{Workspace: ref, SandboxID: b, TargetRoot: "/workspace"},
		{Workspace: ref, SandboxID: a, TargetRoot: "/workspace"},
	}})
	ctx := context.Background()
	inspect := &selectionInspector{allowed: map[string]bool{a: true, b: true}}
	if _, err := q.Select(ctx, ref, workspace.Read, "", inspect); !errors.Is(err, ErrPlacementAmbiguous) {
		t.Fatalf("two authorized placements should be ambiguous: %v", err)
	}
	if len(inspect.seen) != 2 || inspect.seen[0] != a || inspect.seen[1] != b {
		t.Fatalf("expected deterministic inspection order, got %v", inspect.seen)
	}
	inspect.seen = nil
	if chosen, err := q.Select(ctx, ref, workspace.Read, b, inspect); err != nil || chosen.SandboxID != b {
		t.Fatalf("exact pinned selection: %+v: %v", chosen, err)
	}
	if len(inspect.seen) != 1 || inspect.seen[0] != b {
		t.Fatalf("pinned selection inspected unauthorized alternatives: %v", inspect.seen)
	}
}

func TestSelectWorkspacePlacementNeverFallsBackOrSuppressesErrors(t *testing.T) {
	ref, _ := workspace.ParseRef("wrk_0123456789abcdef0123456789abcdef")
	a := "sbx_11111111111111111111111111111111"
	b := "sbx_22222222222222222222222222222222"
	q, _ := NewPlacementQuery(&staticPlacementReader{items: []PlacementCandidate{
		{Workspace: ref, SandboxID: a, TargetRoot: "/workspace"},
		{Workspace: ref, SandboxID: b, TargetRoot: "/workspace"},
	}})
	for _, tc := range []struct {
		name, pinned string
		allowed      map[string]bool
	}{
		{"pinned denied but alternative eligible", a, map[string]bool{b: true}},
		{"pinned unknown but alternative eligible", "sbx_cccccccccccccccccccccccccccccccc", map[string]bool{b: true}},
		{"no eligible placement", "", map[string]bool{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inspect := &selectionInspector{allowed: tc.allowed}
			if _, err := q.Select(context.Background(), ref, workspace.Read, tc.pinned, inspect); !errors.Is(err, ErrPlacementUnavailable) {
				t.Fatalf("unexpected placement outcome: %v", err)
			}
		})
	}
	inspect := &selectionInspector{allowed: map[string]bool{a: true, b: true}, err: context.DeadlineExceeded}
	if _, err := q.Select(context.Background(), ref, workspace.Edit, "", inspect); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("uncertain inspection should fail closed, got %v", err)
	}
	if _, err := q.Select(context.Background(), ref, "unverified", "", inspect); !errors.Is(err, ErrPlacementUnavailable) || len(inspect.seen) != 1 {
		t.Fatal("unsupported operation should not inspect")
	}
	if _, err := q.Select(context.Background(), ref, workspace.Read, "", nil); !errors.Is(err, ErrPlacementUnavailable) {
		t.Fatal("nil inspector was accepted")
	}
	if _, err := q.Select(context.Background(), "", workspace.Read, "", inspect); !errors.Is(err, ErrPlacementUnavailable) {
		t.Fatal("missing Workspace ref was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.Select(ctx, ref, workspace.Read, "", inspect); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled preflight was accepted")
	}
}
