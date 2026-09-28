package domain

import providerplugin "github.com/orz-i/Mender/backend/internal/providerplugin"

// InstallationID remains an installation-domain alias for the shared Provider plugin installation identity.
type InstallationID = providerplugin.InstallationID

// ParseInstallationID validates and returns an InstallationID.
func ParseInstallationID(value string) (InstallationID, error) {
	return providerplugin.ParseInstallationID(value)
}
