package plugin

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type schemaDirection uint8

const (
	schemaRequest schemaDirection = iota
	schemaResponse
)

type schemaNormalizer struct {
	document      *openapi3.T
	sourceVersion string
	direction     schemaDirection

	defs      map[string]any
	resolving map[string]struct{}
}

func newSchemaNormalizer(
	document *openapi3.T,
	sourceVersion string,
	direction schemaDirection,
) *schemaNormalizer {
	return &schemaNormalizer{
		document:      document,
		sourceVersion: sourceVersion,
		direction:     direction,
		defs:          make(map[string]any),
		resolving:     make(map[string]struct{}),
	}
}

func (n *schemaNormalizer) normalizeRef(ref *openapi3.SchemaRef) (any, error) {
	if ref == nil {
		return nil, fmt.Errorf("normalize OpenAPI schema: schema reference must not be nil")
	}
	raw, err := json.Marshal(ref)
	if err != nil {
		return nil, fmt.Errorf("normalize OpenAPI schema: marshal: %w", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("normalize OpenAPI schema: decode: %w", err)
	}
	return n.normalizeNode(value)
}

func (n *schemaNormalizer) attachDefs(root map[string]any) {
	if len(n.defs) == 0 {
		return
	}
	names := make([]string, 0, len(n.defs))
	for name := range n.defs {
		names = append(names, name)
	}
	slices.Sort(names)

	defs := make(map[string]any, len(names))
	for _, name := range names {
		defs[name] = n.defs[name]
	}
	root["$defs"] = defs
}

func (n *schemaNormalizer) normalizeNode(value any) (any, error) {
	switch node := value.(type) {
	case map[string]any:
		if dynamicRef, ok := node["$dynamicRef"].(string); ok && strings.TrimSpace(dynamicRef) != "" {
			return nil, fmt.Errorf(
				"normalize OpenAPI schema: $dynamicRef %q is not supported",
				dynamicRef,
			)
		}

		for key, child := range node {
			normalized, err := n.normalizeNode(child)
			if err != nil {
				return nil, err
			}
			node[key] = normalized
		}

		if rawRef, ok := node["$ref"].(string); ok {
			rewritten, err := n.rewriteRef(rawRef)
			if err != nil {
				return nil, err
			}
			node["$ref"] = rewritten
		}

		delete(node, "discriminator")
		delete(node, "xml")
		delete(node, "externalDocs")
		delete(node, "example")

		if strings.HasPrefix(n.sourceVersion, "3.0") {
			if err := normalizeOpenAPI30Keywords(node); err != nil {
				return nil, err
			}
		} else {
			delete(node, "nullable")
		}

		normalizeRequiredForDirection(node, n.direction)
		return node, nil

	case []any:
		for index, child := range node {
			normalized, err := n.normalizeNode(child)
			if err != nil {
				return nil, err
			}
			node[index] = normalized
		}
		return node, nil

	default:
		return value, nil
	}
}

func (n *schemaNormalizer) rewriteRef(rawRef string) (string, error) {
	rawRef = strings.TrimSpace(rawRef)
	if rawRef == "" {
		return "", fmt.Errorf("normalize OpenAPI schema: empty $ref")
	}
	if strings.HasPrefix(rawRef, "#/components/schemas/") {
		token := strings.TrimPrefix(rawRef, "#/components/schemas/")
		if token == "" || strings.Contains(token, "/") {
			return "", fmt.Errorf(
				"normalize OpenAPI schema: unsupported component schema $ref %q",
				rawRef,
			)
		}
		name, err := decodeJSONPointerToken(token)
		if err != nil {
			return "", fmt.Errorf(
				"normalize OpenAPI schema: component schema $ref %q: %w",
				rawRef,
				err,
			)
		}
		if err := n.ensureDefinition(name); err != nil {
			return "", err
		}
		return "#/$defs/" + token, nil
	}
	if strings.HasPrefix(rawRef, "#/$defs/") {
		return rawRef, nil
	}
	return "", fmt.Errorf(
		"normalize OpenAPI schema: local $ref %q is outside supported component schemas",
		rawRef,
	)
}

func (n *schemaNormalizer) ensureDefinition(name string) error {
	if _, exists := n.defs[name]; exists {
		return nil
	}
	if _, resolving := n.resolving[name]; resolving {
		return nil
	}
	if n.document == nil ||
		n.document.Components == nil ||
		n.document.Components.Schemas == nil {
		return fmt.Errorf(
			"normalize OpenAPI schema: component schema %q is unavailable",
			name,
		)
	}
	ref, exists := n.document.Components.Schemas[name]
	if !exists || ref == nil {
		return fmt.Errorf(
			"normalize OpenAPI schema: component schema %q is unavailable",
			name,
		)
	}

	n.resolving[name] = struct{}{}
	// A placeholder makes recursive A -> A and A -> B -> A graphs terminate.
	n.defs[name] = map[string]any{}
	normalized, err := n.normalizeRef(ref)
	delete(n.resolving, name)
	if err != nil {
		delete(n.defs, name)
		return fmt.Errorf(
			"normalize OpenAPI schema component %q: %w",
			name,
			err,
		)
	}
	n.defs[name] = normalized
	return nil
}

func decodeJSONPointerToken(value string) (string, error) {
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.Grow(len(decoded))
	for index := 0; index < len(decoded); index++ {
		if decoded[index] != '~' {
			builder.WriteByte(decoded[index])
			continue
		}
		if index+1 >= len(decoded) {
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
		index++
		switch decoded[index] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
	}
	if builder.Len() == 0 {
		return "", fmt.Errorf("component name must not be empty")
	}
	return builder.String(), nil
}

func normalizeOpenAPI30Keywords(node map[string]any) error {
	for _, key := range []string{"exclusiveMinimum", "exclusiveMaximum"} {
		value, exists := node[key]
		if !exists {
			continue
		}
		enabled, isBool := value.(bool)
		if !isBool {
			continue
		}
		if !enabled {
			delete(node, key)
			continue
		}

		boundKey := "minimum"
		if key == "exclusiveMaximum" {
			boundKey = "maximum"
		}
		bound, ok := node[boundKey].(float64)
		if !ok {
			return fmt.Errorf(
				"normalize OpenAPI 3.0 schema: %s=true requires numeric %s",
				key,
				boundKey,
			)
		}
		node[key] = bound
		delete(node, boundKey)
	}

	nullable, _ := node["nullable"].(bool)
	delete(node, "nullable")
	if !nullable {
		return nil
	}

	switch rawType := node["type"].(type) {
	case string:
		if rawType != "null" {
			node["type"] = []any{rawType, "null"}
		}
	case []any:
		for _, item := range rawType {
			if item == "null" {
				return nil
			}
		}
		node["type"] = append(rawType, "null")
	}
	return nil
}

func normalizeRequiredForDirection(
	node map[string]any,
	direction schemaDirection,
) {
	properties, ok := node["properties"].(map[string]any)
	if !ok || len(properties) == 0 {
		return
	}
	required, ok := node["required"].([]any)
	if !ok || len(required) == 0 {
		return
	}

	next := make([]any, 0, len(required))
	for _, item := range required {
		name, ok := item.(string)
		if !ok {
			next = append(next, item)
			continue
		}
		property, ok := properties[name].(map[string]any)
		if !ok {
			next = append(next, item)
			continue
		}
		switch direction {
		case schemaRequest:
			if readOnly, _ := property["readOnly"].(bool); readOnly {
				continue
			}
		case schemaResponse:
			if writeOnly, _ := property["writeOnly"].(bool); writeOnly {
				continue
			}
		}
		next = append(next, item)
	}

	if len(next) == 0 {
		delete(node, "required")
		return
	}
	node["required"] = next
}
