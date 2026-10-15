package providerhttp

import "testing"

func TestValidateEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		ok   bool
	}{
		{name: "https", url: "https://example.test/api", ok: true},
		{name: "loopback http", url: "http://127.0.0.1:8080/api", ok: true},
		{name: "loopback ipv6", url: "http://[::1]:8080/api", ok: true},
		{name: "localhost http", url: "http://localhost:8080/api", ok: true},
		{name: "remote http", url: "http://example.test/api"},
		{name: "userinfo", url: "https://user@example.test/api"},
		{name: "relative", url: "/api"},
		{name: "file", url: "file:///tmp/api"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateEndpoint(test.url)
			if test.ok && err != nil {
				t.Fatalf("ValidateEndpoint() error = %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("ValidateEndpoint() error = nil, want error")
			}
		})
	}
}
