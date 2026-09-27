package bootstrap

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	auditdomain "github.com/orz-i/Mender/backend/internal/audit/domain"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	invocationexecutionguard "github.com/orz-i/Mender/backend/internal/invocation/infrastructure/executionguard"
	mcpprovider "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/provider"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestGovernedRuntimeIntegrationMatrix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime, toolID, executor := governedRuntimeFixture(t, RuntimeOptions{
		DatabasePath:   filepath.Join(t.TempDir(), "mender.db"),
		DefaultTimeout: time.Second,
	})
	defer func() { _ = runtime.Close() }()

	allowedProfile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "allowed",
			DisplayName: "Allowed",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	allowedProfile, err = runtime.Profiles.AddTool.Execute(
		ctx,
		profileapplication.AddToolRequest{
			ProfileSlug: allowedProfile.Slug(),
			ToolID:      toolID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	restrictedProfile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "restricted",
			DisplayName: "Restricted",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	catalogContext := mustGovernanceInvocationContext(
		t,
		invocationdomain.OriginInternal,
		"matrix-catalog",
		invocationdomain.CatalogExecutionScope(),
	)
	validArgs := []byte("{\"count\":2}")

	response, err := runtime.InvokeTool.Execute(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: validArgs,
		Context:   catalogContext,
	})
	if err != nil {
		t.Fatalf("catalog Execute() error = %v", err)
	}
	if response.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("catalog outcome = %#v", response.Outcome)
	}
	if executor.Calls() != 1 {
		t.Fatalf("executor calls after catalog success = %d, want 1", executor.Calls())
	}

	invalidResponse, err := runtime.InvokeTool.Execute(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: []byte("{\"count\":\"two\"}"),
		Context:   catalogContext,
	})
	if err != nil {
		t.Fatalf("invalid input Execute() error = %v", err)
	}
	if invalidResponse.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		invalidResponse.Outcome.RejectionCode() != invocationdomain.RejectionInvalidArguments {
		t.Fatalf("invalid input outcome = %#v", invalidResponse.Outcome)
	}
	if executor.Calls() != 1 {
		t.Fatalf("invalid input reached executor; calls = %d", executor.Calls())
	}

	restrictedScope, err := invocationdomain.ProfileExecutionScope(restrictedProfile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	restrictedContext := mustGovernanceInvocationContext(
		t,
		invocationdomain.OriginMCP,
		"matrix:restricted",
		restrictedScope,
	)
	restrictedResponse, err := runtime.InvokeTool.Execute(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: validArgs,
		Context:   restrictedContext,
	})
	if err != nil {
		t.Fatalf("restricted Execute() error = %v", err)
	}
	if restrictedResponse.Outcome.Kind() != invocationdomain.OutcomeKindRejected ||
		restrictedResponse.Outcome.RejectionCode() != invocationdomain.RejectionToolOutsideScope {
		t.Fatalf("restricted outcome = %#v", restrictedResponse.Outcome)
	}
	if executor.Calls() != 1 {
		t.Fatalf("restricted scope reached executor; calls = %d", executor.Calls())
	}
	restrictedAudit, err := runtime.audit.Get(ctx, restrictedResponse.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if restrictedAudit.ProviderID() != nil ||
		restrictedAudit.Origin() != invocationdomain.OriginMCP ||
		restrictedAudit.ScopeKind() != invocationdomain.ScopeProfile ||
		restrictedAudit.ScopeReference() != restrictedProfile.ID().String() ||
		restrictedAudit.OutcomeKind() != invocationdomain.OutcomeKindRejected ||
		restrictedAudit.RejectionCode() != invocationdomain.RejectionToolOutsideScope {
		t.Fatalf("restricted audit = %#v", restrictedAudit)
	}

	allowedScope, err := invocationdomain.ProfileExecutionScope(allowedProfile.ID().String())
	if err != nil {
		t.Fatal(err)
	}
	allowedContext := mustGovernanceInvocationContext(
		t,
		invocationdomain.OriginMCP,
		"matrix:allowed",
		allowedScope,
	)
	allowedResponse, err := runtime.InvokeTool.Execute(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: validArgs,
		Context:   allowedContext,
	})
	if err != nil {
		t.Fatalf("allowed Profile Execute() error = %v", err)
	}
	if allowedResponse.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("allowed Profile outcome = %#v", allowedResponse.Outcome)
	}
	if executor.Calls() != 2 {
		t.Fatalf("executor calls after allowed Profile = %d, want 2", executor.Calls())
	}

	invalidOutput := completeGovernanceOutcome(t, "{\"ok\":\"yes\"}")
	executor.SetOutcome(invalidOutput)
	resultGuardResponse, err := runtime.InvokeTool.Execute(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: validArgs,
		Context:   catalogContext,
	})
	if err != nil {
		t.Fatalf("result guard Execute() error = %v", err)
	}
	if resultGuardResponse.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		resultGuardResponse.Outcome.FailureSource() != invocationdomain.FailureSourceResultGuard ||
		resultGuardResponse.Outcome.FailureCode() != invocationdomain.FailureOutputSchemaMismatch {
		t.Fatalf("result guard outcome = %#v", resultGuardResponse.Outcome)
	}

	toolFailure, err := invocationdomain.Failed(
		invocationdomain.FailureSourceTool,
		invocationdomain.FailureToolError,
		"tool rejected request",
	)
	if err != nil {
		t.Fatal(err)
	}
	executor.SetOutcome(toolFailure)
	toolFailureResponse, err := runtime.InvokeTool.Execute(ctx, invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: validArgs,
		Context:   catalogContext,
	})
	if err != nil {
		t.Fatalf("tool failure Execute() error = %v", err)
	}
	if toolFailureResponse.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		toolFailureResponse.Outcome.FailureSource() != invocationdomain.FailureSourceTool ||
		toolFailureResponse.Outcome.FailureCode() != invocationdomain.FailureToolError {
		t.Fatalf("tool failure outcome = %#v", toolFailureResponse.Outcome)
	}
}

