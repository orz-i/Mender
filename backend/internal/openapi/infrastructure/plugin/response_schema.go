package plugin

import (
	"encoding/json"
	"fmt"
	"mime"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

func buildOperationOutputSchema(
	parsed parsedDocument,
	responses *openapi3.Responses,
) ([]byte, error) {
	if responses == nil || responses.Len() == 0 {
		return nil, nil
	}

	keys := responses.Keys()
	slices.Sort(keys)
	normalizer := newSchemaNormalizer(
		parsed.document,
		parsed.sourceVersion,
		schemaResponse,
	)

	shapes := make([]any, 0)
	reliable := true
	for _, key := range keys {
		if !isSuccessResponseKey(key) {
			continue
		}
		ref := responses.Value(key)
		if ref == nil || ref.Value == nil {
			return nil, fmt.Errorf("successful response %q is unresolved", key)
		}
		response := ref.Value
		if len(response.Content) == 0 {
			shapes = append(shapes, map[string]any{"type": "null"})
			continue
		}

		mediaKeys := make([]string, 0, len(response.Content))
		for mediaType := range response.Content {
			mediaKeys = append(mediaKeys, mediaType)
		}
		slices.Sort(mediaKeys)

		hadJSON := false
		for _, mediaType := range mediaKeys {
			if !isConcreteJSONMediaType(mediaType) {
				return nil, fmt.Errorf(
					"successful response %q media type %q is not supported; only JSON responses are supported",
					key,
					mediaType,
				)
			}
			hadJSON = true
			media := response.Content[mediaType]
			if media == nil || media.Schema == nil {
				reliable = false
				continue
			}
			shape, err := normalizer.normalizeRef(media.Schema)
			if err != nil {
				return nil, fmt.Errorf(
					"successful response %q schema: %w",
					key,
					err,
				)
			}
			shapes = append(shapes, shape)
		}
		if !hadJSON {
			reliable = false
		}
	}

	if !reliable || len(shapes) == 0 {
		return nil, nil
	}

	shapes = deduplicateJSONShapes(shapes)
	var root any
	if len(shapes) == 1 {
		root = shapes[0]
	} else {
		root = map[string]any{"anyOf": shapes}
	}

	if len(normalizer.defs) != 0 {
		rootMap, ok := root.(map[string]any)
		if !ok {
			rootMap = map[string]any{"allOf": []any{root}}
			root = rootMap
		}
		normalizer.attachDefs(rootMap)
	}

	raw, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("encode canonical Tool output schema: %w", err)
	}
	return raw, nil
}

func isSuccessResponseKey(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 3 || value[0] != '2' {
		return false
	}
	if strings.EqualFold(value[1:], "XX") {
		return true
	}
	code, err := strconv.Atoi(value)
	return err == nil && code >= 200 && code <= 299
}

func isConcreteJSONMediaType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	if strings.Contains(mediaType, "*") {
		return false
	}
	return mediaType == "application/json" ||
		(strings.HasPrefix(mediaType, "application/") &&
			strings.HasSuffix(mediaType, "+json"))
}

func deduplicateJSONShapes(values []any) []any {
	type encodedShape struct {
		raw   string
		value any
	}
	encoded := make([]encodedShape, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			// Values came from decoded JSON/schema construction and therefore
			// should always marshal. Keep the value rather than hiding a shape.
			encoded = append(encoded, encodedShape{value: value})
			continue
		}
		key := string(raw)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		encoded = append(encoded, encodedShape{raw: key, value: value})
	}
	slices.SortFunc(encoded, func(a, b encodedShape) int {
		return strings.Compare(a.raw, b.raw)
	})
	result := make([]any, 0, len(encoded))
	for _, item := range encoded {
		result = append(result, item.value)
	}
	return result
}
