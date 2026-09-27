package plugin

import (
	"encoding/json"
	"path/filepath"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()

	descriptor := Descriptor()
	if descriptor.ID != ID {
		t.Fatalf("Descriptor().ID = %s, want %s", descriptor.ID, ID)
	}
	if descriptor.ConfigVersion != ConfigVersion {
		t.Fatalf(
			"Descriptor().ConfigVersion = %d, want %d",
			descriptor.ConfigVersion,
			ConfigVersion,
		)
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Descriptor().Validate() error = %v", err)
	}
}

func TestDecodeConfigAcceptsURLAndFileSources(t *testing.T) {
	t.Parallel()

	ref, err := credentialdomain.NewReference("demo", "api")
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(t.TempDir(), "openapi.yaml")
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "url",
			raw:  []byte(`{"source":{"kind":"url","url":"https://example.test/openapi.yaml"},"serverUrl":"https://api.example.test","operations":{"include":["getUser"]},"credentials":{"ApiKeyAuth":"` + ref.String() + `"}}`),
		},
		{
			name: "file",
			raw:  []byte(`{"source":{"kind":"file","path":"` + filepath.ToSlash(filePath) + `"}}`),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeConfig(json.RawMessage(test.raw)); err != nil {
				t.Fatalf("DecodeConfig() error = %v", err)
			}
		})
	}
}

func TestDecodeConfigRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodeConfig(json.RawMessage(
		`{"source":{"kind":"url","url":"https://example.test/openapi.yaml","unknown":true}}`,
	))
	if err == nil {
		t.Fatal("DecodeConfig() error = nil, want unknown-field error")
	}
}

func TestSourceConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source SourceConfig
		ok     bool
	}{
		{
			name: "https URL",
			source: SourceConfig{
				Kind: SourceURL,
				URL:  "https://example.test/openapi.yaml",
			},
			ok: true,
		},
		{
			name: "loopback URL",
			source: SourceConfig{
				Kind: SourceURL,
				URL:  "http://127.0.0.1:8080/openapi.yaml",
			},
			ok: true,
		},
		{
			name: "remote plain HTTP",
			source: SourceConfig{
				Kind: SourceURL,
				URL:  "http://example.test/openapi.yaml",
			},
		},
		{
			name: "URL fragment",
			source: SourceConfig{
				Kind: SourceURL,
				URL:  "https://example.test/openapi.yaml#section",
			},
		},
		{
			name: "URL with file path",
			source: SourceConfig{
				Kind: SourceURL,
				URL:  "https://example.test/openapi.yaml",
				Path: "/tmp/openapi.yaml",
			},
		},
		{
			name: "absolute file",
			source: SourceConfig{
				Kind: SourceFile,
				Path: "/tmp/openapi.yaml",
			},
			ok: true,
		},
		{
			name: "relative file",
			source: SourceConfig{
				Kind: SourceFile,
				Path: "openapi.yaml",
			},
		},
		{
			name: "unknown kind",
			source: SourceConfig{
				Kind: "inline",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.source.Validate()
			if test.ok && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}

func TestOperationFilterValidation(t *testing.T) {
	t.Parallel()

	if err := (OperationFilter{
		Include: []string{"getUser", "get /users/{id}"},
		Exclude: []string{"deleteUser"},
	}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []OperationFilter{
		{Include: []string{"getUser", "getUser"}},
		{Exclude: []string{" "}},
		{Include: []string{"getUser"}, Exclude: []string{"getUser"}},
	}
	for _, filter := range tests {
		if err := filter.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil, want error", filter)
		}
	}
}

func TestRequiredCredentialRefsAreUniqueAndSorted(t *testing.T) {
	t.Parallel()

	alpha, _ := credentialdomain.NewReference("demo", "alpha")
	beta, _ := credentialdomain.NewReference("demo", "beta")
	config := Config{
		Credentials: map[string]credentialdomain.Reference{
			"B": beta,
			"A": alpha,
			"C": alpha,
		},
	}
	refs := config.RequiredCredentialRefs()
	if len(refs) != 2 || refs[0] != alpha || refs[1] != beta {
		t.Fatalf("RequiredCredentialRefs() = %v, want [%s %s]", refs, alpha, beta)
	}
}

func TestInspectConfigReturnsCredentialMetadata(t *testing.T) {
	t.Parallel()

	alpha, _ := credentialdomain.NewReference("demo", "alpha")
	beta, _ := credentialdomain.NewReference("demo", "beta")
	raw, err := json.Marshal(Config{
		Source: SourceConfig{
			Kind: SourceURL,
			URL:  "https://example.test/openapi.yaml",
		},
		Credentials: map[string]credentialdomain.Reference{
			"Beta":  beta,
			"Alpha": alpha,
			"Alias": alpha,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := InspectConfig(t.Context(), raw)
	if err != nil {
		t.Fatalf("InspectConfig() error = %v", err)
	}
	if len(metadata.CredentialRefs) != 2 ||
		metadata.CredentialRefs[0] != alpha ||
		metadata.CredentialRefs[1] != beta {
		t.Fatalf("InspectConfig() refs = %v", metadata.CredentialRefs)
	}
}

func TestConfigRejectsInvalidCredentialReference(t *testing.T) {
	t.Parallel()

	config := Config{
		Source: SourceConfig{
			Kind: SourceURL,
			URL:  "https://example.test/openapi.yaml",
		},
		Credentials: map[string]credentialdomain.Reference{
			"ApiKey": credentialdomain.Reference("bad"),
		},
	}
	if err := config.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid Credential Reference")
	}
}
