package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	tunneldomain "github.com/orz-i/Mender/backend/internal/tunnel/domain"
)

const randomIDBytes = 16

type Generator struct {
	reader io.Reader
}

func NewGenerator() *Generator {
	return &Generator{reader: rand.Reader}
}

func (g *Generator) NewTunnelID() (tunneldomain.TunnelID, error) {
	random := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(g.reader, random); err != nil {
		return "", fmt.Errorf("generate tunnel id: %w", err)
	}
	id, err := tunneldomain.ParseTunnelID("tun_" + hex.EncodeToString(random))
	if err != nil {
		return "", fmt.Errorf("generate tunnel id: validate: %w", err)
	}
	return id, nil
}
