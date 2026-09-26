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

// ParseReference validates a canonical credential reference.
func ParseReference(value string) (Reference, error) {
	if !strings.HasPrefix(value, referencePrefix) {
		return "", fmt.Errorf("credential reference: invalid scheme")
	}

	remainder := strings.TrimPrefix(value, referencePrefix)
	parts := strings.Split(remainder, "/")
	if len(parts) != 2 || !referencePartPattern.MatchString(parts[0]) || !referencePartPattern.MatchString(parts[1]) {
		return "", fmt.Errorf("credential reference: expected credential://<namespace>/<name>")
	}

	return Reference(value), nil
}

func (r Reference) String() string {
	return string(r)
}
