package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
)

func TestSSHRecoveryHTTPIsOwnerAdministrativeRead(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "jobs-recovery.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	srv := httptest.NewServer(New(runtime))
	defer srv.Close()
	requestJSON(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/sandboxes/invalid/jobs/recovery", nil, http.StatusBadRequest)
	requestJSON(t, srv.Client(), http.MethodGet, srv.URL+"/api/v1/sandboxes/sbx_00000000000000000000000000000011/jobs/recovery", nil, http.StatusNotFound)
}
