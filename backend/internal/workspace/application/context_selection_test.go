package application

import (
	"context"
	"errors"
	"testing"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

const (
	wsA = "wrk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	wsB = "wrk_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeWorkspaceInventory struct {
	accessible []workspace.Ref
	preferred  workspace.Ref
	hasDefault bool
	fails      error
	lists      int
	defaults   int
}

func (f *fakeWorkspaceInventory) ListAccessibleWorkspaces(context.Context, governance.Context) ([]workspace.Ref, error) {
	f.lists++
	return append([]workspace.Ref(nil), f.accessible...), f.fails
}
func (f *fakeWorkspaceInventory) DefaultWorkspace(context.Context, governance.Context) (workspace.Ref, bool, error) {
	f.defaults++
	return f.preferred, f.hasDefault, f.fails
}
func workspaceSelectionCaller(t *testing.T) governance.Context {
	t.Helper()
	p, err := governance.NewPrincipal(governance.PrincipalLocalUser, "owner")
	if err != nil {
		t.Fatal(err)
	}
	c, err := governance.NewContext(governance.OriginCLI, p, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestWorkspaceContextSelectionOnlyUsesApprovedIdentities(t *testing.T) {
	ctx := context.Background()
	caller := workspaceSelectionCaller(t)
	a, _ := workspace.ParseRef(wsA)
	b, _ := workspace.ParseRef(wsB)
	for _, tc := range []struct {
		name       string
		accessible []workspace.Ref
		requested  workspace.Ref
		def        workspace.Ref
		hasDefault bool
		want       workspace.Ref
		wantErr    error
	}{
		{"explicit is visible", []workspace.Ref{b, a}, b, "", false, b, nil},
		{"sole eligible", []workspace.Ref{a}, "", "", false, a, nil},
		{"explicit default", []workspace.Ref{a, b}, "", b, true, b, nil},
		{"multiple without default", []workspace.Ref{b, a}, "", "", false, "", ErrWorkspaceAmbiguous},
		{"no accessible workspaces", nil, "", "", false, "", ErrWorkspaceUnavailable},
		{"explicit absent", []workspace.Ref{a}, b, "", false, "", ErrWorkspaceUnavailable},
		{"revoked default", []workspace.Ref{a}, "", b, true, "", ErrWorkspaceUnavailable},
		{"invalid default state", []workspace.Ref{a}, "", b, false, "", ErrWorkspaceUnavailable},
		{"duplicate inventory", []workspace.Ref{a, a}, "", "", false, "", ErrWorkspaceUnavailable},
		{"malformed inventory", []workspace.Ref{"invalid"}, "", "", false, "", ErrWorkspaceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeWorkspaceInventory{accessible: tc.accessible, preferred: tc.def, hasDefault: tc.hasDefault}
			s, err := NewWorkspaceSelection(db)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Resolve(ctx, caller, tc.requested)
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %s, %v expected %s, %v", got, err, tc.want, tc.wantErr)
			}
			if tc.requested != "" && db.defaults != 0 {
				t.Fatal("explicit selection consulted mutable default")
			}
		})
	}
}
func TestWorkspaceContextSelectionFailsClosedWithoutInventory(t *testing.T) {
	a, _ := workspace.ParseRef(wsA)
	if _, err := NewWorkspaceSelection(nil); err == nil {
		t.Fatal("nil inventory accepted")
	}
	s := WorkspaceSelection{}
	if _, err := s.Resolve(context.Background(), workspaceSelectionCaller(t), a); !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatal(err)
	}
	db := &fakeWorkspaceInventory{accessible: []workspace.Ref{a}, fails: context.DeadlineExceeded}
	s, _ = NewWorkspaceSelection(db)
	if _, err := s.Resolve(context.Background(), workspaceSelectionCaller(t), ""); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Resolve(cancelled, workspaceSelectionCaller(t), ""); !errors.Is(err, context.Canceled) || db.lists != 1 {
		t.Fatal("cancellation reached inventory")
	}
	if _, err := s.Resolve(context.Background(), governance.Context{}, ""); !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatal("untrusted caller accepted")
	}
	if _, err := s.Resolve(context.Background(), workspaceSelectionCaller(t), "../project"); !errors.Is(err, ErrWorkspaceUnavailable) {
		t.Fatal("raw workspace path accepted")
	}
}
