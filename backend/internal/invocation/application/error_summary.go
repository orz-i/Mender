package application

import (
	"errors"
	"regexp"
	"strings"
)

type diagnosticSummaryProvider interface {
	DiagnosticSummary() string
}

var (
	authorizationSecretPattern = regexp.MustCompile(`(?i)\bauthorization\s*[:=]\s*(?:Bearer\s+)?[^\s,;]+`)
	bearerSecretPattern        = regexp.MustCompile(`(?i)\bBearer\s+[^\s,;]+`)
	namedSecretPattern         = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+`)
	prefixedSecretPattern      = regexp.MustCompile(`(?i)\b(?:as_sk_|sk-|ghp_|github_pat_)[A-Za-z0-9_-]{8,}\b`)
)

func auditErrorSummary(err error) string {
	if err == nil {
		return ""
	}

	var provider diagnosticSummaryProvider
	if !errors.As(err, &provider) {
		return ""
	}
	return redactDiagnosticSummary(provider.DiagnosticSummary())
}

func redactDiagnosticSummary(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = authorizationSecretPattern.ReplaceAllString(value, "Authorization=[REDACTED]")
	value = bearerSecretPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	value = namedSecretPattern.ReplaceAllString(value, "$1=[REDACTED]")
	value = prefixedSecretPattern.ReplaceAllString(value, "[REDACTED]")
	return value
}
