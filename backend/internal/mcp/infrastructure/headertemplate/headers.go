package headertemplate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	headerNamePattern   = regexp.MustCompile("^[!#$%&'*+.^_\x60|~0-9A-Za-z-]+$")
	authSchemePattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+._-]*$`)
	envReferencePattern = regexp.MustCompile(`\$\{(?:env:)?([A-Za-z_][A-Za-z0-9_]*)\}|\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)
)

var forbiddenHeaders = map[string]struct{}{
	"connection":        {},
	"content-length":    {},
	"host":              {},
	"proxy-connection":  {},
	"te":                {},
	"trailer":           {},
	"transfer-encoding": {},
	"upgrade":           {},
}

func Validate(headers map[string]string) error {
	for name, value := range headers {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" || !headerNamePattern.MatchString(trimmedName) {
			return fmt.Errorf("HTTP header name %q is invalid", name)
		}
		lowerName := strings.ToLower(trimmedName)
		if _, forbidden := forbiddenHeaders[lowerName]; forbidden {
			return fmt.Errorf("HTTP header %q is controlled by the transport", trimmedName)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("HTTP header %q contains an invalid value", trimmedName)
		}
		if strings.Contains(value, "${") || strings.Contains(value, "{env:") {
			if !containsOnlyValidReferences(value) {
				return fmt.Errorf("HTTP header %q contains an invalid environment reference", trimmedName)
			}
		}
		if isSensitiveHeader(lowerName) && !isSafeSensitiveTemplate(lowerName, value) {
			return fmt.Errorf(
				"HTTP header %q must use an environment reference instead of persisting a secret value",
				trimmedName,
			)
		}
	}
	return nil
}

func Resolve(
	headers map[string]string,
	lookup func(string) (string, bool),
) (map[string]string, error) {
	if err := Validate(headers); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(headers))
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		value := headers[name]
		var resolveErr error
		resolved := envReferencePattern.ReplaceAllStringFunc(value, func(match string) string {
			if resolveErr != nil {
				return ""
			}
			parts := envReferencePattern.FindStringSubmatch(match)
			envName := parts[1]
			if envName == "" {
				envName = parts[2]
			}
			secret, ok := lookup(envName)
			if !ok {
				resolveErr = fmt.Errorf(
					"HTTP header %q requires environment variable %s",
					name,
					envName,
				)
				return ""
			}
			return secret
		})
		if resolveErr != nil {
			return nil, resolveErr
		}
		result[name] = resolved
	}
	return result, nil
}

func isSensitiveHeader(name string) bool {
	switch name {
	case "authorization", "proxy-authorization", "cookie", "set-cookie":
		return true
	}
	for _, marker := range []string{
		"api-key",
		"apikey",
		"access-key",
		"token",
		"secret",
		"password",
	} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func isSafeSensitiveTemplate(name string, value string) bool {
	value = strings.TrimSpace(value)
	if isSingleEnvReference(value) {
		return true
	}
	if name != "authorization" && name != "proxy-authorization" {
		return false
	}

	parts := strings.Fields(value)
	return len(parts) == 2 &&
		authSchemePattern.MatchString(parts[0]) &&
		isSingleEnvReference(parts[1])
}

func isSingleEnvReference(value string) bool {
	match := envReferencePattern.FindStringIndex(value)
	return match != nil && match[0] == 0 && match[1] == len(value)
}

func containsOnlyValidReferences(value string) bool {
	remainder := envReferencePattern.ReplaceAllString(value, "")
	return !strings.Contains(remainder, "${") && !strings.Contains(remainder, "{env:")
}
