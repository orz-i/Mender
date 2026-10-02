package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type Generator struct{}

func NewGenerator() Generator {
	return Generator{}
}

func (Generator) NewActivationID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate provider activation id: %w", err)
	}
	return "act_" + hex.EncodeToString(raw), nil
}
