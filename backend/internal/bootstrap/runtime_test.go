package bootstrap

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/orz-i/Mender/backend/internal/catalog/application"
	catalogdomain "github.com/orz-i/Mender/backend/internal/catalog/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
	"github.com/orz-i/Mender/backend/internal/invocation/infrastructure/builtin"
)

func TestRuntimeExecutesBuiltInToolThroughFullPipeline(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "mender.db")
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{
		DatabasePath:   databasePath,
		DefaultTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			if err := runtime.Close(); err != nil {
				t.Errorf("Runtime.Close() error = %v", err)
			}
		}
	})

	provider, err := runtime.Catalog.RegisterProvider.Execute(
		context.Background(),
		application.RegisterProviderRequest{
			Namespace:   "builtin",
			DisplayName: "Built-in",
		},
	)
	if err != nil {
		t.Fatalf("RegisterProvider.Execute() error = %v", err)
	}

	syncTools := runtime.Catalog.SyncProviderTools(staticSource{
		tools: []application.DiscoveredTool{
			{
				SourceKey:   "echo",
				Name:        "echo",
				Title:       "Echo",
				Description: "Return JSON input unchanged",
				InputSchema: []byte(`{"type":"object"}`),
				Tags:        []string{"builtin", "test"},
			},
		},
	})
	if _, err := syncTools.Execute(context.Background(), provider.ID()); err != nil {
		t.Fatalf("SyncProviderTools.Execute() error = %v", err)
	}

	if err := runtime.RegisterExecutor(provider.ID(), builtin.EchoExecutor{}); err != nil {
		t.Fatalf("RegisterExecutor() error = %v", err)
	}

	searchResults, err := runtime.Catalog.SearchTools.Execute(
		context.Background(),
		application.SearchToolsRequest{
			Query: "echo",
			Limit: 1,
		},
	)
	if err != nil {
		t.Fatalf("SearchTools.Execute() error = %v", err)
	}
	if len(searchResults) != 1 {
		t.Fatalf("SearchTools results = %+v", searchResults)
	}

	const sensitivePayload = "SENSITIVE-PAYLOAD-NOT-FOR-AUDIT"
	arguments := []byte(`{"value":"` + sensitivePayload + `"}`)
	response, err := runtime.InvokeTool.Execute(
		context.Background(),
		invocationapplication.InvokeToolRequest{
			ToolID:    searchResults[0].ToolID,
			Arguments: arguments,
			Caller:    "bootstrap-test",
		},
	)
	if err != nil {
		t.Fatalf("InvokeTool.Execute() error = %v", err)
	}
	if response.Result.Kind() != invocationdomain.ResultKindComplete {
		t.Fatalf("Result kind = %q", response.Result.Kind())
	}
	if !bytes.Equal(response.Result.Content().Bytes(), arguments) {
		t.Fatalf("Result = %s, want %s", response.Result.Content().Bytes(), arguments)
	}

	record, err := runtime.audit.Get(context.Background(), response.InvocationID)
	if err != nil {
		t.Fatalf("audit.Get() error = %v", err)
	}
	if record.ToolID() != searchResults[0].ToolID {
		t.Fatalf("Audit ToolID = %q, want %q", record.ToolID(), searchResults[0].ToolID)
	}
	if record.ProviderID() == nil || *record.ProviderID() != provider.ID() {
		t.Fatalf("Audit ProviderID = %v, want %q", record.ProviderID(), provider.ID())
	}

	// Close before inspecting file bytes so SQLite has released the file.
	if err := runtime.Close(); err != nil {
		t.Fatalf("Runtime.Close() before file inspection error = %v", err)
	}
	closed = true
	//nolint:gosec // databasePath is created under t.TempDir for this integration test.
	contents, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if bytes.Contains(contents, []byte(sensitivePayload)) {
		t.Fatal("SQLite file contains invocation payload that should not be audited")
	}
}

type staticSource struct {
	tools []application.DiscoveredTool
}

func (s staticSource) DiscoverTools(
	context.Context,
	catalogdomain.Provider,
) ([]application.DiscoveredTool, error) {
	return s.tools, nil
}
