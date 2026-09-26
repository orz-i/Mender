package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	profiledomain "github.com/orz-i/Mender/backend/internal/profile/domain"
)

const randomIDBytes = 16

type Generator struct {
	reader io.Reader
}

func NewGenerator() *Generator {
	return &Generator{reader: rand.Reader}
}

func (g *Generator) NewProfileID() (profiledomain.ProfileID, error) {
	random := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(g.reader, random); err != nil {
		return "", fmt.Errorf("generate profile id: %w", err)
	}
	id, err := profiledomain.ParseProfileID("prof_" + hex.EncodeToString(random))
	if err != nil {
		return "", fmt.Errorf("generate profile id: validate: %w", err)
	}
	return id, nil
}
