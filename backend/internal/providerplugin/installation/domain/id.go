package domain

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	installationIDPrefix = "pinst_"
	opaqueIDBytes        = 16
	opaqueIDHexLen       = opaqueIDBytes * 2
)

// InstallationID is Mender's stable opaque identity for one installed plugin package.
type InstallationID string

// ParseInstallationID validates and returns an InstallationID.
func ParseInstallationID(value string) (InstallationID, error) {
	if !strings.HasPrefix(value, installationIDPrefix) {
		return "", fmt.Errorf("plugin installation id: has invalid prefix")
	}
	encoded := strings.TrimPrefix(value, installationIDPrefix)
	if len(encoded) != opaqueIDHexLen {
		return "", fmt.Errorf("plugin installation id: has invalid length")
	}
	if encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("plugin installation id: must use lowercase hexadecimal encoding")
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("plugin installation id: must use hexadecimal encoding")
	}
	return InstallationID(value), nil
}

func (id InstallationID) String() string {
	return string(id)
}
