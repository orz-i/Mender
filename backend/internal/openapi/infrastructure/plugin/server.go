package plugin

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/orz-i/Mender/backend/internal/providerhttp"
)

var serverVariablePattern = regexp.MustCompile(`{([^{}]+)}`)

func resolveEffectiveServer(
	parsed parsedDocument,
	config Config,
	pathItem *openapi3.PathItem,
	operation *openapi3.Operation,
) (string, error) {
	if value := strings.TrimSpace(config.ServerURL); value != "" {
		return validateEffectiveServer(value)
	}

	var selected *openapi3.Server
	switch {
	case operation != nil && operation.Servers != nil && len(*operation.Servers) != 0:
		selected = (*operation.Servers)[0]
	case pathItem != nil && len(pathItem.Servers) != 0:
		selected = pathItem.Servers[0]
	case parsed.document != nil && len(parsed.document.Servers) != 0:
		selected = parsed.document.Servers[0]
	}

	raw := "/"
	if selected != nil {
		expanded, err := expandServerVariables(selected)
		if err != nil {
			return "", err
		}
		raw = expanded
	}

	parsedServer, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("resolve OpenAPI server %q: %w", raw, err)
	}
	if !parsedServer.IsAbs() {
		if parsed.location == nil ||
			(parsed.location.Scheme != "http" && parsed.location.Scheme != "https") {
			return "", fmt.Errorf(
				"resolve OpenAPI server %q: relative server requires a URL description source or explicit serverUrl",
				raw,
			)
		}
		base := *parsed.location
		base.RawQuery = ""
		base.Fragment = ""
		parsedServer = base.ResolveReference(parsedServer)
	}
	return validateEffectiveServer(parsedServer.String())
}

func expandServerVariables(server *openapi3.Server) (string, error) {
	if server == nil {
		return "", fmt.Errorf("resolve OpenAPI server: server must not be nil")
	}
	raw := strings.TrimSpace(server.URL)
	if raw == "" {
		return "", fmt.Errorf("resolve OpenAPI server: URL must not be empty")
	}
	var expansionErr error
	expanded := serverVariablePattern.ReplaceAllStringFunc(raw, func(token string) string {
		if expansionErr != nil {
			return token
		}
		match := serverVariablePattern.FindStringSubmatch(token)
		if len(match) != 2 {
			expansionErr = fmt.Errorf("invalid server variable token %q", token)
			return token
		}
		name := match[1]
		variable, exists := server.Variables[name]
		if !exists || variable == nil {
			expansionErr = fmt.Errorf(
				"resolve OpenAPI server: variable %q is not declared",
				name,
			)
			return token
		}
		if variable.Default == "" {
			expansionErr = fmt.Errorf(
				"resolve OpenAPI server: variable %q has no default",
				name,
			)
			return token
		}
		return variable.Default
	})
	if expansionErr != nil {
		return "", expansionErr
	}
	return expanded, nil
}

func validateEffectiveServer(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if err := providerhttp.ValidateEndpoint(raw); err != nil {
		return "", fmt.Errorf("resolve OpenAPI server: %w", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("resolve OpenAPI server: %w", err)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf(
			"resolve OpenAPI server: URL must not contain query or fragment",
		)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
