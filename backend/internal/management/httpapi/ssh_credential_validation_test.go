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

func TestSSHInvalidPrivateKeyIsClientValidationError(t *testing.T) {
	runtime, err := bootstrap.NewRuntime(context.Background(), bootstrap.RuntimeOptions{
		DatabasePath: filepath.Join(t.TempDir(), "mender.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	server := httptest.NewServer(New(runtime))
	t.Cleanup(server.Close)

	// #nosec G101 -- intentionally malformed non-secret test fixture for response redaction.
	const secret = "PRIVATE-KEY-FIXTURE-MUST-NOT-LEAK"
	result := requestJSON(t, server.Client(), http.MethodPost,
		server.URL+"/api/v1/credentials/native", map[string]any{
			"namespace": "ssh", "name": "invalid", "displayName": "Invalid",
			"secret": secret, "sshKeyPassphrase": "import-only-test",
		}, http.StatusBadRequest)
	if strings.Contains(recorderJSON(result), secret) || strings.Contains(recorderJSON(result), "import-only-test") {
		t.Fatal("SSH import validation response leaked secret material")
	}
	if result["code"] != "validation_error" {
		t.Fatalf("SSH invalid key API result = %#v; expected validation_error", result)
	}
}
