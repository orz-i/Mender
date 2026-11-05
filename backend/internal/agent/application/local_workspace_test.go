package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	agentdomain "github.com/orz-i/Mender/backend/internal/agent/domain"
	agentsqlite "github.com/orz-i/Mender/backend/internal/agent/infrastructure/sqlite"
	gov "github.com/orz-i/Mender/backend/internal/governance/domain"
)

type workspaceVerifierFixture struct {
	resolve   func(context.Context, string) (string, error)
	directory func(context.Context, string) (bool, error)
	regular   func(context.Context, string) (bool, error)
}

func (v workspaceVerifierFixture) ResolvePath(ctx context.Context, path string) (string, error) {
	return v.resolve(ctx, path)
}
func (v workspaceVerifierFixture) IsDirectory(ctx context.Context, path string) (bool, error) {
	return v.directory(ctx, path)
}
func (v workspaceVerifierFixture) IsRegularFile(ctx context.Context, path string) (bool, error) {
	return v.regular(ctx, path)
}

func TestAgentCreationRequiresInjectedWorkspaceVerifierAfterOwnerCheck(t *testing.T) {
	ctx := context.Background()
	store, err := agentsqlite.Open(ctx, filepath.Join(t.TempDir(), "verifier.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	caller := owner(t)
	admit := func(context.Context, gov.Context, agentdomain.Agent, gov.Action) error { return nil }
	if service, err := NewService(store, fakeExecutor{}, admit, caller.Principal(), nil, nil); err == nil {
		_ = service.Close()
		t.Fatal("Agent service accepted a missing local workspace verifier")
	}
	// This directory is deliberately absent. A successful create proves that
	// application uses its supplied mechanism rather than a filesystem fallback.
	root := filepath.Join(t.TempDir(), "owner-selected-virtual-directory")
	var resolutions, directoryChecks int
	verifier := workspaceVerifierFixture{
		resolve: func(_ context.Context, path string) (string, error) {
			resolutions++
			if path != root {
				t.Fatalf("unexpected selected directory %q", path)
			}
			return root, nil
		},
		directory: func(_ context.Context, path string) (bool, error) {
			directoryChecks++
			if path != root {
				t.Fatalf("unexpected canonical directory %q", path)
			}
			return true, nil
		},
	}
	service, err := NewService(store, fakeExecutor{}, admit, caller.Principal(), nil, verifier)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = service.Close() }()
	request := CreateRequest{Name: "Verifier fixture", Backend: agentdomain.BackendOpenCode, WorkspaceRoot: root}
	if _, err := service.Create(ctx, request, profile(t, goodProfileID)); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("Profile selected a local directory: %v", err)
	}
	if resolutions != 0 || directoryChecks != 0 {
		t.Fatal("unauthorized creation inspected the local workspace")
	}
	agent, err := service.Create(ctx, request, caller)
	if err != nil || agent.Spec().WorkspaceRoot != root {
		t.Fatalf("injected workspace was not used: root=%q err=%v", agent.Spec().WorkspaceRoot, err)
	}
	if resolutions != 1 || directoryChecks != 1 {
		t.Fatalf("unexpected verification: resolutions=%d directory checks=%d", resolutions, directoryChecks)
	}
}

func TestWorkspaceRulesRejectRedirectedRootAndPreviewOutsideApprovedRoot(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "approved")
	other := filepath.Join(t.TempDir(), "replacement")
	verifier := workspaceVerifierFixture{
		resolve:   func(context.Context, string) (string, error) { return other, nil },
		directory: func(context.Context, string) (bool, error) { return true, nil },
		regular: func(context.Context, string) (bool, error) {
			t.Fatal("redirected preview reached a file type query")
			return true, nil
		},
	}
	rules := &localWorkspaceRules{workspace: verifier}
	if err := rules.validateRegisteredWorkspaceRoot(ctx, root); !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatalf("redirected approved root accepted: %v", err)
	}
	if err := rules.revalidateReadPreview(ctx, root, "file.txt"); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("redirected preview root accepted: %v", err)
	}
	verifier.resolve = func(_ context.Context, path string) (string, error) {
		if path == root {
			return root, nil
		}
		return filepath.Join(other, "file.txt"), nil
	}
	rules.workspace = verifier
	if err := rules.revalidateReadPreview(ctx, root, "file.txt"); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("preview resolved outside approved root: %v", err)
	}
}

func TestWorkspaceRulesPreserveUnavailableAndForbiddenOnMechanismFailure(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "approved")
	unavailable := errors.New("filesystem fixture unavailable")
	verifier := workspaceVerifierFixture{
		resolve: func(context.Context, string) (string, error) { return "", unavailable },
	}
	rules := &localWorkspaceRules{workspace: verifier}
	if _, err := rules.canonicalLocalWorkspace(ctx, root); !errors.Is(err, agentdomain.ErrInvalid) || !errors.Is(err, unavailable) {
		t.Fatalf("owner registration lost its error category or cause: %v", err)
	}
	if err := rules.validateRegisteredWorkspaceRoot(ctx, root); !errors.Is(err, agentdomain.ErrUnavailable) {
		t.Fatalf("registered root failure did not fail unavailable: %v", err)
	}
	if err := rules.revalidateReadPreview(ctx, root, "file.txt"); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("preview resolution failure did not fail forbidden: %v", err)
	}
	verifier.resolve = func(_ context.Context, path string) (string, error) { return path, nil }
	verifier.regular = func(context.Context, string) (bool, error) { return false, unavailable }
	rules.workspace = verifier
	if err := rules.revalidateReadPreview(ctx, root, "file.txt"); !errors.Is(err, agentdomain.ErrForbidden) {
		t.Fatalf("preview type failure did not fail forbidden: %v", err)
	}
}