func TestMCPToolFailureRoundTripsThroughMender(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	provider, err := runtime.Catalog.RegisterProvider.Execute(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "upstream",
			DisplayName: "Upstream",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	var upstreamCalls atomic.Int32
	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "governance-upstream", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:         "fail",
			InputSchema:  json.RawMessage("{\"type\":\"object\"}"),
			OutputSchema: json.RawMessage("{\"type\":\"object\"}"),
		},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			upstreamCalls.Add(1)
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "upstream application failure"},
				},
			}, nil
		},
	)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	upstreamSession, err := upstream.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upstreamSession.Close() })

	connection, err := mcpprovider.ConnectTransport(ctx, clientTransport)
	if err != nil {
		t.Fatal(err)
	}
	syncResult, err := runtime.AttachMCPProvider(ctx, provider.ID(), connection)
	if err != nil {
		_ = connection.Close()
		t.Fatal(err)
	}
	if syncResult.Total != 1 {
		t.Fatalf("sync result = %#v", syncResult)
	}

	search, err := runtime.Catalog.SearchTools.Execute(ctx, catalogapplication.SearchToolsRequest{
		Query: "fail",
		Limit: 1,
	})
	if err != nil || len(search) != 1 {
		t.Fatalf("search = %#v err=%v", search, err)
	}
	toolID := search[0].ToolID

	profile, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "default",
			DisplayName: "Default",
			Mode:        profiledomain.ExposureDirect,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	profile, err = runtime.Profiles.AddTool.Execute(
		ctx,
		profileapplication.AddToolRequest{
			ProfileSlug: profile.Slug(),
			ToolID:      toolID,
			Direct:      true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	downstream, err := runtime.NewMCPServer(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	downstreamClientTransport, downstreamServerTransport := mcp.NewInMemoryTransports()
	downstreamServerSession, err := downstream.MCPServer().Connect(ctx, downstreamServerTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = downstreamServerSession.Close() })

	client := mcp.NewClient(
		&mcp.Implementation{Name: "governance-downstream", Version: "1.0.0"},
		nil,
	)
	downstreamClient, err := client.Connect(ctx, downstreamClientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = downstreamClient.Close() })

	result, err := downstreamClient.CallTool(ctx, &mcp.CallToolParams{
		Name:      "upstream__fail",
		Arguments: json.RawMessage("{}"),
	})
	if err != nil {
		t.Fatalf("downstream CallTool() transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("downstream result IsError = false: %#v", result)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok ||
		structured["kind"] != "failed" ||
		structured["source"] != "tool" ||
		structured["code"] != "tool_error" {
		t.Fatalf("downstream failure = %#v", result.StructuredContent)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls.Load())
	}

	records, err := runtime.ListAudit.Execute(ctx, 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("audit records = %#v err=%v", records, err)
	}
	record := records[0]
	if record.OutcomeKind() != invocationdomain.OutcomeKindFailed ||
		record.FailureSource() != invocationdomain.FailureSourceTool ||
		record.FailureCode() != invocationdomain.FailureToolError ||
		record.ErrorClass() != auditdomain.ErrorExecution {
		t.Fatalf("audit record = %#v", record)
	}
}

func TestRuntimeProviderSerialGuardPreventsOverlap(t *testing.T) {
	t.Parallel()

	executionGuard := invocationexecutionguard.NewProviderSerial()
	ctx := context.Background()
	runtime, toolID, executor := governedRuntimeFixture(t, RuntimeOptions{
		DatabasePath:   filepath.Join(t.TempDir(), "mender.db"),
		DefaultTimeout: time.Second,
		ExecutionGuard: executionGuard,
	})
	defer func() { _ = runtime.Close() }()

	blocking := newSerialProbeExecutor(t)
	if err := runtime.RegisterExecutor(executor.providerID, blocking); err != nil {
		t.Fatal(err)
	}

	executionContext := mustGovernanceInvocationContext(
		t,
		invocationdomain.OriginInternal,
		"serial-test",
		invocationdomain.CatalogExecutionScope(),
	)
	request := invocationapplication.InvokeToolRequest{
		ToolID:    toolID,
		Arguments: []byte("{\"count\":1}"),
		Context:   executionContext,
	}

	errs := make(chan error, 2)
	go func() {
		_, err := runtime.InvokeTool.Execute(ctx, request)
		errs <- err
	}()

	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("first invocation did not enter executor")
	}

	go func() {
		_, err := runtime.InvokeTool.Execute(ctx, request)
		errs <- err
	}()

	select {
	case <-blocking.secondEntered:
		t.Fatal("second invocation entered executor before first released")
	case <-time.After(50 * time.Millisecond):
	}

	close(blocking.release)

	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent invocation error = %v", err)
		}
	}
	if blocking.MaxActive() != 1 {
		t.Fatalf("max active executor calls = %d, want 1", blocking.MaxActive())
	}
}

