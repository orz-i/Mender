// Package plugin defines package identity and typed extension metadata independently
// of Provider, Sandbox, Tunnel, and their domain-owned resources.
package plugin

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const (
	maxIDLength          = 128
	installationIDPrefix = "pinst_"
)

var pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:\.[a-z][a-z0-9-]*)+$`)
var extensionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// ID identifies software independent of the resource types it extends.
type ID string

func ParseID(value string) (ID, error) {
	if value == "" || len(value) > maxIDLength || value != strings.TrimSpace(value) ||
		!pluginIDPattern.MatchString(value) {
		return "", fmt.Errorf("plugin id: expected lowercase namespaced identifier of at most %d bytes", maxIDLength)
	}
	return ID(value), nil
}

func (id ID) String() string { return string(id) }

// InstallationID identifies an immutable package installation across extension kinds.
type InstallationID string

func ParseInstallationID(value string) (InstallationID, error) {
	if !strings.HasPrefix(value, installationIDPrefix) {
		return "", fmt.Errorf("plugin installation id: has invalid prefix")
	}
	hexPart := strings.TrimPrefix(value, installationIDPrefix)
	if len(hexPart) != 32 || strings.ToLower(hexPart) != hexPart {
		return "", fmt.Errorf("plugin installation id: expected 32 lowercase hex digits")
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", fmt.Errorf("plugin installation id: must use hexadecimal encoding")
	}
	return InstallationID(value), nil
}

func (id InstallationID) String() string { return string(id) }

// ExtensionID names a typed contribution within a package.
type ExtensionID string

func ParseExtensionID(value string) (ExtensionID, error) {
	if !extensionIDPattern.MatchString(value) {
		return "", fmt.Errorf("plugin extension id: expected lowercase slug of at most 64 bytes")
	}
	return ExtensionID(value), nil
}

func (id ExtensionID) String() string { return string(id) }
