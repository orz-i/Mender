package domain

import platformplugin "github.com/orz-i/Mender/backend/internal/plugin"

type InstallationID = platformplugin.InstallationID

func ParseInstallationID(value string) (InstallationID, error) {
	return platformplugin.ParseInstallationID(value)
}
