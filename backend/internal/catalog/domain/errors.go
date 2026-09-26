package domain

import "fmt"

// ValidationError reports a violated domain invariant.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func validationError(field, reason string) error {
	return ValidationError{Field: field, Reason: reason}
}
