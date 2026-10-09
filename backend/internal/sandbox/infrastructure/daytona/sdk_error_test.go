package daytona

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/daytona/clients/sdk-go/pkg/errors"
	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

// Vendor messages can contain arbitrary URLs or credentials; only status
// numbers and tightly bounded structured machine codes can leave the SDK.
func TestSanitizedDaytonaSDKStatusErrors(t *testing.T) {
	for _, test := range []struct {
		status     int
		code       string
		want       error
		statusText string
	}{
		{400, "INVALID_PARAMETER", sandboxdomain.ErrBackendUnavailable, "HTTP 400"},
		{401, "AUTH_ERROR", sandboxdomain.ErrNotAuthorized, "HTTP 401"},
		{403, "NOT_ALLOWED", sandboxdomain.ErrNotAuthorized, "HTTP 403"},
		{404, "NOT_FOUND", sandboxdomain.ErrNotFound, "HTTP 404"},
		{500, "FAILED_TO_START", sandboxdomain.ErrBackendUnavailable, "HTTP 500"},
		{422, "anything-bad/Authorization:Bearer SECRET", sandboxdomain.ErrBackendUnavailable, "HTTP 422"},
	} {
		t.Run(test.statusText, func(t *testing.T) {
			vendor := &sdkerrors.DaytonaError{Message: "Authorization: Bearer SECRET, untrusted URL https://internal.test", StatusCode: test.status, Code: test.code}
			mapped := safeRemoteError(context.Background(), vendor)
			if !errors.Is(mapped, test.want) {
				t.Fatalf("typed error incorrect: %v", mapped)
			}
			if !strings.Contains(mapped.Error(), test.statusText) {
				t.Fatalf("status missing: %v", mapped)
			}
			if strings.Contains(mapped.Error(), "SECRET") || strings.Contains(mapped.Error(), "internal.test") {
				t.Fatal("vendor error leaked credentials or arbitrary URL")
			}
			if strings.Contains(test.code, "/") && strings.Contains(mapped.Error(), test.code) {
				t.Fatal("unsafe machine code leaked")
			}
		})
	}
}
