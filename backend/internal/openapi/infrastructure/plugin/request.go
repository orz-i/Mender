package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func buildHTTPRequest(
	ctx context.Context,
	binding operationBinding,
	rawArguments []byte,
) (*http.Request, error) {
	arguments, err := decodeArgumentObject(rawArguments)
	if err != nil {
		return nil, err
	}

	pathValue := binding.path
	query := make(url.Values)
	headers := make(http.Header)
	cookies := make([]*http.Cookie, 0)

	for _, parameter := range binding.parameters {
		group := canonicalParameterGroup(parameter.In)
		value, exists, err := argumentParameter(arguments, group, parameter.Name)
		if err != nil {
			return nil, err
		}
		if !exists {
			if parameter.Required {
				return nil, fmt.Errorf(
					"serialize OpenAPI parameter %q: required value is missing",
					parameter.Name,
				)
			}
			continue
		}

		switch parameter.In {
		case "path":
			serialized, err := serializePathParameter(value, parameter.Explode)
			if err != nil {
				return nil, fmt.Errorf(
					"serialize OpenAPI path parameter %q: %w",
					parameter.Name,
					err,
				)
			}
			token := "{" + parameter.Name + "}"
			if !strings.Contains(pathValue, token) {
				return nil, fmt.Errorf(
					"serialize OpenAPI path parameter %q: template token is missing",
					parameter.Name,
				)
			}
			pathValue = strings.ReplaceAll(pathValue, token, serialized)

		case "query":
			if err := serializeQueryParameter(
				query,
				parameter.Name,
				value,
				parameter.Explode,
			); err != nil {
				return nil, fmt.Errorf(
					"serialize OpenAPI query parameter %q: %w",
					parameter.Name,
					err,
				)
			}

		case "header":
			serialized, err := serializeSimpleParameter(value)
			if err != nil {
				return nil, fmt.Errorf(
					"serialize OpenAPI header parameter %q: %w",
					parameter.Name,
					err,
				)
			}
			if strings.IndexByte(serialized, 13) >= 0 ||
				strings.IndexByte(serialized, 10) >= 0 ||
				strings.IndexByte(serialized, 0) >= 0 {
				return nil, fmt.Errorf(
					"serialize OpenAPI header parameter %q: invalid header value",
					parameter.Name,
				)
			}
			headers.Set(parameter.Name, serialized)

		case "cookie":
			values, err := serializeFormValues(value, parameter.Explode)
			if err != nil {
				return nil, fmt.Errorf(
					"serialize OpenAPI cookie parameter %q: %w",
					parameter.Name,
					err,
				)
			}
			for _, item := range values {
				//nolint:gosec // outbound Cookie header; Set-Cookie security attributes do not apply.
				cookies = append(cookies, &http.Cookie{
					Name:  parameter.Name,
					Value: item,
				})
			}

		default:
			return nil, fmt.Errorf(
				"serialize OpenAPI parameter %q: unsupported location %q",
				parameter.Name,
				parameter.In,
			)
		}
	}

	target, err := operationURL(binding.serverURL, pathValue)
	if err != nil {
		return nil, err
	}
	target.RawQuery = query.Encode()

	var body io.Reader
	if binding.requestBody != nil {
		bodyValue, exists := arguments["body"]
		if !exists {
			if binding.requestBody.Required {
				return nil, fmt.Errorf("serialize OpenAPI request body: required body is missing")
			}
		} else {
			rawBody, err := json.Marshal(bodyValue)
			if err != nil {
				return nil, fmt.Errorf("serialize OpenAPI request body: %w", err)
			}
			body = bytes.NewReader(rawBody)
			headers.Set("Content-Type", binding.requestBody.MediaType)
		}
	}

	request, err := http.NewRequestWithContext(
		ctx,
		binding.method,
		target.String(),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("build OpenAPI HTTP request: %w", err)
	}
	request.Header = headers
	request.Header.Set("Accept", "application/json, application/*+json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	return request, nil
}

func decodeArgumentObject(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode OpenAPI Tool arguments: %w", err)
	}
	if value == nil {
		return nil, fmt.Errorf("decode OpenAPI Tool arguments: root must be an object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode OpenAPI Tool arguments: multiple JSON values")
		}
		return nil, fmt.Errorf("decode OpenAPI Tool arguments: trailing data: %w", err)
	}
	return value, nil
}

func argumentParameter(
	arguments map[string]any,
	group string,
	name string,
) (any, bool, error) {
	rawGroup, exists := arguments[group]
	if !exists {
		return nil, false, nil
	}
	values, ok := rawGroup.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf(
			"decode OpenAPI Tool arguments: %s must be an object",
			group,
		)
	}
	value, exists := values[name]
	return value, exists, nil
}

func operationURL(serverURL, operationPath string) (*url.URL, error) {
	base := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	pathValue := strings.TrimLeft(strings.TrimSpace(operationPath), "/")
	raw := base
	if pathValue != "" {
		raw += "/" + pathValue
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("build OpenAPI operation URL: %w", err)
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("build OpenAPI operation URL: target must be absolute")
	}
	return parsed, nil
}

func serializePathParameter(value any, explode bool) (string, error) {
	values, isArray, err := scalarOrArray(value)
	if err != nil {
		return "", err
	}
	if !isArray {
		return url.PathEscape(values[0]), nil
	}

	escaped := make([]string, 0, len(values))
	for _, item := range values {
		escaped = append(escaped, url.PathEscape(item))
	}
	_ = explode // simple arrays use comma separation for both explode values.
	return strings.Join(escaped, ","), nil
}

func serializeSimpleParameter(value any) (string, error) {
	values, isArray, err := scalarOrArray(value)
	if err != nil {
		return "", err
	}
	if !isArray {
		return values[0], nil
	}
	return strings.Join(values, ","), nil
}

func serializeQueryParameter(
	query url.Values,
	name string,
	value any,
	explode bool,
) error {
	values, isArray, err := scalarOrArray(value)
	if err != nil {
		return err
	}
	if !isArray {
		query.Set(name, values[0])
		return nil
	}
	if explode {
		for _, item := range values {
			query.Add(name, item)
		}
		return nil
	}
	query.Set(name, strings.Join(values, ","))
	return nil
}

func serializeFormValues(value any, explode bool) ([]string, error) {
	values, isArray, err := scalarOrArray(value)
	if err != nil {
		return nil, err
	}
	if !isArray || explode {
		return values, nil
	}
	return []string{strings.Join(values, ",")}, nil
}

func scalarOrArray(value any) ([]string, bool, error) {
	switch typed := value.(type) {
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, err := scalarString(item)
			if err != nil {
				return nil, true, err
			}
			values = append(values, text)
		}
		return values, true, nil
	default:
		text, err := scalarString(value)
		if err != nil {
			return nil, false, err
		}
		return []string{text}, false, nil
	}
}

func scalarString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case json.Number:
		return typed.String(), nil
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64), nil
	default:
		return "", fmt.Errorf("value of type %T is not a supported scalar", value)
	}
}
