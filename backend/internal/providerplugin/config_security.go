package providerplugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

var (
	configEnvReferencePattern = regexp.MustCompile(
		`^(?:\$\{(?:env:)?[A-Za-z_][A-Za-z0-9_]*\}|\{env:[A-Za-z_][A-Za-z0-9_]*\})$`,
	)
	configAuthSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+._-]*$`)
)

var sensitiveConfigNames = map[string]struct{}{
	"authorization":      {},
	"proxyauthorization": {},
	"cookie":             {},
	"setcookie":          {},
	"apikey":             {},
	"xapikey":            {},
	"accesskey":          {},
	"secretaccesskey":    {},
	"accesstoken":        {},
	"refreshtoken":       {},
	"authtoken":          {},
	"bearertoken":        {},
	"token":              {},
	"password":           {},
	"passphrase":         {},
	"secret":             {},
	"clientsecret":       {},
	"privatekey":         {},
	"credentialvalue":    {},
	"secretvalue":        {},
	"tokenvalue":         {},
	"passwordvalue":      {},
}

// ValidateDescriptorConfig enforces host-owned security semantics expressed by
// a plugin's declarative config/UI schema. Credential-designated or explicitly
// secret-like schema fields must persist a canonical Credential Reference,
// never resolved secret material.
func ValidateDescriptorConfig(descriptor Descriptor, raw json.RawMessage) error {
	if err := descriptor.Validate(); err != nil {
		return fmt.Errorf("provider plugin config policy: descriptor: %w", err)
	}
	value, err := decodeConfigJSON(raw)
	if err != nil {
		return fmt.Errorf("provider plugin config policy: %w", err)
	}
	schema, err := decodeOptionalJSON(descriptor.ConfigSchema)
	if err != nil {
		return fmt.Errorf("provider plugin config policy: config schema: %w", err)
	}
	uiSchema, err := decodeOptionalJSON(descriptor.UISchema)
	if err != nil {
		return fmt.Errorf("provider plugin config policy: UI schema: %w", err)
	}
	return validateAnnotatedConfig(value, schema, uiSchema, "$")
}

// ManagementSafeConfig returns a defensive JSON-compatible projection of one
// Provider configuration. Potential plaintext secrets are omitted while
// canonical Credential References and environment references remain visible.
func ManagementSafeConfig(
	descriptor *Descriptor,
	raw json.RawMessage,
) (any, error) {
	value, err := decodeConfigJSON(raw)
	if err != nil {
		return nil, err
	}

	var schema any
	var uiSchema any
	if descriptor != nil {
		schema, err = decodeOptionalJSON(descriptor.ConfigSchema)
		if err != nil {
			return nil, fmt.Errorf("management config projection: config schema: %w", err)
		}
		uiSchema, err = decodeOptionalJSON(descriptor.UISchema)
		if err != nil {
			return nil, fmt.Errorf("management config projection: UI schema: %w", err)
		}
	}

	safe, keep := sanitizeConfigNode(value, schema, uiSchema)
	if !keep {
		return nil, nil
	}
	return safe, nil
}

func validatePersistedConfigSecrets(raw json.RawMessage) error {
	value, err := decodeConfigJSON(raw)
	if err != nil {
		return err
	}
	return validateSecretFreeValue(value, "$")
}

func validateSecretFreeValue(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for name, child := range typed {
			childPath := configPath(path, name)
			if isSensitiveConfigName(name) && !isSafeSensitiveConfigValue(child) {
				return fmt.Errorf(
					"%s must use a Credential Reference or environment reference instead of persisting secret material",
					childPath,
				)
			}
			if err := validateSecretFreeValue(child, childPath); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := validateSecretFreeValue(
				child,
				fmt.Sprintf("%s[%d]", path, index),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAnnotatedConfig(value, schema, uiSchema any, path string) error {
	if value == nil {
		return nil
	}
	if configNodeRequiresCredential(schema, uiSchema) {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf(
				"provider plugin config policy: %s must be a Credential Reference",
				path,
			)
		}
		if _, err := credentialdomain.ParseReference(text); err != nil {
			return fmt.Errorf(
				"provider plugin config policy: %s must be a Credential Reference: %w",
				path,
				err,
			)
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		for name, child := range typed {
			childSchema := configChildSchema(schema, name)
			childUI := configChildUI(uiSchema, name)
			if err := validateAnnotatedConfig(
				child,
				childSchema,
				childUI,
				configPath(path, name),
			); err != nil {
				return err
			}
		}
	case []any:
		itemSchema := configItemSchema(schema)
		for index, child := range typed {
			if err := validateAnnotatedConfig(
				child,
				itemSchema,
				nil,
				fmt.Sprintf("%s[%d]", path, index),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func sanitizeConfigNode(value, schema, uiSchema any) (any, bool) {
	if value == nil {
		return nil, true
	}
	if configNodeRequiresCredential(schema, uiSchema) {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		if _, err := credentialdomain.ParseReference(text); err != nil {
			return nil, false
		}
		return text, true
	}

	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for name, child := range typed {
			if isSensitiveConfigName(name) && !isSafeSensitiveConfigValue(child) {
				continue
			}
			safe, keep := sanitizeConfigNode(
				child,
				configChildSchema(schema, name),
				configChildUI(uiSchema, name),
			)
			if keep {
				result[name] = safe
			}
		}
		return result, true
	case []any:
		result := make([]any, 0, len(typed))
		itemSchema := configItemSchema(schema)
		for _, child := range typed {
			safe, keep := sanitizeConfigNode(child, itemSchema, nil)
			if keep {
				result = append(result, safe)
			} else {
				result = append(result, nil)
			}
		}
		return result, true
	default:
		return typed, true
	}
}

func configNodeRequiresCredential(schema, uiSchema any) bool {
	schemaObject, _ := schema.(map[string]any)
	if schemaObject != nil {
		if control, _ := schemaObject["x-mender-control"].(string); control == "credential" {
			return true
		}
		if writeOnly, _ := schemaObject["writeOnly"].(bool); writeOnly {
			return true
		}
		if format, _ := schemaObject["format"].(string); format == "password" {
			return true
		}
	}
	uiObject, _ := uiSchema.(map[string]any)
	if uiObject != nil {
		if control, _ := uiObject["control"].(string); control == "credential" {
			return true
		}
	}
	return false
}

func configChildSchema(schema any, name string) any {
	object, _ := schema.(map[string]any)
	if object == nil {
		return nil
	}
	if properties, ok := object["properties"].(map[string]any); ok {
		if child, exists := properties[name]; exists {
			return child
		}
	}
	switch additional := object["additionalProperties"].(type) {
	case map[string]any:
		return additional
	case bool:
		if additional {
			return true
		}
	}
	return nil
}

func configChildUI(uiSchema any, name string) any {
	object, _ := uiSchema.(map[string]any)
	if object == nil {
		return nil
	}
	properties, _ := object["properties"].(map[string]any)
	if properties == nil {
		return nil
	}
	return properties[name]
}

func configItemSchema(schema any) any {
	object, _ := schema.(map[string]any)
	if object == nil {
		return nil
	}
	return object["items"]
}

func isSensitiveConfigName(name string) bool {
	var normalized strings.Builder
	for _, value := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			normalized.WriteRune(value)
		}
	}
	_, ok := sensitiveConfigNames[normalized.String()]
	return ok
}

func isSafeSensitiveConfigValue(value any) bool {
	if value == nil {
		return true
	}
	text, ok := value.(string)
	if !ok {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	if _, err := credentialdomain.ParseReference(text); err == nil {
		return true
	}
	if isConfigEnvironmentReference(text) {
		return true
	}
	parts := strings.Fields(text)
	if len(parts) == 2 &&
		configAuthSchemePattern.MatchString(parts[0]) &&
		(isConfigEnvironmentReference(parts[1]) || isCredentialReference(parts[1])) {
		return true
	}
	return false
}

func isCredentialReference(value string) bool {
	_, err := credentialdomain.ParseReference(value)
	return err == nil
}

func isConfigEnvironmentReference(value string) bool {
	return configEnvReferencePattern.MatchString(value)
}

func decodeConfigJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("config must not be empty")
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("config must be valid JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return value, nil
}

func decodeOptionalJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func configPath(parent, name string) string {
	if parent == "$" {
		return "$." + name
	}
	return parent + "." + name
}
