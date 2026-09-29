package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	skilldomain "github.com/orz-i/Mender/backend/internal/skill/domain"
)

const randomIDBytes = 16

// Generator allocates cryptographically random exact Skill installation IDs.
type Generator struct {
	reader io.Reader
}

// NewGenerator constructs a Generator backed by crypto/rand.
func NewGenerator() *Generator {
	return &Generator{reader: rand.Reader}
}

// NewInstallationID allocates an InstallationID accepted by the Skill domain.
func (g *Generator) NewInstallationID() (skilldomain.InstallationID, error) {
	random := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(g.reader, random); err != nil {
		return "", fmt.Errorf("generate skill installation id: %w", err)
	}
	id, err := skilldomain.ParseInstallationID("sinst_" + hex.EncodeToString(random))
	if err != nil {
		return "", fmt.Errorf("generate skill installation id: validate: %w", err)
	}
	return id, nil
}
