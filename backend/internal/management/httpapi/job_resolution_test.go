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

func TestSSHRecoveryResolveHTTPIsExplicitOwnerOnlyAndRejectsMalformedInput(t *testing.T) {
	ctx := context.Background()
	runtime, err := bootstrap.NewRuntime(ctx, bootstrap.RuntimeOptions{DatabasePath: filepath.Join(t.TempDir(), "recovery-api.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	server := httptest.NewServer(New(runtime))
	defer server.Close()
	base := server.URL + "/api/v1/sandboxes/sbx_" + strings.Repeat("a", 32) + "/jobs/recovery/"
	validID := "sjob_" + strings.Repeat("f", 32)
	good := map[string]any{"fingerprint": strings.Repeat("b", 64), "generation": 1, "reason": "reviewed prelaunch evidence"}
	requestJSON(t, server.Client(), http.MethodPost, base+"not-a-task/resolve", good, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, base+validID+"/resolve", map[string]any{"generation": 1, "fingerprint": good["fingerprint"]}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, base+validID+"/resolve", map[string]any{"generation": 1, "fingerprint": good["fingerprint"], "reason": "ok"}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, base+validID+"/resolve", map[string]any{
		"generation": 1, "fingerprint": good["fingerprint"], "reason": good["reason"], "command": "echo unsafe"}, http.StatusBadRequest)
	requestJSON(t, server.Client(), http.MethodPost, base+validID+"/resolve", good, http.StatusNotFound)
}
