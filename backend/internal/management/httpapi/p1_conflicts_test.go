package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/orz-i/Mender/backend/internal/bootstrap"
	policyapplication "github.com/orz-i/Mender/backend/internal/policy/application"
	profileapplication "github.com/orz-i/Mender/backend/internal/profile/application"
)

func TestP1ReferenceAndStaleRevisionConflictsMapToHTTP409(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		err  error
	}{
		{"stale Profile revision", profileapplication.ErrRevisionConflict},
		{"deleted Profile Tool target", profileapplication.ErrToolReferenceConflict},
		{"deleted Policy reference", policyapplication.ErrReferenceConflict},
		{"Provider still in use", bootstrap.ErrProviderInUse},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeKnownError(response, test.err)
			if response.Code != http.StatusConflict {
				t.Fatalf("HTTP status=%d, want 409", response.Code)
			}
		})
	}
}