func governedRuntimeFixture(
	t *testing.T,
	options RuntimeOptions,
) (*Runtime, catalogdomain.ToolID, *governanceExecutor) {
	t.Helper()

	ctx := context.Background()
	runtime, err := NewRuntime(ctx, options)
	if err != nil {
		t.Fatal(err)
	}

	provider, err := runtime.Catalog.RegisterProvider.Execute(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "governed",
			DisplayName: "Governed",
		},
	)
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}

	syncTools := runtime.Catalog.SyncProviderTools(governanceSource{})
	if _, err := syncTools.Execute(ctx, provider.ID()); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	search, err := runtime.Catalog.SearchTools.Execute(ctx, catalogapplication.SearchToolsRequest{
		Query: "governed",
		Limit: 1,
	})
	if err != nil || len(search) != 1 {
		_ = runtime.Close()
		t.Fatalf("search = %#v err=%v", search, err)
	}

	executor := &governanceExecutor{
		providerID: provider.ID(),
		outcome:    completeGovernanceOutcome(t, "{\"ok\":true}"),
	}
	if err := runtime.RegisterExecutor(provider.ID(), executor); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	return runtime, search[0].ToolID, executor
}

type governanceSource struct{}

func (governanceSource) DiscoverTools(
	context.Context,
	catalogdomain.Provider,
) ([]catalogapplication.DiscoveredTool, error) {
	return []catalogapplication.DiscoveredTool{
		{
			SourceKey:    "governed",
			Name:         "governed",
			Title:        "Governed",
			Description:  "Governed test Tool",
			InputSchema:  []byte("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"type\":\"object\",\"properties\":{\"count\":{\"type\":\"integer\"}},\"required\":[\"count\"],\"additionalProperties\":false}"),
			OutputSchema: []byte("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"type\":\"object\",\"properties\":{\"ok\":{\"type\":\"boolean\"}},\"required\":[\"ok\"],\"additionalProperties\":false}"),
		},
	}, nil
}

