package domain

import (
	"fmt"
	"regexp"
	"strings"
)

const referencePrefix = "credential://"

var referencePartPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// Reference identifies secret material without containing the secret itself.
type Reference string

// NewReference constructs a canonical credential reference from human-owned parts.
func NewReference(namespace string, name string) (Reference, error) {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if !referencePartPattern.MatchString(namespace) ||
		!referencePartPattern.MatchString(name) {
		return "", fmt.Errorf(
			"credential reference: namespace and name must match %s",
			referencePartPattern.String(),
		)
	}
	return Reference(referencePrefix + namespace + "/" + name), nil
}

// ParseReference validates a canonical credential reference.
func ParseReference(value string) (Reference, error) {
	if !strings.HasPrefix(value, referencePrefix) {
		return "", fmt.Errorf("credential reference: invalid scheme")
	}

	remainder := strings.TrimPrefix(value, referencePrefix)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 ||
		!referencePartPattern.MatchString(parts[0]) ||
		!referencePartPattern.MatchString(parts[1]) {
		return "", fmt.Errorf("credential reference: expected credential://<namespace>/<name>")
	}

	return Reference(value), nil
}

func (r Reference) String() string {
	return string(r)
}

func (r Reference) Namespace() string {
	value := strings.TrimPrefix(string(r), referencePrefix)
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func (r Reference) Name() string {
	value := strings.TrimPrefix(string(r), referencePrefix)
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}
