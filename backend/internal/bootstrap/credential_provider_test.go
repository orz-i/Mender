package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	catalogapplication "github.com/orz-i/Mender/backend/internal/catalog/application"
	credentialapplication "github.com/orz-i/Mender/backend/internal/credential/application"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	mcpconfigstore "github.com/orz-i/Mender/backend/internal/mcp/infrastructure/configstore"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

func TestCredentialBackedHTTPProviderEnforcesProfileScope(t *testing.T) {
	t.Setenv("MENDER_PROVIDER_TOKEN", "provider-secret")

	ctx := context.Background()
	var authSeen atomic.Bool
	var upstreamCalls atomic.Int32

	upstream := mcp.NewServer(
		&mcp.Implementation{Name: "credential-provider-test", Version: "1.0.0"},
		nil,
	)
	upstream.AddTool(
		&mcp.Tool{
			Name:         "echo",
			InputSchema:  json.RawMessage("{\"type\":\"object\"}"),
			OutputSchema: json.RawMessage("{\"type\":\"object\"}"),
		},
		func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			upstreamCalls.Add(1)
			var input map[string]any
			if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
				return nil, err
			}
			return &mcp.CallToolResult{StructuredContent: input}, nil
		},
	)
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return upstream },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-secret" {
			http.Error(w, "missing authentication", http.StatusUnauthorized)
			return
		}
		authSeen.Store(true)
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	databasePath := filepath.Join(t.TempDir(), "mender.db")
	runtime, err := NewRuntime(ctx, RuntimeOptions{
		DatabasePath:   databasePath,
		DefaultTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	credential, err := runtime.Credentials.CreateEnvironment.Execute(
		ctx,
		credentialapplication.CreateEnvironmentRequest{
			Namespace:           "upstream",
			Name:                "work",
			DisplayName:         "Upstream Work",
			EnvironmentVariable: "MENDER_PROVIDER_TOKEN",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := credential.Reference()

	provider, err := runtime.RegisterMCPProvider(
		ctx,
		catalogapplication.RegisterProviderRequest{
			Namespace:   "upstream",
			DisplayName: "Upstream",
		},
		mcpconfigstore.Config{
			Transport: mcpconfigstore.TransportHTTP,
			Endpoint:  httpServer.URL,
			CredentialHeaders: []mcpconfigstore.CredentialHeader{
				{
					Name:      "Authorization",
					Scheme:    "Bearer",
					Reference: ref,
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.SyncConfiguredMCPProvider(ctx, provider.ID()); err != nil {
		t.Fatal(err)
	}
	if !authSeen.Load() {
		t.Fatal("credential-backed Authorization header was not observed")
	}

	search, err := runtime.Catalog.SearchTools.Execute(
		ctx,
		catalogapplication.SearchToolsRequest{Query: "echo", Limit: 10},
	)
	if err != nil || len(search) != 1 {
		t.Fatalf("search = %#v err=%v", search, err)
	}
	toolID := search[0].ToolID

	allowed, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "allowed-credential",
			DisplayName: "Allowed Credential",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err = runtime.Profiles.AddTool.Execute(
		ctx,
		profileapplication.AddToolRequest{
			ProfileSlug:    allowed.Slug(),
			ToolID:         toolID,
			CredentialRefs: []credentialdomain.Reference{ref},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := runtime.Credentials.Delete.Execute(ctx, ref); !errors.Is(
		err,
		credentialapplication.ErrCredentialInUse,
	) {
		t.Fatalf("delete in-use Credential error = %v", err)
	}

	allowedScope, _ := invocationdomain.ProfileExecutionScope(allowed.ID().String())
	allowedContext, _ := invocationdomain.NewInvocationContext(
		invocationdomain.OriginMCP,
		"credential-allowed",
		allowedScope,
	)
	allowedResponse, err := runtime.InvokeTool.Execute(
		ctx,
		invocationapplication.InvokeToolRequest{
			ToolID:    toolID,
			Arguments: []byte("{\"path\":\"allowed\"}"),
			Context:   allowedContext,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if allowedResponse.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("allowed outcome = %#v", allowedResponse.Outcome)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("upstream calls after allowed profile = %d, want 1", upstreamCalls.Load())
	}

	denied, err := runtime.Profiles.Create.Execute(
		ctx,
		profileapplication.CreateProfileRequest{
			Slug:        "denied-credential",
			DisplayName: "Denied Credential",
			Mode:        profiledomain.ExposureDynamic,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	denied, err = runtime.Profiles.AddTool.Execute(
		ctx,
		profileapplication.AddToolRequest{
			ProfileSlug: denied.Slug(),
			ToolID:      toolID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	deniedScope, _ := invocationdomain.ProfileExecutionScope(denied.ID().String())
	deniedContext, _ := invocationdomain.NewInvocationContext(
		invocationdomain.OriginMCP,
		"credential-denied",
		deniedScope,
	)
	deniedResponse, err := runtime.InvokeTool.Execute(
		ctx,
		invocationapplication.InvokeToolRequest{
			ToolID:    toolID,
			Arguments: []byte("{\"path\":\"denied\"}"),
			Context:   deniedContext,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if deniedResponse.Outcome.Kind() != invocationdomain.OutcomeKindFailed ||
		deniedResponse.Outcome.FailureCode() != invocationdomain.FailureCredentialOutsideScope {
		t.Fatalf("denied outcome = %#v", deniedResponse.Outcome)
	}
	if upstreamCalls.Load() != 1 {
		t.Fatalf("denied profile reached upstream; calls = %d", upstreamCalls.Load())
	}

	catalogContext, _ := invocationdomain.NewInvocationContext(
		invocationdomain.OriginInternal,
		"credential-catalog",
		invocationdomain.CatalogExecutionScope(),
	)
	catalogResponse, err := runtime.InvokeTool.Execute(
		ctx,
		invocationapplication.InvokeToolRequest{
			ToolID:    toolID,
			Arguments: []byte("{\"path\":\"catalog\"}"),
			Context:   catalogContext,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if catalogResponse.Outcome.Kind() != invocationdomain.OutcomeKindComplete {
		t.Fatalf("catalog outcome = %#v", catalogResponse.Outcome)
	}
	if upstreamCalls.Load() != 2 {
		t.Fatalf("upstream calls after Catalog scope = %d, want 2", upstreamCalls.Load())
	}

	records, err := runtime.ListAudit.Execute(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("audit records = %d, want 3", len(records))
	}
	assertAuditSchemaHasNoCredentialFields(t, databasePath)
}

func assertAuditSchemaHasNoCredentialFields(t *testing.T, databasePath string) {
	t.Helper()
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query("PRAGMA table_info(invocation_audit)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue any
			primaryKey   int
		)
		if err := rows.Scan(
			&cid,
			&name,
			&columnType,
			&notNull,
			&defaultValue,
			&primaryKey,
		); err != nil {
			t.Fatal(err)
		}
		normalized := strings.ToLower(name)
		if strings.Contains(normalized, "credential") ||
			strings.Contains(normalized, "secret") {
			t.Fatalf("Audit schema contains forbidden column %q", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
