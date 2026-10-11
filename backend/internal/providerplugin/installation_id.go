package providerplugin

import platformplugin "github.com/orz-i/Mender/backend/internal/plugin"

// InstallationID identifies a plugin package independent of extension type.
type InstallationID = platformplugin.InstallationID

// ParseInstallationID validates a package installation identity.
func ParseInstallationID(value string) (InstallationID, error) {
	return platformplugin.ParseInstallationID(value)
}