type governanceExecutor struct {
	mu         sync.Mutex
	providerID catalogdomain.ProviderID
	outcome    invocationdomain.Outcome
	calls      int
}

func (e *governanceExecutor) Execute(
	ctx context.Context,
	_ invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver,
	_ invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	if err := ctx.Err(); err != nil {
		return invocationdomain.Outcome{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	return e.outcome, nil
}

func (e *governanceExecutor) Calls() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func (e *governanceExecutor) SetOutcome(outcome invocationdomain.Outcome) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.outcome = outcome
}

type serialProbeExecutor struct {
	outcome       invocationdomain.Outcome
	entered       chan struct{}
	secondEntered chan struct{}
	release       chan struct{}
	active        atomic.Int32
	maxActive     atomic.Int32
	calls         atomic.Int32
}

func newSerialProbeExecutor(t *testing.T) *serialProbeExecutor {
	t.Helper()
	return &serialProbeExecutor{
		outcome:       completeGovernanceOutcome(t, "{\"ok\":true}"),
		entered:       make(chan struct{}, 1),
		secondEntered: make(chan struct{}, 1),
		release:       make(chan struct{}),
	}
}

func (e *serialProbeExecutor) Execute(
	ctx context.Context,
	_ invocationapplication.ExecutionRequest,
	_ invocationapplication.CredentialResolver,
	_ invocationapplication.ProgressReporter,
) (invocationdomain.Outcome, error) {
	call := e.calls.Add(1)
	active := e.active.Add(1)
	defer e.active.Add(-1)
	for {
		current := e.maxActive.Load()
		if active <= current || e.maxActive.CompareAndSwap(current, active) {
			break
		}
	}

	if call == 1 {
		e.entered <- struct{}{}
	} else {
		e.secondEntered <- struct{}{}
	}

	select {
	case <-ctx.Done():
		return invocationdomain.Outcome{}, ctx.Err()
	case <-e.release:
		return e.outcome, nil
	}
}

func (e *serialProbeExecutor) MaxActive() int32 {
	return e.maxActive.Load()
}

func mustGovernanceInvocationContext(
	t *testing.T,
	origin invocationdomain.Origin,
	caller string,
	scope invocationdomain.ExecutionScope,
) invocationdomain.InvocationContext {
	t.Helper()
	executionContext, err := invocationdomain.NewInvocationContext(origin, caller, scope)
	if err != nil {
		t.Fatal(err)
	}
	return executionContext
}

func completeGovernanceOutcome(t *testing.T, raw string) invocationdomain.Outcome {
	t.Helper()
	value, err := invocationdomain.NewJSONValue([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := invocationdomain.Complete(value)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}
