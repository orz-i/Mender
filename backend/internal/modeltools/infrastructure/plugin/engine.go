package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// ToolError is an expected hosted-tool contract failure after a model API
// request itself succeeded.
type ToolError struct {
	Message string
}

func (e ToolError) Error() string {
	return strings.TrimSpace(e.Message)
}

// IsToolError reports whether err represents an expected hosted-tool failure.
func IsToolError(err error) bool {
	var target ToolError
	return errors.As(err, &target)
}

func newToolError(format string, args ...any) error {
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	if message == "" {
		message = "hosted tool failed"
	}
	return ToolError{Message: message}
}
