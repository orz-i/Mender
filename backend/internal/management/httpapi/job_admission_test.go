package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestSSHJobPauseHTTPRequiresTypedExplicitMutation(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "job-pause.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	srv := httptest.NewServer(New(runtime))
	defer srv.Close()
	base := srv.URL + "/api/v1/sandboxes/sbx_00000000000000000000000000000011/jobs/admission"
	requestJSON(t, srv.Client(), http.MethodGet, base, nil, http.StatusNotFound)
	requestJSON(t, srv.Client(), http.MethodPut, base, map[string]any{}, http.StatusBadRequest)
	requestJSON(t, srv.Client(), http.MethodPut, base, map[string]any{"paused": "yes"}, http.StatusBadRequest)
	requestJSON(t, srv.Client(), http.MethodPut, base, map[string]any{"paused": true}, http.StatusNotFound)
}
