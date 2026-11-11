package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	governance "github.com/orz-i/Mender/backend/internal/governance/domain"
	invocationapp "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocation "github.com/orz-i/Mender/backend/internal/invocation/domain"
	sandbox "github.com/orz-i/Mender/backend/internal/sandbox/domain"
	"github.com/orz-i/Mender/backend/internal/toolsuite/infrastructure/curated"
	workspaceapp "github.com/orz-i/Mender/backend/internal/workspace/application"
	workspace "github.com/orz-i/Mender/backend/internal/workspace/domain"
)

func TestNativeWorkspaceToolRequiresFrozenPlacementBeforeAcquisition(t *testing.T) {
	tools, err := curated.PlannedNativeCatalogTools(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.SourceKey() != "workspace.read" {
			continue
		}
		args, err := invocation.NewJSONValue([]byte(`{"path":"main.go"}`))
		if err != nil {
			t.Fatal(err)
		}
		_, err = (NativeWorkspaceTools{}).Execute(context.Background(),
			invocationapp.ExecutionRequest{Tool: tool, Arguments: args}, nil, nil)
		if !errors.Is(err, sandbox.ErrNotAuthorized) {
			t.Fatalf("missing pre-Policy FrozenPlacement: %v", err)
		}
		return
	}
	t.Fatal("missing reviewed workspace.read Tool")
}

func TestNativeWorkspaceStripsOnlyRoutingFieldAndRetainsOriginalArgumentLimits(t *testing.T) {
	raw := []byte(`{"workspaceId":"wrk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","path":"source/main.go","offset":3}`)
	original, err := invocation.NewJSONValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	stripped, err := stripNativeWorkspaceID(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(stripped.Bytes()) != `{"offset":3,"path":"source/main.go"}` {
		t.Fatalf("stripped validated arguments = %s", stripped.Bytes())
	}
	if len(stripped.Bytes()) >= len(original.Bytes()) {
		t.Fatal("routing-only workspaceId was not removed")
	}
	session := &typedSessionFixture{}
	closed := 0
	lease := workspaceapp.NewLease(session, func() error { closed++; return nil }, workspaceapp.ExecutionConstraints{})
	result, err := executeTypedWithArgumentSize(context.Background(), lease, workspace.Read, stripped, 4096, len(original.Bytes()))
	if err != nil || result.Kind() != invocation.OutcomeKindComplete || session.calls != 1 || closed != 1 {
		t.Fatalf("native Workspace adapter lost operation/cleanup: result=%+v err=%v calls=%d close=%d", result, err, session.calls, closed)
	}
	if session.read.CanonicalBytes != len(original.Bytes()) || session.read.Offset != 3 ||
		session.read.Path != "source/main.go" {
		t.Fatalf("typed request dropped original canonical size or input: %+v", session.read)
	}
	// A backend argument budget applies to the *original* canonical bytes,
	// not the shorter payload after an optional Workspace selector was removed.
	session = &typedSessionFixture{}
	closed = 0
	lease = workspaceapp.NewLease(session, func() error { closed++; return nil }, workspaceapp.ExecutionConstraints{
		MaxArgumentBytes: len(stripped.Bytes()),
	})
	result, err = executeTypedWithArgumentSize(context.Background(), lease, workspace.Read, stripped, 4096, len(original.Bytes()))
	if err != nil || result.FailureCode() != invocation.FailureToolError || session.calls != 0 || closed != 1 {
		t.Fatalf("argument-size bypass: result=%+v err=%v calls=%d close=%d", result, err, session.calls, closed)
	}
}

func TestNativeWorkspaceSelectionNeverAcceptsUnverifiedWorkspaceID(t *testing.T) {
	tools, err := curated.PlannedNativeCatalogTools(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.SourceKey() != "workspace.read" {
			continue
		}
		for _, input := range []string{
			`{"workspaceId":"../../host","path":"main.go"}`,
			`{"workspaceId":"wrk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab","path":"main.go"}`,
		} {
			args, err := invocation.NewJSONValue([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			_, err = (NativeWorkspaceTools{}).Extract(context.Background(), tool, args,
				catalogCaller(t))
			if err == nil {
				t.Fatalf("unregistered workspace selected: %s", input)
			}
		}
		return
	}
	t.Fatal("missing native Workspace catalog operation")
}

func catalogCaller(t *testing.T) governance.Context {
	t.Helper()
	p, err := governance.NewPrincipal(governance.PrincipalLocalUser, "native-workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	caller, err := governance.NewContext(governance.OriginCLI, p, governance.CatalogScope())
	if err != nil {
		t.Fatal(err)
	}
	return caller
}
