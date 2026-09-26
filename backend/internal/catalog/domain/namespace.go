package domain

import "regexp"

const maxNamespaceLength = 63

var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$|^[a-z]$`)

// Namespace is a stable human-readable Provider identifier.
type Namespace string

// NewNamespace validates a provider namespace.
//
// Namespaces are deliberately strict and are not silently normalized because
// changing them can affect exported names and configuration references.
func NewNamespace(value string) (Namespace, error) {
	if value == "" {
		return "", validationError("namespace", "must not be empty")
	}
	if len(value) > maxNamespaceLength {
		return "", validationError("namespace", "must be at most 63 characters")
	}
	if !namespacePattern.MatchString(value) {
		return "", validationError("namespace", "must start with a lowercase letter and contain only lowercase letters, digits, or hyphens")
	}

	return Namespace(value), nil
}

func (n Namespace) String() string {
	return string(n)
}
