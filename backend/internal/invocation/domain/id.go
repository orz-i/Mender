package domain

import (
	"encoding/hex"
	"strings"
)

const (
	invocationIDPrefix = "inv_"
	invocationIDBytes  = 16
	invocationIDHexLen = invocationIDBytes * 2
)

// InvocationID is Mender's stable opaque identity for one invocation attempt.
type InvocationID string

// ParseInvocationID validates an InvocationID.
func ParseInvocationID(value string) (InvocationID, error) {
	if !strings.HasPrefix(value, invocationIDPrefix) {
		return "", validationError("invocation_id", "has invalid prefix")
	}

	encoded := strings.TrimPrefix(value, invocationIDPrefix)
	if len(encoded) != invocationIDHexLen {
		return "", validationError("invocation_id", "has invalid length")
	}
	if encoded != strings.ToLower(encoded) {
		return "", validationError("invocation_id", "must use lowercase hexadecimal encoding")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", validationError("invocation_id", "must use hexadecimal encoding")
	}

	return InvocationID(value), nil
}

func (id InvocationID) String() string {
	return string(id)
}
