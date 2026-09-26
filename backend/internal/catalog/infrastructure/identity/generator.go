// Package identity provides infrastructure implementations for Catalog IDs.
package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/orz-i/Mender/backend/internal/catalog/domain"
)

const randomIDBytes = 16

// Generator allocates cryptographically random opaque Catalog identities.
type Generator struct {
	reader io.Reader
}

// NewGenerator constructs a Generator backed by crypto/rand.
func NewGenerator() *Generator {
	return &Generator{reader: rand.Reader}
}

// NewProviderID allocates a ProviderID accepted by the Catalog domain.
func (g *Generator) NewProviderID() (domain.ProviderID, error) {
	raw, err := g.newRawID("prov_")
	if err != nil {
		return "", fmt.Errorf("generate provider id: %w", err)
	}
	id, err := domain.ParseProviderID(raw)
	if err != nil {
		return "", fmt.Errorf("generate provider id: validate: %w", err)
	}
	return id, nil
}

// NewToolID allocates a ToolID accepted by the Catalog domain.
func (g *Generator) NewToolID() (domain.ToolID, error) {
	raw, err := g.newRawID("tool_")
	if err != nil {
		return "", fmt.Errorf("generate tool id: %w", err)
	}
	id, err := domain.ParseToolID(raw)
	if err != nil {
		return "", fmt.Errorf("generate tool id: validate: %w", err)
	}
	return id, nil
}

func (g *Generator) newRawID(prefix string) (string, error) {
	random := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(g.reader, random); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(random), nil
}
