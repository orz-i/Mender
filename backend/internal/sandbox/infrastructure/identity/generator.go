package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	sandboxdomain "github.com/orz-i/Mender/backend/internal/sandbox/domain"
)

type Generator struct{}

func (Generator) NewAdmissionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("sandbox admission id: %w", err)
	}
	return "sadm_" + hex.EncodeToString(raw[:]), nil
}

// NewSandboxID allocates one opaque local/remote Sandbox resource identity.
func (Generator) NewSandboxID() (sandboxdomain.ID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("sandbox id: %w", err)
	}
	return sandboxdomain.ParseID("sbx_" + hex.EncodeToString(raw[:]))
}
