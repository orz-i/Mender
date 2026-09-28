package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	installationdomain "github.com/orz-i/Mender/backend/internal/providerplugin/installation/domain"
)

const randomIDBytes = 16

// Generator allocates cryptographically random opaque plugin installation IDs.
type Generator struct {
	reader io.Reader
}

// NewGenerator constructs a Generator backed by crypto/rand.
func NewGenerator() *Generator {
	return &Generator{reader: rand.Reader}
}

// NewInstallationID allocates an InstallationID accepted by the installation domain.
func (g *Generator) NewInstallationID() (installationdomain.InstallationID, error) {
	random := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(g.reader, random); err != nil {
		return "", fmt.Errorf("generate plugin installation id: %w", err)
	}
	id, err := installationdomain.ParseInstallationID("pinst_" + hex.EncodeToString(random))
	if err != nil {
		return "", fmt.Errorf("generate plugin installation id: validate: %w", err)
	}
	return id, nil
}
