package providerplugin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidatePersistedConfigSecretsRejectsPlaintextSensitiveValues(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"apiKey":"plaintext"}`,
		`{"headers":{"Authorization":"Bearer plaintext"}}`,
		`{"nested":{"clientSecret":"plaintext"}}`,
		`{"password":{"value":"plaintext"}}`,
	} {
		if err := validatePersistedConfigSecrets(json.RawMessage(raw)); err == nil {
			t.Fatalf("validatePersistedConfigSecrets(%s) error = nil", raw)
		}
	}
}

func TestValidatePersistedConfigSecretsAllowsReferencesAndOrdinaryStrings(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		`{"apiKey":"credential://demo/token"}`,
		`{"headers":{"Authorization":"Bearer ${MENDER_TOKEN}"}}`,
		`{"headers":{"X-API-Key":"${env:MENDER_TOKEN}"}}`,
		`{"credentials":{"api_key":"credential://demo/token"}}`,
		`{"model":"tokenizer-v2","secretName":"remote-secret-name"}`,
	} {
		if err := validatePersistedConfigSecrets(json.RawMessage(raw)); err != nil {
			t.Fatalf("validatePersistedConfigSecrets(%s) error = %v", raw, err)
		}
	}
}

func TestValidateDescriptorConfigRequiresCredentialReferences(t *testing.T) {
	t.Parallel()

	descriptor := Descriptor{
		ID:            ID("dev.example.secure"),
		Version:       "1.0.0",
		DisplayName:   "Secure",
		ConfigVersion: 1,
		ConfigSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"auth":{"type":"string","x-mender-control":"credential"},
				"password":{"type":"string","format":"password"},
				"nested":{
					"type":"object",
					"properties":{
						"secret":{"type":"string","writeOnly":true}
					}
				}
			}
		}`),
		UISchema: json.RawMessage(`{
			"properties":{
				"uiAuth":{"control":"credential"}
			}
		}`),
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("descriptor Validate() error = %v", err)
	}

	valid := json.RawMessage(
		`{"auth":"credential://demo/auth","password":"credential://demo/password","uiAuth":"credential://demo/ui","nested":{"secret":"credential://demo/nested"}}`,
	)
	if err := ValidateDescriptorConfig(descriptor, valid); err != nil {
		t.Fatalf("ValidateDescriptorConfig(valid) error = %v", err)
	}

	for _, raw := range []string{
		`{"auth":"literal"}`,
		`{"password":"literal"}`,
		`{"uiAuth":"literal"}`,
		`{"nested":{"secret":"literal"}}`,
	} {
		err := ValidateDescriptorConfig(descriptor, json.RawMessage(raw))
		if err == nil || !strings.Contains(err.Error(), "Credential Reference") {
			t.Fatalf("ValidateDescriptorConfig(%s) error = %v", raw, err)
		}
	}
}

func TestManagementSafeConfigOmitsPotentialPlaintextSecrets(t *testing.T) {
	t.Parallel()

	descriptor := Descriptor{
		ID:            ID("dev.example.secure"),
		Version:       "1.0.0",
		DisplayName:   "Secure",
		ConfigVersion: 1,
		ConfigSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"auth":{"type":"string","x-mender-control":"credential"},
				"password":{"type":"string","format":"password"}
			}
		}`),
	}

	projected, err := ManagementSafeConfig(
		&descriptor,
		json.RawMessage(
			`{"endpoint":"https://example.test","apiKey":"plaintext","auth":"credential://demo/auth","password":"plaintext","headers":{"Authorization":"Bearer ${TOKEN}","X-API-Key":"literal"}}`,
		),
	)
	if err != nil {
		t.Fatalf("ManagementSafeConfig() error = %v", err)
	}
	object, ok := projected.(map[string]any)
	if !ok {
		t.Fatalf("projected config type = %T", projected)
	}
	if object["endpoint"] != "https://example.test" {
		t.Fatalf("endpoint = %#v", object["endpoint"])
	}
	if object["auth"] != "credential://demo/auth" {
		t.Fatalf("auth = %#v", object["auth"])
	}
	if _, exists := object["apiKey"]; exists {
		t.Fatalf("apiKey leaked into management config: %#v", object)
	}
	if _, exists := object["password"]; exists {
		t.Fatalf("password leaked into management config: %#v", object)
	}
	headers, ok := object["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %#v", object["headers"])
	}
	if headers["Authorization"] != "Bearer ${TOKEN}" {
		t.Fatalf("Authorization = %#v", headers["Authorization"])
	}
	if _, exists := headers["X-API-Key"]; exists {
		t.Fatalf("X-API-Key leaked into management config: %#v", headers)
	}
}
