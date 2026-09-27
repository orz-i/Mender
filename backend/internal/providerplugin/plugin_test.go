package providerplugin

import (
	"testing"
)

func TestParseID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		ok    bool
	}{
		{name: "mcp", value: "io.mender.provider.mcp", ok: true},
		{name: "vendor", value: "com.example.openapi-v2", ok: true},
		{name: "empty", value: "", ok: false},
		{name: "not namespaced", value: "mcp", ok: false},
		{name: "uppercase", value: "io.Mender.provider.mcp", ok: false},
		{name: "surrounding whitespace", value: " io.mender.provider.mcp", ok: false},
		{name: "empty segment", value: "io..mender", ok: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseID(test.value)
			if test.ok {
				if err != nil {
					t.Fatalf("ParseID() error = %v", err)
				}
				if got.String() != test.value {
					t.Fatalf("ParseID() = %q, want %q", got, test.value)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseID() error = nil, want validation error")
			}
		})
	}
}

func TestDescriptorValidate(t *testing.T) {
	t.Parallel()

	valid := Descriptor{
		ID:            ID("io.mender.provider.mcp"),
		Version:       "1.0.0",
		DisplayName:   "MCP",
		Description:   "Model Context Protocol",
		ConfigVersion: 1,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := valid
	invalid.ConfigVersion = 0
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid config version")
	}
}
