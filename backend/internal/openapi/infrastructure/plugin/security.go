package plugin

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
	invocationapplication "github.com/orz-i/Mender/backend/internal/invocation/application"
)

type securityBindingKind string

const (
	securityAPIKey securityBindingKind = "api_key"
	securityBearer securityBindingKind = "bearer"
)

type securityBinding struct {
	SchemeName string
	Kind       securityBindingKind
	In         string
	Name       string
	Reference  credentialdomain.Reference
}

func selectOperationSecurity(
	document *openapi3.T,
	operation *openapi3.Operation,
	configured map[string]credentialdomain.Reference,
) ([]securityBinding, error) {
	requirements := effectiveSecurityRequirements(document, operation)
	if len(requirements) == 0 {
		return nil, nil
	}

	var reasons []string
	for _, requirement := range requirements {
		if len(requirement) == 0 {
			return nil, nil
		}

		bindings, reason := satisfySecurityRequirement(
			document,
			requirement,
			configured,
		)
		if reason == "" {
			return bindings, nil
		}
		reasons = append(reasons, reason)
	}

	return nil, fmt.Errorf(
		"no OpenAPI Security Requirement is satisfiable: %s",
		strings.Join(reasons, "; "),
	)
}

func effectiveSecurityRequirements(
	document *openapi3.T,
	operation *openapi3.Operation,
) openapi3.SecurityRequirements {
	if operation != nil && operation.Security != nil {
		return *operation.Security
	}
	if document == nil {
		return nil
	}
	return document.Security
}

func satisfySecurityRequirement(
	document *openapi3.T,
	requirement openapi3.SecurityRequirement,
	configured map[string]credentialdomain.Reference,
) ([]securityBinding, string) {
	names := make([]string, 0, len(requirement))
	for name := range requirement {
		names = append(names, name)
	}
	slices.Sort(names)

	bindings := make([]securityBinding, 0, len(names))
	for _, name := range names {
		ref, ok := configured[name]
		if !ok {
			return nil, fmt.Sprintf("scheme %q has no Credential binding", name)
		}
		scheme, reason := resolveSupportedSecurityScheme(document, name)
		if reason != "" {
			return nil, reason
		}
		scheme.Reference = ref
		bindings = append(bindings, scheme)
	}
	return bindings, ""
}

func resolveSupportedSecurityScheme(
	document *openapi3.T,
	name string,
) (securityBinding, string) {
	if document == nil || document.Components == nil {
		return securityBinding{}, fmt.Sprintf("scheme %q is not defined", name)
	}
	ref, exists := document.Components.SecuritySchemes[name]
	if !exists || ref == nil || ref.Value == nil {
		return securityBinding{}, fmt.Sprintf("scheme %q is not defined", name)
	}
	scheme := ref.Value

	switch strings.ToLower(strings.TrimSpace(scheme.Type)) {
	case "apikey":
		location := strings.ToLower(strings.TrimSpace(scheme.In))
		switch location {
		case "header", "query", "cookie":
		default:
			return securityBinding{}, fmt.Sprintf(
				"scheme %q apiKey location %q is not supported",
				name,
				scheme.In,
			)
		}
		parameterName := strings.TrimSpace(scheme.Name)
		if parameterName == "" {
			return securityBinding{}, fmt.Sprintf(
				"scheme %q apiKey name is empty",
				name,
			)
		}
		return securityBinding{
			SchemeName: name,
			Kind:       securityAPIKey,
			In:         location,
			Name:       parameterName,
		}, ""

	case "http":
		if !strings.EqualFold(strings.TrimSpace(scheme.Scheme), "bearer") {
			return securityBinding{}, fmt.Sprintf(
				"scheme %q HTTP scheme %q is not supported",
				name,
				scheme.Scheme,
			)
		}
		return securityBinding{
			SchemeName: name,
			Kind:       securityBearer,
			In:         "header",
			Name:       "Authorization",
		}, ""

	default:
		return securityBinding{}, fmt.Sprintf(
			"scheme %q type %q is not supported",
			name,
			scheme.Type,
		)
	}
}

func applyOperationSecurity(
	ctx context.Context,
	request *http.Request,
	bindings []securityBinding,
	resolver invocationapplication.CredentialResolver,
) error {
	for _, binding := range bindings {
		if resolver == nil {
			return fmt.Errorf(
				"authorize OpenAPI security scheme %q with %s: credential resolver is unavailable",
				binding.SchemeName,
				binding.Reference,
			)
		}

		secret, err := resolver.Resolve(ctx, binding.Reference)
		if err != nil {
			return fmt.Errorf(
				"authorize OpenAPI security scheme %q with %s: %w",
				binding.SchemeName,
				binding.Reference,
				err,
			)
		}
		err = applySecuritySecret(request, binding, secret)
		for index := range secret {
			secret[index] = 0
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func applySecuritySecret(
	request *http.Request,
	binding securityBinding,
	secret []byte,
) error {
	value := string(secret)
	if strings.IndexByte(value, 0) >= 0 ||
		strings.IndexByte(value, 13) >= 0 ||
		strings.IndexByte(value, 10) >= 0 {
		return fmt.Errorf(
			"authorize OpenAPI security scheme %q: credential contains invalid control characters",
			binding.SchemeName,
		)
	}

	switch binding.Kind {
	case securityBearer:
		request.Header.Set("Authorization", "Bearer "+value)
		return nil

	case securityAPIKey:
		switch binding.In {
		case "header":
			request.Header.Set(binding.Name, value)
		case "query":
			query := request.URL.Query()
			query.Set(binding.Name, value)
			request.URL.RawQuery = query.Encode()
		case "cookie":
			//nolint:gosec // outbound Cookie header; Set-Cookie security attributes do not apply.
			request.AddCookie(&http.Cookie{Name: binding.Name, Value: value})
		default:
			return fmt.Errorf(
				"authorize OpenAPI security scheme %q: unsupported apiKey location %q",
				binding.SchemeName,
				binding.In,
			)
		}
		return nil

	default:
		return fmt.Errorf(
			"authorize OpenAPI security scheme %q: unsupported binding kind %q",
			binding.SchemeName,
			binding.Kind,
		)
	}
}
