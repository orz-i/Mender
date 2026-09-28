package plugin

import (
	"encoding/json"
	"testing"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

func TestDescriptor(t *testing.T) {
	t.Parallel()
	descriptor := Descriptor()
	if descriptor.ID != ID || descriptor.ConfigVersion != ConfigVersion {
		t.Fatalf("Descriptor() = %+v", descriptor)
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Descriptor().Validate() error = %v", err)
	}
}

func TestDecodeConfigEngineVariants(t *testing.T) {
	t.Parallel()

	ref, err := credentialdomain.NewReference("model", "api")
	if err != nil {
		t.Fatal(err)
	}
	for _, engine := range []Engine{EngineOpenAI, EngineAnthropic, EngineGemini} {
		engine := engine
		t.Run(string(engine), func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(Config{
				Engine:        engine,
				Model:         "model-1",
				CredentialRef: ref,
				Capabilities:  []Capability{CapabilityWebSearch},
			})
			if err != nil {
				t.Fatal(err)
			}
			config, err := DecodeConfig(raw)
			if err != nil {
				t.Fatalf("DecodeConfig() error = %v", err)
			}
			if config.Engine != engine || !config.HasCapability(CapabilityWebSearch) {
				t.Fatalf("config = %+v", config)
			}
		})
	}
}

func TestDecodeConfigRejectsUnknownAndInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"engine":"other","model":"m","credentialRef":"credential://model/api","capabilities":["web_search"]}`,
		`{"engine":"openai","model":"","credentialRef":"credential://model/api","capabilities":["web_search"]}`,
		`{"engine":"openai","model":"m","credentialRef":"bad","capabilities":["web_search"]}`,
		`{"engine":"openai","model":"m","credentialRef":"credential://model/api","capabilities":[]}`,
		`{"engine":"openai","model":"m","credentialRef":"credential://model/api","capabilities":["code_execution"]}`,
		`{"engine":"openai","model":"m","credentialRef":"credential://model/api","capabilities":["web_search","web_search"]}`,
		`{"engine":"openai","model":"m","credentialRef":"credential://model/api","capabilities":["web_search"],"unknown":true}`,
	}
	for _, raw := range tests {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeConfig(json.RawMessage(raw)); err == nil {
				t.Fatal("DecodeConfig() error = nil, want validation error")
			}
		})
	}
}

func TestInspectConfigReturnsOnlyAPICredential(t *testing.T) {
	t.Parallel()

	ref, _ := credentialdomain.NewReference("model", "api")
	raw, _ := json.Marshal(Config{
		Engine:        EngineOpenAI,
		Model:         "model-1",
		CredentialRef: ref,
		Capabilities:  []Capability{CapabilityWebSearch},
	})
	metadata, err := InspectConfig(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.CredentialRefs) != 1 || metadata.CredentialRefs[0] != ref {
		t.Fatalf("metadata = %+v", metadata)
	}
}
