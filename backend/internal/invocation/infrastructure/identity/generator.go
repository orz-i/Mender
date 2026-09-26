package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	invocationdomain "github.com/orz-i/Mender/backend/internal/invocation/domain"
)

const randomIDBytes = 16

// Generator allocates cryptographically random Invocation IDs.
type Generator struct {
	reader io.Reader
}

func NewGenerator() *Generator {
	return &Generator{reader: rand.Reader}
}

func (g *Generator) NewInvocationID() (invocationdomain.InvocationID, error) {
	random := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(g.reader, random); err != nil {
		return "", fmt.Errorf("generate invocation id: %w", err)
	}

	id, err := invocationdomain.ParseInvocationID("inv_" + hex.EncodeToString(random))
	if err != nil {
		return "", fmt.Errorf("generate invocation id: validate: %w", err)
	}
	return id, nil
}
