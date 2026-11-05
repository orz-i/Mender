package application

import (
	"encoding/json"

	credentialdomain "github.com/orz-i/Mender/backend/internal/credential/domain"
)

// Creation specifications carry trusted management input without exposing a
// backend implementation or persistence model. Backend adapters validate these
// values before any connection, credential resolution or resource creation.
type DockerTemplate struct {
	ImageDigest  string
	UserID       uint32
	GroupID      uint32
	CPUmilli     uint32
	MemoryMiB    uint32
	WorkspaceMiB uint32
	PIDs         uint32
}

type DaytonaTemplate struct {
	CredentialRef credentialdomain.Reference
	ImageDigest   string
	Target        string
	CPU           int
	MemoryGiB     int
	DiskGiB       int
	TTLMinutes    int
}

type SSHRemoteTemplate struct {
	Host          string                     `json:"host"`
	Port          uint16                     `json:"port"`
	User          string                     `json:"user"`
	Root          string                     `json:"root"`
	HostPublicKey string                     `json:"hostPublicKey"`
	CredentialRef credentialdomain.Reference `json:"credentialRef"`
}

type ExternalConfig struct {
	ConfigVersion uint32
	JSON          json.RawMessage
}
