package domain

import (
	"encoding/hex"
	"strings"
)

const (
	providerIDPrefix = "prov_"
	toolIDPrefix     = "tool_"
	opaqueIDBytes    = 16
	opaqueIDHexLen   = opaqueIDBytes * 2
)

// ProviderID is Mender's stable opaque identity for a Provider.
type ProviderID string

// ParseProviderID validates and returns a ProviderID.
func ParseProviderID(value string) (ProviderID, error) {
	if err := validateOpaqueID(value, providerIDPrefix, "provider_id"); err != nil {
		return "", err
	}
	return ProviderID(value), nil
}

func (id ProviderID) String() string {
	return string(id)
}

// ToolID is Mender's stable opaque identity for a Tool.
type ToolID string

// ParseToolID validates and returns a ToolID.
func ParseToolID(value string) (ToolID, error) {
	if err := validateOpaqueID(value, toolIDPrefix, "tool_id"); err != nil {
		return "", err
	}
	return ToolID(value), nil
}

func (id ToolID) String() string {
	return string(id)
}

func validateOpaqueID(value, prefix, field string) error {
	if !strings.HasPrefix(value, prefix) {
		return validationError(field, "has invalid prefix")
	}

	encoded := strings.TrimPrefix(value, prefix)
	if len(encoded) != opaqueIDHexLen {
		return validationError(field, "has invalid length")
	}
	if encoded != strings.ToLower(encoded) {
		return validationError(field, "must use lowercase hexadecimal encoding")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return validationError(field, "must use hexadecimal encoding")
	}

	return nil
}
