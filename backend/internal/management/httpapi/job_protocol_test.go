package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestSSHJobProtocolActivationIsOwnerOnlyAndRequiresConfirmation(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "protocol-api.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	path := server.URL + "/api/v1/sandboxes/sbx_" + strings.Repeat("a", 32) + "/jobs/protocol/activate-v2"
	requestJSON(t, server.Client(), http.MethodPost, path, map[string]any{}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, path, map[string]any{"confirm": false}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, path, map[string]any{"confirm": true, "command": "whoami"}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, path, map[string]any{"confirm": true}, http.StatusNotFound)
}
