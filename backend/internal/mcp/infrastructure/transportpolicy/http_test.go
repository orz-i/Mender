package transportpolicy

import "testing"

func TestValidateHTTPEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "remote https", endpoint: "https://example.test/mcp"},
		{name: "loopback http ipv4", endpoint: "http://127.0.0.1:8080/mcp"},
		{name: "loopback http ipv6", endpoint: "http://[::1]:8080/mcp"},
		{name: "localhost http", endpoint: "http://localhost:8080/mcp"},
		{name: "remote plaintext", endpoint: "http://example.test/mcp", wantErr: true},
		//nolint:gosec // This fixture intentionally verifies that URL userinfo is rejected.
		{name: "userinfo", endpoint: "https://user:secret@example.test/mcp", wantErr: true},
		{name: "non http scheme", endpoint: "file:///tmp/mcp", wantErr: true},
		{name: "relative", endpoint: "/mcp", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHTTPEndpoint(tc.endpoint)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateHTTPEndpoint(%q) expected error", tc.endpoint)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateHTTPEndpoint(%q) error = %v", tc.endpoint, err)
			}
		})
	}
}
