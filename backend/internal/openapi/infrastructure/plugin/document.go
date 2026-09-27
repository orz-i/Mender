package plugin

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/getkin/kin-openapi/openapi3"
)

const maxDocumentDiagnosticRunes = 4096

type parsedDocument struct {
	document      *openapi3.T
	location      *url.URL
	sourceVersion string
}

func loadOpenAPIDocument(
	ctx context.Context,
	sources sourceLoader,
	source SourceConfig,
) (parsedDocument, error) {
	loaded, err := sources.Load(ctx, source)
	if err != nil {
		return parsedDocument{}, err
	}

	loader := openapi3.NewLoader()
	loader.Context = ctx
	loader.IsExternalRefsAllowed = false
	loader.ReadFromURIFunc = nil

	document, err := loader.LoadFromDataWithPath(loaded.data, loaded.location)
	if err != nil {
		return parsedDocument{}, fmt.Errorf(
			"parse OpenAPI description: %s",
			boundedDocumentDiagnostic(err.Error()),
		)
	}
	if document == nil {
		return parsedDocument{}, fmt.Errorf("parse OpenAPI description: parser returned nil document")
	}

	sourceVersion := strings.TrimSpace(document.OpenAPI)
	normalizedVersion, err := normalizedParserVersion(sourceVersion)
	if err != nil {
		return parsedDocument{}, err
	}
	// kin-openapi v0.145.0 predates the 3.2.1 patch release and recognizes
	// 3.2.0 as the 3.2 family marker. Patch releases are semantically compatible,
	// so normalize only for the parser model while preserving sourceVersion.
	document.OpenAPI = normalizedVersion

	if err := document.Validate(ctx, openapi3.EnableMultiError()); err != nil {
		return parsedDocument{}, fmt.Errorf(
			"validate OpenAPI description: %s",
			boundedDocumentDiagnostic(err.Error()),
		)
	}

	return parsedDocument{
		document:      document,
		location:      cloneURL(loaded.location),
		sourceVersion: sourceVersion,
	}, nil
}

func normalizedParserVersion(value string) (string, error) {
	switch value {
	case "3.0", "3.0.0", "3.0.1", "3.0.2", "3.0.3", "3.0.4":
		return value, nil
	case "3.1", "3.1.0", "3.1.1", "3.1.2":
		return value, nil
	case "3.2", "3.2.0":
		return value, nil
	case "3.2.1":
		return "3.2.0", nil
	default:
		return "", fmt.Errorf(
			"parse OpenAPI description: unsupported OpenAPI version %q",
			value,
		)
	}
}

func boundedDocumentDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown OpenAPI error"
	}
	if utf8.RuneCountInString(value) <= maxDocumentDiagnosticRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxDocumentDiagnosticRunes]) + "…"
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return nil
	}
	copy := *value
	if value.User != nil {
		user := *value.User
		copy.User = &user
	}
	return &copy
}
